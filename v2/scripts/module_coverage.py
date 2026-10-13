#!/usr/bin/env python3
"""Coverage gate for the use-case modules.

Every resource and data source the built provider registers has to be used by
at least one module under modules/, so a new type cannot ship without a
runnable, tested configuration. Reads the provider schema `task validate-tf`
dumps to artifacts/schema.json (the registered set as the binary reports it,
not as a grep of the Go sources guesses it) and scans modules/*/*.tf for
`resource "<type>"` and `data "<type>"` blocks.

Exit status 1 lists every uncovered type; 0 prints the type → modules table.
"""

import json
import re
import sys
from pathlib import Path

BLOCK = re.compile(r'^\s*(resource|data)\s+"(runpod_[a-z0-9_]+)"', re.MULTILINE)


def registered_types(schema_path: Path) -> tuple[set[str], set[str]]:
    with schema_path.open() as fh:
        doc = json.load(fh)
    schemas = doc["provider_schemas"]
    provider = next(iter(schemas.values()))
    return set(provider.get("resource_schemas", {})), set(provider.get("data_source_schemas", {}))


def module_usage(modules_dir: Path) -> dict[tuple[str, str], set[str]]:
    usage: dict[tuple[str, str], set[str]] = {}
    for tf in sorted(modules_dir.glob("*/*.tf")):
        for kind, name in BLOCK.findall(tf.read_text()):
            usage.setdefault((kind, name), set()).add(tf.parent.name)
    return usage


def main() -> int:
    if len(sys.argv) != 3:
        print(f"usage: {sys.argv[0]} <schema.json> <modules-dir>", file=sys.stderr)
        return 2
    resources, data_sources = registered_types(Path(sys.argv[1]))
    usage = module_usage(Path(sys.argv[2]))

    missing = []
    for kind, names in (("resource", resources), ("data", data_sources)):
        for name in sorted(names):
            modules = usage.get((kind, name))
            if modules:
                print(f"{kind:8} {name:40} {', '.join(sorted(modules))}")
            else:
                missing.append(f"{kind} {name}")

    total = len(resources) + len(data_sources)
    if missing:
        print(f"\n{len(missing)} of {total} registered types are used by no module:", file=sys.stderr)
        for entry in missing:
            print(f"  {entry}", file=sys.stderr)
        return 1
    print(f"\nall {total} registered types ({len(resources)} resources, {len(data_sources)} data sources) are used by a module")
    return 0


if __name__ == "__main__":
    sys.exit(main())
