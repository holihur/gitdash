# 项目看板（Projects）

每个仓库可以创建多个看板式 **项目（Project）**。一个项目由三部分组成：

- **列（Columns）** — 纵向的工作阶段（新建项目自动带 `To Do / In Progress / Done` 三列）
- **泳道（Swimlanes）** — 横跨所有列的横向分组（例如按优先级 / epic / 团队）；每个项目自带一个 `Default` 泳道，卡片也可以放在特殊的 *未分组* 行（不隶属任何泳道）
- **卡片（Cards）** — 关联本仓库 issue 的卡片（显示 `#编号`、标题与 open/closed 状态），或纯文本便签

在网页端（仓库 → **项目** 标签页）可以拖拽卡片在列之间、泳道之间移动。

## 网页端使用

1. 打开仓库 → **项目** 标签页
2. 创建项目（名称 + 描述）
3. 点击项目进入看板：拖拽卡片跨列 / 跨泳道，点列里的 `+` 添加卡片（输入 `#12` 关联 12 号 issue，否则文本作为便签）
4. 看板工具栏可添加 / 删除列和泳道

## API

所有接口需要登录（会话 cookie 或 PAT），写操作还需仓库写权限。基础路径：`/api/users/{owner}/repos/{name}/projects`。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/` | 列出项目（含 `card_count`） |
| POST | `/` | 创建项目 `{name, description}` — 自动初始化默认列 + `Default` 泳道 |
| PATCH | `/{id}` | 重命名 / 修改描述 |
| DELETE | `/{id}` | 删除项目（级联删除列、泳道、卡片） |
| GET | `/{id}/board` | 看板快照：`{project, columns, swimlanes, cards}` |
| GET/POST | `/{id}/columns` | 列出 / 创建列 |
| PATCH/DELETE | `/{id}/columns/{cid}` | 重命名、排序（`position`）/ 删除（列内卡片一并删除） |
| GET/POST | `/{id}/swimlanes` | 列出 / 创建泳道 |
| PATCH/DELETE | `/{id}/swimlanes/{lid}` | 重命名、排序 / 删除（卡片退回未分组） |
| GET/POST | `/{id}/cards` | 列出（附带 issue 标题与状态）/ 创建卡片 `{column_id, swimlane_id?, issue_number?, note?}` |
| PATCH | `/{id}/cards/{card}` | 移动 `{column_id, swimlane_id, position}` 和/或编辑 `{note}` |
| DELETE | `/{id}/cards/{card}` | 删除卡片 |

示例：

```bash
curl -X POST -H "Authorization: Bearer <PAT>" \
  -d '{"name":"sprint-1","description":"九月冲刺"}' \
  http://your-host:8080/api/users/alice/repos/demo/projects
```
