"""参数化（表单化）流水线 + 入站 webhook 触发流水线。"""

import uuid

PARAM_YAML = (
    "on: [workflow_dispatch]\n"
    "params:\n"
    "  - name: version\n"
    "    description: Release version\n"
    "    required: true\n"
    "  - name: environment\n"
    "    type: choice\n"
    "    options: [staging, production]\n"
    "    default: staging\n"
    "  - name: dry_run\n"
    "    type: boolean\n"
    "    default: 'true'\n"
    "steps:\n"
    "  - name: build\n"
    "    run: echo build\n"
)


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def _p(owner, repo, suffix=""):
    return f"/users/{owner}/repos/{repo}/pipeline{suffix}"


def _commit(c, owner, repo, path, content):
    c.post(
        f"/users/{owner}/repos/{repo}/commits",
        json={"message": f"add {path}", "changes": [{"path": path, "action": "create", "content": content}]},
        expect=201,
    )


def _setup(user_factory):
    an, _, c = user_factory("pp")
    repo = f"pp-{_uuid()}"
    c.post("/repos", json={"name": repo, "private": False}, expect=201)
    _commit(c, an, repo, "README.md", f"# {repo}\n")
    _commit(c, an, repo, ".gitdash.yml", PARAM_YAML)
    return an, c, repo


def test_pipeline_params_and_form(user_factory):
    an, c, repo = _setup(user_factory)

    # 参数声明可读（供前端渲染表单）
    got = c.get(_p(an, repo) + "/params?file=.gitdash.yml", expect=200).json()
    names = [p["name"] for p in got["params"]]
    assert names == ["version", "environment", "dry_run"]
    assert got["params"][1]["type"] == "choice"
    assert got["params"][1]["options"] == ["staging", "production"]

    # 缺少必填 → 400
    c.post(_p(an, repo) + "/runs", json={"file": ".gitdash.yml", "inputs": {}}, expect=400)
    # 非法选项 → 400
    c.post(
        _p(an, repo) + "/runs",
        json={"file": ".gitdash.yml", "inputs": {"version": "1.0", "environment": "nope"}},
        expect=400,
    )
    # 非法布尔 → 400
    c.post(
        _p(an, repo) + "/runs",
        json={"file": ".gitdash.yml", "inputs": {"version": "1.0", "dry_run": "maybe"}},
        expect=400,
    )

    # 合法输入 → 创建运行，且默认值被套用
    created = c.post(
        _p(an, repo) + "/runs",
        json={"file": ".gitdash.yml", "inputs": {"version": "1.2.3"}},
        expect=201,
    ).json()
    run = created["runs"][0]
    assert run["inputs"]["version"] == "1.2.3"
    assert run["inputs"]["environment"] == "staging"
    assert run["inputs"]["dry_run"] == "true"

    c.delete(f"/repos/{repo}", expect=204)


def test_incoming_webhook_triggers_pipeline(user_factory):
    an, c, repo = _setup(user_factory)

    hook = c.post(f"/users/{an}/repos/{repo}/incoming-webhooks", json={"name": "ci"}, expect=201).json()
    token = hook["token"]

    # 入站 webhook 触发流水线（workflow_dispatch 语义）
    r = c.session.post(
        f"{c.base}/api/hooks/incoming/{an}/{repo}",
        headers={"X-Gitdash-Token": token},
        json={"pipeline": {"file": ".gitdash.yml", "inputs": {"version": "9.9.9"}}},
        timeout=15,
    )
    assert r.status_code == 201, r.text
    body = r.json()
    assert len(body["runs"]) == 1
    assert body["runs"][0]["event"] == "workflow_dispatch"
    assert body["runs"][0]["inputs"]["version"] == "9.9.9"
    assert body["runs"][0]["inputs"]["environment"] == "staging"

    # 缺失必填参数 → 400
    r = c.session.post(
        f"{c.base}/api/hooks/incoming/{an}/{repo}",
        headers={"X-Gitdash-Token": token},
        json={"pipeline": {"file": ".gitdash.yml"}},
        timeout=15,
    )
    assert r.status_code == 400

    # 既无 title 也无 pipeline → 400
    r = c.session.post(
        f"{c.base}/api/hooks/incoming/{an}/{repo}",
        headers={"X-Gitdash-Token": token},
        json={},
        timeout=15,
    )
    assert r.status_code == 400

    c.delete(f"/repos/{repo}", expect=204)
