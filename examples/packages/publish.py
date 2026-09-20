#!/usr/bin/env python3
"""Publish one minimal package per gitdash registry, then verify it downloads.

This is the runnable companion to the example projects in this directory: it
exercises the whole private package registry (npm / pypi / composer / cargo /
go / rubygems / maven) over plain HTTP using only the Python standard library,
so it works even when the native package manager toolchains are not installed.

It doubles as a manual end-to-end test of a running gitdash instance:

    export GITDASH_URL=http://127.0.0.1:8080
    export GITDASH_USER=alice
    export GITDASH_PAT=<repo-scoped PAT>
    python3 examples/packages/publish.py

Create the PAT under *Settings -> Keys -> PAT* with the ``repo`` scope.
Use the native example projects in this directory with the real tools
(``npm publish`` / ``twine`` / ``cargo publish`` / ...) as described in
the packages docs at https://holihur.github.io/gitdash/packages/publish-install/.
"""
from __future__ import annotations

import base64
import gzip
import hashlib
import io
import json
import os
import re
import sys
import tarfile
import urllib.error
import urllib.request
import uuid
import zipfile

BASE = os.environ.get("GITDASH_URL", "http://127.0.0.1:8080").rstrip("/")
USER = os.environ.get("GITDASH_USER", "")
PAT = os.environ.get("GITDASH_PAT", "")

if not USER or not PAT:
    sys.exit("set GITDASH_USER and GITDASH_PAT (a repo-scoped personal access token)")

SUFFIX = uuid.uuid4().hex[:6]
AUTH = base64.b64encode(f"{USER}:{PAT}".encode()).decode()

PASSED: list[str] = []


def request(
    method: str,
    path: str,
    data: bytes | None = None,
    headers: dict[str, str] | None = None,
    expect: int | tuple[int, ...] | None = None,
) -> tuple[int, bytes]:
    url = path if path.startswith("http") else BASE + path
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Authorization", f"Basic {AUTH}")
    for key, value in (headers or {}).items():
        req.add_header(key, value)
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            status, body = resp.status, resp.read()
    except urllib.error.HTTPError as exc:
        status, body = exc.code, exc.read()
    if expect is not None:
        allowed = expect if isinstance(expect, tuple) else (expect,)
        if status not in allowed:
            raise RuntimeError(f"{method} {path} -> {status} (expected {allowed}): {body[:300]!r}")
    return status, body


def ok(name: str) -> None:
    PASSED.append(name)
    print(f"  ok  {name}")


def tgz(files: dict[str, bytes]) -> bytes:
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w:gz") as tf:
        for name, content in files.items():
            info = tarfile.TarInfo(name)
            info.size = len(content)
            tf.addfile(info, io.BytesIO(content))
    return buf.getvalue()


def zip_bytes(files: dict[str, bytes]) -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as zf:
        for name, content in files.items():
            zf.writestr(name, content)
    return buf.getvalue()


def multipart(fields: dict[str, str], files: dict[str, tuple[str, bytes]]) -> tuple[bytes, str]:
    boundary = "----gitdash" + uuid.uuid4().hex
    chunks: list[bytes] = []
    for key, value in fields.items():
        chunks.append(
            f'--{boundary}\r\nContent-Disposition: form-data; name="{key}"\r\n\r\n{value}\r\n'.encode()
        )
    for key, (filename, content) in files.items():
        chunks.append(
            f'--{boundary}\r\nContent-Disposition: form-data; name="{key}"; '
            f'filename="{filename}"\r\nContent-Type: application/octet-stream\r\n\r\n'.encode()
            + content
            + b"\r\n"
        )
    chunks.append(f"--{boundary}--\r\n".encode())
    return b"".join(chunks), f"multipart/form-data; boundary={boundary}"


def json_body(path: str) -> dict:
    _, body = request("GET", path, expect=200)
    return json.loads(body)


# ---- npm -----------------------------------------------------------------


