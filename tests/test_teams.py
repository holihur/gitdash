"""组织团队 + 仓库团队授权 + 权限审计黑盒测试。"""

from __future__ import annotations

import uuid


def _uuid() -> str:
    return uuid.uuid4().hex[:8]


def _commit(path: str) -> dict:
    return {
        "message": "add",
        "changes": [{"path": path, "action": "create", "content": "x\n"}],
    }


def test_org_teams_and_repo_grants(user_factory):
    on, _, owner = user_factory("ot")
    mn, _, member = user_factory("tm")
    org = f"ot{_uuid()}"
    repo = f"tr-{_uuid()}"
    owner.post("/orgs", json={"name": org, "display": "Org"}, expect=201)
    owner.post("/repos", json={"name": repo, "namespace": org, "private": True}, expect=201)
    try:
        # 建团队 + 加成员
        team = owner.post(f"/orgs/{org}/teams", json={"name": "devs"}, expect=201).json()
        owner.post(f"/orgs/{org}/teams/{team['id']}/members", json={"username": mn}, expect=204)
        assert owner.get(f"/orgs/{org}/teams/{team['id']}/members", expect=200).json()["members"] == [mn]
        assert owner.get(f"/orgs/{org}/teams", expect=200).json()[0]["member_count"] == 1

        # 重复团队名 409；非 owner 建团队 404
        owner.post(f"/orgs/{org}/teams", json={"name": "devs"}, expect=409)
        member.post(f"/orgs/{org}/teams", json={"name": "x"}, expect=404)

        # 授权团队 write 到仓库
        owner.put(
            f"/users/{org}/repos/{repo}/team-grants/{team['id']}",
            json={"permission": "write"},
            expect=204,
        )
        grants = owner.get(f"/users/{org}/repos/{repo}/team-grants", expect=200).json()
        assert grants[0]["team_name"] == "devs" and grants[0]["permission"] == "write"

        # 成员获得 write：可推送，仓库详情 role=write
        member.post(f"/users/{org}/repos/{repo}/commits", json=_commit("t1.txt"), expect=201)
        assert member.get(f"/users/{org}/repos/{repo}", expect=200).json()["role"] == "write"

        # 权限审计包含团队来源
        entries = owner.get(f"/users/{org}/repos/{repo}/access", expect=200).json()
        assert any(
            e["subject"] == mn and "team:devs" in e["sources"] and e["role"] == "write"
            for e in entries
        )
        # 审计条目按用户聚合：同一用户只出现一次
        assert sum(1 for e in entries if e["subject"] == mn) == 1

        # 非 admin 不能管理团队授权
        member.put(
            f"/users/{org}/repos/{repo}/team-grants/{team['id']}",
            json={"permission": "read"},
            expect=404,
        )

        # 撤销后成员失去写权限
        owner.delete(f"/users/{org}/repos/{repo}/team-grants/{team['id']}", expect=204)
        member.post(f"/users/{org}/repos/{repo}/commits", json=_commit("t2.txt"), expect=404)
        # 非法角色 400
        owner.put(
            f"/users/{org}/repos/{repo}/team-grants/{team['id']}",
            json={"permission": "owner"},
            expect=400,
        )
        # 删除团队级联清理
        owner.delete(f"/orgs/{org}/teams/{team['id']}/members/{mn}", expect=204)
        assert owner.get(f"/orgs/{org}/teams/{team['id']}/members", expect=200).json()["members"] == []
        owner.delete(f"/orgs/{org}/teams/{team['id']}", expect=204)
    finally:
        try:
            owner.delete(f"/users/{org}/repos/{repo}", expect=204)
            owner.delete(f"/orgs/{org}", expect=204)
        except Exception:
            pass
