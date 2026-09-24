"""徽章系统黑盒测试：仅管理端定义/授予，可发用户/仓库/组织，目标最多挂 3 个。"""

from __future__ import annotations

import uuid

import pytest

# 以 PNG 魔数开头的字节（http.DetectContentType 会判定为 image/png）。
PNG = b"\x89PNG\r\n\x1a\n" + b"0" * 32


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


@pytest.fixture
def badge_env(user_factory):
    uname, token, c = user_factory("bd")
    repo = f"bd-{_uuid()}"
    c.post("/repos", json={"name": repo}, expect=201)
    org = f"bdorg{_uuid()[:8]}"
    c.post("/orgs", json={"name": org, "display": "Badge Org"}, expect=201)
    yield uname, token, c, repo, org
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass
    try:
        c.delete(f"/orgs/{org}", expect=204)
    except Exception:
        pass


def _create_badge(admin, label, description="", with_image=True):
    data = {"label": label, "description": description}
    files = {"image": ("icon.png", PNG, "image/png")} if with_image else None
    r = admin.request("POST", "/admin/badges", data=data, files=files) if files else admin.post("/admin/badges", data=data)
    assert r.status_code == 201, r.text
    return r.json()


def test_only_admin_can_manage_badges(admin, badge_env):
    _, _, user, _, _ = badge_env
    # 普通用户不能创建 / 列表
    user.post("/admin/badges", data={"label": "x"}, expect=401)
    user.get("/admin/badges", expect=401)
    # 管理端可创建（带图标）
    b = _create_badge(admin, f"Verified-{_uuid()[:6]}", "verified account")
    assert b["has_image"] is True

    # slug 冲突
    admin.post("/admin/badges", data={"label": "dup", "slug": b["slug"]}, expect=409)
    # 缺 label
    admin.post("/admin/badges", data={"label": " "}, expect=400)

    # 删除
    admin.request("DELETE", f"/admin/badges/{b['id']}", expect=204)
    admin.request("DELETE", f"/admin/badges/{b['id']}", expect=404)


def test_grant_to_user_repo_org_and_display(admin, badge_env):
    uname, _, c, repo, org = badge_env
    badges = [_create_badge(admin, f"B{i}-{_uuid()[:4]}") for i in range(4)]
    ids = [b["id"] for b in badges]

    # 任意徽章可发给任意目标：user / repo / org
    admin.post(f"/admin/badges/{ids[0]}/grants", json={"kind": "user", "owner": uname}, expect=204)
    admin.post(f"/admin/badges/{ids[1]}/grants", json={"kind": "repo", "owner": uname, "repo": repo}, expect=204)
    admin.post(f"/admin/badges/{ids[2]}/grants", json={"kind": "org", "owner": org}, expect=204)
    # 重复授予 / 非法目标 / 不存在目标
    admin.post(f"/admin/badges/{ids[0]}/grants", json={"kind": "user", "owner": uname}, expect=409)
    admin.post(f"/admin/badges/{ids[0]}/grants", json={"kind": "user", "owner": ""}, expect=400)
    admin.post(f"/admin/badges/{ids[0]}/grants", json={"kind": "user", "owner": "nobody-xyz"}, expect=404)

    # 公开可见（默认已挂出）
    pub = c.get(f"/badges?kind=user&owner={uname}", expect=200).json()
    assert ids[0] in [x["id"] for x in pub]
    assert c.get(f"/badges?kind=repo&owner={uname}&repo={repo}", expect=200).json()[0]["id"] == ids[1]
    assert c.get(f"/badges?kind=org&owner={org}", expect=200).json()[0]["id"] == ids[2]

    # 授予用户目标 -> 站内信
    inbox = c.get("/inbox", expect=200).json()
    assert any(n["kind"] == "system" and n["action"] == "badge_granted" for n in inbox)

    # 目标自己选择挂哪些（最多 3）：把 4 个都授予再设置
    for i in (1, 2, 3):
        admin.post(f"/admin/badges/{ids[i]}/grants", json={"kind": "user", "owner": uname}, expect=204)
    own = c.get(f"/badges/owned?kind=user&owner={uname}", expect=200).json()
    assert len(own["granted"]) == 4 and own["max"] == 3
    # 请求 4 个 + 一个未授予的 -> 只保留前 3 个已授予
    c.put(
        "/badges/display",
        json={"kind": "user", "owner": uname, "badge_ids": [ids[0], ids[1], ids[2], ids[3], 99999]},
        expect=200,
    )
    disp = c.get(f"/badges?kind=user&owner={uname}", expect=200).json()
    assert [x["id"] for x in disp] == [ids[0], ids[1], ids[2]]

    # 非本人无权设置
    from conftest import ApiClient

    other_c = ApiClient(c.base)
    name2 = f"other-{_uuid()}"
    other_c.token = other_c.post(
        "/auth/register", json={"username": name2, "password": "test-pass-123456"}, expect=201
    ).json()["token"]
    other_c.get(f"/badges/owned?kind=user&owner={uname}", expect=403)
    other_c.put(
        "/badges/display",
        json={"kind": "user", "owner": uname, "badge_ids": [ids[3]]},
        expect=403,
    )

    # 撤销：从展示与获得中移除
    admin.request("DELETE", f"/admin/badges/{ids[0]}/grants", params={"kind": "user", "owner": uname}, expect=204)
    admin.request("DELETE", f"/admin/badges/{ids[0]}/grants", params={"kind": "user", "owner": uname}, expect=404)
    own = c.get(f"/badges/owned?kind=user&owner={uname}", expect=200).json()
    assert ids[0] not in [x["id"] for x in own["granted"]]


