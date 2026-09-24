# 交接文档：系统徽章 + 仓库角色权限 + 组织团队

> 面向接手「徽章 / 仓库权限」相关模块的同学。
> 技术栈：Go（Gin/标准库 mux + GORM，SQLite/PostgreSQL）+ React/TS（Vite + Vitest）。
> 本文件为**已跟踪**的工作文档；仓库根目录的 `HANDOVER.md` 是历史遗留且被 `.gitignore` 忽略，不要混用。

---

## 0. 本轮范围与提交

聚焦两个模块：

1. **系统徽章（badges）**：仅管理后台定义/授予，可发用户 / 仓库 / 组织，目标最多挂 3 个。
2. **仓库角色权限（access/roles）**：`read/triage/write/maintain/admin/owner` 六档；组织成员默认角色可配置；组织团队批量授权；权限审计。

本轮提交（`git log --oneline -6`）：

```
2ac3102 feat(access): 组织团队授权 + 权限审计 + 评审/合并能力 + 无权限置灰
f014d91 chore(repos): 清理死代码并让删除 release 按 write 校验
7a56eb6 fix(repos): 仓库详情返回完整角色 + 前端按能力放行写操作
3522ea9 feat(orgs): 组织成员默认角色可配置
c7f2272 feat(repos): 仓库角色权限系统（read/triage/write/maintain/admin）
d049f90 feat(badges): 系统徽章 + 用户/组织主页 Hero 布局
```

> 其后又补了一个 `RepoRole` 修复（下一提交）：组织成员若在某个仓库是**更高角色的协作者/团队成员**，此前会被组织默认角色“吃掉”，现已改为取最高。

---

## 1. 快速开始

```bash
# 后端：编译 + 单测（含 store/api）
cd backend && go build ./... && go test ./...

# 后端：跑主程序（本地）
go run .

# 前端
cd frontend && pnpm install && pnpm dev      # 开发
pnpm test            # Vitest（jsdom）
pnpm exec tsc --noEmit && pnpm exec eslint src --max-warnings=0

# 黑盒 API 测试（pytest，需先构建被测二进制）
cd backend && go build -o /tmp/gitdash-server-pytest .
cd tests && GITDASH_BIN=/tmp/gitdash-server-pytest .venv/bin/python -m pytest -q

# 或使用 Taskfile
task test:go ; task test:frontend ; task test:api
```

**注意：CI 有“端点覆盖率门禁”** —— `scripts/route-coverage.py --min 100`，即**每个 API 路由都必须被黑盒测试命中至少一次**。新增路由时务必同步加 pytest 用例。

---

## 2. 数据模型（本轮新增）

`backend/internal/store/models.go`（AutoMigrate 注册在 `migrate.go`）：

### 徽章
| 表 | 说明 |
|---|---|
| `badges` | 徽章定义：`slug`(unique)、`label`、`description`、`created_at` |
| `badge_images` | 图标（单独表，避免列表加载大字段）：`badge_id`(PK)、`content_type`、`data`、`updated_at` |
| `badge_grants` | 授予：`badge_id + kind(user/repo/org) + owner + repo` 唯一 |
| `badge_displays` | 挂出：`kind + owner + repo + badge_id` 唯一，`position` 排序，最多 3 个 |

### 权限 / 团队
| 表 / 字段 | 说明 |
|---|---|
| `orgs.default_member_role` | 组织成员在组织仓库中的默认角色（默认 `write`） |
| `org_teams` | 组织团队：`org + name` 唯一 |
| `org_team_members` | 团队成员：`team_id + username` |
| `repo_team_grants` | 团队授权：`owner + repo + team_id`，`permission` 为五档角色 |

> `repo_collabs.permission` 由原来的 `read/write` 扩展为 `read/triage/write/maintain/admin`（历史值不变，仍然有效）。

---

## 3. 权限模型（核心）

### 3.1 角色等级（`backend/internal/store/roles.go`）

`read(1) < triage(2) < write(3) < maintain(4) < admin(5) < owner(6)`

| 角色 | 能力 |
|---|---|
| `read` | 浏览代码 / issue / PR、clone |
| `triage` | read + 议题/PR 管理（标签、里程碑、指派、关闭/重开、评论、review）；**不能推代码** |
| `write` | triage + 推代码 / 文件操作 / release / pages / 触发流水线 / 合并 PR |
| `maintain` | write + 仓库设置（webhook、deploy key、分支保护、环境变量/密钥、mirror、gc、pages 配置、pipelines 开关） |
| `admin` | maintain + 协作者、可见性、团队授权、权限审计 |
| `owner` | 仓库所有者（用户本人 / 组织 owner），最高；删除仓库等专属 |

### 3.2 `RepoRole` 解析优先级（`store/repos.go`）

```
owner(本人 / 组织 owner)
  > max( 组织成员默认角色 , 协作者 permission , 团队授权 permission )
  > 公开仓库 read（任何访问者，含匿名）
  > ""（无权限）
```

