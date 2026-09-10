# Projects (Kanban Boards)

Each repository can have multiple kanban-style **Projects**. A project is a board made of:

- **Columns** — the vertical stages of work (a new project starts with `To Do / In Progress / Done`)
- **Swimlanes** — horizontal groupings across all columns (e.g. by priority, epic or team); every project starts with a `Default` swimlane, and cards can also live in a special *ungrouped* row (no swimlane)
- **Cards** — either linked to an issue of the same repository (shows `#number`, title and open/closed state) or a plain text note

Cards can be dragged between columns and across swimlanes in the web UI (repo → **Projects** tab).

## Web UI

1. Open a repository → **Projects** tab
2. Create a project (name + description)
3. Click the project to open the board; drag cards between columns/swimlanes, use the `+` in a column to add a card (`#12` links issue 12, otherwise the text becomes a note)
4. Add / delete columns and swimlanes from the board toolbar

## API

All endpoints require authentication (session cookie or PAT) and write access to the repository for mutations. Base path: `/api/users/{owner}/repos/{name}/projects`.

| Method | Path | Description |
|---|---|---|
| GET | `/` | List projects (with `card_count`) |
| POST | `/` | Create project `{name, description}` — seeds default columns + `Default` swimlane |
| PATCH | `/{id}` | Rename / edit description |
| DELETE | `/{id}` | Delete project (cascades columns, swimlanes, cards) |
| GET | `/{id}/board` | Board snapshot: `{project, columns, swimlanes, cards}` |
| GET/POST | `/{id}/columns` | List / create column |
| PATCH/DELETE | `/{id}/columns/{cid}` | Rename, reorder (`position`) / delete (cards deleted too) |
| GET/POST | `/{id}/swimlanes` | List / create swimlane |
| PATCH/DELETE | `/{id}/swimlanes/{lid}` | Rename, reorder / delete (cards fall back to ungrouped) |
| GET/POST | `/{id}/cards` | List (with issue title/state) / create card `{column_id, swimlane_id?, issue_number?, note?}` |
| PATCH | `/{id}/cards/{card}` | Move `{column_id, swimlane_id, position}` and/or edit `{note}` |
| DELETE | `/{id}/cards/{card}` | Delete card |

Example:

```bash
curl -X POST -H "Authorization: Bearer <PAT>" \
  -d '{"name":"sprint-1","description":"September sprint"}' \
  http://your-host:8080/api/users/alice/repos/demo/projects
```
