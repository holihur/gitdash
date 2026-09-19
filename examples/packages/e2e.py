#!/usr/bin/env python3
"""Automated end-to-end run of the package examples against a live instance.

Two layers:

1. ``publish.py`` + ``consume.py`` (standard library only) — always run, so the
   protocol round trip is covered even without any package manager installed.
2. A real publish + install per ecosystem, using the native client **when it is
   on ``PATH``** (npm, cargo, pip). Missing tools are reported as ``skip``.

Usage:

    export GITDASH_URL=http://127.0.0.1:8080
    export GITDASH_USER=alice
    export GITDASH_PAT=<repo-scoped PAT>
    python3 examples/packages/e2e.py

Exits non-zero if any step fails; skips are not failures. Suitable for CI on a
runner that has node / rust / python available.
"""
from __future__ import annotations

import base64
import hashlib
import io
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.request
import zipfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
URL = os.environ.get("GITDASH_URL", "http://127.0.0.1:8080").rstrip("/")
USER = os.environ.get("GITDASH_USER", "")
PAT = os.environ.get("GITDASH_PAT", "")
OWNER = os.environ.get("GITDASH_OWNER") or USER
HOST = URL.split("://", 1)[-1]
RUN = int(time.time()) % 100000

if not USER or not PAT:
    sys.exit("set GITDASH_USER and GITDASH_PAT (a repo-scoped PAT)")

BASIC = "Basic " + base64.b64encode(f"{USER}:{PAT}".encode()).decode()
RESULTS: list[tuple[str, str, str]] = []  # (name, status, detail)


def run(cmd: list[str], cwd: Path | None = None, env: dict | None = None, capture: bool = False) -> str:
    proc = subprocess.run(
        cmd, cwd=str(cwd) if cwd else None, env=env,
        capture_output=capture, text=True,
    )
    if proc.returncode != 0:
        detail = (proc.stdout or "") + (proc.stderr or "") if capture else ""
        raise RuntimeError(f"{' '.join(cmd)} failed\n{detail[-800:]}")
    return (proc.stdout or "") if capture else ""


def record(name: str, status: str, detail: str = "") -> None:
    RESULTS.append((name, status, detail))
    print(f"  {status:4} {name}" + (f" — {detail}" if detail else ""))


def step(name: str, fn) -> None:
    try:
        detail = fn()
        record(name, "ok" if detail is None else "ok", detail or "")
    except Exception as exc:  # noqa: BLE001 - runner reports and continues
        record(name, "FAIL", str(exc).splitlines()[0][:200])


# ---- layer 1: stdlib scripts -------------------------------------------------


def stdlib() -> None:
    env = {**os.environ}
    run([sys.executable, str(HERE / "publish.py")], env=env)
    run([sys.executable, str(HERE / "consume.py")], env=env)


# ---- npm ---------------------------------------------------------------------


def _npmrc(directory: Path) -> None:
    (directory / ".npmrc").write_text(
        f"registry={URL}/api/packages/npm/{OWNER}/\n"
        f"//{HOST}/api/packages/npm/{OWNER}/:_authToken={PAT}\n"
        f"//{HOST}/:_authToken={PAT}\n"
    )


def npm() -> str:
    if not shutil.which("npm"):
        record("npm (native)", "skip", "npm not installed")
        return
    version = f"1.0.{RUN}"

    def go() -> str:
        with tempfile.TemporaryDirectory() as td:
            pub = Path(td) / "pub"
            shutil.copytree(HERE / "npm/hello", pub)
            pkg = json.loads((pub / "package.json").read_text())
            pkg["version"] = version
            (pub / "package.json").write_text(json.dumps(pkg))
            _npmrc(pub)
            run(["npm", "publish", "--loglevel=error", "--no-audit", "--no-fund",
                 "--registry", f"{URL}/api/packages/npm/{OWNER}/"], cwd=pub)

            cons = Path(td) / "cons"
            shutil.copytree(HERE / "npm/consume", cons)
            pkg = json.loads((cons / "package.json").read_text())
            pkg["dependencies"] = {"hello": f"^{version}"}
            (cons / "package.json").write_text(json.dumps(pkg))
            _npmrc(cons)
            run(["npm", "install", "--loglevel=error", "--no-audit", "--no-fund"], cwd=cons)
            out = run(["node", "-e", "process.stdout.write(require('hello')())"], cwd=cons, capture=True)
            assert out.strip() == "hello from gitdash", out
        return f"hello@{version}"

    step("npm (native)", go)


# ---- cargo -------------------------------------------------------------------


def _cargo_config(directory: Path) -> None:
    (directory / ".cargo").mkdir(exist_ok=True)
    (directory / ".cargo/config.toml").write_text(
        f'[registries.gitdash]\nindex = "sparse+{URL}/api/packages/cargo/{OWNER}/index/"\n\n'
        '[registry]\nglobal-credential-providers = ["cargo:token"]\n'
    )


def cargo() -> None:
    if not shutil.which("cargo"):
        record("cargo (native)", "skip", "cargo not installed")
        return
    version = f"0.1.{RUN}"

    def go() -> str:
        with tempfile.TemporaryDirectory() as td:
            env = {**os.environ, "CARGO_HOME": str(Path(td) / "cargo-home")}
            pub = Path(td) / "pub"
            shutil.copytree(HERE / "cargo/hello-lib", pub)
            _cargo_config(pub)
            toml = (pub / "Cargo.toml").read_text()
            (pub / "Cargo.toml").write_text(re.sub(r'version = "0\.1\.0"', f'version = "{version}"', toml, count=1))
            run(["cargo", "login", "--registry", "gitdash", PAT], cwd=pub, env=env)
            run(["cargo", "publish", "--registry", "gitdash", "--allow-dirty", "--no-verify"], cwd=pub, env=env)

            cons = Path(td) / "cons"
            shutil.copytree(HERE / "cargo/consume", cons)
            _cargo_config(cons)
            toml = (cons / "Cargo.toml").read_text()
            (cons / "Cargo.toml").write_text(re.sub(r'version = "0\.1"', f'version = "{version}"', toml, count=1))
            out = run(["cargo", "run", "--quiet"], cwd=cons, env=env, capture=True)
            assert out.strip() == "hello from gitdash", out
        return f"hello-lib@{version}"

    step("cargo (native)", go)