def npm() -> None:
    name = f"hello-{SUFFIX}"
    tarball = tgz(
        {
            "package/package.json": json.dumps({"name": name, "version": "1.0.0"}).encode(),
            "package/index.js": b'module.exports = () => "hello from gitdash";\n',
        }
    )
    body = json.dumps(
        {
            "_id": name,
            "name": name,
            "dist-tags": {"latest": "1.0.0"},
            "versions": {"1.0.0": {"name": name, "version": "1.0.0"}},
            "_attachments": {
                f"{name}-1.0.0.tgz": {
                    "content_type": "application/octet-stream",
                    "length": len(tarball),
                    "data": base64.b64encode(tarball).decode(),
                }
            },
        }
    ).encode()
    request("PUT", f"/api/packages/npm/{USER}/{name}", body, {"Content-Type": "application/json"}, 201)
    meta = json_body(f"/api/packages/npm/{USER}/{name}")
    assert meta["dist-tags"]["latest"] == "1.0.0"
    _, got = request("GET", meta["versions"]["1.0.0"]["dist"]["tarball"], expect=200)
    assert got == tarball
    ok("npm")


# ---- pypi ------------------------------------------------------------------


def pypi() -> None:
    name = f"hello_py_{SUFFIX}"
    wheel = zip_bytes(
        {
            f"{name}-1.0.0.dist-info/METADATA": (
                f"Metadata-Version: 2.1\nName: {name}\nVersion: 1.0.0\n"
            ).encode(),
            f"{name}/__init__.py": b'def hello():\n    return "hello from gitdash"\n',
        }
    )
    payload, content_type = multipart(
        {"name": name, "version": "1.0.0"},
        {"content": (f"{name}-1.0.0-py3-none-any.whl", wheel)},
    )
    request("POST", f"/api/packages/pypi/{USER}/", payload, {"Content-Type": content_type}, 200)
    _, index = request("GET", f"/api/packages/pypi/{USER}/simple/{name}/", expect=200)
    assert f"{name}-1.0.0-py3-none-any.whl" in index.decode()
    href = re.search(r'href="([^"]+)"', index.decode()).group(1)
    _, got = request("GET", href, expect=200)
    assert got == wheel
    ok("pypi")


# ---- composer --------------------------------------------------------------


def composer() -> None:
    name = f"hello-{SUFFIX}"
    archive = zip_bytes(
        {
            "composer.json": json.dumps({"name": f"{USER}/{name}", "version": "1.0.0"}).encode(),
            "src/hello.php": b'<?php function hello() { return "hello from gitdash"; }\n',
        }
    )
    request(
        "PUT",
        f"/api/packages/composer/{USER}/{USER}/{name}?version=1.0.0",
        archive,
        None,
        201,
    )
    pkgs = json_body(f"/api/packages/composer/{USER}/packages.json")
    entry = pkgs["packages"][f"{USER}/{name}"]["1.0.0"]
    _, got = request("GET", entry["dist"]["url"], expect=200)
    assert got == archive
    ok("composer")


# ---- cargo -----------------------------------------------------------------


def _crate(name: str) -> bytes:
    cargo_toml = f'[package]\nname = "{name}"\nversion = "0.1.0"\n'.encode()
    tar = io.BytesIO()
    with tarfile.open(fileobj=tar, mode="w") as tf:
        info = tarfile.TarInfo(f"{name}-0.1.0/Cargo.toml")
        info.size = len(cargo_toml)
        tf.addfile(info, io.BytesIO(cargo_toml))
    gz = io.BytesIO()
    with gzip.GzipFile(fileobj=gz, mode="wb") as fh:
        fh.write(tar.getvalue())
    return gz.getvalue()


