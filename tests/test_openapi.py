"""OpenAPI (Swagger) 文档端点 —— 冒烟测试。"""

from urllib.parse import urljoin


def test_openapi_json_public(anon):
    resp = anon.get("/openapi.json", expect=200)
    spec = resp.json()
    assert resp.headers["content-type"].startswith("application/json")
    assert spec["info"]["title"] == "gitdash API"
    assert spec["info"]["version"]
    assert spec.get("paths"), "spec 不含任何路径"
    for p in ("/auth/register", "/auth/login", "/repos"):
        assert p in spec["paths"]


def test_swagger_ui_smoke(anon, base_url):
    resp = anon.session.get(urljoin(base_url, "/api/swagger/index.html"), timeout=15)
    assert resp.status_code == 200
    assert "swagger-ui" in resp.text
    doc = anon.session.get(urljoin(base_url, "/api/swagger/doc.json"), timeout=15)
    assert doc.status_code == 200
    assert doc.json()["info"]["title"] == "gitdash API"
