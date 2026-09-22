"""语言配色：公开语言列表 + 管理端覆盖保存/校验/恢复。"""


def test_languages_public_lists_known_languages(anon):
    data = anon.get("/languages", expect=200).json()
    assert isinstance(data["languages"], list)
    assert "Go" in data["languages"]
    assert "C++" in data["languages"]
    # 默认配色由后端提供，且覆盖全部已知语言
    assert isinstance(data["defaults"], dict)
    assert data["defaults"]["Go"] == "#00ADD8"
    assert set(data["languages"]).issubset(set(data["defaults"]))
    assert isinstance(data["colors"], dict)


def test_admin_saves_language_colors(admin, anon):
    before = anon.get("/languages", expect=200).json()["colors"]
    try:
        admin.post("/admin/language-colors", json={"colors": {"Go": "#123abc"}}, expect=200)
        after = anon.get("/languages", expect=200).json()["colors"]
        assert after["Go"] == "#123abc"

        # 非法颜色被拒绝
        bad = admin.post("/admin/language-colors", json={"colors": {"Go": "red"}})
        assert bad.status_code == 400
    finally:
        admin.post("/admin/language-colors", json={"colors": before}, expect=200)