def cargo() -> None:
    name = f"hello_lib_{SUFFIX}"
    crate = _crate(name)
    cksum = hashlib.sha256(crate).hexdigest()
    meta = json.dumps(
        {"name": name, "vers": "0.1.0", "deps": [], "cksum": cksum, "features": {}}
    ).encode() + b"\n"
    base = f"/api/packages/cargo/{USER}"
    cfg = json_body(f"{base}/config.json")
    request("PUT", f"{base}/api/v1/crates/new", meta + crate, None, (200, 201))
    _, index = request("GET", f"{base}/index/{name[:2]}/{name[2:4]}/{name}", expect=200)
    assert json.loads(index.decode().splitlines()[0])["vers"] == "0.1.0"
    _, got = request("GET", f"{cfg['dl']}/{name}/0.1.0/{name}-0.1.0.crate", expect=200)
    assert got == crate
    ok("cargo")


# ---- go --------------------------------------------------------------------


def goproxy() -> None:
    name = f"hello-{SUFFIX}"
    module = f"example.com/{USER}/{name}"
    version = "v1.0.0"
    mod = f"module {module}\n\ngo 1.22\n".encode()
    archive = zip_bytes(
        {
            f"{name}@{version}/go.mod": mod,
            f"{name}@{version}/hello.go": b'package hello\n\nfunc Hello() string { return "hello from gitdash" }\n',
        }
    )
    base = f"/api/packages/go/{USER}/{module}"
    request("PUT", f"{base}/@v/{version}.zip", archive, None, (200, 201))
    _, listing = request("GET", f"{base}/@v/list", expect=200)
    assert version in listing.decode()
    _, got = request("GET", f"{base}/@v/{version}.zip", expect=200)
    assert got == archive
    ok("go")


# ---- rubygems --------------------------------------------------------------


def rubygems() -> None:
    name = f"hello-{SUFFIX}"
    spec = (
        "--- !ruby/object:Gem::Specification\n"
        f"name: {name}\n"
        "version: !ruby/object:Gem::Version\n"
        "  version: 1.0.0\n"
    ).encode()
    gz = io.BytesIO()
    with gzip.GzipFile(fileobj=gz, mode="wb") as fh:
        fh.write(spec)
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w") as tf:
        data = gz.getvalue()
        info = tarfile.TarInfo("metadata.gz")
        info.size = len(data)
        tf.addfile(info, io.BytesIO(data))
    gem = buf.getvalue()

    request("POST", f"/api/packages/rubygems/{USER}/api/v1/gems", gem, None, 201)
    _, got = request("GET", f"/api/packages/rubygems/{USER}/gems/{name}-1.0.0.gem", expect=200)
    assert got == gem
    ok("rubygems")


# ---- maven -----------------------------------------------------------------


def maven() -> None:
    group = f"com/example/{USER}"
    art = f"hello-{SUFFIX}"
    base = f"/api/packages/maven/{USER}/{group}/{art}"
    jar = b"PK\x03\x04 demo jar bytes"
    pom = b"<project><modelVersion>4.0.0</modelVersion></project>"
    request("PUT", f"{base}/1.0.0/{art}-1.0.0.jar", jar, None, 201)
    request("PUT", f"{base}/1.0.0/{art}-1.0.0.pom", pom, None, 201)
    _, got = request("GET", f"{base}/1.0.0/{art}-1.0.0.jar", expect=200)
    assert got == jar
    _, meta = request("GET", f"{base}/maven-metadata.xml", expect=200)
    assert b"<version>1.0.0</version>" in meta
    ok("maven")


def main() -> int:
    print(f"publishing examples to {BASE} as {USER}")
    for name, fn in [
        ("npm", npm),
        ("pypi", pypi),
        ("composer", composer),
        ("cargo", cargo),
        ("go", goproxy),
        ("rubygems", rubygems),
        ("maven", maven),
    ]:
        try:
            fn()
        except Exception as exc:  # noqa: BLE001 - demo script: report and keep going
            print(f"FAIL  {name}: {exc}", file=sys.stderr)
    print(f"\n{len(PASSED)}/7 registries passed")
    return 0 if len(PASSED) == 7 else 1


if __name__ == "__main__":
    raise SystemExit(main())