- 组织默认角色存在 `orgs.default_member_role`。
- 协作者与团队取最高：`max(collab, team)`。
- 相关函数：`RoleRank`、`RoleAtLeast`、`ValidCollabRole`、`RepoRole`、`CanDo`、`CanRead`、`CanWrite`。

### 3.3 后端放行

`backend/internal/api/api.go`：

```go
// 统一入口；min ∈ read/triage/write/maintain/admin/owner
func (a *API) requireRole(w, r, min string) (owner, name string, ok bool)

// 兼容旧签名：write=false→read，write=true→write
func (a *API) requireAccess(w, r, write bool) (string, string, bool)
```

- `read` 级别沿用可见性规则（anonymous > public > private + CanRead）；更高等级要求 `RepoRole >= min`。
- 本轮把约 87 个仓库级写/管理接口按矩阵从 `requireAccess(true)` / `requireOwner` 改成 `requireRole(w, r, "<role>")`；`requireOwner` 已删除。
- **新增仓库级写接口时，请用 `requireRole` 并显式选等级**，不要再写 `requireAccess(true)`。

组织级设置（团队、成员、默认角色、可见性）用 `OrgRole(org, me) == "owner"` 校验。

### 3.4 前端能力判断（`frontend/src/lib/repo-role.ts`）

```ts
roleAtLeast(role, min) / canRead / canTriage / canWrite / canReview / canMaintain / canAdmin / isRepoOwner
COLLAB_ROLES = ["read","triage","write","maintain","admin"]
```

- `repo.role` 由后端返回**完整角色**（`getRepo` 已修复，不再压成 owner/write/read）。
- PR：`canReview`(=triage) 控制评审表单，`canMerge`(=write) 控制合并/草稿/自动合并。
- 设置卡片：对 admin/owner 专属项权限不足时**置灰**（`Gated` 组件）而非隐藏。

---

## 4. 徽章系统

### 4.1 工作方式
- **仅管理后台**（`adminAuth`）可创建徽章、上传图标、授予/撤销；任意徽章可发任意目标。
- **授予即默认挂出**（未满 3 个时追加），目标可隐藏/选择，最多 3 个（`store.MaxDisplayedBadges`）。
- 目标设置入口：个人 `Profile`、组织设置、仓库设置（`BadgeDisplayPicker`）。
- 授予会给目标写站内信（`system/badge_granted`）：用户→本人；组织→全体成员；仓库→owner。

### 4.2 展示与性能
- 公开展示位置：用户/组织主页（Hero 头部）、仓库页头部、仓库卡片、Explore 搜索结果。
- **批量拉取**：前端 `lib/api/badges.ts` 里 `requestBadges` 在同 tick 合并成一次 `POST /api/badges/batch`（按 kind 分组）；后端 `DisplayedBadgesBatch` 按 50 个目标分块 + 行值 `IN ((?,?))`，仅 3 类查询（displays / badges / images）。
- **缓存**：模块级 TTL 60s；`setDisplay` 成功后 `invalidateBadges(kind, owner, repo)`。
- 图标：内容嗅探校验，png/jpeg/gif/webp ≤1MB，响应 `Cache-Control: public, max-age=300`。

---

## 5. API 端点（本轮新增/变更）

### 徽章 · 管理端（`adminAuth`）
```
GET    /api/admin/badges
POST   /api/admin/badges                 # multipart: slug/label/description/image
PATCH  /api/admin/badges/{id}
DELETE /api/admin/badges/{id}
POST   /api/admin/badges/{id}/image      # multipart: image
GET    /api/admin/badges/{id}/grants
POST   /api/admin/badges/{id}/grants     # {kind, owner, repo}
DELETE /api/admin/badges/{id}/grants?kind=&owner=&repo=
```

### 徽章 · 公开 / 目标设置
```
GET  /api/badges?kind=&owner=&repo=      # 目标挂出的徽章（公开）
POST /api/badges/batch                   # {kind, targets:[{owner,repo}]} → {items}
GET  /api/badges/{id}/image
GET  /api/badges/owned?kind=&owner=&repo= # 已获得/已挂出（需写权限）
PUT  /api/badges/display                  # {kind,owner,repo,badge_ids}（需写权限）
```

### 组织团队 / 仓库团队授权 / 审计
```
GET    /api/orgs/{org}/teams
POST   /api/orgs/{org}/teams                       # {name}  （仅 owner）
DELETE /api/orgs/{org}/teams/{id}                  # 级联清理成员与授权
GET    /api/orgs/{org}/teams/{id}/members
POST   /api/orgs/{org}/teams/{id}/members          # {username}
DELETE /api/orgs/{org}/teams/{id}/members/{username}

GET    /api/users/{owner}/repos/{name}/team-grants
PUT    /api/users/{owner}/repos/{name}/team-grants/{teamId}   # {permission}（需 admin）
DELETE /api/users/{owner}/repos/{name}/team-grants/{teamId}
GET    /api/users/{owner}/repos/{name}/access      # 权限审计（需 admin）
```

### 组织
```
PATCH /api/orgs/{org}   # 新增 default_member_role（read/triage/write/maintain/admin，仅 owner）
```

---

## 6. 代码地图

