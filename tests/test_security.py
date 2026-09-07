"""安全修复黑盒测试：SSRF 防护（webhook 投递 / 仓库导入）。

对应已修复的三个高危问题中的两个可黑盒验证项：
- Webhook SSRF：默认禁止投递到回环/私有/云元数据地址，仅允许公网目标
  （GITDASH_SSRF_ALLOW_PRIVATE=1 才放开）。
- 仓库导入 SSRF：导入 URL 在创建时即拒绝回环/私有/元数据地址。
- OAuth 同名账号绑定劫持：需要真实 GitHub OAuth 凭据完成回调流程，
  无法纯黑盒验证，不在此覆盖（由带 mock provider 的后端白盒测试保证）。

实例来源见 conftest.py（GITDASH_BIN / GITDASH_API_URL）。
"""

import time
import uuid

import pytest


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


# ---- 导入 SSRF：创建时即拒绝内网目标 ----

# 回环、RFC1918、link-local（云元数据）与解析到回环的主机名
_PRIVATE_IMPORT_URLS = [
    "http://127.0.0.1:8080/evil.git",
    "https://10.0.0.1/evil.git",
    "http://192.168.1.1/evil.git",
    "https://172.16.0.5/evil.git",
    "http://169.254.169.254/latest/meta-data/",
    "http://100.100.100.200/latest/meta-data/",
    "http://localhost/evil.git",
    "ssh://git@192.168.0.10/srv/evil.git",
]


@pytest.mark.parametrize("url", _PRIVATE_IMPORT_URLS)
def test_import_blocks_private_targets(user_factory, url):
    _, _, c = user_factory()
    c.post("/imports", json={"url": url}, expect=400)


def test_import_still_accepts_public_target(user_factory):
    """公网 https 目标仍可发起导入（不实际拉取成功与否不重要，只验证不被误拦）。"""
    _, _, c = user_factory()
    name = f"imp-{_uuid()}"
    r = c.post(
        "/imports", json={"url": "https://github.com/octocat/Hello-World.git", "name": name},
        expect=202,
    ).json()
    assert r["name"] == name
    # 清理（导入可能仍在进行/失败均无妨）
    try:
        c.delete(f"/repos/{name}", expect=204)
    except AssertionError:
        pass


# ---- Webhook SSRF：创建允许，但投递被 SSRF 防护拦截 ----

@pytest.fixture
def hook_env(user_factory):
    name, _, alice = user_factory("alice")
    repo = f"sec-{_uuid()}"
    alice.post("/repos", json={"name": repo}, expect=201)
    yield name, repo, alice
    try:
        alice.delete(f"/repos/{repo}", expect=204)
    except AssertionError:
        pass


def _wait_delivery(client, owner, repo, hook_id, timeout=20):
    """轮询 webhook 投递记录直到出现（spool 消费有 ~2s 间隔）。"""
    path = f"/users/{owner}/repos/{repo}/webhooks/{hook_id}/deliveries"
    deadline = time.time() + timeout
    while time.time() < deadline:
        dl = client.get(path, expect=200).json()
        if dl:
            return dl[0]
        time.sleep(0.5)
    raise AssertionError(f"no delivery record for hook {hook_id}")


@pytest.mark.parametrize(
    "url",
    [
        "http://127.0.0.1:12345/hook",
        "http://10.1.2.3:8080/hook",
        "http://192.168.10.10/hook",
        "https://169.254.169.254/latest/meta-data/",
    ],
)
def test_webhook_delivery_blocks_private_targets(hook_env, url):
    an, repo, alice = hook_env
    hook = alice.post(
        f"/users/{an}/repos/{repo}/webhooks", json={"url": url}, expect=201
    ).json()

    # 触发一次事件（issue 评论事件会走 API 侧 webhook spool）
    issue = alice.post(
        f"/repos/{repo}/issues", json={"title": "trigger", "body": "x"}, expect=201
    ).json()
    alice.post(f"/repos/{repo}/issues/{issue['number']}/comments", json={"body": "go"}, expect=201)

    d = _wait_delivery(alice, an, repo, hook["id"])
    assert d["status"] in ("retry", "failed")
    assert d["code"] == 0
    assert "ssrf" in d["error"].lower()
    # 不应有任何成功投递记录
    all_dl = alice.get(
        f"/users/{an}/repos/{repo}/webhooks/{hook['id']}/deliveries", expect=200
    ).json()
    assert all(x["status"] != "success" for x in all_dl)


def test_webhook_delivery_public_target_recorded(hook_env):
    """公网 https 目标：投递尝试会正常发起并留下记录（失败与否取决于可达性，
    不应被 SSRF 防护拦截；不可解析域名会被视为不可达而跳过，属预期行为）。"""
    an, repo, alice = hook_env
    hook = alice.post(
        f"/users/{an}/repos/{repo}/webhooks",
        json={"url": "https://example.com/hook"},
        expect=201,
    ).json()
    issue = alice.post(
        f"/repos/{repo}/issues", json={"title": "trigger", "body": "x"}, expect=201
    ).json()
    alice.post(f"/repos/{repo}/issues/{issue['number']}/comments", json={"body": "go"}, expect=201)

    d = _wait_delivery(alice, an, repo, hook["id"])
    assert "ssrf" not in d["error"].lower()
