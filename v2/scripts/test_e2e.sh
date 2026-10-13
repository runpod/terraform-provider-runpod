#!/usr/bin/env bash
#
# The full live end-to-end run behind `task test-e2e`: every tests/*.tftest.hcl
# file, cheapest first, one `terraform test` per file so that one failure does
# not stop the rest, then a read-only sweep (scripts/leftovers/main.tf) for
# anything named tf-test- still on the account. It creates and bills real
# resources on the API RUNPOD_API_BASE_URL points at: production when unset.
#
# Environment:
#   RUNPOD_API_KEY           required for a live run; never printed
#   RUNPOD_API_BASE_URL      REST v2 host; production when unset
#   RUNPOD_REST_V1_URL (then RUNPOD_BASE_URL), RUNPOD_GRAPHQL_URL
#                            hosts of the deprecated shims; production when unset
#   TF_CLI_CONFIG_FILE       the dev.tfrc `task build` writes; task test-e2e sets it
#   TF_VAR_registry_username, TF_VAR_registry_password
#                            turn on the registry test; TF_VAR_ecr_resource_arn
#                            adds its ECR delegation half
#   E2E_CLUSTER=1            opt in to the cluster test: two H100 80GB pods
#   E2E_HOST_ACCOUNT=1       opt in to legacy_machine: needs a host account
#   E2E_SSH_KEYS=yes         opt in to account_ssh_keys: replaces the account's SSH keys
#   E2E_GPU_TYPE="NVIDIA A40"  GPU for the pod tests; default NVIDIA L40S (TF_VAR_gpu_type_id)
#   E2E_ONLY="a b"           run only these test files (names without .tftest.hcl)
#   E2E_DRY_RUN=1            print the pre-flight and the plan, run nothing, exit 0
#   E2E_LOG_ROOT             where the timestamped log folder goes; artifacts/e2e
#
# Exit status: 0 every test that ran passed, 1 a test failed, 2 the pre-flight
# stopped the run before anything live, 130 interrupted.
#
# Written for the bash 3.2 macOS ships: no associative arrays, no mapfile.

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tests_dir="$repo/tests"
provider_version="${E2E_PROVIDER_VERSION:-2.0.0}"

# Production defaults of the SDK: runpod.DefaultBaseURL, and
# legacy.DefaultRestV1URL and legacy.DefaultGraphQLURL for the shims.
production_v2="https://api.runpod.io"
production_rest_v1="https://rest.runpod.io/v1"
production_graphql="https://api.runpod.io/graphql"

# Cheapest first; cost_of below says what each file bills.
tests_in_order=(
  catalog_discovery billing_report legacy template serverless_endpoint
  serverless_endpoint_cpu registry account_ssh_keys legacy_machine
  network_volume cpu_pod pod_lifecycle logs gpu_pod cluster
)

cost_of() {
  case "$1" in
    catalog_discovery | billing_report | legacy) echo "free: reads only" ;;
    template) echo "free: a template is metadata" ;;
    serverless_endpoint | serverless_endpoint_cpu) echo "free while idle: zero workers" ;;
    registry) echo "free: a registry login" ;;
    account_ssh_keys) echo "free, but replaces the account's SSH keys" ;;
    legacy_machine) echo "registers a host machine" ;;
    network_volume) echo "a 10 GB network volume for the run" ;;
    cpu_pod) echo "one 4-vCPU cpu5c pod for the run" ;;
    pod_lifecycle | logs) echo "one GPU pod for the run" ;;
    gpu_pod) echo "one GPU pod and a 10 GB volume for the run" ;;
    cluster) echo "two H100 80GB pods for the run, the most expensive" ;;
    *) echo "not in the cost list, so it runs last" ;;
  esac
}

# on, off or invalid, for the yes/no environment switches.
flag() {
  case "$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')" in
    1 | y | yes | true) echo on ;;
    "" | 0 | n | no | false) echo off ;;
    *) echo invalid ;;
  esac
}

in_list() {
  local needle="$1" item
  shift
  for item in "$@"; do
    [ "$item" = "$needle" ] && return 0
  done
  return 1
}

duration() {
  printf '%dm%02ds' $(($1 / 60)) $(($1 % 60))
}

# Ctrl-C, a closed terminal or a TERM reaches every process in the foreground
# group, the tees included. They ignore it, so the cleanup terraform does after
# an interrupt, and the sweep after that, still reach the terminal and the logs.
log_to() {
  trap '' INT HUP TERM
  exec tee -a "$1"
}

