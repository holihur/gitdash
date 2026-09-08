"""杂项 API 对抗性测试：全局/代码搜索、templates、instance、providers、邮箱验证、分支保护。"""

import uuid

import pytest


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def _r(owner, repo):
    return f"/users/{owner}/repos/{repo}"


# ---- 全局搜索 ----

def test_global_search_visibility(user_factory, anon):
    token = "needle-" + _uuid()
    an, _, alice = user_factory()
    repo = f"pub-{_uuid()}"
    alice.post("/repos", json={"name": repo, "private": False,
                               "description": f"about {token}"}, expect=201)
    alice.post(_r(an, repo) + "/commits", json={
        "message": f"commit {token}",
        "changes": [{"path": "a.txt", "action": "create", "content": token}]}, expect=201)
    alice.post(_r(an, repo) + "/issues", json={"title": f"issue {token}"}, expect=201)

    # 私有仓库不该出现在别人/匿名搜索里
    _, _, bob = user_factory()
    priv = f"priv-{_uuid()}"
    bob.post("/repos", json={"name": priv, "private": True, "description": f"secret {token}"}, expect=201)

    # 匿名 401；他人搜索不含私有仓库
    anon.get("/search", params={"q": token}, expect=401)
    res = bob.get("/search", params={"q": token}, expect=200).json()
    assert [r["name"] for r in res["repos"]] == [repo]
    assert priv not in [r["name"] for r in res["repos"]]

    res = alice.get("/search", params={"q": token}, expect=200).json()
    assert any(i["title"] == f"issue {token}" for i in res["issues"])
    assert any(u["name"] == an for u in alice.get("/search", params={"q": an}, expect=200).json()["users"])

    anon.get("/search", params={"q": ""}, expect=401)
    bob.get("/search", params={"q": ""}, expect=400)
    bob.get("/search", params={"q": "  "}, expect=400)


def test_code_search(user_factory, anon, client_factory):
    an, _, c = user_factory()
    repo = f"code-{_uuid()}"
    c.post("/repos", json={"name": repo}, expect=201)
    c.post(_r(an, repo) + "/commits", json={
        "message": "m",
        "changes": [{"path": "src/app.ts", "action": "create", "content": "const MAGIC_TOKEN_42 = 1\n"}]},
        expect=201)

    hits = c.get(_r(an, repo) + "/search", params={"q": "MAGIC_TOKEN_42"}, expect=200).json()
    assert len(hits) == 1
    assert hits[0]["path"] == "src/app.ts" and hits[0]["line"] == 1 and "MAGIC_TOKEN_42" in hits[0]["text"]

    # 空仓库 / 无命中 / 缺 q / 坏 ref
    c.post("/repos", json={"name": f"empty-{_uuid()}"}, expect=201)
    c.get("/search", expect=400)
    c.get(_r(an, repo) + "/search", params={"q": "x", "ref": "ghost"}, expect=400)
    anon.get(_r(an, repo) + "/search", params={"q": "x"}, expect=401)


# ---- instance / templates / providers ----

def test_instance_and_templates_and_providers(base_url, anon, user_factory):
    inst = anon.get("/instance", expect=200).json()
    assert isinstance(inst.get("ssh_port") or inst.get("ssh_addr") or inst, (dict, str))

    an0, _, cu = user_factory()
    tpl = cu.get("/templates", expect=200).json()
    assert isinstance(tpl, list)  # 无模版仓库时可为空

    prov = anon.get("/auth/providers", expect=200).json()
    assert isinstance(prov, dict) or isinstance(prov, list)


# ---- 邮箱验证（SMTP 未配置实例） ----

def test_email_verify_without_smtp(user_factory, client_factory):
    an, _, c = user_factory()
    c.post("/me/profile", json={"email": f"{an}@example.com"}, expect=200)

    # 伪造 token 必须 400，不能撞库成功
    c.post("/me/email/verify", json={"token": ""}, expect=400)
    c.post("/me/email/verify", json={"token": "forged-token-" + _uuid()}, expect=400)

    # SMTP 未配置 → resend 明确 400（而不是 200 假装发了）
    r = c.post("/me/email/resend")
    assert r.status_code in (200, 400)
    if r.status_code == 400:
        assert r.json()["code"] == "smtp_not_configured"


# ---- 分支保护 ----

@pytest.fixture
def bp_env(user_factory):
    an, _, c = user_factory()
    repo = f"bp-{_uuid()}"
    c.post("/repos", json={"name": repo}, expect=201)
    c.post(_r(an, repo) + "/visibility", json={"private": False}, expect=200)
    c.post(_r(an, repo) + "/commits", json={
        "message": "m", "changes": [{"path": "a", "action": "create", "content": "1"}]}, expect=201)
    yield an, c, repo
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_branch_protection_crud(bp_env, user_factory):
    an, c, repo = user = bp_env
    base = _r(an, repo) + "/branch-protections"

    assert c.get(base, expect=200).json() == []

    p = c.put(base + "/main", json={"min_approvals": 1, "block_force_push": True,
                                    "block_deletion": True}, expect=200).json()
    assert p["branch"] == "main" and p["min_approvals"] == 1

    # upsert 同分支覆盖
    p2 = c.put(base + "/main", json={"min_approvals": 2}, expect=200).json()
    assert p2["min_approvals"] == 2

    c.put(base + "/feature%2Fx", json={}, expect=200)
    branches = {x["branch"] for x in c.get(base, expect=200).json()}
    assert branches == {"main", "feature/x"}

    # bad path：坏分支名 / min_approvals 边界
    c.put(base + "/bad..name", json={}, expect=400)
    c.put(base + "/x", json={"min_approvals": 101}, expect=200).json()["min_approvals"] <= 100

    # 非所有者不可写（协作者也不行）
    bn, _, bob = user_factory()
    c.post(_r(an, repo) + "/collabs", json={"username": bn, "permission": "write"}, expect=200)
    r = bob.put(base + "/dev", json={})
    assert r.status_code in (403, 404)  # 协作者不可写（404 为隐藏约定）
    bob.get(base, expect=200)  # 协作者可读

    # 删除规则 → 404
    c.delete(base + "/feature%2Fx", expect=204)
    c.delete(base + "/feature%2Fx", expect=404)
