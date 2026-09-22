---
title: "Authorization gRPC plane"
weight: 1
summary: "AuthzService: the authorization plane a standalone SSH gateway calls over gRPC."
---

`AuthzService` is Gitdash's **authorization plane (control plane)**: it consolidates
the auth checks the SSH server performs in-process into a stable set of unary gRPC
methods. The default all-in-one deployment still uses the in-process path; with
`GITDASH_ROLE=ssh` the same binary runs as a **standalone SSH gateway** whose auth
checks all go through this service.

It only makes **decisions**, never touches repository data.

## Role & status

```
            (optional) standalone SSH gateway
   GITDASH_ROLE=ssh │  gRPC: AuthorizePublicKey / IsIPBanned / CanRead / CanWrite / BranchProtection
                    ▼
                ┌───────────────────┐
                │  gitdash process   │
                │  AuthzService      │  ← the authorization plane described here
                │  + store (source)  │
                └───────────────────┘
                        ▲
                        │ all-in-one: in-process direct calls (default, unchanged)
                ┌───────┴────────┐
                │ internal/sshserver │
                └────────────────┘
```

- **Default (all-in-one)**: `internal/sshserver` reads `store` directly via
  `authz.StoreAuthorizer`; behavior is unchanged and `GITDASH_ROLE=ssh` is not set.
- **Split deployment**: with `GITDASH_ROLE=ssh` (or `GITDASH_SSH_ONLY=1`) the same
  binary runs as an **SSH-gateway-only** process — it opens no database and starts no
  HTTP API; all authz goes through the gRPC plane.
- The SSH side depends only on the `authz.Authorizer` abstraction, so local (store)
  and remote (gRPC) are interchangeable.

## Deployment topologies

### Topology A: single machine (all-in-one, default)

HTTP API and SSH share one process; SSH uses `StoreAuthorizer` to read the DB
directly. Leaving `GITDASH_ROLE` unset selects this topology (unchanged behavior).

```
  browser / git CLI
     │ HTTP :8080        │ SSH :2222
     ▼                   ▼
  ┌──────────────────────────────────────────┐
  │              gitdash process              │
  │  HTTP API  +  built-in SSH (StoreAuthorizer)
  │  store (SQLite / PostgreSQL) = sole authority
  └──────────────────────────────────────────┘
     │
  GITDASH_DATA/repos (local disk)
```

### Topology B: split (API + SSH gateway)

The API machine holds the database and the authorization plane; the SSH gateway
machine opens no database and authenticates over gRPC. Both machines must share
the repository directory (`GITDASH_DATA/repos`) and its push-event spool.

```
        API / control-plane machine                SSH gateway machine
  ┌────────────────────────────┐        ┌────────────────────────────┐
  │  gitdash serve             │        │  gitdash serve             │
  │  GITDASH_GRPC_ADDR=:9090   │◄──gRPC─┤  GITDASH_ROLE=ssh          │
  │                            │ TLS +  │                            │
  │  ┌──────────────┐          │ Bearer │  ┌──────────────────────┐  │
  │  │  HTTP API    │          │ token  │  │ sshserver            │  │
  │  ├──────────────┤          │        │  │ (RemoteAuthorizer)   │  │
  │  │ authz gRPC   │          │        │  └──────────┬───────────┘  │
  │  ├──────────────┤          │        │             │              │
  │  │ store (auth) │          │        │             │ git-upload/  │
  │  └──────────────┘          │        │             │ receive-pack │
  └─────────────┬──────────────┘        └─────────────┼──────────────┘
                │                                     │
                └────────► shared GITDASH_DATA/repos ◄────────┘
                           (NFS / one persistent volume)
  browser ──HTTP──► API machine                git CLI ──SSH──► gateway
```

> The gateway machine needs only: the SSH port, `GITDASH_GRPC_ADDR/TOKEN`, and a
> shared `GITDASH_DATA`. It opens no database; branch-protection rules are fetched
> through the plane too.

## Enabling & configuration

The authorization plane is **off by default**. It only listens when
`GITDASH_GRPC_ADDR` is explicitly set:

| Env var | Default | Description |
|---|---|---|
| `GITDASH_GRPC_ADDR` | empty (off) | gRPC listen address. When empty the plane is not started (zero impact on existing deployments). Prefer `127.0.0.1:9090` or a private address |
| `GITDASH_GRPC_TOKEN` | empty | Service token. **Required when `GITDASH_GRPC_ADDR` is set**, otherwise startup fails (`grpc authz: GITDASH_GRPC_TOKEN must be set`) |

