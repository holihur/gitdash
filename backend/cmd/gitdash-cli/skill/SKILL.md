---
name: gitdash-cli
description: Use the `gitdash-cli` command to operate a self-hosted gitdash instance — list or create repositories, file/list issues, and open/list pull requests. Use when the user mentions gitdash, gitdash-cli, or asks to manage gitdash repos, issues, or pull requests.
---

# gitdash-cli

`gitdash-cli` is the command-line client for **gitdash** (a self-hosted Git service).
It is to gitdash what `gh` is to GitHub: use it to work with repositories, issues and
pull requests from the terminal. Prefer `gitdash-cli` over raw HTTP calls when a task
involves a gitdash server.

## When to use this skill

Trigger when the user asks to:

- list or create repositories on a gitdash instance
- file, list, or look up issues
- open, list, or inspect pull requests
- check which account is authenticated against gitdash

## Authentication

Authenticate once (interactive):

```bash
gitdash-cli login                       # browser OAuth 2.0 device flow (recommended)
gitdash-cli login --method pat          # paste a personal access token (PAT)
gitdash-cli login --host http://localhost:8080
gitdash-cli me                          # verify
gitdash-cli logout                      # remove stored credentials
```

Credentials are stored in `~/.config/gitdash/config.json` (mode 0600).

For non-interactive or CI use, skip `login` and provide credentials via environment
variables (they override the config file):

```bash
export GITDASH_HOST=http://localhost:8080
export GITDASH_TOKEN=<personal-access-token>
```

`--host` / `--token` flags work on any command and override both env and config.

## Conventions

- Repositories are addressed as `owner/repo` (e.g. `alice/demo`).
- Global flags may appear anywhere: `--host <url>`, `--token <t>`, `--json`.
- Use `--json` to get raw JSON suitable for parsing with `jq`:

  ```bash
  gitdash-cli --json repo list | jq '.[].name'
  ```

## Commands

| Command | Description |
| --- | --- |
| `gitdash-cli me` | Show the authenticated user |
| `gitdash-cli repo list` | List your repositories |
| `gitdash-cli repo create [--private=false] [--description D] <name>` | Create a repository |
| `gitdash-cli issue list <owner/repo>` | List issues |
| `gitdash-cli issue create <owner/repo> --title T [--body B]` | Create an issue |
| `gitdash-cli pr list <owner/repo>` | List pull requests |
| `gitdash-cli pr create <owner/repo> --title T --head H --base B [--body B]` | Open a pull request |
| `gitdash-cli copilot list <owner/repo>` | List AI copilot sessions |
| `gitdash-cli copilot create <owner/repo> [--byok NAME] [--issue N] [--prompt P]` | Create a copilot session |
| `gitdash-cli copilot run <owner/repo> <session-id> [--text M]` | Drive a session and stream the agent's work |
| `gitdash-cli copilot fix <owner/repo> <issue-number> [--byok NAME] [--instructions P] [--detach]` | Have the agent fix an issue; auto-opens a PR |
| `gitdash-cli issue fix <owner/repo> <issue-number> [--detach]` | Alias of `copilot fix` |
| `gitdash-cli skill show` | Print this skill document |
| `gitdash-cli skill install` | Install this skill for Claude Code / opencode / pi |

Note: flags are parsed Go-style, so place them **before** the positional argument
(`repo create --private demo`, not `repo create demo --private`).

## Examples

```bash
# who am I
gitdash-cli me

# create a private repo and list it
gitdash-cli repo create --private my-service
gitdash-cli repo list

# file an issue
gitdash-cli issue create alice/my-service \
  --title "Login fails on Safari" \
  --body "Steps to reproduce: ..."

# open a pull request from a feature branch
gitdash-cli pr create alice/my-service \
  --title "Fix Safari login" \
  --head fix/safari-login \
  --base main

# list open pull requests
gitdash-cli pr list alice/my-service
```

## AI copilot (issue → PR)

`gitdash-cli copilot fix` is the headless equivalent of clicking "Fix with Copilot" on an
issue: it creates a session linked to the issue, runs the agent (which reads, edits and
runs commands in a checkout of the repo), and — once the agent pushes — gitdash opens a
pull request whose body closes the issue. Use `--detach` to only create the session.

```bash
# let the agent fix issue #14 and open a PR (streams progress to stderr)
gitdash-cli copilot fix alice/my-service 14

# only create the session; run it later / from the web UI
gitdash-cli copilot fix alice/my-service 14 --detach

# with a specific BYOK key and extra instructions
gitdash-cli copilot fix alice/my-service 14 --byok work --instructions "add a regression test"

# list sessions and their linked issue / PR
gitdash-cli copilot list alice/my-service

# drive an existing session with a free-form message
gitdash-cli copilot run alice/my-service 3 --text "also update the changelog"
```

If more than one BYOK key is configured, pass `--byok <name|id>` (list them in the web UI
under Profile → BYOK). The command requires the server to have the copilot agent runtime
available; otherwise it fails with `agent runtime unavailable`.

## Errors

- `401` / "not logged in" → run `gitdash-cli login` or set `GITDASH_TOKEN`.
- "no host configured" → set `GITDASH_HOST` or pass `--host`.
- "invalid repository" → repository arguments must be `owner/repo`.
