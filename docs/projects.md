# Projects (Kanban Boards)

Each repository can have multiple kanban-style **Projects**. A project is a board made of:

- **Columns** — the vertical stages of work (a new project starts with `To Do / In Progress / Done`)
- **Swimlanes** — horizontal groupings across all columns (e.g. by priority, epic or team); every project starts with a `Default` swimlane, and cards can also live in a special *ungrouped* row (no swimlane)
- **Cards** — either linked to an issue of the same repository (shows `#number`, title and open/closed state) or a plain text note

Cards can be dragged between columns and across swimlanes in the web UI (repo → **Projects** tab).
The same data can be viewed as a **table (List)** or a **timeline (Gantt)**; cards carry optional
`start_date` / `due_date` (YYYY-MM-DD) for scheduling.

## Views

A project toolbar switches between three views over the same cards:

- **Board** — the kanban grid (columns × swimlanes) with drag & drop
- **List** — a table of every card with its column, swimlane, start and due date (overdue dates in red)
- **Gantt** — a timeline of cards that have at least one date, grouped by swimlane, with a *today*
  marker and an “Unscheduled” section for cards without dates

Dates are edited from the card dialog (pencil icon on a card, or a List/Gantt row).

## Web UI

1. Open a repository → **Projects** tab
2. Create a project (name + description)
3. Click the project to open it; drag cards between columns/swimlanes, use the `+` in a column to add a card (`#12` links issue 12, otherwise the text becomes a note)
4. Use the toolbar to switch **Board / List / Gantt**, and to add / delete columns and swimlanes
5. Click a card's pencil icon to edit its note and start / due dates

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
| GET/POST | `/{id}/cards` | List (with issue title/state and dates) / create card `{column_id, swimlane_id?, issue_number?, note?, start_date?, due_date?}` |
| PATCH | `/{id}/cards/{card}` | Move `{column_id, swimlane_id, position}` and/or edit `{note, start_date, due_date}` (empty string clears a date) |
| DELETE | `/{id}/cards/{card}` | Delete card |

Example:

```bash
curl -X POST -H "Authorization: Bearer <PAT>" \
  -d '{"name":"sprint-1","description":"September sprint"}' \
  http://your-host:8080/api/users/alice/repos/demo/projects
```
