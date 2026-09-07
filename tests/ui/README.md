# gitdash 黑盒 E2E UI 测试（Playwright + headless Chrome）

与后端**完全独立、隔离**的 UI 自动化测试（对齐 `tests/` 黑盒 API 测试的风格）：

- **纯 UI 驱动**：所有用例通过 headless Chrome 在真实页面上点击 / 输入 / 断言，
  不发任何 API 请求（唯一的例外：自启实例后用 `GET /api/health` 做就绪探测，
  与 `scripts/e2e.sh`、`tests/conftest.py` 的做法一致，仅用于等待启动）。
- **用例隔离**：用户名 / 仓库名随机生成；每个用例使用全新 browser context
  （cookie / localStorage 完全隔离），无跨用例顺序依赖。
- **语言确定性**：context 强制 `locale: en-US`，选择器依赖英文文案与稳定 id
  （`#username`、`#password`、`#repo-name`、`#issue-title` 等）。

## 运行方式

浏览器复用全局 Playwright 缓存 `~/.cache/ms-playwright`（若缺chromium 执行
`npx playwright install chromium` 一次性下载，之后离线可用）。

### 方式一：指向一个已构建好的二进制（推荐，Taskfile 同款）

被测二进制必须**内嵌前端**（`backend/internal/webui/dist` 存在真实产物）：

```bash
# 1) 构建带内嵌前端的二进制（或直接 task test:ui，自动完成）
task frontend && (cd backend && go build -o /tmp/gitdash-server-ui .)

# 2) 运行测试：夹具在「独立临时数据目录 + 随机端口」上启动全新实例，结束自动清理
cd tests/ui
GITDASH_BIN=/tmp/gitdash-server-ui npx playwright test
```

或一键：`task test:ui`

### 方式二：指向一个已运行的实例（例如本地 dev server）

```bash
cd tests/ui
GITDASH_UI_URL=http://127.0.0.1:8080 npx playwright test
```

两种方式都没配置时整组 skip。

## 常用命令

```bash
npx playwright test --grep @happy   # 只跑 happy path
npx playwright show-report          # 查看报告（含失败截图/trace）
```

## 覆盖范围（一个业务一个文件）

| 文件 | 覆盖 |
| --- | --- |
| `src/auth.spec.ts` | 注册即登录、登出/重登、错误密码拒绝 |
| `src/repos.spec.ts` | 建仓库（README 模板）、卡片/clone 命令、重名与非法名拒绝 |
| `src/code.spec.ts` | 仓库代码浏览：README.md 树/渲染、Commits 初始提交 |
| `src/issues.spec.ts` | Issue 创建与列表展示、空标题禁提交 |

> 发现的产品 bug 记录在同目录 `bug.md`。
