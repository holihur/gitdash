"""私有包注册表黑盒测试（npm / pypi / go / cargo / rubygems / composer / maven）。

所有包管理器客户端走 Basic 认证（密码 = PAT），测试覆盖：
- 未认证 401
- 非 owner 发布 403
- 发布 → 元数据/索引 → 下载闭环
"""
from __future__ import annotations

import base64
import gzip
import io
import json
import tarfile
import uuid

import pytest
import requests


@pytest.fixture
def pkg_user(user_factory, client_factory):
    """注册用户 + repo scope PAT，返回 (username, pat_token, client)。"""
    username, token, client = user_factory("pkg")
    resp = client.post("/tokens", json={"name": "pkg", "scopes": ["repo"]}, expect=201)
    return username, resp.json()["token"], client_factory(resp.json()["token"])


def basic(username: str, pat: str) -> dict:
    creds = base64.b64encode(f"{username}:{pat}".encode()).decode()
    return {"Authorization": f"Basic {creds}"}


# ---- 通用 ----


def test_packages_require_auth(base_url, pkg_user):
    username, _, _ = pkg_user
    r = requests.get(f"{base_url}/api/packages/npm/{username}/some-pkg", timeout=10)
    assert r.status_code == 401


def test_publish_forbidden_for_other_user(base_url, pkg_user, user_factory):
    username, pat, _ = pkg_user
    other, _, _ = user_factory("pkg")
    r = requests.put(
        f"{base_url}/api/packages/npm/{other}/lib",
        json={"name": "lib", "versions": {"1.0.0": {}}, "_attachments": {}},
        headers=basic(username, pat),
        timeout=10,
    )
    assert r.status_code == 403


# ---- npm ----


def test_npm_publish_packument_tarball(base_url, pkg_user):
    username, pat, _ = pkg_user
    pkg = f"mypkg-{uuid.uuid4().hex[:8]}"
    tarball = b"fake-tgz-content"
    body = {
        "_id": pkg,
        "name": pkg,
        "versions": {"1.0.0": {"name": pkg, "version": "1.0.0"}},
        "_attachments": {f"{pkg}-1.0.0.tgz": {"length": len(tarball), "data": base64.b64encode(tarball).decode()}},
    }
    r = requests.put(
        f"{base_url}/api/packages/npm/{username}/{pkg}",
        json=body, headers=basic(username, pat), timeout=10,
    )
    assert r.status_code == 201

    meta = requests.get(
        f"{base_url}/api/packages/npm/{username}/{pkg}",
        headers=basic(username, pat), timeout=10,
    ).json()
    assert meta["name"] == pkg
    assert meta["dist-tags"]["latest"] == "1.0.0"
    tar_url = meta["versions"]["1.0.0"]["dist"]["tarball"]
    dl = requests.get(tar_url, headers=basic(username, pat), timeout=10)
    assert dl.status_code == 200
    assert dl.content == tarball


# ---- pypi (twine) ----


def test_pypi_upload_simple_index_download(base_url, pkg_user):
    username, pat, _ = pkg_user
    pkg = f"mypkg_{uuid.uuid4().hex[:8]}"
    files = {"content": (f"{pkg}-1.0.0-py3-none-any.whl", b"wheel-bytes")}
    data = {"name": pkg, "version": "1.0.0"}
    r = requests.post(
        f"{base_url}/api/packages/pypi/{username}/",
        data=data, files=files, headers=basic(username, pat), timeout=10,
    )
    assert r.status_code == 200

    index = requests.get(
        f"{base_url}/api/packages/pypi/{username}/simple/", headers=basic(username, pat), timeout=10
    )
    assert pkg in index.text

    proj = requests.get(
        f"{base_url}/api/packages/pypi/{username}/simple/{pkg}/", headers=basic(username, pat), timeout=10
    )
    assert f"{pkg}-1.0.0-py3-none-any.whl" in proj.text
    dl_url = proj.text.split('href="')[1].split('"')[0]
    dl = requests.get(dl_url, headers=basic(username, pat), timeout=10)
    assert dl.content == b"wheel-bytes"


# ---- go (GOPROXY) ----


