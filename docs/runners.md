# Runners Guide (Self-hosted CI Agents)

`gitdash-runner` agents let you run repository pipelines on your own machines.
Each agent dials out to the gitdash server over a WebSocket, picks up jobs, runs
`.gitdash.yml` pipelines in local Docker containers, and streams logs/status back.

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
  mounts rejected.
