# Runners Guide (Self-hosted CI Agents)

`gitdash-runner` agents let you run repository pipelines on your own machines.
Each agent dials out to the gitdash server over a WebSocket, picks up jobs, runs
`.gitdash.yml` pipelines in local Docker containers, and streams logs/status back. Start it with `gitdash-runner run -exec host` to also allow pipelines that omit `image` to run directly on the runner host via `sh` (no container sandbox — opt-in).

See the full guide (Chinese): [runners.zh-CN.md](./runners.zh-CN.md)

## Quick start

1. **Server**: start with `GITDASH_QUEUE=redis` (the runner hub requires Redis).
2. **Issue a one-time registration token** (valid 10 min):
   - personal scope: *Profile → Runners → Issue registration token*
   - org scope (org owner): `POST /api/runners/registration-token` `{"scope":"org","org":"<org>"}`
   - global (site admin): `POST /api/admin/runners/registration-token`
3. **Register & start** the agent:

   ```bash
   gitdash-runner register \
     -server http://gitdash.example:8080 \
     -name build-01 -labels docker,go1.22 -token <TOKEN>
   gitdash-runner run
   ```

4. **Target the agent** in `.gitdash.yml`:

   ```yaml
   image: alpine:3.19
   runs-on: [docker]   # must be a subset of the agent's labels
   steps:
     - name: build
       run: echo build
   ```

Without `runs-on`, pipelines run on the server's local Docker as before (backward
compatible). If no online agent matches the labels, the run fails immediately;
if an agent goes offline mid-run, its run is marked `failed (runner went offline)`
— no silent re-execution. Remote runs can be cancelled from the run detail view.

## Parallel sub-steps and conditional steps (.gitdash.yml)

Anywhere a step lives, `parallel:` swaps sequential execution for concurrent
sub-steps (any failure fails the group after all sub-steps finish). `when:` is a
[CEL](https://github.com/google/cel-go) boolean expression checked at run time —
unsatisfied steps (or whole groups) are skipped and still count as completed:

```yaml
image: alpine:3.19
steps:
  - name: checks            # parallel sub-steps run concurrently, logs interleaved
    parallel:
      - name: lint-js
        run: npm lint
      - name: lint-go
        run: go vet ./...
  - name: deploy            # sub-steps may each carry when
    when: branch == "main" && event == "push"
    parallel:
      - name: deploy-staging
        run: p deploy staging
      - name: deploy-prod
        when: tag.matches(r'v\d+')
        run: p deploy prod
```

Variables available to `when` (compare with `==` / `!=`, combine with `&&` / `||`,
`!` and parentheses, string functions like `startsWith` / `matches`):

| var     | value                                                       |
|---------|-------------------------------------------------------------|
| `branch`| short branch name (`""` for tags/refs)                      |
| `tag`   | short tag name (`""` unless a tag push)                     |
| `ref`   | full ref (`refs/heads/main`, `refs/tags/v1`)                |
| `event` | `push` or `manual` (manual = triggered from the UI/API)     |

Note: tag pushes now trigger the pipeline too (`when: tag != ""` enables or
gates tag-only jobs). Progress counts each sub-step as one unit.

## Host execution mode (no Docker)

Pipelines that **omit `image`** run each step directly in a host `sh -ec` — useful
when the machine has no Docker. The workspace snapshot is unpacked to a temp
directory and `CI=1`, `GITDASH_REPO`/`GITDASH_REF`/`GITDASH_SHA` plus the repo-level
environment variables and the DSL `env` entries are injected (`volumes` is ignored).
On a key collision, the DSL `env` wins over repo-level variables.

Enable it explicitly (off by default, independently on each side):

```bash
# server: allow builtin execution of image-less pipelines
GITDASH_PIPELINE_EXEC=host gitdash serve

# agent: allow this runner to execute image-less pipelines
gitdash-runner run -exec host
```

```yaml
# .gitdash.yml — no image, everything else unchanged
env:
  - GREETING=hello
steps:
  - name: build
    run: go build ./...
```

- Not enabled → the run fails with `host execution is disabled (…)` explaining how
  to enable it; pipelines with an `image` are unaffected (still Docker-sandboxed).
- **No container sandbox**: no network isolation, resource limits, or capability
  drops — `env` and `timeout` still apply, but step code has full access to the
  host filesystem/network. Enable only on fully trusted machines, never as root.
- Requires POSIX `sh` (Linux/macOS; not supported for Windows agents).
- Black-box coverage: `tests/test_pipeline_host.py`.

## Security model

- Agents execute arbitrary repository code on their host — deploy them on trusted
  CI machines only, and never run them as root on data-bearing hosts.
- Scoping: a personal runner only ever receives that user's repos; an org runner
  only that org's repos; global runners are admin-issued.
- Workspaces are delivered as a server-side `git archive` snapshot over the
  agent's own connection — no git credentials are ever sent to the agent.
- The agent applies the same sandbox as the builtin executor: no network by
  default (`GITDASH_PIPELINE_NETWORK` to override), 512MB memory / 1 CPU / 128
  pids, `no-new-privileges`, all capabilities dropped, `/var/run/docker.sock`
  mounts rejected. **Host mode bypasses the entire sandbox** (see above) — enable
  it only on fully trusted machines.
