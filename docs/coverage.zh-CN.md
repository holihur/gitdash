# 覆盖率与找 bug 路线图

> 覆盖率的目的是**发现问题**，数字只是目标：每新覆盖一个端点/一行代码，都是
> 可能藏 bug 的地方。本文记录分阶段计划与已发现的缺陷。

## 指标

| 指标 | 度量方式 | 当前 | 目标 |
|---|---|---|---|
| Go 单测覆盖率 | `go test ./... -covermode=atomic -coverprofile=…`（Codecov `unittests`） | ~30% | 80% |
| 黑盒 API 端点覆盖率 | pytest 设 `GITDASH_ROUTE_COVERAGE_FILE` + `scripts/route-coverage.py` | **306/306（100%）** | 100% |
| UI（Playwright）端点覆盖率 | `scripts/ui-api-coverage.py`（前端调用的端点 × 路由命中） | 101/192（52.6%） | UI 可达面的 100% |

端点指标是**路由命中**覆盖：每个注册到 `http.ServeMux` 的 pattern 至少被请求
命中一次。`internal/api/routecov.go` 包装 mux，记录清单（`route<TAB>pattern`）
与每个 pattern 的首次命中（`hit<TAB>pattern`）。

## 「UI 端点覆盖率 100%」的范围

306 条路由并非都有网页入口（包注册表、Docker `/v2/`、SSH、OAuth 回调、admin
面板、git 协议等）。因此该面**由前端源码定义**：`scripts/ui-api-coverage.py`
静态提取 UI 能调用的全部端点（当前 192 个），并与记录的路由命中做结构化匹配
（字面段可匹配路由通配符）。100% 目标针对这个 UI 可达集，而不是全部清单。同一
次运行也会标出「前端调用了但无对应路由」的路径（前后端不一致）。

## 本地度量

```bash
# 单测
cd backend && go test ./... -covermode=atomic -coverprofile=/tmp/unit.out
go tool cover -func=/tmp/unit.out | tail -1

# 黑盒 API 端点覆盖
cd backend && go build -cover -o /tmp/gitdash-server .
cd tests && GITDASH_BIN=/tmp/gitdash-server \
  GITDASH_ROUTE_COVERAGE_FILE=/tmp/routes-api.txt .venv/bin/python -m pytest -q
python3 scripts/route-coverage.py /tmp/routes-api.txt --min 100

# UI 端点覆盖（二进制需内嵌前端）
(cd frontend && pnpm run build)
rm -rf backend/internal/webui/dist && mkdir -p backend/internal/webui/dist
cp -r frontend/dist/. backend/internal/webui/dist/ && touch backend/internal/webui/dist/.gitkeep
(cd backend && go build -cover -o /tmp/gitdash-server-ui .)
cd tests/ui && GITDASH_BIN=/tmp/gitdash-server-ui \
  GITDASH_ROUTE_COVERAGE_FILE=/tmp/routes-ui.txt npx playwright test
python3 scripts/ui-api-coverage.py --src frontend/src --coverage /tmp/routes-ui.txt --list-missing
```

一键：`bash scripts/coverage-blackbox.sh`。

## 分阶段计划

1. **阶段 1 —— 基建 + API 端点覆盖（已完成）。**
   路由清单/命中记录、检查脚本、CI 对黑盒 API 门禁 100%；探测未覆盖端点并修
   掉暴露的问题。
2. **阶段 2 —— UI 端点覆盖（进行中）。** 从前端提取 UI 可达面（当前 192 个端
   点），Playwright 已覆盖 101 个（52.6%）。按批次补页面流程（仓库设置、项目看
   板、流水线、PR、copilot、删除类），直到该面 100%；CI 暂以报告形式非阻断。
3. **阶段 3 —— Go 单测爬坡。** 按包提升单测（先 gitsvc / store / api），为每
   个已发现缺陷补回归测试。
4. **阶段 4 —— 收紧门禁。** Codecov project 80% 改为阻断；UI 端点覆盖对 UI
   子集门禁。

## 已发现缺陷

| # | 模块 | 现象 | 状态 |
|---|---|---|---|
| 1 | Admin 认证 | `POST /api/admin/password` 改密后不吊销其它管理端会话（用户改密会） | 已修（`store.DeleteAdminSessionsExcept`） |
| 2 | 包注册表 | `GET /api/packages/{owner}`（全部类型）恒返回 `[]`：`ListPackages` 过滤 `type = ''` | 已修 |
| 3 | Composer | `GET /api/packages/composer/{owner}/p2/{vendor}/{name}` 的 `packages` 返回嵌套**数组**而非以 `vendor/name` 为键的对象，Composer 2 客户端无法解析 | 已修 |
| 4 | UI 测试 | `repos.spec.ts` 在仓库列表卡片上找 clone 命令，但该命令已移到仓库页头部 SSH 下拉 | 已修（测试） |
| 5 | UI 测试 | `explore.spec.ts` 用旧搜索占位符（`…users…` → `…users, code…`） | 已修（测试） |
| 6 | CI 抖动 | `test_pipeline_host.py::test_host_cache_reuse` 全量跑时偶发 `429 too_many_runs`，疑似进行中运行计数竞态 | 待查 |
| 7 | Runner | 管理端全局注册 token 返回 `scope: ""`，而 user/org 返回 `user:x` / `org:x`；有歧义但与内部 `"" = 全局` 约定一致 | 记录 |
