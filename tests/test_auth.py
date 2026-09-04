"""认证 / 会话 / 基础端点 —— happy path + bad path。"""

import pytest

PASSWORD = "test-pass-123456"


def test_health_public(anon):
    r = anon.get("/health", expect=200)
    assert r.json() == {"status": "ok"}


def test_version_public(anon):
    r = anon.get("/version", expect=200)
    body = r.json()
    assert "version" in body and body["version"]


# ---- register happy path ----

def test_register_success(client_factory):
    name = f"u-{uuid4hex()}"
    r = client_factory().post(
        "/auth/register", json={"username": name, "password": PASSWORD}, expect=201
    )
    body = r.json()
    assert body["username"] == name
    assert len(body["token"]) > 20


def test_register_then_me_then_logout(user_factory):
    username, token, client = user_factory()
    me = client.get("/me", expect=200)
    assert me.json() == {"username": username}

    client.post("/auth/logout", expect=204)
    # token 注销后立即失效
    client.get("/me", expect=401)


def test_register_username_normalized_lowercase(client_factory):
    r = client_factory().post(
        "/auth/register",
        json={"username": "MiXeD-Case", "password": PASSWORD},
        expect=201,
    )
    assert r.json()["username"] == "mixed-case"


def test_login_happy(user_factory, client_factory):
    username, _, _ = user_factory()
    r = client_factory().post(
        "/auth/login", json={"username": username, "password": PASSWORD}, expect=200
    )
    body = r.json()
    assert body["username"] == username
    assert len(body["token"]) > 20
    # 新 token 可用
    client_factory(body["token"]).get("/me", expect=200)


# ---- register bad path ----

@pytest.mark.parametrize(
    "username",
    ["x", "a" * 33, "has space", "", "中文名", "a/b"],
)
def test_register_invalid_username(anon, username):
    anon.post(
        "/auth/register",
        json={"username": username, "password": PASSWORD},
        expect=400,
    )


@pytest.mark.parametrize("password", ["short", "", "1234567"])
def test_register_weak_password(anon, password):
    anon.post(
        "/auth/register", json={"username": f"u-{uuid4hex()}", "password": password},
        expect=400,
    )


def test_register_duplicate_username(anon):
    name = f"u-{uuid4hex()}"
    anon.post("/auth/register", json={"username": name, "password": PASSWORD}, expect=201)
    anon.post("/auth/register", json={"username": name, "password": PASSWORD}, expect=409)


def test_register_malformed_json(anon):
    r = anon.request("POST", "/auth/register", data="not-json{")
    assert r.status_code == 400
    assert "error" in r.json()


# ---- login bad path ----

def test_login_wrong_password(user_factory, anon):
    username, _, _ = user_factory()
    anon.post("/auth/login", json={"username": username, "password": "wrong-pass"}, expect=401)


def test_login_unknown_user(anon):
    anon.post(
        "/auth/login", json={"username": f"ghost-{uuid4hex()}", "password": PASSWORD},
        expect=401,
    )


def test_login_empty_body(anon):
    anon.post("/auth/login", json={}, expect=401)


# ---- 无 token 访问受保护端点 ----

@pytest.mark.parametrize(
    "method,path",
    [
        ("GET", "/repos"),
        ("POST", "/repos"),
        ("GET", "/keys"),
        ("POST", "/keys"),
        ("GET", "/me"),
    ],
)
def test_protected_endpoints_require_auth(anon, method, path):
    anon.request(method, path, expect=401)


def uuid4hex() -> str:
    import uuid

    return uuid.uuid4().hex[:10]