dry_run="$(flag "${E2E_DRY_RUN:-}")"

log_root="${E2E_LOG_ROOT:-$repo/artifacts/e2e}"
if ! mkdir -p "$log_root" || ! log_root="$(cd "$log_root" && pwd)"; then
  echo "Cannot create the log folder under E2E_LOG_ROOT=$log_root" >&2
  exit 2
fi
run_dir="$log_root/$(date +%Y%m%d-%H%M%S)"
[ "$dry_run" = off ] || run_dir="$run_dir-dry-run"
if [ -e "$run_dir" ]; then
  n=2
  while [ -e "$run_dir-$n" ]; do n=$((n + 1)); done
  run_dir="$run_dir-$n"
fi
if ! mkdir -p "$run_dir/tests"; then
  echo "Cannot create the log folder $run_dir" >&2
  exit 2
fi

# Everything from here on goes to the terminal and to e2e.log. A FIFO rather
# than a process substitution, so the exit handler can wait for tee to flush.
# It lives in a local temp folder: a FIFO on a shared or network mount, where
# the log folder may be, can fail writes into it.
fifo_dir="$(mktemp -d "${TMPDIR:-/tmp}/test_e2e.XXXXXX")" || exit 2
mkfifo "$fifo_dir/log" || exit 2
log_to "$run_dir/e2e.log" < "$fifo_dir/log" &
tee_pid=$!
exec 3>&1 4>&2 > "$fifo_dir/log" 2>&1

armed=0     # set once something live has started; the sweep runs from then on
sweeping=0
finished=0
interrupts=0

# The two handlers are only ever called by trap, which shellcheck cannot see.
# shellcheck disable=SC2317
on_interrupt() {
  interrupts=$((interrupts + 1))
  if [ "$armed" != 1 ]; then
    echo
    echo "Interrupted before anything live ran."
    exit 130
  fi
  if [ "$sweeping" = 1 ]; then
    echo
    echo "Interrupt: the leftovers sweep only reads and finishes in seconds; the summary follows it."
  elif [ "$interrupts" -eq 1 ]; then
    echo
    echo "Interrupt: the remaining files are skipped, and the leftovers sweep runs."
  else
    echo
    echo "Another interrupt: terraform may stop without destroying what it made. The leftovers sweep lists it."
  fi
}

# shellcheck disable=SC2317
on_exit() {
  local status=$?
  if [ "$armed" = 1 ] && [ "$finished" != 1 ]; then
    finish_run
    status=$exit_code
  fi
  exec 1>&3 2>&4 3>&- 4>&-
  wait "$tee_pid" 2> /dev/null
  [ -z "$fifo_dir" ] || rm -rf "$fifo_dir"
  exit "$status"
}

trap on_interrupt INT TERM HUP
trap on_exit EXIT

echo "Runpod Terraform provider $provider_version, live end-to-end run"
[ "$dry_run" = on ] && echo "DRY RUN: nothing below calls the API or terraform test."
echo "Logs: $run_dir"
echo

# --- Pre-flight: every check runs before anything live ----------------------

problems=()
ok() { printf '  ok    %s\n' "$1"; }
info() { printf '  info  %s\n' "$1"; }
warn() { printf '  WARN  %s\n' "$1"; }
fail() {
  problems+=("$1")
  printf '  FAIL  %s\n' "$1"
}

echo "Pre-flight"

if [ -n "${RUNPOD_API_KEY:-}" ]; then
  ok "RUNPOD_API_KEY is set (value not shown)"
elif [ "$dry_run" = on ]; then
  info "RUNPOD_API_KEY is not set; a dry run does not need it"
else
  fail "RUNPOD_API_KEY is not set: put it in $repo/.env (see .env.example) or export it"
fi

tf_platform=""
if ! command -v terraform > /dev/null 2>&1; then
  fail "terraform is not on PATH: install Terraform 1.13 or newer"