def _zip_bytes(files: dict[str, bytes]) -> bytes:
    import zipfile

    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as zf:
        for name, content in files.items():
            zf.writestr(name, content)
    return buf.getvalue()


def test_go_goproxy(base_url, pkg_user):
    username, pat, _ = pkg_user
    module = f"example.com/{username}/lib"
    zip_data = _zip_bytes({f"lib@v1.0.0/go.mod": b"module example.com/lib\n"})
    h = basic(username, pat)
    r = requests.put(
        f"{base_url}/api/packages/go/{username}/{module}/@v/v1.0.0.zip",
        data=zip_data, headers=h, timeout=10,
    )
    assert r.status_code == 201

    lst = requests.get(f"{base_url}/api/packages/go/{username}/{module}/@v/list", headers=h, timeout=10)
    assert "v1.0.0" in lst.text
    info = requests.get(f"{base_url}/api/packages/go/{username}/{module}/@v/v1.0.0.info", headers=h, timeout=10).json()
    assert info["Version"] == "v1.0.0"
    mod = requests.get(f"{base_url}/api/packages/go/{username}/{module}/@v/v1.0.0.mod", headers=h, timeout=10)
    assert mod.text.startswith("module ")
    got = requests.get(f"{base_url}/api/packages/go/{username}/{module}/@v/v1.0.0.zip", headers=h, timeout=10)
    assert got.content == zip_data


# ---- cargo ----


def test_cargo_publish_index_download(base_url, pkg_user):
    username, pat, _ = pkg_user
    crate = f"mycrate_{uuid.uuid4().hex[:8]}"
    h = basic(username, pat)
    base = f"{base_url}/api/packages/cargo/{username}"

    cfg = requests.get(f"{base}/config.json", headers=h, timeout=10).json()
    assert "/dl" in cfg["dl"]

    crate_bytes = b"crate-tarball"
    meta = f'{{"name":"{crate}","vers":"0.1.0","deps":[],"cksum":"","features":{{}}}}\n'.encode()
    r = requests.put(f"{base}/api/v1/crates/new", data=meta + crate_bytes, headers=h, timeout=10)
    assert r.status_code == 201

    idx = requests.get(f"{base}/index/{crate[:2]}/{crate[2:4]}/{crate}", headers=h, timeout=10)
    assert idx.status_code == 200
    entries = [json.loads(line) for line in idx.text.splitlines() if line.strip()]
    entry = entries[0]
    assert entry["vers"] == "0.1.0"
    dl = requests.get(f"{cfg['dl']}/{crate}/0.1.0/{crate}-0.1.0.crate", headers=h, timeout=10)
    assert dl.content == crate_bytes


# ---- rubygems ----


def test_gem_push_and_download(base_url, pkg_user):
    username, pat, _ = pkg_user
    h = basic(username, pat)
    gem_name = f"mygem-{uuid.uuid4().hex[:8]}"

    # 构造最小 .gem：未压缩 tar，内含 metadata.gz（YAML）
    spec = f"--- !ruby/object:Gem::Specification\nname: {gem_name}\nversion: !ruby/object:Gem::Version\n  version: 1.0.0\n".encode()
    gzbuf = io.BytesIO()
    with gzip.GzipFile(fileobj=gzbuf, mode="wb") as gz:
        gz.write(spec)
    tbuf = io.BytesIO()
    with tarfile.open(fileobj=tbuf, mode="w") as tf:
        data = gzbuf.getvalue()
        ti = tarfile.TarInfo("metadata.gz")
        ti.size = len(data)
        tf.addfile(ti, io.BytesIO(data))
    gem_bytes = tbuf.getvalue()

    r = requests.post(f"{base_url}/api/packages/rubygems/{username}/api/v1/gems", data=gem_bytes, headers=h, timeout=10)
    assert r.status_code == 201

    dl = requests.get(f"{base_url}/api/packages/rubygems/{username}/gems/{gem_name}-1.0.0.gem", headers=h, timeout=10)
    assert dl.status_code == 200
    assert dl.content == gem_bytes


# ---- composer ----


