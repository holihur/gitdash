"""系统包仓库（apt / yum / apk / brew / snap）黑盒 API 测试。

发布制品（multipart: meta + file）后校验各类型索引与下载端点。
"""
from __future__ import annotations

import json

DEB = b"!<arch>\n" + b"fake-deb-body"
RPM = b"\xed\xab\xee\xdb" + b"fake-rpm-body"
APK = b"\x1f\x8b" + b"fake-apk-body"
BOTTLE = b"\x1f\x8b" + b"fake-bottle-body"
SNAP = b"hsqs" + b"fake-snap-body"


def _publish(c, typ, owner, repo, filename, content, meta):
    return c.post(
        f"/packages/{typ}/{owner}/{repo}/publish",
        files={"file": (filename, content, "application/octet-stream")},
        data={"meta": json.dumps(meta)},
        expect=201,
    ).json()


def test_system_package_repos(user_factory):
    owner, _token, c = user_factory("sys")

    # apt
    _publish(c, "apt", owner, "deb", "hello_1.0_amd64.deb", DEB, {
        "name": "hello", "version": "1.0", "arch": "amd64",
        "description": "hello deb", "maintainer": "a@b.c", "depends": "libc6",
    })
    pkgs = c.get(f"/packages/apt/{owner}/deb/dists/stable/main/binary-amd64/Packages", expect=200).text
    assert "Package: hello" in pkgs and "SHA256:" in pkgs and "pool/hello_1.0_amd64.deb" in pkgs
    assert c.get(f"/packages/apt/{owner}/deb/dists/stable/main/binary-amd64/Packages.gz", expect=200).content
    rel = c.get(f"/packages/apt/{owner}/deb/dists/stable/main/Release", expect=200).text
    assert "Architectures: amd64" in rel
    assert c.get(f"/packages/apt/{owner}/deb/pool/hello_1.0_amd64.deb", expect=200).content == DEB

    # yum
    _publish(c, "yum", owner, "rpm", "hello-1.0-1.x86_64.rpm", RPM, {
        "name": "hello", "version": "1.0", "release": "1", "arch": "x86_64",
        "summary": "hello rpm", "license": "MIT",
    })
    repomd = c.get(f"/packages/yum/{owner}/rpm/repodata/repomd.xml", expect=200).text
    assert "primary.xml.gz" in repomd
    primary = c.get(f"/packages/yum/{owner}/rpm/repodata/primary.xml.gz", expect=200).content
    assert primary[:2] == b"\x1f\x8b"
    assert c.get(f"/packages/yum/{owner}/rpm/hello-1.0-1.x86_64.rpm", expect=200).content == RPM

    # apk
    _publish(c, "apk", owner, "alpine", "hello-1.0-r0.apk", APK, {
        "name": "hello", "version": "1.0-r0", "arch": "x86_64", "description": "hello apk",
    })
    index = c.get(f"/packages/apk/{owner}/alpine/x86_64/APKINDEX.tar.gz", expect=200).content
    assert index[:2] == b"\x1f\x8b"
    assert c.get(f"/packages/apk/{owner}/alpine/x86_64/hello-1.0-r0.apk", expect=200).content == APK

    # brew
    _publish(c, "brew", owner, "tap", "hello-1.0.bottle.tar.gz", BOTTLE, {
        "name": "hello", "version": "1.0", "arch": "arm64", "description": "hello brew",
    })
    formula = c.get(f"/packages/brew/{owner}/tap/api/formula/hello.json", expect=200).json()
    assert formula["name"] == "hello" and formula["versions"]["stable"] == "1.0"
    assert c.get(f"/packages/brew/{owner}/tap/bottles/hello-1.0.bottle.tar.gz", expect=200).content == BOTTLE

    # snap
    _publish(c, "snap", owner, "store", "hello_1.0_amd64.snap", SNAP, {
        "name": "hello", "version": "1.0", "arch": "amd64",
    })
    snaps = c.get(f"/packages/snap/{owner}/store/index.json", expect=200).json()
    assert snaps["snaps"] and snaps["snaps"][0]["name"] == "hello"
    assert c.get(f"/packages/snap/{owner}/store/download/hello_1.0_amd64.snap", expect=200).content == SNAP

    # 列表页识别这些类型
    assert c.get(f"/packages/{owner}/apt", expect=200).json()


def test_system_package_requires_permission(user_factory):
    owner, _token, _c = user_factory("syso")
    _other, _tok2, c2 = user_factory("sysx")
    c2.post(
        f"/packages/apt/{owner}/deb/publish",
        files={"file": ("x_1.0_amd64.deb", DEB, "application/octet-stream")},
        data={"meta": json.dumps({"name": "x", "version": "1.0", "arch": "amd64"})},
        expect=403,
    )
