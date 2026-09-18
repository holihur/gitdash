#!/usr/bin/env python3
"""UI-reachable API surface coverage.

The web UI calls a subset of the API. This script extracts the endpoints the
frontend actually calls (statically, from ``frontend/src``) and checks which of
them were exercised by a Playwright run that recorded route hits via
``GITDASH_ROUTE_COVERAGE_FILE`` (see ``internal/api/routecov.go``).

Two useful signals:
  * coverage   — UI-called endpoints a page flow actually hit
  * unknowns   — UI-called paths that match no registered route, i.e. a
                 frontend/backend mismatch (a likely bug)

Matching is structural: methods and every dynamic segment are normalized, so
``GET /users/${owner}/repos/${name}`` matches ``GET /api/users/{owner}/repos/{name}``.

Usage:
    scripts/ui-api-coverage.py --src frontend/src \\
        --coverage /tmp/gitdash-route-coverage-ui.txt [--min 100] [--list-missing]
"""
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

HELPERS = ("reqPage", "sendForm", "req", "send")
OPEN_RE = re.compile(r"\b(?:reqPage|sendForm|req|send)\s*(?:<[^>]*>)?\s*\(")
LITERAL_RE = re.compile(r"^\s*(?P<q>`|\"|')(?P<path>.*?)(?P=q)", re.DOTALL)
METHOD_RE = re.compile(r"method\s*:\s*[\"']([A-Za-z]+)[\"']")
# ${...} allowing one level of nested braces (e.g. `${type ? `/${type}` : ""}`)
INTERP_RE = re.compile(r"\$\{(?:[^{}]|\{(?:[^{}]|\{[^{}]*\})*\})*\}")


def read_call(text: str, open_paren: int) -> str:
    """Return the call text from ``open_paren`` to its matching close paren."""
    depth = 0
    i = open_paren
    quote = ""
    while i < len(text):
        ch = text[i]
        if quote:
            if ch == "\\":
                i += 2
                continue
            if ch == quote:
                quote = ""
            i += 1
            continue
        if ch in "`\"'":
            quote = ch
        elif ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
            if depth == 0:
                return text[open_paren: i + 1]
        i += 1
    return text[open_paren:]


def clean_raw(raw: str) -> str | None:
    """Normalize a frontend path literal; None if too dynamic to interpret."""
    raw = raw.split("?", 1)[0]

    def repl(m: re.Match[str]) -> str:
        body = m.group(0)
        if any(k in body for k in ("pageQuery", "URLSearchParams", "qs")):
            return ""  # query-string builder, not a path segment
        return "{}"

    raw = INTERP_RE.sub(repl, raw)
    raw = raw.replace("...", "{}")
    if "$" in raw or "?" in raw:
        return None  # too dynamic (object spread, conditional segment, ...)

    # drop interpolations glued to a segment (e.g. "/inbox{}" from a query var)
    raw = re.sub(r"(?<!/)\{}", "", raw)
    if raw.startswith("/api/"):
        pass
    elif raw.startswith("/"):
        raw = "/api" + raw
    else:
        raw = "/api/" + raw
    raw = re.sub(r"/{2,}", "/", raw)
    return raw.rstrip("/") or "/"


def extract_frontend(src: Path) -> set[str]:
    out: set[str] = set()
    for f in src.rglob("*"):
        if f.suffix not in (".ts", ".tsx"):
            continue
        text = f.read_text(encoding="utf-8")
        for m in OPEN_RE.finditer(text):
            call = read_call(text, m.end() - 1)
            lit = LITERAL_RE.match(call[1:])
            if not lit:
                continue
            path = clean_raw(lit.group("path"))
            if path is None:
                continue
            mm = METHOD_RE.search(call)
            if mm:
                method = mm.group(1).upper()
            elif "sendForm" in m.group(0):
                method = "POST"
            else:
                method = "GET"
            out.add(f"{method} {path}")
    return out


def normalize_pattern(pattern: str) -> str:
    if not re.match(r"^[A-Z]+ ", pattern):
        pattern = "GET " + pattern
    method, _, path = pattern.partition(" ")
    path = path.split("?", 1)[0]
    path = re.sub(r"\{[^}]*\.\.\.\}", "{...}", path)  # {rest...} = multi-segment
    path = re.sub(r"\{[^}]*\}", "{}", path)
    path = re.sub(r"/{2,}", "/", path).rstrip("/") or "/"
    return f"{method.upper()} {path}"


def route_matches(a: str, b: str) -> bool:
    """Structural match: literal segments can match a `{}`/`{...}` wildcard."""
    am, _, ap = a.partition(" ")
    bm, _, bp = b.partition(" ")
    if am != bm:
        return False
    asg = [s for s in ap.split("/") if s != ""]
    bsg = [s for s in bp.split("/") if s != ""]
    i = j = 0
    while i < len(asg) and j < len(bsg):
        x, y = asg[i], bsg[j]
        if x == "{...}" or y == "{...}":
            # consume the rest of the other side, then both must be exhausted
            return i == len(asg) - 1 or j == len(bsg) - 1
        if x == "{}" or y == "{}" or x == y:
            i += 1
            j += 1
            continue
        return False
    return i == len(asg) and j == len(bsg)


def parse_coverage(path: Path) -> tuple[set[str], set[str]]:
    inventory: set[str] = set()
    hits: set[str] = set()
    for line in path.read_text().splitlines():
        kind, _, pattern = line.partition("\t")
        if not pattern:
            continue
        norm = normalize_pattern(pattern)
        if kind == "route":
            inventory.add(norm)
        elif kind == "hit":
            hits.add(norm)
    return inventory, hits


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--src", type=Path, required=True, help="frontend source root")
    ap.add_argument("--coverage", type=Path, required=True, help="GITDASH_ROUTE_COVERAGE_FILE")
    ap.add_argument("--min", type=float, default=0.0)
    ap.add_argument("--list-missing", action="store_true")
    args = ap.parse_args()

    if not args.coverage.is_file():
        print(f"coverage file not found: {args.coverage}", file=sys.stderr)
        return 2

    expected = extract_frontend(args.src)
    inventory, hits = parse_coverage(args.coverage)

    known = {e for e in expected if any(route_matches(e, inv) for inv in inventory)}
    unknown = sorted(expected - known)
    covered = {e for e in known if any(route_matches(e, h) for h in hits)}
    missing = sorted(known - covered)
    pct = 100.0 * len(covered) / len(known) if known else 100.0

    print(f"UI-reachable API surface: {len(expected)} endpoints called by the frontend")
    print(f"UI endpoint coverage: {len(covered)}/{len(known)} = {pct:.1f}%")
    if unknown:
        print(f"WARNING: {len(unknown)} frontend call(s) match no registered route:")
        for u in unknown[:20]:
            print(f"  ? {u}")
        if len(unknown) > 20:
            print(f"  ... and {len(unknown) - 20} more")
    if missing:
        print(f"not exercised by UI flows ({len(missing)}):")
        for u in (missing if args.list_missing else missing[:25]):
            print(f"  - {u}")
        if not args.list_missing and len(missing) > 25:
            print(f"  ... and {len(missing) - 25} more (use --list-missing)")

    if pct + 1e-9 < args.min:
        print(f"FAIL: {pct:.1f}% < required {args.min:.1f}%", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
