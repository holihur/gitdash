---
title: "Authorization gRPC plane"
weight: 1
summary: "AuthzService: the authorization plane a future standalone SSH gateway calls over gRPC."
---

`AuthzService` is Gitdash's **authorization plane (control plane)**: it consolidates
the auth checks that the SSH server currently performs in-process into a stable set
of unary gRPC methods, so a future **standalone SSH gateway** can call them over gRPC.

It only makes **decisions**, never touches repository data, and **does not change any
behavior of the existing in-process SSH / API / hooks**.

## Role & status

```
            (future) standalone SSH gateway
                        │  gRPC: AuthorizePublicKey / IsIPBanned / CanRead / CanWrite
                        ▼
                ┌───────────────────┐
                │  gitdash process   │
                │  AuthzService      │  ← the authorization plane described here
                │  + store (source)  │
                └───────────────────┘
                        ▲
                        │ in-process direct calls (status quo, untouched)
                ┌───────┴────────┐
                │ internal/sshserver │
                └────────────────┘
```

- **Status (phase 1)**: the authorization plane is a **purely additive side service**,
  off by default, with no consumers yet. `internal/sshserver` still calls `store`
  directly and behaves exactly as before.
- **Goal**: freeze the contract first. When the standalone `gitdash-ssh` is extracted
  later, only "swap the client + move the process" remains — no redesign of authz.

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
> trusted network, keep it on a private network or add mTLS (see "Security model / Roadmap").

## Service contract

Proto: `backend/proto/authz/v1/authz.proto`, package `gitdash.authz.v1`, service `AuthzService`.

| RPC | Request → Response | Semantics (matches the status quo) |
|---|---|---|
| `AuthorizePublicKey` | `{key_type, key_blob}` → `{authorized, username, reason}` | see below |
| `IsIPBanned` | `{ip}` → `{banned}` | equals `store.IsIPBanned` (admin IP/CIDR blacklist) |
| `CanRead` | `{owner, repo, username}` → `{allowed}` | equals `store.CanRead` (read = clone/fetch/archive, repo ban included) |
| `CanWrite` | `{owner, repo, username}` → `{allowed}` | equals `store.CanWrite` (write = push, repo ban included) |

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

## Security model

- **Off by default**: no `GITDASH_GRPC_ADDR`, no listener.
- **Mandatory service token**: every unary RPC is checked by an interceptor for
  `authorization: Bearer <GITDASH_GRPC_TOKEN>`; missing/mismatched → `UNAUTHENTICATED`.
  Comparison uses `crypto/subtle.ConstantTimeCompare`.
- **No reflection**: gRPC reflection is not registered, minimizing the exposed surface;
  clients must ship the proto.
- **Inbound size limit**: `MaxRecvMsgSize = 1 MiB` (auth requests are tiny).
- **No sensitive data returned**: only boolean decisions plus the matched username;
  no key/rule details.

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

1. **This phase (done)**: additive, off-by-default, token-guarded authorization plane
   covering all current SSH auth calls; nothing else touched.
2. **Standalone SSH gateway**: extract `internal/sshserver` into its own binary, switch
   auth to this service; pick the data-plane model per deployment (shared storage /
   stateless proxy).
3. **Transport hardening**: mTLS + service identity instead of a shared token when
   crossing machines.
4. **Multi-instance / partitioning**: horizontal scale with `region`/IP routing
   (see the follow-up design doc).

## Non-goals (this phase)

- Not splitting the SSH process, not changing `internal/sshserver`
- Not changing the API / hooks / frontend / clone URLs
- Not introducing a data-plane transport endpoint
