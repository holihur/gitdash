"""仓库可见性三档安全测试：private（默认）/ public（登录可读）/ anonymous（匿名只读）。"""
from __future__ import annotations

import requests


def test_repo_visibility_levels(base_url, repo_factory, user_factory):
    repo, c = repo_factory("vis")
    owner = c.get("/me", expect=200).json()["username"]
    _, other_tok, _ = user_factory("rv")
    oh = {"Authorization": f"Bearer {other_tok}"}
    base = f"{base_url}/api/users/{owner}/repos/{repo}"

    def anon(path: str) -> int:
        return requests.get(base + path, timeout=10).status_code

    def other(path: str) -> int:
        return requests.get(base + path, headers=oh, timeout=10).status_code

    paths = ["", "/branches", "/issues", "/releases", "/tree?ref=main"]

    def set_vis(v: str) -> None:
        c.post(f"/users/{owner}/repos/{repo}/visibility", json={"visibility": v}, expect=200)

    # 默认 private：owner 可读，其他登录用户与匿名都 404
    for p in paths:
        assert c.get(f"/users/{owner}/repos/{repo}{p}", expect=200) is not None
        assert other(p) == 404, p
        assert anon(p) == 404, p

    # 非 owner 不能改可见性
    assert requests.post(
        f"{base_url}/api/users/{owner}/repos/{repo}/visibility",
        json={"visibility": "public"}, headers=oh, timeout=10,
    ).status_code == 404
    # 非法值拒绝
    assert c.post(
        f"/users/{owner}/repos/{repo}/visibility", json={"visibility": "bogus"}, expect=None
    ).status_code == 400

    # public：登录用户可读，匿名仍不可读
    set_vis("public")
    for p in paths:
        assert other(p) == 200, p
        assert anon(p) == 404, p

    # anonymous：匿名可读（只读）
    set_vis("anonymous")
    for p in paths:
        assert anon(p) == 200, p
        assert other(p) == 200, p
    # 写操作仍要求登录（匿名 401 / 无权限 404）
    assert requests.post(f"{base}/issues", json={"title": "x"}, timeout=10).status_code == 401

    # 回到 private：匿名/他人再次 404
    set_vis("private")
    for p in paths:
        assert anon(p) == 404, p
        assert other(p) == 404, p