def test_composer_upload_packages_json(base_url, pkg_user):
    username, pat, _ = pkg_user
    h = basic(username, pat)
    vendor = username
    name = f"lib{uuid.uuid4().hex[:6]}"
    zip_data = b"composer-zip"
    r = requests.put(
        f"{base_url}/api/packages/composer/{username}/{vendor}/{name}?version=1.0.0",
        data=zip_data, headers=h, timeout=10,
    )
    assert r.status_code == 201

    pkgs = requests.get(f"{base_url}/api/packages/composer/{username}/packages.json", headers=h, timeout=10).json()
    entry = pkgs["packages"].get(f"{vendor}/{name}", {}).get("1.0.0")
    assert entry is not None
    dl = requests.get(entry["dist"]["url"], headers=h, timeout=10)
    assert dl.content == zip_data


# ---- maven ----


def test_maven_put_get(base_url, pkg_user):
    username, pat, _ = pkg_user
    h = basic(username, pat)
    base = f"{base_url}/api/packages/maven/{username}"
    group = f"com/example/{username}"
    art = "mylib"

    pom = b"<project/>"
    r = requests.put(f"{base}/{group}/{art}/1.0.0/{art}-1.0.0.pom", data=pom, headers=h, timeout=10)
    assert r.status_code == 201

    got = requests.get(f"{base}/{group}/{art}/1.0.0/{art}-1.0.0.pom", headers=h, timeout=10)
    assert got.status_code == 200
    assert got.content == pom

    meta = b"<metadata/>"
    requests.put(f"{base}/{group}/{art}/maven-metadata.xml", data=meta, headers=h, timeout=10)
    got_meta = requests.get(f"{base}/{group}/{art}/maven-metadata.xml", headers=h, timeout=10)
    assert got_meta.content == meta


# ---- listing / delete ----


def test_package_list_and_delete(base_url, pkg_user):
    username, pat, _ = pkg_user
    h = basic(username, pat)
    pkg = f"del-{uuid.uuid4().hex[:8]}"
    body = {
        "name": pkg,
        "versions": {"1.0.0": {}},
        "_attachments": {f"{pkg}-1.0.0.tgz": {"length": 3, "data": base64.b64encode(b"abc").decode()}},
    }
    requests.put(f"{base_url}/api/packages/npm/{username}/{pkg}", json=body, headers=h, timeout=10)

    listed = requests.get(f"{base_url}/api/packages/{username}/npm", headers=h, timeout=10).json()
    assert any(p["name"] == pkg for p in listed)

    r = requests.delete(f"{base_url}/api/packages/npm/{username}/{pkg}", headers=h, timeout=10)
    assert r.status_code == 204
    r2 = requests.get(f"{base_url}/api/packages/npm/{username}/{pkg}", headers=h, timeout=10)
    assert r2.status_code == 404


# ---- 扩展：integrity / dist-tags / yank / maven metadata / pypi json / audit / counts ----


def test_npm_integrity_dist_tags(base_url, pkg_user):
    username, pat, _ = pkg_user
    h = basic(username, pat)
    pkg = f"itg-{uuid.uuid4().hex[:8]}"
    tarball = b"tarball-bytes"
    body = {
        "name": pkg,
        "versions": {"1.0.0": {}, "2.0.0": {}},
        "dist-tags": {"beta": "2.0.0"},
        "_attachments": {
            f"{pkg}-1.0.0.tgz": {"data": base64.b64encode(tarball).decode()},
        },
    }
    requests.put(f"{base_url}/api/packages/npm/{username}/{pkg}", json=body, headers=h, timeout=10)

    meta = requests.get(f"{base_url}/api/packages/npm/{username}/{pkg}", headers=h, timeout=10).json()
    v1 = meta["versions"]["1.0.0"]
    assert v1["dist"]["shasum"]  # sha1
    assert v1["dist"]["integrity"].startswith("sha512-")
    assert meta["dist-tags"].get("beta") == "2.0.0"

    r = requests.put(
        f"{base_url}/api/packages/npm/{username}/{pkg}/-/package/dist-tags/stable",
        json="1.0.0", headers=h, timeout=10,
    )
    assert r.status_code == 200
    tags = requests.get(
        f"{base_url}/api/packages/npm/{username}/{pkg}/-/package/dist-tags", headers=h, timeout=10
    ).json()
    assert tags["stable"] == "1.0.0"