### 后端
| 文件 | 作用 |
|---|---|
| `store/roles.go` | 角色常量、等级、`RoleAtLeast`、`ValidCollabRole` |
| `store/repos.go` | `RepoRole`、`CanDo/CanRead/CanWrite`、`accessibleReposSubquery`（含团队分支）、`getRepo` 角色回传 |
| `store/orgs.go` | `SetOrgInfo`（含默认角色）、`OrgDefaultMemberRole`、`OrgRole` |
| `store/org_teams.go` | 团队/成员/仓库授权 CRUD、`RepoTeamRole`、`AccessEntries` |
| `store/badges.go` | 徽章 CRUD、授予/展示、批量、图片、目标校验 |
| `store/models.go` / `migrate.go` | 表定义与 AutoMigrate |
| `api/api.go` | `requireRole` / `requireAccess`、路由注册 |
| `api/badges.go` | 徽章管理端 + 公开 + 展示 + 批量 |
| `api/teams.go` | 团队、团队授权、权限审计 |
| `api/orgs.go` | 组织资料 / 默认角色 / profile |
| `api/repos.go` | `getRepo` 返回完整角色 |

### 前端
| 文件 | 作用 |
|---|---|
| `lib/repo-role.ts` | 角色等级与能力函数 |
| `lib/api/badges.ts` | 徽章 API + 批处理器 + TTL 缓存 + `invalidateBadges` |
| `lib/api/orgs.ts` | 团队 API、`updateOrg(default_member_role)` |
| `lib/api/repos.ts` | `repoTeamGrants` / `grantRepoTeam` / `revokeRepoTeam` / `repoAccess` |
| `components/badge-chip.tsx` / `badge-strip.tsx` / `badge-display-picker.tsx` | 徽章展示与挂载 |
| `components/org-teams-card.tsx` | 组织设置里的团队管理 |
| `components/repo-team-access.tsx` | 仓库团队授权 + 权限总览 |
| `admin/sections/BadgesSection.tsx` | 管理后台徽章管理 |
| `pages/OrgSettings.tsx` | 组织设置（显示名/简介/封面/默认角色/团队/徽章） |
| `pages/repoview/settings-tab.tsx` | 仓库设置（含「你的权限」卡片 + `Gated` 置灰） |
| `pages/repos/RepoCard.tsx`、`pages/Explore.tsx`、`pages/UserPage.tsx`、`pages/OrgPage.tsx`、`repoview/repo-header.tsx` | 徽章展示与角色门控 |

---

## 7. 测试

- Go 单测：
  - `store/roles_test.go`（角色矩阵、组织默认角色、成员+协作者取最高）
  - `store/org_teams_test.go`（团队角色、`AccessEntries`、团队进入可访问列表）
  - `store/badges_test.go`（CRUD/图标/授予/展示/批量/级联）
- 黑盒 pytest：
  - `test_badges.py`、`test_teams.py`、`test_roles.py`、`test_orgs*.py`、`test_collabs.py`
  - 徽章 12 条 + 团队 10 条路由均被命中（满足覆盖率门禁）
- 前端 Vitest：
  - `repo-role.test.ts`、`collabs-dialog.test.tsx`、`org-settings.test.tsx`
  - `badge-*`、`repo-team-access.test.tsx`、`admin-badges.test.tsx`

跑全量：`task test`（go + frontend）、`task test:api`（pytest）。

---

## 8. 已知限制 / 后续 TODO

1. **团队授权仅限组织仓库**（`owner` 必须为组织名）；个人仓库只能加协作者。
2. `AccessEntries` 对同一用户不合并来源（既在组织又是协作者会出现多条）；如需「最终有效角色」视图可再聚合。
3. 组织成员**默认角色是组织级**，不能按团队/按仓库覆盖（团队授权可覆盖，但不是“按仓库改默认”）。
4. 评审/合并目前是固定映射（triage / write），没有做成可配置开关；如需“可评审不可合并”的显式开关，可在仓库设置加字段。
5. 徽章无删除图标接口（只能整枚删除）；无 emoji 兜底（无图用奖章图标）。
6. `getRepo` 返回完整角色后，前端仍有个别 `role === "read"` 之类的比较，如遇边界请改用 `repo-role.ts` 的能力函数。

---

## 9. 约定与坑

- **新增仓库级写接口**：一律 `a.requireRole(w, r, store.RoleXxx)`，按权限矩阵选等级；组织级用 `OrgRole == "owner"`。
- **角色字符串**：统一用 `store.RoleRead/RoleTriage/...` 常量，前端用 `repo-role.ts`。
- **改 API 注解后**：重新生成 swagger：`cd backend && go run github.com/swaggo/swag/cmd/swag init -g internal/api/doc.go -o internal/api/docs`（`docs.go` 会一起提交）。
- **新增路由必须加黑盒测试**，否则 CI 端点覆盖率门禁失败。
- 仓库根 `HANDOVER.md` 被 `.gitignore` 忽略；本文件为正式交接文档。
- 并发注意：本仓库曾出现其他自动化改动（如嵌入式 Bleve 代码搜索）混入工作区；提交前请先 `git status` / `git diff` 确认范围。
