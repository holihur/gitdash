# gitdash 交接文档（CI / PR / 邮件 / 搜索）

> 面向接手 #3、#4 的同事。记录已完成内容、待办设计、代码地图、开发与测试规范。
> 语言：Go（后端）+ React/TS（前端）。远程：`origin` = GitHub，`gitdash` = 自建实例。

## 0. 本轮起点

- 基线：`origin/main`
- 本轮提交（10 个，已 `git log --oneline origin/main..HEAD` 可查）：

```
d880034 feat(pulls): merge queue
b4665d1 feat(pulls): auto-merge
ad1d49d feat(pulls): CODEOWNERS required reviewers
643861d feat(pulls): apply inline code suggestions
344be4e feat(pulls): draft pull requests
d0963a7 test(api): black-box coverage for pipeline secrets, cache and artifacts
3739a4e feat(pipeline): downloadable run artifacts
1c66911 feat(pipeline): cross-run cache
5408886 feat(pipeline): repository CI secrets
c694ebb refactor(web): split god pages into focused components and hooks
```

## 1. 已完成

### 1.1 上帝页面拆分（纯重构，`c694ebb`）

把「功能挤在一个文件」的页面按职责拆开，所有拆分均为纯抽取、行为不变：

| 页面 | 行数变化 | 抽出的模块 |
|---|---|---|
| `frontend/src/pages/RepoIssues.tsx` | 720 → 506 | `components/issues/{issue-item,issue-filters}.tsx` |
| `frontend/src/pages/RepoView.tsx` | 600 → 404 | `pages/repoview/{use-repo-routing,use-repo-data}.ts` |
| `components/projects-board.tsx` | 561 → 398 | `components/projects-board-view.tsx` |
| `pages/repoview/settings-tab.tsx` | 461 → 44 | `pages/repoview/settings/*Card.tsx`（9 个） |
| `pages/repoview/code-tab.tsx` | 464 → 389 | `pages/repoview/repo-code-body.tsx` |

### 1.2 #1 CI：secrets + cache + artifacts

**secrets（`5408886`）**
- 存储：`backend/internal/store/secrets.go`；表 `repo_secrets`；AES-256-GCM 静态加密
  （`GITDASH_SECRET_KEY` 或自动生成的 `data/secrets.key`，0600）。API 永不回传明文。
- DSL：顶层 `secrets: [NAME, ...]` 白名单。
- 执行：`pipeline/pipeline.go` 的 `executeRun` 解析白名单并注入 env；`pipeline/masking.go`
  在写日志时把值替换为 `***`。远程 runner 经 `runner.Job.Env` 下发。
- API：`GET/PUT/DELETE /users/{owner}/repos/{name}/secrets`（owner-only），`internal/api/secrets.go`。
- 前端：`pages/repoview/settings/SecretsCard.tsx`。

**cache（`1c66911`）**
- DSL：`cache: { key, paths }`（`key` 默认 `default`；`paths` 必须是工作区内相对路径）。
- 执行：`pipeline/cache.go`，运行前 restore、成功后 save（原子替换）。
- 目录：服务端 `data/cache/{owner}/{repo}/{key}`；runner 用 `GITDASH_RUNNER_CACHE` 或临时目录。

**artifacts（`3739a4e`）**
- DSL：`artifacts: { paths: [...] }`，成功后归档到 `data/artifacts/{owner}/{repo}/run-{id}`。
- API：`GET .../pipeline/runs/{id}/artifacts`（列表）、`.../artifacts/download`（tar.gz），`internal/api/artifacts.go`。
- run DTO 增加 `has_artifacts`；删仓库时 `pipeline.DeleteArtifacts`。
- **限制**：远程 runner 暂不上传 artifacts（仅服务端本地执行生成）。

DSL 解析：`backend/internal/pipeline/dsl.go`（新增 `readCache` / `readArtifacts` / `readNestedList`）。
文档：`frontend/src/components/pipeline-docs.tsx` + `locales/{en,zh-CN}.ts` 的 `pipeline.docs.*`。

### 1.3 #2 PR：五个子功能

