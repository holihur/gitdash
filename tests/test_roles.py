"""仓库角色权限黑盒测试：read/triage/write/maintain/admin 的能力边界 + owner 专属。"""

from __future__ import annotations

import uuid

import pytest


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def _p(owner: str, repo: str, suffix: str = "") -> str:
    return f"/users/{owner}/repos/{repo}{suffix}"


def _commit_body(path: str) -> dict:
    return {
        "message": "add",
        "changes": [{"path": path, "action": "create", "content": "x\n"}],
    }


@pytest.fixture
def role_env(user_factory):
    an, _, alice = user_factory("ro")
    repo = f"ro-{_uuid()}"
    alice.post("/repos", json={"name": repo, "private": True}, expect=201)
    users: dict[str, tuple[str, object]] = {}
    for i, role in enumerate(["read", "triage", "write", "maintain", "admin"]):
        un, _, c = user_factory(f"r{i}")
        alice.post(_p(an, repo, "/collabs"), json={"username": un, "permission": role}, expect=200)
        users[role] = (un, c)
    yield an, alice, repo, users
    try:
        alice.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_invalid_role_rejected(role_env, user_factory):
    an, alice, repo, _ = role_env
    _, _, other = user_factory("rx")
    alice.post(
        _p(an, repo, "/collabs"),
        json={"username": other.get("/me", expect=200).json()["username"], "permission": "owner"},
        expect=400,
    )


def test_org_member_default_role(user_factory):
    on, _, owner = user_factory("og")
    mn, _, member = user_factory("om")
    org = f"og{_uuid()[:8]}"
    repo = f"or-{_uuid()[:8]}"
    owner.post("/orgs", json={"name": org, "display": "Org"}, expect=201)
    owner.post("/repos", json={"name": repo, "namespace": org, "private": True}, expect=201)
    owner.post(f"/orgs/{org}/members", json={"username": mn, "role": "member"}, expect=200)
    try:
        # 默认成员角色 write：成员可推送。
        member.post(f"/users/{org}/repos/{repo}/commits", json=_commit_body("o1.txt"), expect=201)
        # owner 改为 read：成员变只读。
        owner.patch(f"/orgs/{org}", json={"default_member_role": "read"}, expect=200)
        member.get(f"/users/{org}/repos/{repo}/tree", expect=200)
        member.post(f"/users/{org}/repos/{repo}/commits", json=_commit_body("o2.txt"), expect=404)
        # 非法角色拒绝。
        owner.patch(f"/orgs/{org}", json={"default_member_role": "owner"}, expect=400)
        # 非 owner 不能修改。
        member.patch(f"/orgs/{org}", json={"default_member_role": "write"}, expect=404)
        # profile 回传该设置。
        assert owner.get(f"/orgs/{org}/profile", expect=200).json()["default_member_role"] == "read"
    finally:
        owner.delete(f"/users/{org}/repos/{repo}", expect=204)
        owner.delete(f"/orgs/{org}", expect=204)


def test_repo_detail_reports_full_role(role_env):
    an, alice, repo, users = role_env
    for role in ["read", "triage", "write", "maintain", "admin"]:
        _, c = users[role]
        assert c.get(f"/users/{an}/repos/{repo}", expect=200).json()["role"] == role
    assert alice.get(f"/users/{an}/repos/{repo}", expect=200).json()["role"] == "owner"


def test_read_role(role_env):
    an, _, repo, users = role_env
    _, c = users["read"]
    c.get(_p(an, repo, "/tree"), expect=200)
    c.post(_p(an, repo, "/issues"), json={"title": "x"}, expect=404)
    c.post(_p(an, repo, "/commits"), json=_commit_body("a.txt"), expect=404)
    c.post(_p(an, repo, "/webhooks"), json={"url": "https://e.com/h"}, expect=404)
    c.post(_p(an, repo, "/collabs"), json={"username": an, "permission": "read"}, expect=404)


def test_triage_role(role_env):
    an, _, repo, users = role_env
    _, c = users["triage"]
    # 可管理议题 / 评论
    it = c.post(_p(an, repo, "/issues"), json={"title": "t"}, expect=201).json()
    c.patch(_p(an, repo, f"/issues/{it['number']}"), json={"state": "closed"}, expect=200)
    c.post(_p(an, repo, f"/issues/{it['number']}/comments"), json={"body": "hi"}, expect=201)
    # 不能推代码 / 改设置 / 管协作者
    c.post(_p(an, repo, "/commits"), json=_commit_body("b.txt"), expect=404)
    c.post(_p(an, repo, "/webhooks"), json={"url": "https://e.com/h"}, expect=404)
    c.post(_p(an, repo, "/collabs"), json={"username": an, "permission": "read"}, expect=404)


def test_write_role(role_env):
    an, _, repo, users = role_env
    _, c = users["write"]
    c.post(_p(an, repo, "/commits"), json=_commit_body("c.txt"), expect=201)
    # 不能改仓库设置（maintain）/ 管协作者（admin）
    c.post(_p(an, repo, "/webhooks"), json={"url": "https://e.com/h"}, expect=404)
    c.post(_p(an, repo, "/collabs"), json={"username": an, "permission": "read"}, expect=404)
    c.post(_p(an, repo, "/visibility"), json={"visibility": "public"}, expect=404)


def test_maintain_role(role_env, user_factory):
    an, _, repo, users = role_env
    _, c = users["maintain"]
    c.post(_p(an, repo, "/webhooks"), json={"url": "https://e.com/h"}, expect=201)
    # 不能管协作者 / 改可见性
    _, _, other = user_factory("rm")
    c.post(
        _p(an, repo, "/collabs"),
        json={"username": other.get("/me", expect=200).json()["username"], "permission": "read"},
        expect=404,
    )
    c.post(_p(an, repo, "/visibility"), json={"visibility": "public"}, expect=404)


def test_admin_role(role_env, user_factory):
    an, _, repo, users = role_env
    _, c = users["admin"]
    _, _, other = user_factory("ra")
    other_name = other.get("/me", expect=200).json()["username"]
    # admin 可加协作者、改可见性
    c.post(_p(an, repo, "/collabs"), json={"username": other_name, "permission": "read"}, expect=200)
    c.post(_p(an, repo, "/visibility"), json={"visibility": "public"}, expect=200)
    # 但不能删除仓库（owner 专属）
    c.delete(f"/repos/{repo}", expect=404)
    # 撤销协作者
    c.delete(_p(an, repo, f"/collabs/{other_name}"), expect=204)
