"""GDPR 合规黑盒测试：数据可携带权（/me/export）与删除后的匿名化（deleted-user）。"""

import uuid

import pytest


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def _r(owner, repo):
    return f"/users/{owner}/repos/{repo}"


def test_me_export_contains_user_data(user_factory, client_factory):
    an, _, c = user_factory()
    repo = f"exp-{_uuid()}"
    c.post("/repos", json={"name": repo}, expect=201)
    c.post(_r(an, repo) + "/commits", json={
        "message": "m", "changes": [{"path": "a", "action": "create", "content": "1"}]}, expect=201)
    c.post(_r(an, repo) + "/issues", json={"title": "t1", "body": "b1"}, expect=201)
    c.post(_r(an, repo) + "/issues/1/comments", json={"body": "c1"}, expect=201)

    r = c.get("/me/export")
    assert r.status_code == 200
    assert "attachment" in r.headers.get("Content-Disposition", "")
    data = r.json()
    assert data["profile"]["username"] == an
    assert [x["name"] for x in data["repos"]] == [repo]
    assert any(i["title"] == "t1" and i["owner"] == an for i in data["issues"])
    assert any(cm["body"] == "c1" and cm["owner"] == an for cm in data["comments"])

    # 未认证不可导出
    client_factory().get("/me/export", expect=401)


def test_deleted_user_content_is_anonymized(admin, user_factory):
    """被遗忘权：删除用户后，其在他人生成内容中的身份必须匿名化。"""
    victim = f"victim-{_uuid()}"
    host = f"host-{_uuid()}"

    from conftest import ApiClient

    def _mk(name):
        c = ApiClient(admin.base)
        c.token = c.post("/auth/register",
                         json={"username": name, "password": "test-pass-123456"},
                         expect=201).json()["token"]
        return c

    v, h = _mk(victim), _mk(host)
    repo = f"anon-{_uuid()}"
    h.post("/repos", json={"name": repo}, expect=201)

    # victim 在 host 的仓库里留下一系列活动痕迹
    h.post(_r(host, repo) + "/collabs", json={"username": victim, "permission": "write"}, expect=200)
    v.post(_r(host, repo) + "/commits", json={
        "message": "m", "changes": [{"path": "a", "action": "create", "content": "1"}]}, expect=201)
    issue = v.post(_r(host, repo) + "/issues", json={"title": "v issue", "body": "vb"},
                   expect=201).json()
    v.post(_r(host, repo) + f"/issues/{issue['number']}/comments", json={"body": "v comment"}, expect=201)
    # victim 自己的仓库 + 包
    vrepo = f"own-{_uuid()}"
    v.post("/repos", json={"name": vrepo}, expect=201)

    # admin 删除 victim
    admin.delete(f"/admin/users/{victim}", expect=204)

    # host 视角：issue/评论保留但作者匿名
    issues = h.get(_r(host, repo) + "/issues", expect=200).json()
    target = next(i for i in issues if i["title"] == "v issue")
    assert target["author"] == "deleted-user"
    comments = h.get(_r(host, repo) + f"/issues/{issue['number']}/comments", expect=200).json()
    assert all(cm["author"] == "deleted-user" for cm in comments)

    # victim 的仓库与账号整体消失
    v.get("/me", expect=401)
    h.get(_r(victim, vrepo), expect=404)

    # 匿名后登录/注册同名不受影响
    _mk(victim).get("/me", expect=200)
