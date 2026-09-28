"""Kubernetes / Helm（type ``k8s``）与 Dart / Pub（type ``dart``）包仓库黑盒测试。

覆盖：
- Helm 仓库：multipart 发布 chart（Chart.yaml 自动解析 / 显式 meta）→ index.yaml → 下载
- Dart Pub hosted API：手工发布 + 原生 ``versions/new`` 上传流程 → 包/版本 JSON → 归档下载
- 权限：非 owner 发布被拒
"""
from __future__ import annotations

import base64
import io
import json
import tarfile
import uuid

import pytest
import requests


@pytest.fixture
def kd_user(user_factory, client_factory):
    """注册用户 + repo scope PAT，返回 (username, pat_token, client)。"""
    username, _token, client = user_factory("kd")
    resp = client.post("/tokens", json={"name": "kd", "scopes": ["repo"]}, expect=201)
    return username, resp.json()["token"], client_factory(resp.json()["token"])


def basic(username: str, pat: str) -> dict:
    creds = base64.b64encode(f"{username}:{pat}".encode()).decode()
    return {"Authorization": f"Basic {creds}"}


def _tar_gz(files: dict[str, bytes]) -> bytes:
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w:gz") as tar:
        for name, data in files.items():
            info = tarfile.TarInfo(name)
            info.size = len(data)
            tar.addfile(info, io.BytesIO(data))
    return buf.getvalue()


def chart_tgz(name: str, version: str, app_version: str = "1.0.0") -> bytes:
    chart = (
        f"apiVersion: v2\nname: {name}\nversion: {version}\n"
        f"appVersion: {app_version}\ndescription: test chart\n"
    ).encode()
    return _tar_gz({f"{name}/Chart.yaml": chart, f"{name}/values.yaml": b"replicas: 1\n"})


def pub_tgz(name: str, version: str) -> bytes:
    pubspec = (
        f"name: {name}\nversion: {version}\ndescription: test package\n"
        "environment:\n  sdk: '>=2.17.0 <4.0.0'\n"
    ).encode()
    return _tar_gz({"pubspec.yaml": pubspec, "lib/main.dart": b"void main() {}\n"})


# ---- Helm (k8s) ----


def test_k8s_helm_repository(base_url, kd_user):
    username, pat, _ = kd_user
    repo = f"charts-{uuid.uuid4().hex[:6]}"
    chart = "hello"
    version = "1.2.3"

    r = requests.post(
        f"{base_url}/api/packages/k8s/{username}/{repo}/publish",
        files={"file": (f"{chart}-{version}.tgz", chart_tgz(chart, version), "application/gzip")},
        headers=basic(username, pat),
        timeout=30,
    )
    assert r.status_code == 201, r.text
    assert r.json()["type"] == "k8s"

    index = requests.get(
        f"{base_url}/api/packages/k8s/{username}/{repo}/index.yaml",
        headers=basic(username, pat),
        timeout=10,
    )
    assert index.status_code == 200, index.text
    body = index.text
    assert "apiVersion: v1" in body
    assert f"name: {chart}" in body
    assert f"version: {version}" in body
    assert "digest: sha256:" in body

    dl = requests.get(
        f"{base_url}/api/packages/k8s/{username}/{repo}/charts/{chart}-{version}.tgz",
        headers=basic(username, pat),
        timeout=10,
    )
    assert dl.status_code == 200
    assert dl.content[:2] == b"\x1f\x8b"


def test_k8s_requires_permission(base_url, kd_user, user_factory):
    owner, _pat, _ = kd_user
    other, other_pat, _ = user_factory("kdx")
    r = requests.post(
        f"{base_url}/api/packages/k8s/{owner}/charts/publish",
        files={"file": ("x-1.0.0.tgz", chart_tgz("x", "1.0.0"), "application/gzip")},
        headers=basic(other, other_pat),
        timeout=30,
    )
    assert r.status_code == 403


# ---- Dart / Pub ----


def test_dart_manual_publish(base_url, kd_user):
    username, pat, _ = kd_user
    name = f"hello_{uuid.uuid4().hex[:6]}"
    version = "0.1.0"

    r = requests.post(
        f"{base_url}/api/packages/dart/{username}/publish",
        files={"file": (f"{name}-{version}.tar.gz", pub_tgz(name, version), "application/gzip")},
        headers=basic(username, pat),
        timeout=30,
    )
    assert r.status_code == 201, r.text
    assert r.json()["name"] == name

    pkg = requests.get(
        f"{base_url}/api/packages/dart/{username}/api/packages/{name}",
        headers=basic(username, pat),
        timeout=10,
    )
    assert pkg.status_code == 200, pkg.text
    data = pkg.json()
    assert data["name"] == name
    assert data["latest"]["version"] == version
    assert data["latest"]["pubspec"]["name"] == name
    assert data["latest"]["archive_sha256"]

    one = requests.get(
        f"{base_url}/api/packages/dart/{username}/api/packages/{name}/versions/{version}",
        headers=basic(username, pat),
        timeout=10,
    )
    assert one.status_code == 200
    assert one.json()["version"] == version

    dl = requests.get(
        f"{base_url}/api/packages/dart/{username}/api/packages/{name}/download/{version}",
        headers=basic(username, pat),
        timeout=10,
    )
    assert dl.status_code == 200
    assert dl.content[:2] == b"\x1f\x8b"


def test_dart_native_publish_flow(base_url, kd_user):
    username, pat, _ = kd_user
    name = f"native_{uuid.uuid4().hex[:6]}"
    version = "2.0.0"

    new_upload = requests.get(
        f"{base_url}/api/packages/dart/{username}/api/packages/versions/new",
        headers=basic(username, pat),
        timeout=10,
    )
    assert new_upload.status_code == 200, new_upload.text
    payload = new_upload.json()
    assert payload["url"].endswith("/api/packages/versions/newUpload")

    up = requests.post(
        payload["url"],
        files={"file": (f"{name}-{version}.tar.gz", pub_tgz(name, version), "application/gzip")},
        data=payload.get("fields") or {},
        headers=basic(username, pat),
        timeout=30,
    )
    assert up.status_code == 200, up.text
    assert up.json()["success"]["message"]

    pkg = requests.get(
        f"{base_url}/api/packages/dart/{username}/api/packages/{name}",
        headers=basic(username, pat),
        timeout=10,
    )
    assert pkg.status_code == 200
    assert pkg.json()["latest"]["version"] == version


def test_dart_requires_permission(base_url, kd_user, user_factory):
    owner, _pat, _ = kd_user
    other, other_pat, _ = user_factory("dartx")
    r = requests.post(
        f"{base_url}/api/packages/dart/{owner}/publish",
        files={"file": ("x-1.0.0.tar.gz", pub_tgz("x", "1.0.0"), "application/gzip")},
        headers=basic(other, other_pat),
        timeout=30,
    )
    assert r.status_code == 403


def test_new_package_types_are_listed(base_url, kd_user):
    username, pat, _ = kd_user
    for typ in ("k8s", "dart"):
        r = requests.get(
            f"{base_url}/api/packages/{username}/{typ}",
            headers=basic(username, pat),
            timeout=10,
        )
        assert r.status_code == 200, f"{typ}: {r.text}"