# ---- pypi --------------------------------------------------------------------


def _wheel(name: str, version: str) -> tuple[str, bytes]:
    dist = f"hello_py-{version}.dist-info"
    files = {
        "hello_py/__init__.py": b'def hello():\n    return "hello from gitdash"\n',
        f"{dist}/METADATA": (
            f"Metadata-Version: 2.1\nName: {name}\nVersion: {version}\n"
            "Summary: gitdash package registry demo\nRequires-Python: >=3.9\n"
        ).encode(),
        f"{dist}/WHEEL": b"Wheel-Version: 1.0\nGenerator: gitdash-example\nRoot-Is-Purelib: true\nTag: py3-none-any\n",
    }
    record_lines = [
        f"{path},sha256={base64.urlsafe_b64encode(hashlib.sha256(data).digest()).rstrip(b'=').decode()},{len(data)}"
        for path, data in files.items()
    ]
    record_lines.append(f"{dist}/RECORD,,")
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as zf:
        for path, data in files.items():
            zf.writestr(path, data)
        zf.writestr(f"{dist}/RECORD", "\n".join(record_lines) + "\n")
    return f"hello_py-{version}-py3-none-any.whl", buf.getvalue()


def pypi() -> None:
    pip = shutil.which("pip") or shutil.which("pip3")
    if not pip:
        record("pypi (native)", "skip", "pip not installed")
        return
    version = f"1.0.{RUN}"

    def go() -> str:
        fname, wheel = _wheel("hello-py", version)
        boundary = "----gitdash" + os.urandom(8).hex()
        body = b"".join([
            f'--{boundary}\r\nContent-Disposition: form-data; name="name"\r\n\r\nhello-py\r\n'.encode(),
            f'--{boundary}\r\nContent-Disposition: form-data; name="version"\r\n\r\n{version}\r\n'.encode(),
            f'--{boundary}\r\nContent-Disposition: form-data; name="content"; filename="{fname}"\r\n'
            f"Content-Type: application/octet-stream\r\n\r\n".encode() + wheel + b"\r\n",
            f"--{boundary}--\r\n".encode(),
        ])
        req = urllib.request.Request(f"{URL}/api/packages/pypi/{OWNER}/", data=body, method="POST")
        req.add_header("Authorization", BASIC)
        req.add_header("Content-Type", f"multipart/form-data; boundary={boundary}")
        with urllib.request.urlopen(req, timeout=60) as resp:
            assert resp.status == 200, resp.status

        with tempfile.TemporaryDirectory() as td:
            target = Path(td) / "site"
            run([pip, "install", "--disable-pip-version-check", "--no-cache-dir",
                 "--index-url", f"http://{USER}:{PAT}@{HOST}/api/packages/pypi/{OWNER}/simple",
                 "--trusted-host", HOST.split(":")[0], "--target", str(target), "hello-py"])
            env = {**os.environ, "PYTHONPATH": str(target)}
            out = run([sys.executable, "-c", "import hello_py; print(hello_py.hello())"], env=env, capture=True)
            assert out.strip() == "hello from gitdash", out
        return f"hello-py@{version}"

    step("pypi (native)", go)


# ---- go ----------------------------------------------------------------------


def goproxy() -> None:
    if not shutil.which("go"):
        record("go (native)", "skip", "go not installed")
        return
    if URL.startswith("http://"):
        # The go command refuses to send GOPROXY credentials over plain HTTP,
        # so a private proxy needs HTTPS. (publish.py/consume.py still cover
        # the Go module protocol itself.)
        record("go (native)", "skip", "go client needs HTTPS to send credentials")
        return

    def go() -> str:
        with tempfile.TemporaryDirectory() as td:
            cons = Path(td) / "cons"
            shutil.copytree(HERE / "go/consume", cons)
            env = {
                **os.environ,
                "GOPROXY": f"http://{USER}:{PAT}@{HOST}/api/packages/go/{OWNER}",
                "GOSUMDB": "off",
                "GOFLAGS": "-insecure",
                "GOMODCACHE": str(Path(td) / "modcache"),
            }
            run(["go", "mod", "tidy"], cwd=cons, env=env)
            out = run(["go", "run", "."], cwd=cons, env=env, capture=True)
            assert out.strip() == "hello from gitdash", out
        return "example.com/gitdash/hello@v1.0.0"

    step("go (native)", go)


def main() -> int:
    print(f"running package examples against {URL} as {USER} (namespace {OWNER})")
    step("publish.py + consume.py", stdlib)
    npm()
    cargo()
    pypi()
    goproxy()

    skipped = [n for n, s, _ in RESULTS if s == "skip"]
    failed = [(n, d) for n, s, d in RESULTS if s == "FAIL"]
    print(f"\n{len(RESULTS) - len(skipped) - len(failed)}/{len(RESULTS)} ran"
          + (f", {len(skipped)} skipped" if skipped else ""))
    for name, detail in failed:
        print(f"  FAIL {name}: {detail}", file=sys.stderr)
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
