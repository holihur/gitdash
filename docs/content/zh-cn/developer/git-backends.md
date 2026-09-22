---
title: "Git 后端抽象层"
weight: 3
summary: "gitsvc.Backend 抽象与如何新增实现。"
---

Gitdash 通过单一抽象 `gitsvc.Backend`（`backend/internal/gitsvc/backend.go`）
访问仓库。默认实现 `cli` 调用系统 `git` 二进制；其它实现（纯 Go 的 `go-git`、
libgit2 绑定、forge API、测试用的内存假实现等）只需实现该接口并注册即可。

## 接口

`Backend` 按仓库维度提供语义化操作：

- 生命周期/存储：`Init`、`EnsureHooks`、`ReposDir`、`SpoolDir`、`RepoPath`、
  `Exists`、`IsEmptyRepo`、`CreateBare`、`InitTemplate`、`Delete`、`ForkRepo`、
  `ImportRepo`、`PushMirror`、`RepoSize`、`GC`
- 引用：`HeadBranch`、`Branches`、`Tags`、`CreateRef`、`DeleteRef`、
  `SetHeadBranch`、`InvalidateRefs`
- 读取：`Tree`、`ListDir`、`ReadBlob`、`BlameFile`、`RawCommit`、
  `RawCommits`、`Commits`、`LastCommit`、`CommitDiff`、`DiffStats`、
  `DiffPatch`、`SearchWith`
- 写入：`WriteCommit`、`RevSHA`、`CanFastForward`、`MergeCheck`、
  `MergeFastForward`、`MergeNonFF`、`MergeRebase`、`RevertCommit`、
  `ApplyPatchSeries`
- 逃生舱：`GitOut`（裸命令，仅 CLI 实现有意义）

SSH 服务和流水线工作区需要本地路径，因此保留了存储相关方法
（`RepoPath`/`ReposDir`/`SpoolDir`/`GitOut`）。若你的后端不基于 bare 目录，
这些方法返回 `""` 或错误即可。

## 新增一个实现

1. 定义实现 `Backend` 的类型。内嵌 `gitsvc.Backend`（或 `gitsvc.NewCLI()`
   返回的 CLI 后端）即可继承未覆盖的方法：

   ```go
   type MyBackend struct {
       gitsvc.Backend // 未实现的方法回退到 CLI 后端
       // ... 你的字段
   }

   func (m *MyBackend) ReadBlob(owner, name, ref, file string) (*gitsvc.Blob, error) {
       // ...
   }
   ```

2. 在包 `init` 中注册工厂，并在启动时安装：

   ```go
   func init() {
       gitsvc.RegisterBackend("mine", func() gitsvc.Backend { return &MyBackend{} })
   }

   // 在 main() 中，gitsvc.Init(dataDir) 之前：
   gitsvc.SetBackend(myBackend) // 或 gitsvc.SetBackend(must(gitsvc.NewBackend("mine")))
   gitsvc.Init(dataDir)         // 调用当前后端的 Init
   ```

3. 用现有测试套件验证你的后端（至少
   `go test ./internal/gitsvc/...`）。纯函数与后端无关，保持可用：
   `ValidName`、`ValidRef`、`CleanPath`、`ParseCodeowners`、`Search`、
   `LoadCodeowners`。

## 说明

- `gitsvc.Init(dataDir)` 是 `main` 唯一的配置入口，负责初始化当前安装的后端。
- `gitsvc.CurrentBackend()` 返回当前后端；`gitsvc.NewCLI()` 构造默认后端。
- 这里刻意不决定 `GITDASH_*` 环境变量接线：请在 `main()` 中按配置选择后端，
  并在 `Init` 之前调用 `SetBackend`。
