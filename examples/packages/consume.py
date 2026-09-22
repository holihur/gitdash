#!/usr/bin/env python3
"""Consume the packages in a gitdash namespace, one per registry.

Companion to ``publish.py``: that one publishes, this one proves the packages
are *usable* — it lists the owner's packages, pulls each one back through the
protocol the real client uses (npm packument + tarball, pypi simple index, ...)
and prints the copy-pasteable install command for the native tool.

Only the Python standard library is used, so it needs no package manager
toolchains and doubles as a manual read-side end-to-end test:

    export GITDASH_URL=http://127.0.0.1:8080
    export GITDASH_USER=bob          # any user that may read the packages
    export GITDASH_PAT=<repo-scoped PAT>
    export GITDASH_OWNER=alice       # namespace to consume (default: GITDASH_USER)
    python3 examples/packages/consume/consume.py

The example consumer projects next to this script show the matching native
configuration (``.npmrc`` / ``pip.conf`` / ``.cargo/config.toml`` / ...).
"""
from __future__ import annotations

import base64
import json
import os
import re
import sys
import urllib.error
import urllib.request

BASE = os.environ.get("GITDASH_URL", "http://127.0.0.1:8080").rstrip("/")
USER = os.environ.get("GITDASH_USER", "")
PAT = os.environ.get("GITDASH_PAT", "")
OWNER = os.environ.get("GITDASH_OWNER") or USER

if not USER or not PAT:
    sys.exit("set GITDASH_USER and GITDASH_PAT (a PAT that may read the packages)")

HOST = BASE.replace("https://", "").replace("http://", "").rstrip("/")
AUTH = base64.b64encode(f"{USER}:{PAT}".encode()).decode()
CONSUMED: list[str] = []


def get(path: str, expect: int = 200) -> bytes:
    url = path if path.startswith("http") else BASE + path
    req = urllib.request.Request(url)
    req.add_header("Authorization", f"Basic {AUTH}")
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            status, body = resp.status, resp.read()
    except urllib.error.HTTPError as exc:
        status, body = exc.code, exc.read()
    if status != expect:
        raise RuntimeError(f"GET {path} -> {status} (expected {expect}): {body[:200]!r}")
    return body


def ok(type_: str, detail: str) -> None:
    CONSUMED.append(type_)
    print(f"  ok  {type_:9} {detail}")


def install_command(pkg: dict) -> str:
    t, owner, name, version = pkg["type"], pkg["owner"], pkg["name"], pkg["version"]
    if t == "npm":
        return f"npm install {name} --registry {BASE}/api/packages/npm/{owner}/"
    if t == "pypi":
        return f"pip install {name} --index-url http://<user>:<PAT>@{HOST}/api/packages/pypi/{owner}/simple"
    if t == "composer":
        return f"composer require {name}"
    if t == "cargo":
        return f"cargo add {name} --registry gitdash"
    if t == "go":
        return f"go get {name}@{version}"
    if t == "rubygems":
        return f"gem install {name} --source http://<user>:<PAT>@{HOST}/api/packages/rubygems/{owner}"
    if t == "maven":
        parts = name.split("/")
        return f"mvn dependency:get -DremoteRepositories=gitdash -Dartifact={'.'.join(parts[:-1])}:{parts[-1]}:{version}"
    if t == "apt":
        return f"deb [trusted=yes] {BASE}/api/packages/apt/{owner}/{name} stable main"
    if t == "yum":
        return f"[gitdash]\\nbaseurl={BASE}/api/packages/yum/{owner}/{name}\\ngpgcheck=0"
    if t == "apk":
        return f"{BASE}/api/packages/apk/{owner}/{name}"
    if t == "brew":
        return f"brew tap {owner}/{name}"
    if t == "snap":
        return f"curl -LO {BASE}/api/packages/snap/{owner}/{name}/download/{version}"
    return f"{name}@{version}"


def consume(pkg: dict) -> str:
    t, owner, name, version, filename = (
        pkg["type"], pkg["owner"], pkg["name"], pkg["version"], pkg["filename"],
    )
    if t == "npm":
        meta = json.loads(get(f"/api/packages/npm/{owner}/{name}"))
        get(meta["versions"][version]["dist"]["tarball"])
    elif t == "pypi":
        index = get(f"/api/packages/pypi/{owner}/simple/{name}/").decode()
        get(re.search(r'href="([^"]+)"', index).group(1))
    elif t == "composer":
        pkgs = json.loads(get(f"/api/packages/composer/{owner}/packages.json"))
        get(pkgs["packages"][name][version]["dist"]["url"])
    elif t == "cargo":
        cfg = json.loads(get(f"/api/packages/cargo/{owner}/config.json"))
        get(f"/api/packages/cargo/{owner}/index/{name[:2]}/{name[2:4]}/{name}")
        get(f"{cfg['dl']}/{name}/{version}/{filename}")
    elif t == "go":
        get(f"/api/packages/go/{owner}/{name}/@v/list")
        get(f"/api/packages/go/{owner}/{name}/@v/{version}.zip")
    elif t == "rubygems":
        get(f"/api/packages/rubygems/{owner}/gems/{filename}")
    elif t == "maven":
        get(f"/api/packages/maven/{owner}/{name}/{version}/{filename}")
    elif t == "apt":
        arch = json.loads(pkg.get("meta") or "{}").get("arch", "amd64")
        index = get(f"/api/packages/apt/{owner}/{name}/dists/stable/main/binary-{arch}/Packages").decode()
        get(f"/api/packages/apt/{owner}/{name}/" + re.search(r"Filename: (\S+)", index).group(1))
    elif t == "yum":
        get(f"/api/packages/yum/{owner}/{name}/repodata/repomd.xml")
        get(f"/api/packages/yum/{owner}/{name}/{filename}")
    elif t == "apk":
        arch = json.loads(pkg.get("meta") or "{}").get("arch", "x86_64")
        get(f"/api/packages/apk/{owner}/{name}/{arch}/APKINDEX.tar.gz")
        get(f"/api/packages/apk/{owner}/{name}/{arch}/{filename}")
    elif t == "brew":
        pkg_name = json.loads(pkg.get("meta") or "{}").get("name", name)
        get(f"/api/packages/brew/{owner}/{name}/api/formula/{pkg_name}.json")
        get(f"/api/packages/brew/{owner}/{name}/bottles/{filename}")
    elif t == "snap":
        get(f"/api/packages/snap/{owner}/{name}/index.json")
        get(f"/api/packages/snap/{owner}/{name}/download/{filename}")
    else:
        raise RuntimeError(f"unsupported type {t}")
    return install_command(pkg)


def main() -> int:
    print(f"consuming {OWNER}'s packages from {BASE} as {USER}")
    registry = json.loads(get(f"/api/packages/{OWNER}"))
    # 每个类型取一个包演示该生态的读取路径
    seen: dict[str, dict] = {}
    for p in registry:
        seen.setdefault(p["type"], p)
    if not seen:
        sys.exit(f"no packages in {OWNER}'s namespace — publish some first (see publish.py)")

    for t, pkg in sorted(seen.items()):
        try:
            cmd = consume(pkg)
        except Exception as exc:  # noqa: BLE001 - demo script: report and keep going
            print(f"FAIL  {t}: {exc}", file=sys.stderr)
            continue
        ok(t, cmd)

    print(f"\n{len(CONSUMED)}/{len(seen)} registries consumed")
    return 0 if len(CONSUMED) == len(seen) else 1


if __name__ == "__main__":
    raise SystemExit(main())
