"""Admin 封禁（用户 / 仓库 / 组织）与系统 template 用户黑盒测试。"""

import uuid


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def test_admin_ban_user_flow(admin, user_factory, client_factory):
    uname, _, c = user_factory()
    c.post("/repos", json={"name": "proj", "private": False}, expect=201)

    # 封禁用户：登录 403、现有会话 401
    admin.post(f"/admin/users/{uname}/ban", json={"banned": True}, expect=204)
    assert (
        client_factory()
        .post("/auth/login", json={"username": uname, "password": "test-pass-123456"})
        .status_code
        == 403
    )
    c.get("/me", expect=401)

    # 解封后可登录，并在收件箱看到系统封禁通知
    admin.post(f"/admin/users/{uname}/ban", json={"banned": False}, expect=204)
    tok = (
        client_factory()
        .post("/auth/login", json={"username": uname, "password": "test-pass-123456"}, expect=200)
        .json()["token"]
    )
    after = client_factory(tok)
    notes = after.get("/inbox", expect=200).json()
    assert any(n.get("kind") == "system" and n.get("action") == "banned_user" for n in notes)


def test_admin_ban_repo_blocks_access(admin, user_factory):
    uname, _, c = user_factory()
    repo = f"ban-{_uuid()}"
    c.post("/repos", json={"name": repo, "private": False}, expect=201)

    _, _, other = user_factory()
    other.get(f"/users/{uname}/repos/{repo}", expect=200)

    admin.post(f"/admin/repos/{uname}/{repo}/ban", json={"banned": True}, expect=204)
    other.get(f"/users/{uname}/repos/{repo}", expect=404)

    listed = admin.get(f"/admin/repos?q={repo}", expect=200).json()
    row = next(r for r in listed if r["owner"] == uname and r["name"] == repo)
    assert row["banned"] is True

    # 解封恢复访问
    admin.post(f"/admin/repos/{uname}/{repo}/ban", json={"banned": False}, expect=204)
    other.get(f"/users/{uname}/repos/{repo}", expect=200)


def test_admin_ban_org_hides_and_blocks(admin, user_factory):
    uname, _, c = user_factory()
    org = f"org-{_uuid()}"
    c.post("/orgs", json={"name": org, "display": "Org"}, expect=201)
    c.post("/repos", json={"name": "svc", "namespace": org, "private": False}, expect=201)

    _, _, other = user_factory()
    other.get(f"/users/{org}/repos/svc", expect=200)

    admin.post(f"/admin/orgs/{org}/ban", json={"banned": True}, expect=204)
    other.get(f"/users/{org}/repos/svc", expect=404)

    listed = admin.get(f"/admin/orgs?q={org}", expect=200).json()
    row = next(o for o in listed if o["name"] == org)
    assert row["banned"] is True

    # org owner 收到系统通知
    notes = c.get("/inbox", expect=200).json()
    assert any(n.get("kind") == "system" and n.get("action") == "banned_org" for n in notes)


def test_admin_template_user_protected(admin):
    # template 用户由服务端启动时 seed（banned=true，系统专用）
    listed = admin.get("/admin/users?q=template", expect=200).json()
    tmpl = next((u for u in listed if u["username"] == "template"), None)
    assert tmpl is not None, "system template user should be seeded on startup"
    assert tmpl["banned"] is True

    admin.post("/admin/users/template/ban", json={"banned": False}, expect=403)
    admin.delete("/admin/users/template", expect=403)