def test_badge_update_image_and_grants(admin, badge_env):
    uname, _, c, _, _ = badge_env
    b = _create_badge(admin, f"Upd-{_uuid()[:6]}")

    # 更新文字信息
    upd = admin.patch(f"/admin/badges/{b['id']}", json={"label": "Renamed", "description": "d"}, expect=200).json()
    assert upd["label"] == "Renamed"
    admin.patch(f"/admin/badges/{b['id']}", json={"label": " "}, expect=400)
    admin.patch("/admin/badges/999999", json={"label": "x"}, expect=404)

    # 替换图标
    replaced = admin.request(
        "POST", f"/admin/badges/{b['id']}/image", files={"image": ("i.png", PNG, "image/png")}
    )
    assert replaced.status_code == 200, replaced.text
    assert replaced.json()["has_image"] is True

    # emoji 兜底：可创建 / 更新；过期或含控制字符拒绝
    created = admin.post(
        "/admin/badges", data={"label": f"Emoji-{_uuid()[:4]}", "emoji": "⭐"}, expect=201
    ).json()
    assert created["emoji"] == "⭐"
    emoji_upd = admin.patch(f"/admin/badges/{b['id']}", json={"emoji": "🏅"}, expect=200).json()
    assert emoji_upd["emoji"] == "🏅"
    admin.patch(f"/admin/badges/{b['id']}", json={"emoji": "x" * 9}, expect=400)
    admin.patch(f"/admin/badges/{b['id']}", json={"emoji": "bad\u0000value"}, expect=400)

    # 删除图标：保留徽章定义与 emoji，且可重复调用
    dele = admin.request("DELETE", f"/admin/badges/{b['id']}/image", expect=200).json()
    assert dele["has_image"] is False and dele["emoji"] == "🏅"
    admin.request("DELETE", f"/admin/badges/{b['id']}/image", expect=200)
    admin.request("DELETE", "/admin/badges/999999/image", expect=404)

    # 授予记录列表
    admin.post(f"/admin/badges/{b['id']}/grants", json={"kind": "user", "owner": uname}, expect=204)
    grants = admin.get(f"/admin/badges/{b['id']}/grants", expect=200).json()
    assert len(grants) == 1 and grants[0]["kind"] == "user" and grants[0]["owner"] == uname



def test_badges_batch(admin, badge_env):
    uname, _, c, repo, _org = badge_env
    b = _create_badge(admin, f"Batch-{_uuid()[:6]}")
    admin.post(f"/admin/badges/{b['id']}/grants", json={"kind": "repo", "owner": uname, "repo": repo}, expect=204)

    res = c.post(
        "/badges/batch",
        json={
            "kind": "repo",
            "targets": [{"owner": uname, "repo": repo}, {"owner": uname, "repo": "nope"}],
        },
        expect=200,
    ).json()
    key = f"{uname}/{repo}"
    assert key in res["items"] and res["items"][key][0]["id"] == b["id"]
    assert f"{uname}/nope" not in res["items"]
    # 非法 kind -> 400
    c.post("/badges/batch", json={"kind": "bogus", "targets": []}, expect=400)


def test_badge_image_public(admin, badge_env):
    b = _create_badge(admin, f"Img-{_uuid()[:6]}")
    _, _, c, _, _ = badge_env
    r = c.get(f"/badges/{b['id']}/image", expect=200)
    assert r.headers["Content-Type"].startswith("image/png")
    assert r.content == PNG
    # 无图标 -> 404
    b2 = _create_badge(admin, f"NoImg-{_uuid()[:6]}", with_image=False)
    c.get(f"/badges/{b2['id']}/image", expect=404)