```bash
GITDASH_GRPC_ADDR=127.0.0.1:9090 \
GITDASH_GRPC_TOKEN=$(head -c 32 /dev/urandom | base64) \
./gitdash serve
```

> The address is not forced to loopback; the operator decides. When exposed beyond a
> trusted network, keep it on a private network or enable TLS (see below).

## Split deployment (SSH gateway)

Run the API and SSH on different machines: the API side enables the authorization
plane, the SSH side starts as a gateway.

API side (main process, source of truth):

```bash
GITDASH_GRPC_ADDR=0.0.0.0:9090 \
GITDASH_GRPC_TOKEN=$(head -c 32 /dev/urandom | base64) \
GITDASH_GRPC_TLS_CERT=/etc/gitdash/grpc.crt \
GITDASH_GRPC_TLS_KEY=/etc/gitdash/grpc.key \
./gitdash serve
```

SSH gateway side (SSH only, no database):

```bash
GITDASH_ROLE=ssh \
GITDASH_DATA=/shared/gitdash-data \        # shared repo dir + spool with the API (shared storage)
GITDASH_SSH_ADDR=:2222 \
GITDASH_GRPC_ADDR=api.internal:9090 \
GITDASH_GRPC_TOKEN=<same token as the API> \
GITDASH_GRPC_CA=/etc/gitdash/grpc-ca.crt \  # CA when TLS is enabled
./gitdash serve
```

| Env var | Default | Description |
|---|---|---|
| `GITDASH_ROLE` | empty | Set to `ssh` for SSH-gateway-only mode; unset keeps all-in-one |
| `GITDASH_SSH_ONLY` | empty | `1` is equivalent to `GITDASH_ROLE=ssh` (legacy alias) |
| `GITDASH_GRPC_ADDR` | empty | Required on the gateway: authorization-plane address |
| `GITDASH_GRPC_TOKEN` | empty | Required on the gateway: same service token as the API |
| `GITDASH_GRPC_CA` | empty | Optional: enables TLS on the gateway and verifies the server cert with this CA |
| `GITDASH_GRPC_SERVER_NAME` | empty | Optional: TLS SNI / server certificate hostname |
| `GITDASH_GRPC_TLS_CERT` / `GITDASH_GRPC_TLS_KEY` | empty | Optional on the API: both set enables TLS on the plane |

> Deployment requirements: the SSH machine must reach the repository directory
> (shared storage/NFS); push events written to the `post-receive` spool are consumed
> by the API dispatcher; branch-protection rules are fetched via the plane, so the
> SSH machine needs no database connection.

## Service contract

Proto: `backend/proto/authz/v1/authz.proto`, package `gitdash.authz.v1`, service `AuthzService`.

| RPC | Request → Response | Semantics (matches the status quo) |
|---|---|---|
| `AuthorizePublicKey` | `{key_type, key_blob}` → `{authorized, username, reason}` | see below |
| `IsIPBanned` | `{ip}` → `{banned}` | equals `store.IsIPBanned` (admin IP/CIDR blacklist) |
| `CanRead` | `{owner, repo, username}` → `{allowed}` | equals `store.CanRead` (read = clone/fetch/archive, repo ban included) |
| `CanWrite` | `{owner, repo, username}` → `{allowed}` | equals `store.CanWrite` (write = push, repo ban included) |
| `BranchProtection` | `{owner, repo, branch}` → `{protected, block_deletion, block_force_push}` | equals `store.GetBranchProtection`; no rule → `protected=false` (caller allows) |

### AuthorizePublicKey decision order

Aligned line-by-line with `internal/sshserver`'s `PublicKeyCallback`:

1. Iterate registered public keys, matching **exactly** on `(key_type, key_blob)`
   (no prefix/fuzzy matching);
2. Matched and account **not banned** → `authorized=true` with `username`;
3. Matched but account **banned** → `authorized=false`, `reason="account is banned"`;
4. No match → `authorized=false`, `reason="unknown public key"`.

> **Security note**: matching happens **server-side**; no public key list is ever
> returned — the authorization plane never becomes an information leak.

### Mapping to existing calls

| Status quo (`internal/sshserver`) | Authorization plane RPC |
|---|---|
| `PublicKeyCallback`: `st.PublicKeys()` scan + `st.IsUserBanned` | `AuthorizePublicKey` |
| `handleConn`: `st.IsIPBanned(host)` | `IsIPBanned` |
| `runGit`: `st.CanWrite(...)` | `CanWrite` |
| `runGit`: `st.CanRead(...)` | `CanRead` |
| `pre-receive` hook: `st.GetBranchProtection(...)` | `BranchProtection` |

