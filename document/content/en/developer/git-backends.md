---
title: "Git backends"
weight: 3
summary: "The gitsvc.Backend abstraction and how to add an implementation."
---

Gitdash talks to repositories through a single abstraction,
`gitsvc.Backend` (in `backend/internal/gitsvc/backend.go`). The default
implementation, `cli`, shells out to the system `git` binary. Alternative
implementations (pure-Go `go-git`, libgit2 bindings, a forge API, an in-memory
fake for tests, ...) only need to implement the interface and register
themselves.

## The interface

`Backend` groups repository-scoped, semantic operations:

- lifecycle/storage: `Init`, `EnsureHooks`, `ReposDir`, `SpoolDir`, `RepoPath`,
  `Exists`, `IsEmptyRepo`, `CreateBare`, `InitTemplate`, `Delete`, `ForkRepo`,
  `ImportRepo`, `PushMirror`, `RepoSize`, `GC`
- refs: `HeadBranch`, `Branches`, `Tags`, `CreateRef`, `DeleteRef`,
  `SetHeadBranch`, `InvalidateRefs`
- reading: `Tree`, `ListDir`, `ReadBlob`, `BlameFile`, `RawCommit`,
  `RawCommits`, `Commits`, `LastCommit`, `CommitDiff`, `DiffStats`,
  `DiffPatch`, `SearchWith`
- writing: `WriteCommit`, `RevSHA`, `CanFastForward`, `MergeCheck`,
  `MergeFastForward`, `MergeNonFF`, `MergeRebase`, `RevertCommit`,
  `ApplyPatchSeries`
- escape hatch: `GitOut` (raw command; CLI-only)

Storage-specific methods (`RepoPath`, `ReposDir`, `SpoolDir`, `GitOut`) exist
because SSH serving and pipeline workspaces need a local path. A backend that is
not backed by a bare-repo directory can return `""`/an error for them.

## Adding an implementation

1. Create a type implementing `Backend`. Embed `gitsvc.Backend` (or `*cliBackend`
   via `gitsvc.NewCLI()`) to inherit the methods you do not override:

   ```go
   type MyBackend struct {
       gitsvc.Backend // fall back to the CLI backend where convenient
       // ... your fields
   }

   func (m *MyBackend) ReadBlob(owner, name, ref, file string) (*gitsvc.Blob, error) {
       // ...
   }
   ```

2. Register a factory during package `init` and install it at startup:

   ```go
   func init() {
       gitsvc.RegisterBackend("mine", func() gitsvc.Backend { return &MyBackend{} })
   }

   // in main(), before gitsvc.Init(dataDir):
   gitsvc.SetBackend(myBackend) // or gitsvc.SetBackend(must(gitsvc.NewBackend("mine")))
   gitsvc.Init(dataDir)         // calls the active backend's Init
   ```

3. Run the existing test suite against your backend (at least
   `go test ./internal/gitsvc/...`). Pure helpers stay backend-agnostic:
   `ValidName`, `ValidRef`, `CleanPath`, `ParseCodeowners`, `Search`,
   `LoadCodeowners`.

## Notes

- `gitsvc.Init(dataDir)` is the single configuration entry point for `main`; it
  initializes whatever backend is currently installed.
- `gitsvc.CurrentBackend()` returns the active backend; `gitsvc.NewCLI()` builds
  the default one.
- `GITDASH_*` environment wiring is intentionally not decided here: pick your
  backend from config in `main()` and call `SetBackend` before `Init`.