else
  tf_json="$(CHECKPOINT_DISABLE=1 terraform version -json 2> /dev/null)"
  tf_version="$(printf '%s\n' "$tf_json" | sed -n 's/.*"terraform_version": *"\([^"]*\)".*/\1/p')"
  tf_platform="$(printf '%s\n' "$tf_json" | sed -n 's/.*"platform": *"\([^"]*\)".*/\1/p')"
  tf_major="${tf_version%%.*}"
  tf_minor="${tf_version#*.}"
  tf_minor="${tf_minor%%.*}"
  case "$tf_major.$tf_minor" in
    *[!0-9.]* | .* | *.) fail "could not read the terraform version from 'terraform version -json'" ;;
    *)
      if [ "$tf_major" -gt 1 ] || { [ "$tf_major" -eq 1 ] && [ "$tf_minor" -ge 13 ]; }; then
        ok "terraform $tf_version (1.13 or newer), running as $tf_platform"
      else
        fail "terraform $tf_version is older than 1.13, the first that reads the variable blocks in tests/*.tftest.hcl"
      fi
      ;;
  esac
fi

mirror=""
if [ -z "${TF_CLI_CONFIG_FILE:-}" ]; then
  fail "TF_CLI_CONFIG_FILE is not set: run this through 'task test-e2e', which points it at artifacts/dev.tfrc"
elif [ ! -r "$TF_CLI_CONFIG_FILE" ]; then
  fail "TF_CLI_CONFIG_FILE=$TF_CLI_CONFIG_FILE is not readable: run 'task build'"
else
  ok "TF_CLI_CONFIG_FILE=$TF_CLI_CONFIG_FILE"
  mirror="$(sed -n 's/^[[:space:]]*path[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$TF_CLI_CONFIG_FILE" | head -n 1)"
  [ -n "$mirror" ] || fail "$TF_CLI_CONFIG_FILE has no filesystem_mirror path to install the provider from"
fi

if [ -n "$mirror" ] && [ -n "$tf_platform" ]; then
  binary="$mirror/registry.terraform.io/runpod/runpod/$provider_version/$tf_platform/terraform-provider-runpod_v$provider_version"
  if [ -x "$binary" ]; then
    ok "provider $provider_version for $tf_platform in the local mirror"
  else
    fail "the local mirror has no provider $provider_version build for $tf_platform ($binary): run 'task build' on this machine"
  fi
fi

if [ -n "${RUNPOD_API_BASE_URL:-}" ]; then
  info "REST v2 calls go to $RUNPOD_API_BASE_URL (RUNPOD_API_BASE_URL)"
else
  info "REST v2 calls go to $production_v2, production (RUNPOD_API_BASE_URL is unset)"
fi
if [ -n "${RUNPOD_REST_V1_URL:-}" ]; then
  info "deprecated REST v1 shims go to $RUNPOD_REST_V1_URL (RUNPOD_REST_V1_URL)"
elif [ -n "${RUNPOD_BASE_URL:-}" ]; then
  info "deprecated REST v1 shims go to $RUNPOD_BASE_URL (RUNPOD_BASE_URL, the 1.x name)"
else
  info "deprecated REST v1 shims go to $production_rest_v1, production (RUNPOD_REST_V1_URL is unset)"
fi
if [ -n "${RUNPOD_GRAPHQL_URL:-}" ]; then
  info "deprecated GraphQL shims go to $RUNPOD_GRAPHQL_URL (RUNPOD_GRAPHQL_URL)"
else
  info "deprecated GraphQL shims go to $production_graphql, production (RUNPOD_GRAPHQL_URL is unset)"
fi
if [ -n "${RUNPOD_API_BASE_URL:-}" ] &&
  { [ -z "${RUNPOD_REST_V1_URL:-}${RUNPOD_BASE_URL:-}" ] || [ -z "${RUNPOD_GRAPHQL_URL:-}" ]; }; then
  warn "RUNPOD_API_BASE_URL moves only the v2 calls: the shims above still reach production"
fi

for name in E2E_DRY_RUN E2E_CLUSTER E2E_HOST_ACCOUNT; do
  if [ "$(flag "${!name:-}")" = invalid ]; then
    fail "$name=${!name} is not a yes/no value: use 1 to turn it on, or leave it unset"
  fi
done
ssh_keys=off
if [ -n "${E2E_SSH_KEYS:-}" ]; then
  ssh_keys="$(flag "$E2E_SSH_KEYS")"
  [ "$ssh_keys" != invalid ] || fail "E2E_SSH_KEYS=$E2E_SSH_KEYS is not a yes/no value: use yes or no"
fi
cluster="$(flag "${E2E_CLUSTER:-}")"
host_account="$(flag "${E2E_HOST_ACCOUNT:-}")"

# The fifteen in cost order, then any file the list does not know yet.
all_tests=("${tests_in_order[@]}")
for file in "$tests_dir"/*.tftest.hcl; do
  [ -e "$file" ] || continue
  name="$(basename "$file" .tftest.hcl)"
  in_list "$name" "${all_tests[@]}" || all_tests+=("$name")
done

only=()
for word in ${E2E_ONLY:-}; do
  word="${word#tests/}"
  word="${word%.tftest.hcl}"
  if in_list "$word" "${all_tests[@]}"; then
    only+=("$word")
  else
    fail "E2E_ONLY names '$word', which is not a test file in tests/"
  fi
done
[ ${#only[@]} -gt 0 ] && info "E2E_ONLY: ${only[*]}"
[ -z "${E2E_GPU_TYPE:-}" ] || export TF_VAR_gpu_type_id="$E2E_GPU_TYPE"
info "GPU for the pod tests: ${TF_VAR_gpu_type_id:-NVIDIA L40S} (E2E_GPU_TYPE overrides)"
info "an \"at least \$5 in your account\" error means the key's own account or team has no balance"

# --- The SSH key test ----------------------------------------------------------
# Off unless E2E_SSH_KEYS=yes: it replaces the account's whole key set.

ssh_decision=""
if [ -e "$tests_dir/account_ssh_keys.tftest.hcl" ] &&
  { [ ${#only[@]} -eq 0 ] || in_list account_ssh_keys "${only[@]}"; }; then
  case "$ssh_keys" in
    on) ssh_decision=run ;;
    invalid) ssh_decision="E2E_SSH_KEYS is not yes or no" ;;
    *) ssh_decision="replaces the account's SSH keys; E2E_SSH_KEYS=yes runs it" ;;
  esac
fi
if [ "$ssh_decision" = run ]; then
  echo
  echo "  !!!  account_ssh_keys REPLACES THE WHOLE SET OF SSH KEYS ON THIS RUNPOD ACCOUNT"
  echo "  !!!  with one test key, and leaves that key there afterwards. Your own keys are"
  echo "  !!!  not put back. Copy them from the console's account settings first."
fi

# --- The plan ----------------------------------------------------------------

plan_action=()
plan_reason=()
runnable=0
for name in "${all_tests[@]}"; do
  action=skip
  reason=""
  if [ ! -e "$tests_dir/$name.tftest.hcl" ]; then
    reason="tests/$name.tftest.hcl not found"
  elif [ ${#only[@]} -gt 0 ] && ! in_list "$name" "${only[@]}"; then
    reason="not in E2E_ONLY"
  else
    case "$name" in
      registry)
        if [ -z "${TF_VAR_registry_username:-}" ] || [ -z "${TF_VAR_registry_password:-}" ]; then
          reason="needs TF_VAR_registry_username and TF_VAR_registry_password"
        else
          action=run
          [ -n "${TF_VAR_ecr_resource_arn:-}" ] ||
            reason="without the ECR delegation half: TF_VAR_ecr_resource_arn is unset"
        fi
        ;;
      cluster)
        if [ "$cluster" = on ]; then action=run; else reason="opt-in: E2E_CLUSTER=1 (two H100 80GB pods for the run)"; fi
        ;;
      legacy_machine)
        if [ "$host_account" = on ]; then action=run; else reason="opt-in: E2E_HOST_ACCOUNT=1 (needs a host account)"; fi
        ;;
      account_ssh_keys)
        if [ "$ssh_decision" = run ]; then action=run; else reason="$ssh_decision"; fi
        ;;
      *) action=run ;;
    esac
  fi
  [ "$action" = run ] && runnable=$((runnable + 1))
  plan_action+=("$action")
  plan_reason+=("$reason")
done

echo
echo "Plan, cheapest first"
i=0
for name in "${all_tests[@]}"; do
  if [ "${plan_action[$i]}" = run ]; then
    note="$(cost_of "$name")"
    [ -z "${plan_reason[$i]}" ] || note="$note; ${plan_reason[$i]}"
    printf '  %2d  %-24s run   %s\n' $((i + 1)) "$name" "$note"
  else
    printf '  %2d  %-24s skip  %s\n' $((i + 1)) "$name" "${plan_reason[$i]}"
  fi
  i=$((i + 1))
done
echo "  $runnable to run, $((${#all_tests[@]} - runnable)) to skip"
echo

if [ ${#problems[@]} -gt 0 ]; then
  if [ "$dry_run" = on ]; then
    echo "Dry run: a live run would stop at the pre-flight (${#problems[@]} problem(s) above)."
    exit 0
  fi
  echo "Pre-flight failed (${#problems[@]} problem(s) above). Nothing live ran."
  exit 2
fi
if [ "$dry_run" = on ]; then
  echo "Dry run: the pre-flight passed. Nothing ran."
  exit 0
fi
if [ "$runnable" -eq 0 ]; then
  echo "Every file is skipped. Nothing live ran."
  exit 0
fi

# --- Sweep and summary, defined before anything live so the exit handler has them

sweep_count=""
sweep_note=""

# The sweep itself. It writes its findings to files, because sweep() runs it as
# a separate job.
sweep_job() {
  local dir="$1" include_machines="$2" status count="" note=""
  if ! terraform -chdir="$dir" init -input=false -no-color > "$dir/init.log" 2>&1; then
    tail -n 20 "$dir/init.log"
    echo "the sweep's terraform init failed; see leftovers/init.log" > "$dir/note"
    return
  fi
  terraform -chdir="$dir" apply -auto-approve -input=false -no-color \
    -var "include_machines=$include_machines" 2>&1 | log_to "$run_dir/leftovers.log"
  status=${PIPESTATUS[0]}
  if [ "$status" -ne 0 ] && [ "$include_machines" = true ]; then
    echo "Listing with host machines failed; listing again without them."
    terraform -chdir="$dir" apply -auto-approve -input=false -no-color \
      -var include_machines=false 2>&1 | log_to "$run_dir/leftovers.log"
    status=${PIPESTATUS[0]}
    [ "$status" -ne 0 ] || note="host machines could not be listed; check tf-test-machine in the console"
  fi
  if [ "$status" -eq 0 ]; then
    count="$(terraform -chdir="$dir" output -raw leftover_count 2> /dev/null)"
  else
    note="the sweep failed; see leftovers.log and check the console for tf-test- objects"
  fi
  printf '%s' "$count" > "$dir/count"
  printf '%s' "$note" > "$dir/note"
  # The state holds nothing the log does not; the provider copy is large.
  rm -rf "$dir/.terraform" "$dir/.terraform.lock.hcl" "$dir/terraform.tfstate" "$dir/terraform.tfstate.backup"
}

# The sweep runs as a job in its own process group (set -m), so a Ctrl-C at the
# terminal reaches this script, which counts it, but not the sweep's
# terraform: the sweep only reads, takes seconds, and is the one list of what
# an interrupted run left behind.
sweep() {
  local dir="$run_dir/leftovers" include_machines=false i=0 name pid
  for name in "${all_tests[@]}"; do
    if [ "$name" = legacy_machine ] && [ "${plan_action[$i]}" = run ]; then include_machines=true; fi
    i=$((i + 1))
  done

  echo "=== Leftovers sweep: read-only, lists everything named tf-test- (leftovers.log). Ctrl-C does not stop it."
  sweeping=1
  mkdir -p "$dir"
  rm -f "$dir/count" "$dir/note"
  cp "$repo/scripts/leftovers/main.tf" "$dir/main.tf"
  set -m
  sweep_job "$dir" "$include_machines" < /dev/null &
  pid=$!
  set +m
  # wait returns early when a trapped signal arrives; keep waiting until the
  # job is really gone.
  while kill -0 "$pid" 2> /dev/null; do
    wait "$pid"
  done
  sweep_count="$(cat "$dir/count" 2> /dev/null)"
  sweep_note="$(cat "$dir/note" 2> /dev/null)"
  [ -e "$dir/note" ] || sweep_note="the sweep stopped before it finished; check the console for tf-test- objects"
  rm -f "$dir/count" "$dir/note"
  sweeping=0
}

summary() {
  local passed=0 failed=0 skipped=0 other=0 i=0 name row
  printf '%-26s %-12s %-8s %s\n' "file" "result" "time" "reason"
  for name in "${all_tests[@]}"; do
    row="$(printf '%-26s %-12s %-8s %s' "$name" "${result[$i]:-NOT RUN}" "${result_time[$i]:--}" "${result_reason[$i]:-}")"
    printf '%s\n' "${row%"${row##*[! ]}"}"
    case "${result[$i]:-}" in
      PASS) passed=$((passed + 1)) ;;
      FAIL) failed=$((failed + 1)) ;;
      SKIP) skipped=$((skipped + 1)) ;;
      *) other=$((other + 1)) ;;
    esac
    i=$((i + 1))
  done
  echo
  echo "$passed passed, $failed failed, $skipped skipped, $other interrupted or not run"
  if [ -n "$sweep_count" ] && [ "$sweep_count" -eq 0 ] 2> /dev/null; then
    echo "Leftovers: nothing named tf-test- on the account."
  elif [ -n "$sweep_count" ]; then
    echo "Leftovers: $sweep_count object(s) named tf-test- on the account; leftovers.log lists them. Delete them in the console, pods and clusters first."
  fi
  [ -z "$sweep_note" ] || echo "Leftovers: $sweep_note."
  if in_list account_ssh_keys "${ran_names[@]}"; then
    echo "SSH keys: the test key ending terraform-provider-test is on the account by design. Delete it and paste your own keys back."
  fi
  echo "Logs: $run_dir"
}

finish_run() {
  finished=1
  ran_names=()
  local i=0 name
  for name in "${all_tests[@]}"; do
    case "${result[$i]:-}" in PASS | FAIL | INTERRUPTED) ran_names+=("$name") ;; esac
    i=$((i + 1))
  done

  sweep
  echo
  echo "=== Summary"
  summary | log_to "$run_dir/summary.txt"

  if [ "$interrupts" -gt 0 ]; then
    exit_code=130
  elif in_list FAIL "${result[@]}"; then
    exit_code=1
  else
    exit_code=0
  fi
}

# --- The live run --------------------------------------------------------------

# A lock left by an earlier build records that binary's checksum and would
# refuse this one, so start clean, as task test-tf does.
echo "Initialising tests/ against the local mirror (init.log)"
rm -rf "$tests_dir/.terraform" "$tests_dir/.terraform.lock.hcl"
if ! terraform -chdir="$tests_dir" init -backend=false -input=false -no-color > "$run_dir/init.log" 2>&1; then
  tail -n 20 "$run_dir/init.log"
  echo "terraform init failed. Nothing live ran."
  exit 2
fi
if ! grep -q "version *= *\"$provider_version\"" "$tests_dir/.terraform.lock.hcl"; then
  grep -n "version" "$tests_dir/.terraform.lock.hcl"
  echo "terraform init did not select provider $provider_version. Nothing live ran."
  exit 2
fi

armed=1
result=()
result_time=()
result_reason=()
i=0
position=0
for name in "${all_tests[@]}"; do
  if [ "${plan_action[$i]}" != run ]; then
    result+=(SKIP)
    result_time+=("-")
    result_reason+=("${plan_reason[$i]}")
    i=$((i + 1))
    continue
  fi
  position=$((position + 1))
  if [ "$interrupts" -gt 0 ]; then
    result+=("NOT RUN")
    result_time+=("-")
    result_reason+=("interrupted before it started")
    i=$((i + 1))
    continue
  fi

  log="$run_dir/tests/$name.log"
  echo "=== [$position/$runnable] $name: $(cost_of "$name")"
  started=$SECONDS
  terraform -chdir="$tests_dir" test -no-color -filter="$name.tftest.hcl" 2>&1 | log_to "$log"
  status=${PIPESTATUS[0]}
  result_time+=("$(duration $((SECONDS - started)))")

  failed_runs="$(sed -n 's/^ *run "\([^"]*\)"\.\.\. fail.*/\1/p' "$log" | tr '\n' ' ')"
  failed_runs="${failed_runs% }"
  if [ "$interrupts" -gt 0 ] && grep -q 'Two interrupts received' "$log"; then
    echo "terraform stopped at the second Ctrl-C without destroying what $name made. The leftovers sweep lists it."
    result+=(INTERRUPTED)
    result_reason+=("second Ctrl-C: terraform exited before cleanup; see tests/$name.log and the sweep")
  elif [ "$interrupts" -gt 0 ]; then
    result+=(INTERRUPTED)
    result_reason+=("stopped by Ctrl-C; see tests/$name.log and the sweep")
  elif [ "$status" -eq 0 ] && grep -Eq 'Success! [1-9][0-9]* passed, 0 failed' "$log"; then
    result+=(PASS)
    result_reason+=("${plan_reason[$i]}")
  elif [ "$status" -eq 0 ]; then
    result+=(FAIL)
    result_reason+=("terraform test reported no passing run; see tests/$name.log")
  elif [ -n "$failed_runs" ]; then
    result+=(FAIL)
    result_reason+=("failed run(s): $failed_runs; see tests/$name.log")
  else
    result+=(FAIL)
    result_reason+=("terraform test exited $status; see tests/$name.log")
  fi
  echo "=== $name: ${result[$i]}"
  echo
  i=$((i + 1))
done

finish_run
exit "$exit_code"
