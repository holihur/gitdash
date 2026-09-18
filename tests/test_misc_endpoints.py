"""杂项公共端点：健康检查、Swagger、Docker registry、OAuth 提供方禁用语义。

覆盖此前未触达的路由，并验证「默认关闭」的安全语义。
"""

import base64

import pytest
import requests


@pytest.fixture
def pat_creds(user_factory):
    username, _, client = user_factory("reg")
    token = client.post("/tokens", json={"name": "reg", "scopes": ["repo"]}, expect=201).json()["token"]
    return username, token


def test_health_live(base_url):
    r = requests.get(f"{base_url}/api/health/live", timeout=10)
    assert r.status_code == 200 and r.json()["status"] == "ok"


def test_swagger_redirect(base_url):
    r = requests.get(f"{base_url}/api/swagger", timeout=10, allow_redirects=False)
    assert r.status_code == 301
    assert r.headers["Location"].endswith("/api/swagger/")


def test_docker_registry_version_endpoint(base_url, pat_creds):
    username, token = pat_creds

    # 未认证：401 且带 registry 挑战头
    r = requests.get(f"{base_url}/v2/", timeout=10)
    assert r.status_code == 401
    assert r.headers.get("Docker-Distribution-API-Version") == "registry/2.0"
    assert "Basic" in r.headers.get("WWW-Authenticate", "")

    # PAT 认证后版本检查通过
    creds = base64.b64encode(f"{username}:{token}".encode()).decode()
    ok = requests.get(f"{base_url}/v2/", headers={"Authorization": f"Basic {creds}"}, timeout=10)
    assert ok.status_code == 200, ok.text


def test_metrics_endpoint(base_url):
    r = requests.get(f"{base_url}/metrics", timeout=10)
    assert r.status_code == 200
    assert "# HELP" in r.text or "# TYPE" in r.text


def test_device_verify_requires_login(base_url):
    """设备流验证页必须要求登录，匿名访问跳回首页。"""
    r = requests.get(f"{base_url}/login/oauth/device", timeout=10, allow_redirects=False)
    assert r.status_code in (301, 302, 303)
    assert r.headers["Location"] == "/"


def test_oauth_providers_disabled_by_default(base_url):
    """未配置的 GitHub / OIDC 登录入口必须明确报错，不能跳转到外部或 500。"""
    r = requests.get(f"{base_url}/api/auth/github", timeout=10, allow_redirects=False)
    assert r.status_code == 404 and r.json()["code"] == "oauth_disabled"

    r = requests.get(
        f"{base_url}/api/auth/github/callback?code=x&state=y", timeout=10, allow_redirects=False
    )
    assert r.status_code in (301, 302, 303)

    r = requests.get(f"{base_url}/api/auth/oidc/start", timeout=10, allow_redirects=False)
    assert r.status_code == 404 and r.json()["code"] == "oidc_disabled"

    r = requests.get(
        f"{base_url}/api/auth/oidc/callback?code=x&state=y", timeout=10, allow_redirects=False
    )
    assert r.status_code in (301, 302, 303)
    assert "auth_error" in r.headers.get("Location", "")
