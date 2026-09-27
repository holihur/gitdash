"""Passkey（WebAuthn / FIDO2）黑盒测试。

完整注册 / 登录的密码学校验由 Go 集成测试（backend/tests/passkey_test.go）
使用软件认证器覆盖；这里覆盖 API 表面与坏路径（同时保证端点覆盖率门禁）。
"""


def test_passkey_provider_advertised(anon):
    providers = anon.get("/auth/providers", expect=200).json()
    assert providers.get("passkey", {}).get("enabled") is True


def test_passkey_requires_auth(anon):
    anon.get("/me/passkeys", expect=401)
    anon.post("/me/passkeys/register/begin", json={}, expect=401)
    anon.post("/me/passkeys/register/finish", json={}, expect=401)
    anon.delete("/me/passkeys/1", expect=401)


def test_passkey_register_begin_and_finish_bad_paths(user_factory):
    _, _, client = user_factory("pk")

    # 初始列表为空
    body = client.get("/me/passkeys", expect=200).json()
    assert body["passkeys"] == []

    # 开始注册：返回一次性 session 与 WebAuthn 选项
    begin = client.post("/me/passkeys/register/begin", json={"name": "Laptop"}, expect=200).json()
    assert begin["session_id"]
    public_key = begin["public_key"]["publicKey"]
    assert public_key["challenge"]
    assert public_key["rp"]["id"]
    assert public_key["user"]["id"]

    # 缺少 credential
    client.post("/me/passkeys/register/finish",
                json={"session_id": begin["session_id"]}, expect=400)
    # 非法 credential
    client.post("/me/passkeys/register/finish",
                json={"session_id": begin["session_id"], "credential": {"id": "x"}}, expect=400)
    # session 已被消费：再次提交仍是 400
    client.post("/me/passkeys/register/finish",
                json={"session_id": begin["session_id"], "credential": {"id": "x"}}, expect=400)

    # 删除不存在的凭据
    client.delete("/me/passkeys/999999", expect=404)


def test_passkey_login_begin_and_finish_bad_paths(anon, user_factory):
    user_factory("pkl")

    # discoverable（无用户名）
    begin = anon.post("/auth/passkey/begin", json={}, expect=200).json()
    assert begin["session_id"]
    assert begin["public_key"]["publicKey"]["challenge"]

    # 缺少 credential
    anon.post("/auth/passkey/finish",
              json={"session_id": begin["session_id"]}, expect=400)
    # 非法 credential
    anon.post("/auth/passkey/finish",
              json={"session_id": begin["session_id"], "credential": {"id": "x"}}, expect=401)

    # 用户名限定（AllowCredentials 为空时退化为 discoverable）
    named = anon.post("/auth/passkey/begin", json={"username": "no-such-user"}, expect=200).json()
    assert named["session_id"]

    # 过期 / 未知 session
    anon.post("/auth/passkey/finish",
              json={"session_id": "does-not-exist", "credential": {"id": "x"}}, expect=401)
