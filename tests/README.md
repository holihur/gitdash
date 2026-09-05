# gitdash 黑盒 API 测试（pytest + requests）

与后端**完全独立、隔离**的接口自动化测试：

- 纯黑盒：只通过 HTTP API 与被测 gitdash 实例通信；不 import 后端代码，
  测试代码不执行 `go build` / `go test`。
- 用例隔离：用户名 / 仓库名 / issue 全部随机生成，用例之间无共享状态、
  无执行顺序依赖；每次会话只使用一个全新实例，结束即销毁。

## 运行方式

依赖由 [uv](https://docs.astral.sh/uv/) 管理（`pyproject.toml` + `uv.lock`）。

### 方式一：指向一个已构建好的二进制（推荐，CI 同款）

```bash
# 1) 单独构建被测二进制（与测试代码解耦）
cd backend && go build -o /tmp/gitdash-server . && cd ..

# 2) 运行测试：夹具会在「独立临时数据目录 + 随机端口」上启动全新实例
cd tests
GITDASH_BIN=/tmp/gitdash-server uv run pytest -v
```

### 方式二：指向一个已运行的实例（例如本地 dev server）

```bash
cd tests
GITDASH_API_URL=http://127.0.0.1:8080 uv run pytest -v
```

两种方式都没配置时整组 skip。

## 覆盖范围（happy path + bad path）

| 文件 | 覆盖 |
| --- | --- |
| `test_auth.py` | 注册 / 登录 / me / 登出、401、400、409、非法 JSON |
| `test_repos.py` | 仓库 CRUD、名称/重复校验、多用户隔离、重复删除 |
| `test_collabs.py` | 协作者管理、读写权限、角色列表、同名仓库、级联删除 |
| `test_issues.py` | Issue 创建/列表/关闭/重开、编号递增、参数校验、隔离、删除级联 |
| `test_ssh_keys.py` | 公钥增删查、非法公钥、指纹全局唯一（跨用户 409） |
| `test_webhooks.py` | Webhook CRUD、URL 校验、owner-only 隔离、级联删除 |
| `test_profile.py` | 资料字段、改密流程、MFA(TOTP)启用/两步登录/禁用、隔离 |
| `test_refs.py` | 分支/标签管理（增删查、HEAD 保护、bad path、权限） |
| `test_fileops.py` | 网页文件/文件夹 CRUD(提交式)、bad path、读写协作者权限 |
| `test_pulls.py` | PR 创建/diff/关闭重开/合并、分叉冲突、bad path 与隔离(SSH 场景) |
| `test_gpg.py` | GPG 公钥增删查、非法/重复(跨用户)校验、401 |
| `test_watch_inbox.py` | Watch/收件箱：关注/取关、自动关注、通知全状态流、已读/删除、401/404 |

> 提示：用户没有删除 API，若对着**常驻实例**反复跑会产生残留用户；
> 对 CI / 一次性实例（方式一）无影响。