| 子项 | 提交 | 关键点 |
|---|---|---|
| Draft PR | `344be4e` | `pull_requests.draft`；`POST /pulls/{n}/draft`；草稿合并返回 409 `pull_is_draft` |
| Code suggestion | `643861d` | 行内评论里 ` ```suggestion ` 块；`POST /pulls/{n}/comments/{id}/apply`；`suggestion_applied_sha` 标记一次性 |
| CODEOWNERS | `ad1d49d` | `internal/gitsvc/codeowners.go` 解析器；分支保护 `require_codeowners`；`GET /pulls/{n}/codeowners` |
| Auto-merge | `b4665d1` | `pull_requests.auto_merge(_method)`；`POST /pulls/{n}/auto-merge`；review/pipeline-success 触发 |
| Merge queue | `d880034` | 表 `merge_queue`；分支保护 `merge_queue`；合并在该分支上改为 202 入队、串行处理 |

- 合并逻辑已抽到 `backend/internal/api/merge.go`：`mergeGateError`（门禁）/ `executeMerge`（执行）/
  `tryAutoMerge` / `processMergeQueue` / `dequeuePull`。**新增合并门禁请只在这里加**，手动合并与
  自动合并/队列共用。
- 分支保护模型：`backend/internal/store/branchprotection.go` + `models.go`（`branchProtectionRow`）。
- CI 成功回调：`pipeline.SetRunSuccessHook`（`pipeline/pipeline.go`），在 `main.go` 注入
  `a.AutoMergeForRepo`。
- 前端：`pages/RepoPulls.tsx`（合并/草稿/自动合并/队列/CODEOWNERS）、`components/diff-view.tsx`
  （suggestion 应用）、`pages/repoview/pull-diff.tsx`（`CodeownersBadge`）、
  `pages/repoview/settings/BranchProtectionsCard.tsx`。

### 1.4 黑盒测试（`d0963a7` + 各功能自带）

`tests/` 下 pytest 黑盒（HTTP 打真实进程）新增：
`test_pipeline_secrets.py`、`test_pipeline_host.py`（secrets/cache/artifacts 端到端）、
`test_codeowners.py`、`test_auto_merge.py`、`test_merge_queue.py`，以及 `test_pulls.py` 的
draft/suggestion 用例。

## 2. 待办 #3：邮件一等频道

### 2.1 现状（请先读，避免重复造轮子）

- SMTP：`backend/internal/notify/email.go`，`NewSender()`（`GITDASH_SMTP_HOST` 未设置时返回 nil，全 no-op）。
- **活动邮件通知已实现**：`notify.EmailHandler(st, sender)` 挂在调度器上
  （`main.go`：`dispatcher.Run(apiSpool, 2s, notify.EmailHandler(st, sender), pipeline.PullHandler(st))`）。
  收件人 = `store.NotifyRecipients`（watcher + owner/org 成员 - actor），再经 `store.EmailTargets`
  按 `users.notify_email = true` 过滤。issue/PR/review 事件都会发信。
- 收件箱（站内信）：`backend/internal/store/notifications.go`、`internal/api/inbox.go`。

**结论：#3 只剩两块 —— reply-by-email 与 patch-by-email。**

### 2.2 reply-by-email（邮件回复即评论）

目标：用户直接回复通知邮件，内容作为评论落库。

落地建议（无需内建邮件服务器）：
1. **出站**：`notify/email.go` 的 `buildMessage` 增加 `Message-ID` 与 `Reply-To`：
   `reply+<token>@<GITDASH_MAIL_REPLY_DOMAIN>`。`token` 用 HMAC-SHA256（密钥取
   `GITDASH_SECRET_KEY` 或独立 `GITDASH_MAIL_SECRET`）对
   `owner|repo|kind|number|username` 签名（截断）。把 `Message-ID` 也写入
   评论/通知，便于线程。
2. **入站**：新增 `POST /api/mail/inbound`（`internal/api/mail.go`），由邮件提供商/MTA 管道
   以共享密钥调用，body 为 `{to, from, subject, text}`。
   - 从 `to` 提取 token → 验签 → 还原 `owner/repo/kind/number/username`；
   - 清洗正文：去掉引用行（`>` 开头）、签名（`-- ` 之后）、限长；
   - `store.CreateComment(owner, repo, kind, number, username, body, nil)`；
   - 用现有 `a.notify(...)` / `a.Publish` 广播。
   - 安全：共享密钥 + HMAC；只允许 `kind ∈ {issue,pull}`；对同一 token 可加限流。
3. **部署文档**：在 `docs/` 补一节，演示用 Postfix `aliases` 管道或 Mailgun/Postmark inbound
   webhook 调用该端点。
4. 测试：`tests/test_mail_inbound.py`（伪造已签名 token 的入站请求 → 断言评论出现；伪造签名 → 401/400）。

### 2.3 patch-by-email（`git send-email` 兼容）

目标：`git send-email --to=patches@<domain>` 或上传 `.patch`/mbox → 自动建分支 + 开 PR。

落地建议：
1. `internal/gitsvc/` 增加 `ApplyPatchSeries(owner, name, base, patchData []byte) (branch, sha string, err error)`：
   临时 clone → `git checkout -b patches/<ts>` → `git am --3way` → push 回 bare 仓库。
   复用 `gitsvc.WriteCommit` 的思路（见 `internal/gitsvc/commit.go`）。
2. `POST /api/users/{owner}/repos/{name}/patches`（`Content-Type: text/plain`，PAT 鉴权），
   调 `ApplyPatchSeries` 后用现有逻辑 `store.CreatePull`（source=新分支，target=默认分支）。
   多封 patch 是一个 mbox，`git am` 可一次吃掉。
3. 若走 2.2 的入站邮件：识别 `Subject: [PATCH`，转交同一处理函数。
4. 前端可不改（结果就是普通 PR）；可选在 Explore/仓库页加使用提示。
5. 测试：`tests/test_patches.py` 用 `git format-patch` 造 patch，POST 后断言 PR 存在、diff 正确。

## 3. 待办 #4：全局代码搜索

### 3.1 现状

- 仓库内代码搜索：`GET /api/users/{owner}/repos/{name}/search`（`internal/api/search.go` 的 `search`）。
- 全局搜索仅有**仓库**维度：`GET /api/search`（`globalSearch`，按名称/描述/topics）。
- 前端：`src/lib/api/search.ts`（仅 `searchRepo`）、`pages/Explore.tsx`。

### 3.2 设计（MVP 用 grep，后续再上索引）

1. **API**：`GET /api/search/code?q=&repo=&lang=&path=&limit=`（`internal/api/search.go` 新增 handler）。
   - `q` 支持内联限定符：`repo:owner/name`、`lang:go`、`path:src/`、`symbol:Foo`；
     服务端解析出限定符 + 关键词（也可前端解析后传结构化参数）。
   - 权限：仅搜索当前用户可访问的仓库（复用现有「可访问仓库列表」查询，注意 private/协作者/组织/公开）。
   - 实现：对候选仓库的默认分支跑 `git grep -n -I --fixed-strings`（或 `-E` 支持 `symbol:` 单词边界），
     并发池 + 总超时 + 每仓库结果上限；`lang:` 用扩展名映射过滤，`path:` 直接作为 grep pathspec。
   - 返回：`{results: [{owner, repo, path, line, text, ...}], truncated: bool}`。
2. **前端**：
   - `src/lib/api/search.ts` 加 `searchCode(...)`，`src/lib/api/types.ts` 加结果类型。
   - `pages/Explore.tsx` 增加「代码」结果分区（复用现有 `code-search.tsx` 的展示样式）。
   - i18n：`explore.*` 新增 key，**en 与 zh-CN 必须同步**（见规范）。
3. **规模化（可选后续）**：增量索引（zoekt/trigram）放到 `internal/search/`，用后台任务维护；
   API 层保持不变即可平滑替换。
4. **测试**：`tests/test_code_search.py`（多仓库、限定符、权限隔离、private 不可见）。

## 4. 开发规范

- **后端**：Go 标准库 HTTP + GORM（SQLite/PG）。
  - 新表：在 `internal/store/models.go` 定义 `xxxRow` + `TableName()`，并在
    `internal/store/migrate.go` 的 `AutoMigrate(...)` 注册。AutoMigrate 只加列/表，不改主键。
  - 新接口：handler 放 `internal/api/<feature>.go`，路由注册在 `internal/api/api.go`，
    请求体 DTO 放 `internal/api/dto_*.go`，handler 上写 swagger 注解。
  - **注意结构体转换**：`store.Comment` 由 `Comment(commentRow)` 转换、`BranchProtection` 由
    `BranchProtection(row)` 转换 —— 两侧字段集合必须**完全一致**（含新增字段），否则编译失败。
    `PullRequest` 是独立 DTO，需在 `pullToDTO` 手动同步。
- **前端**：React + TS + Tailwind + shadcn/ui 风格组件。
  - API：`src/lib/api/<area>.ts`；类型：`src/lib/api/types.ts`。
  - **i18n 强制**：`src/locales/en.ts` 与 `zh-CN.ts` key 必须完全对齐（`src/test/locales.test.ts` 会失败）；
    其余语言只能是 en 的子集。
- **OpenAPI**：改完注解跑 `task swagger`（或
  `cd backend && go run github.com/swaggo/swag/cmd/swag init -g internal/api/doc.go -o internal/api/docs`），
  并提交 `internal/api/docs/*`。
- **提交习惯**：Conventional Commits；一个功能一个提交；纯重构单独提交（本项目历史如此）。
  「先做→测试→提交」再进入下一项。

## 5. 测试与构建命令

```bash
# 后端单测 + vet
cd backend && go test ./...
go vet ./...

# 前端类型检查 + 单测 + 构建
cd frontend && pnpm exec tsc --noEmit && pnpm test && pnpm build

# 黑盒 API 测试（需 uv；单独构建被测二进制）
cd backend && go build -o /tmp/gitdash-server-pytest .
cd tests && GITDASH_BIN=/tmp/gitdash-server-pytest .venv/bin/python -m pytest -q
#   或：GITDASH_BIN=/tmp/gitdash-server-pytest task test:api

# 一键（较全）
task test        # Go + 前端 + E2E 冒烟
task test:all    # 再加 pytest + Playwright
```

> 本机若无网络，`uv run` 可能因镜像 403 失败；可直接用已存在的
> `tests/.venv/bin/python -m pytest`。

## 6. 关键代码地图

| 关注点 | 文件 |
|---|---|
| 流水线 DSL 解析 | `backend/internal/pipeline/dsl.go` |
| 流水线执行编排 | `backend/internal/pipeline/pipeline.go`（`executeRun`）、`executor.go`（`RunInWorkspace`） |
| secrets / cache / artifacts | `store/secrets.go`、`pipeline/{masking,cache,artifacts}.go`、`api/{secrets,artifacts}.go` |
| 远程 runner 协议 | `backend/internal/runner/{proto,remote,hub}.go`、`backend/cmd/gitdash-runner/main.go` |
| PR 合并门禁/执行 | `backend/internal/api/merge.go` |
| CODEOWNERS | `backend/internal/gitsvc/codeowners.go`、`api/codeowners.go` |
| 分支保护 | `backend/internal/store/branchprotection.go`、`api/branchprotection.go` |
| 评论/行内评论 | `backend/internal/store/comments.go`、`api/comments.go`、`frontend/src/components/diff-view.tsx` |
| 通知（站内 + 邮件） | `store/notifications.go`、`notify/email.go`、`api/inbox.go` |
| 搜索 | `backend/internal/api/search.go`、`frontend/src/lib/api/search.ts`、`pages/Explore.tsx` |
| main 装配（队列/回调/spool） | `backend/main.go` |

## 7. 已知坑 / 限制

1. **远程 runner 与本地执行不一致**：runner 复用 `pipeline.RunInWorkspace`，但 `artifactsDir`/`cacheDir`
   在 runner 侧未初始化 —— cache 走 `GITDASH_RUNNER_CACHE`/临时目录，**artifacts 仅在服务端本地执行生成**。
   secrets 会经 WS 下发到 agent（注意传输层应启用 TLS/wss）。
2. **审批与 head 绑定**：审批记录 `commit_sha`，head 前进后旧 approve 失效（CODEOWNERS/最少审批都如此）。
   suggestion 应用会产生新提交，需重新审批。
3. **合并队列语义**：入队后**不会**立即合并；队首门禁未满足会阻塞后续（保持顺序）。CI 成功/新 review 会触发重试。
4. **GORM 结构体转换约束**：见 4 节，新增字段务必两侧同步。
5. **DSL 解析器是行式解析**：新增嵌套块要写专用 reader（参考 `readCache`/`readArtifacts`），
   并更新 `dsl.go` 顶部注释与 `pipeline-docs.tsx`。
6. **SMTP 未配置时 `Sender` 为 nil**：所有发信点都要判空（现有代码已如此）。
7. **OpenAPI 不同步**：`docs.go/swagger.json/yaml` 是提交物，改了 API 记得重新生成。
8. **`push` 事件不触发邮件**（`EmailHandler` 显式跳过），如需可另行加开关。

## 8. 验收清单（接手方）

- [ ] #3.1 reply-by-email：入站端点 + 签名校验 + 正文清洗 + 黑盒测试；部署文档。
- [ ] #3.2 patch-by-email：`ApplyPatchSeries` + 端点 + 自动开 PR + 黑盒测试。
- [ ] #4 全局代码搜索：`/search/code` + 限定符 + 权限隔离 + 前端结果区 + 黑盒测试。
- [ ] 每项完成：`go test ./...`、`tsc`、`eslint`、`vitest`、`pnpm build`、相关 pytest 全绿；
      `task swagger` 已更新；i18n en/zh 对齐。
- [ ] 每个功能一个提交，Conventional Commits。
