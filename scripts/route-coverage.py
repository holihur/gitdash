#!/usr/bin/env python3
"""Report black-box API endpoint coverage from a GITDASH_ROUTE_COVERAGE_FILE.

The server (when started with GITDASH_ROUTE_COVERAGE_FILE=/path) appends
``route<TAB>PATTERN`` lines for its inventory and ``hit<TAB>PATTERN`` lines for
every route pattern actually exercised. This script compares the two sets for
the API surface and exits non-zero when coverage is below ``--min``.

Usage:
    scripts/route-coverage.py /tmp/gitdash-routes.txt [--min 100] [--list-missing]
"""
from __future__ import annotations

import argparse
import sys
from pathlib import Path

# API surface: routes we require black-box tests to exercise. The catch-all
# static route ("/") and the login/oauth HTML pages are included where they are
# part of the API surface.
DEFAULT_PREFIXES = ("/api/", "/login/", "/v2/", "/metrics")


def parse(path: Path) -> tuple[set[str], set[str]]:
    inventory: set[str] = set()
    hits: set[str] = set()
    for line in path.read_text().splitlines():
        kind, _, pattern = line.partition("\t")
        if not pattern:
            continue
        if kind == "route":
            inventory.add(pattern)
        elif kind == "hit":
            hits.add(pattern)
    return inventory, hits


def matches(pattern: str, prefixes: tuple[str, ...], exclude: tuple[str, ...]) -> bool:
    if pattern in exclude:
        return False
    # Patterns carry a method prefix ("GET /api/x"); non-method routes
    # (e.g. "/v2/") do not. Match on the path portion only.
    _, _, path = pattern.partition(" ")
    if not path:
        path = pattern
    if path in exclude:
        return False
    return any(path.startswith(p) for p in prefixes)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("coverage_file", type=Path)
    ap.add_argument("--min", type=float, default=0.0, help="minimum percent required")
    ap.add_argument("--list-missing", action="store_true")
    ap.add_argument("--prefixes", default=",".join(DEFAULT_PREFIXES))
    ap.add_argument("--exclude", default="/")
    args = ap.parse_args()

    if not args.coverage_file.is_file():
        print(f"route coverage file not found: {args.coverage_file}", file=sys.stderr)
        return 2

    prefixes = tuple(p for p in args.prefixes.split(",") if p)
    exclude = tuple(p for p in args.exclude.split(",") if p)
    inventory, hits = parse(args.coverage_file)
    wanted = {p for p in inventory if matches(p, prefixes, exclude)}
    hit = {p for p in hits if matches(p, prefixes, exclude)}
    missing = sorted(wanted - hit)
    pct = 100.0 * len(wanted & hit) / len(wanted) if wanted else 100.0

    print(
        f"API endpoint coverage: {len(wanted & hit)}/{len(wanted)} = {pct:.1f}% "
        f"({len(hit - wanted)} extra non-inventory hits)"
    )
    if missing:
        print(f"missing {len(missing)} endpoint(s):")
        if args.list_missing:
            for p in missing:
                print(f"  - {p}")
        else:
            for p in missing[:20]:
                print(f"  - {p}")
            if len(missing) > 20:
                print(f"  ... and {len(missing) - 20} more (use --list-missing)")

    if pct + 1e-9 < args.min:
        print(f"FAIL: {pct:.1f}% < required {args.min:.1f}%", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