## Security model

- **Off by default**: no `GITDASH_GRPC_ADDR`, no listener.
- **Mandatory service token**: every unary RPC is checked by an interceptor for
  `authorization: Bearer <GITDASH_GRPC_TOKEN>`; missing/mismatched → `UNAUTHENTICATED`.
  Comparison uses `crypto/subtle.ConstantTimeCompare`.
- **No reflection**: gRPC reflection is not registered, minimizing the exposed surface;
  clients must ship the proto.
- **Inbound size limit**: `MaxRecvMsgSize = 1 MiB` (auth requests are tiny).
- **No sensitive data returned**: only boolean decisions plus the matched username/rule;
  no public key list.
- **Transport hardening (optional TLS)**: set `GITDASH_GRPC_TLS_CERT/KEY` on the API
  and `GITDASH_GRPC_CA` on the gateway to enable TLS. Always enable it across
  untrusted networks.

## Usage examples

### grpcurl (no reflection; pass the proto)

```bash
grpcurl -plaintext \
  -H "authorization: Bearer $GITDASH_GRPC_TOKEN" \
  -import-path backend/proto -proto authz/v1/authz.proto \
  -d '{"ip":"10.0.0.1"}' \
  127.0.0.1:9090 gitdash.authz.v1.AuthzService/IsIPBanned
```

### Go client

```go
conn, err := grpc.NewClient("127.0.0.1:9090",
    grpc.WithTransportCredentials(insecure.NewCredentials()))
if err != nil {
    return err
}
client := authzv1.NewAuthzServiceClient(conn)

ctx := metadata.AppendToOutgoingContext(context.Background(),
    "authorization", "Bearer "+os.Getenv("GITDASH_GRPC_TOKEN"))

resp, err := client.CanWrite(ctx, &authzv1.CanWriteRequest{
    Owner: "alice", Repo: "demo", Username: "bob",
})
```

## Code generation

After changing the proto, regenerate with `buf` (bundled compiler, no `protoc` needed):

```bash
task proto        # installs pinned plugins + buf generate
```

Generated code: `backend/internal/grpcserver/authzv1/*.pb.go` (**committed**).

- Pinned tool versions in `Taskfile.yml`: `protoc-gen-go v1.36.12`,
  `protoc-gen-go-grpc v1.6.2`, `buf v1.47.2`
- buf config: `backend/buf.yaml`, `backend/buf.gen.yaml`
- Consider a CI step `task proto && git diff --exit-code` to prevent generated drift

## Testing

### White-box unit tests (same package)

```bash
cd backend && go test ./internal/grpcserver/...
```

Coverage: key hit / unknown / banned, IP blacklist hit and miss, `CanRead`/`CanWrite`
owner/public-repo/collaborator branches, and missing/wrong token returning
`UNAUTHENTICATED`.

### Black-box tests (separate process + real network)

`backend/tests/blackbox/` is an **independent black-box test** for the authorization
plane: it builds the Gitdash binary on the fly (or reuses `GITDASH_BIN`), starts it as a
child process on a fresh temp data dir + random ports, and interacts only over the
HTTP / gRPC wire — it imports no server-side implementation (only the generated gRPC
client stub), so it exercises `main.go`'s real wiring (env → gRPC listener → store).

```bash
cd backend && go test ./tests/blackbox/ -v
# reuse a prebuilt binary:
# GITDASH_BIN=/tmp/gitdash-server go test ./tests/blackbox/ -v
```

Covers: missing/wrong token → `UNAUTHENTICATED`; IP blacklist (effective after being
added via the admin API); `CanRead`/`CanWrite` (owner / stranger / public repo);
`AuthorizePublicKey` (hit / unknown / banned). Skipped under `-short` (building +
spawning a process is heavy).

## Roadmap

1. **Phase 1 (done)**: additive, off-by-default, token-guarded authorization plane
   covering all current SSH auth calls.
2. **Standalone SSH gateway (done)**: `GITDASH_ROLE=ssh` runs the same binary in
   SSH-only mode; all authz (including branch-protection rules) goes through the
   plane; all-in-one behavior is unchanged.
3. **Transport hardening (done, opt-in)**: TLS via `GITDASH_GRPC_TLS_CERT/KEY` +
   `GITDASH_GRPC_CA`; may later upgrade to mTLS + service identity.
4. **Multi-instance / partitioning**: horizontal scale with `region`/IP routing
   (see the follow-up design doc).

## Non-goals (this phase)

- Not changing the default all-in-one (API + in-process SSH) behavior
- Not introducing a data-plane transport endpoint (repo directory still relies on
  shared storage)
