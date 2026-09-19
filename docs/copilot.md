# BYOK Copilot

A repo-scoped AI copilot. Each **session** is a checkout of the repository driven by a
standalone **agent runtime** over a small HTTP+SSE protocol
([`copilot-api/1`](../deps/agent/docs/copilot-api.md)). The LLM is **bring your own key
(BYOK)**: gitdash never ships or proxies an LLM key — it passes the Anthropic-compatible
key you configure to the agent process via environment variables.

> gitdash and the agent are decoupled: gitdash only speaks `copilot-api/1`. The agent can
> be replaced, and its binary path / endpoint are configurable.

## Concepts

- **BYOK key** — a user-level LLM key with a `provider` preset (`anthropic` / `compatible`
  / `ollama`, see below); `base_url` points at an Anthropic-compatible endpoint (official,
  a gateway, or local Ollama). The key is stored in the DB and **never returned** by any
  read API; the UI only shows `key_set = true`.
- **Copilot session** — a per-repo chat bound to one BYOK key and one workspace
  (`<data>/copilots/{owner}/{repo}/ws-<id>`) checked out on branch `copilot/session-<id>`;
  it can be linked to an issue (`issue_number`) and auto-opens a PR when done.
- **Agent runtime** — the `agent` binary (built from the [`deps/agent`](../deps/agent)
  submodule) started per session as `agent -C <workspace> -api-addr 127.0.0.1:<port>`.
  It exposes a chat API + web UI and runs the think-act-observe loop with `shell` and
  `read`/`write`/`edit` tools confined to the workspace.

## Provider presets

The agent's LLM client only speaks the **Anthropic Messages-compatible protocol**
(`POST {base_url}/v1/messages`), so every preset must point at an Anthropic-compatible
endpoint:

| provider | default base_url | key required | notes |
| --- | --- | --- | --- |
| `anthropic` | `https://api.anthropic.com` | yes | official Anthropic endpoint |
| `compatible` | none (required) | yes | any Anthropic-compatible gateway (LiteLLM / one-api / custom) that can front OpenAI / vLLM / Ollama |
| `ollama` | `http://127.0.0.1:11434` | no | local Ollama (needs its Anthropic-compatible `/v1/messages`); a placeholder key is stored when none is given |

In **Profile → BYOK** the **Test connection** button sends one minimal request to verify
provider / base_url / model / key (`POST /api/me/byok/test`). Endpoints are SSRF-guarded:
private/loopback/metadata addresses are rejected unless the host is in
`GITDASH_LLM_ALLOW_HOSTS` or `GITDASH_SSRF_ALLOW_PRIVATE=1` is set.

## How it works

1. Configure one or more BYOK keys in **Profile → BYOK**.
2. On a repo's **Copilot** tab, create a session: pick a BYOK key and optionally add
   extra instructions.
3. Open the session: the browser opens a WebSocket to gitdash, which forwards messages to
   the agent's `POST /api/chat` and streams `delta` / `tool_*` events back. History is
   replayed from `GET /api/messages`.
4. **Closed loop**: after each turn ends (`done`), gitdash runs
   `git add -A && git commit && git push origin HEAD:refs/heads/copilot/session-<id>`
   in the workspace. The branch shows up in gitdash, ready for review / a PR.
   Before each turn gitdash fetches and (when clean) rebases the session branch onto the
   default branch, so upstream commits flow back into the agent's view.
5. **Issue → PR**: when the session is linked to an issue, a successful push makes
   gitdash open a PR automatically (title `fix: <issue title>`, body `Closes #N`, source =
   session branch, target = default branch) and record `pr_number` on the session
   (idempotent: an existing open PR from the same branch is reused). Start it from the
   issue page (**Fix with Copilot**) or `gitdash-cli copilot fix`.

## Requirements

- The `agent` binary. It is built from the `deps/agent` submodule and shipped alongside
  gitdash in releases (the installer installs it too). For local development, build it
  with `task agent` (or `go build -o agent ./cmd/agent` in `deps/agent`).
- Set `GITDASH_COPILOT_AGENT_BIN` if the binary is not next to gitdash / in `PATH`.
- To use an already-running agent instead of spawning one per session, set
  `GITDASH_COPILOT_AGENT_URL` (e.g. `http://127.0.0.1:8790`). This is a single shared
  runtime, so it is mainly useful for testing.
- No Docker required.

## Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `GITDASH_COPILOT_AGENT_BIN` | `agent` next to gitdash, else `PATH` | Agent runtime binary |
| `GITDASH_COPILOT_AGENT_URL` | empty | External agent base URL (skip spawning) |
| `GITDASH_LLM_ALLOW_HOSTS` | empty | Comma-separated `host` or `host:port` allowed to bypass the LLM SSRF guard (e.g. a private gateway or local Ollama). Scoped to BYOK/copilot only; webhook/import SSRF protection is unaffected. `GITDASH_SSRF_ALLOW_PRIVATE=1` still allows all private addresses. |

Injected into every agent process: `LLM_API_KEY_FILE` (a short-lived 0600 file so
the key is not exposed via the process environment), `LLM_BASE_URL`, `LLM_MODEL`,
`LLM_PROVIDER`, `LLM_AUTH_STYLE` (all from the selected BYOK preset; `ollama` uses a
placeholder key when none is given). The agent also accepts `LLM_API_KEY` directly.

## CLI

```bash
gitdash-cli copilot fix <owner/repo> <issue-number> [--byok <name|id>] [--instructions <text>]
gitdash-cli copilot fix <owner/repo> <issue-number> --detach   # create the session only
gitdash-cli copilot list <owner/repo>
gitdash-cli copilot run <owner/repo> <session-id> --text "..."
```

`copilot fix` is the headless equivalent of the web UI's **Fix with Copilot**: it creates
the issue-linked session, drives the agent over WebSocket, and prints the auto-opened PR
when the turn completes. `issue fix` is an alias.

## API

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/me/byok` | List my BYOK keys (no plaintext key) |
| POST | `/api/me/byok` | Create a BYOK key (`ollama` may omit `api_key`) |
| POST | `/api/me/byok/test` | Test the connection (`api_key` may be blank and falls back to the key at `id`) |
| PUT | `/api/me/byok/{id}` | Update a BYOK key (empty `api_key` keeps it) |
| DELETE | `/api/me/byok/{id}` | Delete a BYOK key |
| GET | `/api/users/{owner}/repos/{name}/copilots` | List sessions |
| POST | `/api/users/{owner}/repos/{name}/copilots` | Create a session (optional `issue_number` links an issue) |
| GET | `/api/users/{owner}/repos/{name}/copilots/{id}` | Session detail |
| GET | `/api/users/{owner}/repos/{name}/copilots/{id}/messages` | Conversation history |
| GET | `/api/users/{owner}/repos/{name}/copilots/{id}/chat` | Bidirectional chat (WebSocket) |
| POST | `/api/users/{owner}/repos/{name}/copilots/{id}/stop` | Cancel the running turn |
| DELETE | `/api/users/{owner}/repos/{name}/copilots/{id}` | Delete session + workspace |

### Chat WebSocket

Client → server: `{"type":"user","text":"…"}` / `{"type":"cancel"}`.

Server → client: `{"type":"history","messages":[…]}`, `{"type":"status","status":"idle|running"}`,
`{"type":"delta","text":"…"}`, `{"type":"tool_start","name","input"}`,
`{"type":"tool_end","name","result","is_error"}`, `{"type":"done","text"}`,
`{"type":"error","error"}`.