def test_cargo_yank_unyank(base_url, pkg_user):
    username, pat, _ = pkg_user
    h = basic(username, pat)
    crate = f"yankcrate{uuid.uuid4().hex[:6]}"
    meta = f'{{"name":"{crate}","vers":"0.2.0","deps":[],"cksum":"","features":{{}}}}\n'.encode()
    base = f"{base_url}/api/packages/cargo/{username}"
    requests.put(f"{base}/api/v1/crates/new", data=meta + b"crate", headers=h, timeout=10)

    r = requests.delete(f"{base}/api/v1/crates/{crate}/0.2.0/yank", headers=h, timeout=10)
    assert r.status_code == 200
    idx = requests.get(f"{base}/index/{crate[:2]}/{crate[2:4]}/{crate}", headers=h, timeout=10)
    assert '"yanked":true' in idx.text

    requests.put(f"{base}/api/v1/crates/{crate}/0.2.0/yank", headers=h, timeout=10)
    idx = requests.get(f"{base}/index/{crate[:2]}/{crate[2:4]}/{crate}", headers=h, timeout=10)
    assert '"yanked":false' in idx.text


def test_maven_auto_metadata_and_sha1(base_url, pkg_user):
    import hashlib
    username, pat, _ = pkg_user
    h = basic(username, pat)
    base = f"{base_url}/api/packages/maven/{username}"
    grp = f"com/ex{uuid.uuid4().hex[:4]}"
    for v in ("1.0.0", "1.1.0"):
        requests.put(f"{base}/{grp}/auto/{v}/auto-{v}.jar", data=f"jar-{v}".encode(), headers=h, timeout=10)

    meta = requests.get(f"{base}/{grp}/auto/maven-metadata.xml", headers=h, timeout=10)
    assert meta.status_code == 200
    assert "<version>1.1.0</version>" in meta.text
    assert "<latest>1.1.0</latest>" in meta.text

    sha1 = requests.get(f"{base}/{grp}/auto/1.1.0/auto-1.1.0.jar.sha1", headers=h, timeout=10)
    assert sha1.text.strip() == hashlib.sha1(b"jar-1.1.0").hexdigest()


def test_pypi_json_api(base_url, pkg_user):
    username, pat, _ = pkg_user
    h = basic(username, pat)
    pkg = f"jsonpkg{uuid.uuid4().hex[:6]}"
    requests.post(
        f"{base_url}/api/packages/pypi/{username}/",
        data={"name": pkg, "version": "0.1.0"},
        files={"content": (f"{pkg}-0.1.0.tar.gz", b"sdists")},
        headers=h, timeout=10,
    )
    js = requests.get(
        f"{base_url}/api/packages/pypi/{username}/pypi/{pkg}/json", headers=h, timeout=10
    ).json()
    assert js["info"]["name"] == pkg
    rel = js["releases"]["0.1.0"][0]
    assert rel["digests"]["sha256"]
    assert rel["size"] == len(b"sdists")


def test_download_counter_and_audit(base_url, pkg_user):
    username, pat, _ = pkg_user
    h = basic(username, pat)
    pkg = f"cnt-{uuid.uuid4().hex[:8]}"
    body = {
        "name": pkg,
        "versions": {"1.0.0": {}},
        "_attachments": {f"{pkg}-1.0.0.tgz": {"data": base64.b64encode(b"xyz").decode()}},
    }
    requests.put(f"{base_url}/api/packages/npm/{username}/{pkg}", json=body, headers=h, timeout=10)

    tar_url = f"{base_url}/api/packages/npm/{username}/{pkg}/-/{pkg}-1.0.0.tgz"
    requests.get(tar_url, headers=h, timeout=10)
    requests.get(tar_url, headers=h, timeout=10)

    listed = requests.get(f"{base_url}/api/packages/{username}/npm", headers=h, timeout=10).json()
    row = next(p for p in listed if p["name"] == pkg)
    assert row["downloads"] >= 2

    audit = requests.get(f"{base_url}/api/packages/{username}/audit", headers=h, timeout=10).json()
    assert "publish" in [a["action"] for a in audit]
