# BYOK Copilot (MVP)

A repo-scoped AI copilot feature. Each **session** runs in its own isolated Docker
container. The LLM is **bring your own key (BYOK)**: gitdash never ships or proxies
an LLM API key — it injects the Anthropic-compatible key you configure into the
container as environment variables.

> MVP scope: sessions are long-lived containers with `docker logs` for output.
> Interactive chat / commit-push integration are future work.

## Concepts

- **BYOK key** — a user-level Anthropic-compatible API key (`provider` is fixed to
  `anthropic`; `base_url` allows Anthropic-compatible endpoints/proxies). The key is
  stored in the DB and **never returned** by any read API; the UI only shows
  `key_set = true`.
- **Copilot session** — a per-repo instance bound to one BYOK key. Each session is
  one Docker container (`gitdash-copilot-<id>`) with a clone of the repo mounted at
  `/workspace`.

## How it works

1. Configure one or more BYOK keys in **Profile → BYOK**.
2. On a repo's **Copilot** tab, create a session: pick a BYOK key, optionally set
   `image`, `prompt` (task) and `command`.
3. gitdash clones the repo into `<data>/copilots/{owner}/{repo}/ws-<id>` and runs:

   ```text
   docker run -d --name gitdash-copilot-<id> \
     --workdir /workspace -v <workspace>:/workspace \
     --network <GITDASH_COPILOT_NETWORK|bridge> \
     --memory 512m --cpus 1.0 --pids-limit 128 \
     --security-opt no-new-privileges --cap-drop ALL \
     -e GITDASH_COPILOT_ID=<id> -e GITDASH_OWNER=<owner> \
     -e GITDASH_REPO=<repo> -e GITDASH_REF=<branch> -e GITDASH_TASK=<prompt> \
     -e LLM_APIKEY=<key> [-e LLM_BASE_URL=<url>] [-e LLM_MODEL=<model>] \
     <image> [sh -ec "<command>"]
   ```

   If `command` is omitted, the image's own entrypoint runs and is expected to read
   the `GITDASH_*` / `LLM_*` environment variables.

4. `Start` / `Stop` / `Delete` manage the container. Logs come from `docker logs`.

## Requirements

- Docker CLI + daemon on the server.
- The agent `image` must be available locally or pullable. Provide `image` per
  session, or set a server default:

  ```bash
  GITDASH_COPILOT_IMAGE=your/agent:latest
  ```

## Environment variables

| Variable | Description |
| --- | --- |
| `GITDASH_COPILOT_IMAGE` | Default image when a session does not specify one |
| `GITDASH_COPILOT_NETWORK` | Container network (default `bridge`; `none` disables LLM access) |

Injected into every container: `GITDASH_COPILOT_ID`, `GITDASH_OWNER`, `GITDASH_REPO`,
`GITDASH_REF`, `GITDASH_TASK`, `LLM_APIKEY`, and optionally
`LLM_BASE_URL` / `LLM_MODEL`.

## API

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/me/byok` | List my BYOK keys (no plaintext key) |
| POST | `/api/me/byok` | Create a BYOK key |
| PUT | `/api/me/byok/{id}` | Update a BYOK key (empty `api_key` keeps it) |
| DELETE | `/api/me/byok/{id}` | Delete a BYOK key |
| GET | `/api/users/{owner}/repos/{name}/copilots` | List sessions |
| POST | `/api/users/{owner}/repos/{name}/copilots` | Create + start a session |
| GET | `/api/users/{owner}/repos/{name}/copilots/{id}` | Session detail + logs |
| POST | `/api/users/{owner}/repos/{name}/copilots/{id}/start` | Start a session |
| POST | `/api/users/{owner}/repos/{name}/copilots/{id}/stop` | Stop a session |
| DELETE | `/api/users/{owner}/repos/{name}/copilots/{id}` | Delete session + workspace |
