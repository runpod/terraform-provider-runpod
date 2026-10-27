#!/usr/bin/env bash
#
# The live run against a published release, behind `task test-release`: the
# provider comes from the Terraform Registry and the use-case modules from the
# release tag on GitHub, never from this tree or the local build.
#
# It copies tests/ and the leftovers sweep into artifacts/release-test/<version>/,
# pins runpod/runpod there to exactly that version, has terraform fetch v2/modules
# at tag v<version> next to the copy (a test run block takes only a local or
# registry module source, so the copy's ../modules/<name> sources resolve to the
# tag's modules), then runs scripts/test_e2e.sh over the copy with the same plan,
# opt-ins and logs as task test-e2e.
#
# Environment:
#   E2E_PROVIDER_VERSION   the release to test, without the v; task test-release sets it
#   every test_e2e.sh variable, such as E2E_CLUSTER=1 and E2E_DRY_RUN=1
#
# Exit status: that of test_e2e.sh; 2 when the copy cannot be prepared.

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version="${E2E_PROVIDER_VERSION:-}"
module_repo="github.com/runpod/terraform-provider-runpod"

case "$version" in
  "" | *[!0-9a-z.-]*) echo "E2E_PROVIDER_VERSION='$version' is not a release number such as 2.0.0" >&2; exit 2 ;;
esac

# A CLI config that names a local mirror or a dev override would install the
# local build instead of the published release.
for name in TF_CLI_CONFIG_FILE TERRAFORM_CONFIG; do
  if [ -n "${!name:-}" ]; then
    echo "$name=${!name} is set: a release test installs from the registry, so run it with $name unset" >&2
    exit 2
  fi
done
for cfg in "$HOME/.terraformrc" "$HOME"/.terraform.d/*.tfrc; do
  [ -r "$cfg" ] || continue
  if grep -Eq 'provider-mirror|runpod/runpod' "$cfg"; then
    echo "$cfg installs runpod/runpod from somewhere other than the registry; move it aside for the release test" >&2
    exit 2
  fi
done

root="$repo/artifacts/release-test/$version"
echo "Preparing $root"
rm -rf "$root"
mkdir -p "$root/tests" "$root/leftovers" "$root/modules" || exit 2
cp "$repo"/tests/*.tftest.hcl "$repo/tests/main.tf" "$root/tests/" || exit 2
cp -R "$repo/tests/fixtures" "$root/tests/fixtures" || exit 2
cp "$repo/scripts/leftovers/main.tf" "$root/leftovers/main.tf" || exit 2

# The root pins decide which provider terraform installs; the tagged modules
# carry the release's own pin.
for file in "$root/tests/main.tf" "$root/leftovers/main.tf"; do
  sed -i.orig -E "s/^([[:space:]]+version[[:space:]]*=[[:space:]]*)\"[^\"]*\"/\1\"= $version\"/" "$file" &&
    rm -f "$file.orig" || exit 2
  grep -q "version *= *\"= $version\"" "$file" || {
    echo "could not pin runpod/runpod to $version in $file" >&2
    exit 2
  }
done

echo "Fetching $module_repo//v2/modules at v$version"
if ! terraform -chdir="$root/modules" init -input=false -no-color -backend=false \
  -from-module="$module_repo//v2/modules?ref=v$version" > "$root/modules-init.log" 2>&1; then
  tail -n 20 "$root/modules-init.log"
  echo "Could not fetch the modules at tag v$version. Nothing live ran." >&2
  exit 2
fi
rm -rf "$root/modules/.terraform" "$root/modules/.terraform.lock.hcl"

echo "Module sources in the copy, resolved against the tag's modules:"
sed -n 's/^[[:space:]]*source[[:space:]]*=[[:space:]]*"\(\.\.\/modules\/[^"]*\)".*/\1/p' "$root"/tests/*.tftest.hcl |
  sort -u | while read -r source; do
  if [ -f "$root/tests/$source/versions.tf" ]; then
    echo "  $source  <- $module_repo//v2/modules/${source#../modules/}?ref=v$version"
  else
    echo "  $source  MISSING at v$version" >&2
    exit 2
  fi
done || exit 2

E2E_TESTS_DIR="$root/tests" E2E_LEFTOVERS_TF="$root/leftovers/main.tf" \
  E2E_RUN_PREFIX="release-$version-" E2E_FROM_REGISTRY=1 \
  exec bash "$repo/scripts/test_e2e.sh"
