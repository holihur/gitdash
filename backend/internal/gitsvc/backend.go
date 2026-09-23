package gitsvc

import (
	"context"
	"fmt"
	"sync"
)

// Backend abstracts every repository-scoped git operation performed by gitdash.
//
// The default implementation, "cli", shells out to the `git` binary (see
// cli.go). Alternative implementations (pure-Go go-git, libgit2 bindings, a
// remote forge API, an in-memory fake used in tests, ...) only need to
// implement this interface and install themselves through SetBackend.
//
// Implementations receive owner/name pairs and resolve repository locations
// internally. Storage-specific helpers (RepoPath, ReposDir, SpoolDir, GitOut)
// are part of the interface for compatibility with the CLI workflow; backends
// that are not backed by a local bare-repo directory may return an empty path
// or an error for them.
type Backend interface {
	// ---- lifecycle / storage ----

	// Init prepares the backend rooted at dataDir (creating directories etc.).
	Init(dataDir string) error
	// EnsureHooks (re)installs any server-side hooks for all repositories.
	EnsureHooks() error
	// ReposDir returns the root directory holding bare repositories ("" when N/A).
	ReposDir() string
	// SpoolDir returns the directory receiving push-event spool files ("" when N/A).
	SpoolDir() string
	// RepoPath returns the on-disk path of a bare repository ("" when N/A).
	RepoPath(owner, name string) string
	// Exists reports whether the repository exists.
	Exists(owner, name string) bool
	// IsEmptyRepo reports whether the repository has no commits/refs yet.
	IsEmptyRepo(owner, name string) bool
	// CreateBare creates an empty bare repository.
	CreateBare(owner, name string) error
	// InitTemplate seeds a freshly created bare repository with a default
	// README and pipeline file on the main branch.
	InitTemplate(owner, name string) error
	// Delete removes the repository and invalidates caches.
	Delete(owner, name string) error
	// ForkRepo mirrors a repository into a new owner/name.
	ForkRepo(sourceOwner, sourceName, targetOwner, targetName string) error
	// ImportRepo mirrors a remote URL into a new owner/name.
	// credential ("user:token", may be empty) is used for HTTPS basic auth.
	ImportRepo(url, targetOwner, targetName, privateKey, credential string) error
	// PushMirror pushes all refs to a remote URL.
	PushMirror(owner, name, url, privateKey string) error
	// RepoSize returns the repository disk usage in bytes.
	RepoSize(owner, name string) (int64, error)
	// RepoLanguages returns the byte breakdown of tracked blobs by language.
	RepoLanguages(owner, name, ref string) ([]LanguageStat, error)
	// GC runs garbage collection and reports the reclaimed bytes.
	GC(owner, name string) (*GCResult, error)

	// ---- refs ----

	HeadBranch(owner, name string) (string, error)
	Branches(owner, name string) ([]Branch, error)
	Tags(owner, name string) ([]Tag, error)
	CreateRef(owner, name, kind, refName, from string) (string, error)
	DeleteRef(owner, name, kind, refName string) error
	SetHeadBranch(owner, name, branch string) error
	InvalidateRefs(owner, name string)

	// ---- reading ----

	Tree(owner, name, ref, dir string) ([]Entry, error)
	ListDir(owner, name, ref, dir string) ([]string, error)
	ReadBlob(owner, name, ref, file string) (*Blob, error)
	ReadRawFile(owner, name, ref, file string) ([]byte, error)
	ListTreeFiles(owner, name, ref string) ([]TreeFile, error)
	ReadBlobsBatch(owner, name, ref string, paths []string, maxSize int64) (map[string][]byte, error)
	BlameFile(owner, name, ref, file string) (*Blame, error)
	RawCommit(owner, name, sha string) ([]byte, error)
	RawCommits(owner, name string, shas []string) map[string][]byte
	Commits(owner, name, ref string, limit, offset int, query string) ([]Commit, error)
	CommitInfo(owner, name, sha string) (*Commit, error)
	LastCommit(owner, name, ref, path string) (*Commit, error)
	CommitDiff(owner, name, sha string) ([]DiffFile, string, error)
	DiffStats(owner, name, base, head string) ([]DiffFile, error)
	DiffPatch(owner, name, base, head string) (string, error)
	SearchWith(ctx context.Context, owner, name, query string, opts SearchOpts) ([]SearchHit, error)

	// ---- writing ----

	WriteCommit(owner, name, branch, message, author string, changes []FileChange) (string, error)
	RevSHA(owner, name, rev string) (string, error)
	CanFastForward(owner, name, target, source string) bool
	MergeCheck(owner, name, target, source string) (mergeable, conflicted bool)
	MergeFastForward(owner, name, target, source string) (string, error)
	MergeNonFF(owner, name, target, source, message, committer, method string) (string, error)
	MergeRebase(owner, name, target, source, committer string) (string, error)
	RevertCommit(owner, name, branch, sha, message, committer string) (string, error)
	ApplyPatchSeries(owner, name, base string, patchData []byte) (branch, sha string, err error)

	// ---- escape hatch ----

	// GitOut runs a raw git command. It is only meaningful for implementations
	// that shell out to git; others may return an error.
	GitOut(dir string, args ...string) (string, error)
}

// ---- backend registry ----

var (
	backendMu       sync.RWMutex
	backend         Backend
	backendRegistry = map[string]func() Backend{}
)

func init() {
	// The CLI backend is always available and is the default.
	RegisterBackend("cli", NewCLI)
	backend = NewCLI()
}

// RegisterBackend registers a named backend factory. Registering a name twice
// replaces the previous factory. It is intended to be called from init() in
// the package that provides an alternative implementation.
func RegisterBackend(name string, factory func() Backend) {
	backendMu.Lock()
	defer backendMu.Unlock()
	backendRegistry[name] = factory
}

// NewBackend constructs a registered backend by name.
func NewBackend(name string) (Backend, error) {
	backendMu.RLock()
	factory, ok := backendRegistry[name]
	backendMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown git backend %q", name)
	}
	return factory(), nil
}

// SetBackend installs the backend used by the package-level helpers. It panics
// on a nil backend. Call Init afterwards to configure the backend's storage.
func SetBackend(b Backend) {
	if b == nil {
		panic("gitsvc: SetBackend(nil)")
	}
	backendMu.Lock()
	backend = b
	backendMu.Unlock()
}

// CurrentBackend returns the currently installed backend.
func CurrentBackend() Backend {
	backendMu.RLock()
	defer backendMu.RUnlock()
	return backend
}

// Init configures the active backend rooted at dataDir. It is the single entry
// point used by main(); after SetBackend it will initialize the custom backend.
func Init(dataDir string) error { return CurrentBackend().Init(dataDir) }

// ---- package-level delegating wrappers (keep the historical API) ----

func ReposDir() string { return CurrentBackend().ReposDir() }

// SpoolDir push 事件 spool 目录（post-receive hook 写入，webhook 调度器消费）
func SpoolDir() string { return CurrentBackend().SpoolDir() }

// RepoPath returns the on-disk path of a bare repository.
func RepoPath(owner, name string) string { return CurrentBackend().RepoPath(owner, name) }

// Exists reports whether the repository exists.
func Exists(owner, name string) bool { return CurrentBackend().Exists(owner, name) }

// IsEmptyRepo reports whether the repository has no commits/refs yet.
func IsEmptyRepo(owner, name string) bool { return CurrentBackend().IsEmptyRepo(owner, name) }

func CreateBare(owner, name string) error { return CurrentBackend().CreateBare(owner, name) }
func InitTemplate(owner, name string) error {
	return CurrentBackend().InitTemplate(owner, name)
}
func Delete(owner, name string) error { return CurrentBackend().Delete(owner, name) }
func ForkRepo(sourceOwner, sourceName, targetOwner, targetName string) error {
	return CurrentBackend().ForkRepo(sourceOwner, sourceName, targetOwner, targetName)
}
func ImportRepo(url, targetOwner, targetName, privateKey, credential string) error {
	return CurrentBackend().ImportRepo(url, targetOwner, targetName, privateKey, credential)
}
func PushMirror(owner, name, url, privateKey string) error {
	return CurrentBackend().PushMirror(owner, name, url, privateKey)
}
func RepoSize(owner, name string) (int64, error) { return CurrentBackend().RepoSize(owner, name) }

// RepoLanguages returns the byte breakdown of tracked blobs by language.
func RepoLanguages(owner, name, ref string) ([]LanguageStat, error) {
	return CurrentBackend().RepoLanguages(owner, name, ref)
}

func EnsureHooks() error                       { return CurrentBackend().EnsureHooks() }
func GC(owner, name string) (*GCResult, error) { return CurrentBackend().GC(owner, name) }

func HeadBranch(owner, name string) (string, error) { return CurrentBackend().HeadBranch(owner, name) }
func Branches(owner, name string) ([]Branch, error) { return CurrentBackend().Branches(owner, name) }
func Tags(owner, name string) ([]Tag, error)        { return CurrentBackend().Tags(owner, name) }
func CreateRef(owner, name, kind, refName, from string) (string, error) {
	return CurrentBackend().CreateRef(owner, name, kind, refName, from)
}
func DeleteRef(owner, name, kind, refName string) error {
	return CurrentBackend().DeleteRef(owner, name, kind, refName)
}
func SetHeadBranch(owner, name, branch string) error {
	return CurrentBackend().SetHeadBranch(owner, name, branch)
}
func InvalidateRefs(owner, name string) { CurrentBackend().InvalidateRefs(owner, name) }

func Tree(owner, name, ref, dir string) ([]Entry, error) {
	return CurrentBackend().Tree(owner, name, ref, dir)
}
func ListDir(owner, name, ref, dir string) ([]string, error) {
	return CurrentBackend().ListDir(owner, name, ref, dir)
}
func ReadBlob(owner, name, ref, file string) (*Blob, error) {
	return CurrentBackend().ReadBlob(owner, name, ref, file)
}

// ReadRawFile 读取仓库文件的原始字节（静态网站托管用）。
func ReadRawFile(owner, name, ref, file string) ([]byte, error) {
	return CurrentBackend().ReadRawFile(owner, name, ref, file)
}

// ListTreeFiles 列出 ref 上全部文件（路径 + 大小），供代码索引遍历。
func ListTreeFiles(owner, name, ref string) ([]TreeFile, error) {
	return CurrentBackend().ListTreeFiles(owner, name, ref)
}

// ReadBlobsBatch 批量读取多个文件内容（单个 git 进程）。
func ReadBlobsBatch(owner, name, ref string, paths []string, maxSize int64) (map[string][]byte, error) {
	return CurrentBackend().ReadBlobsBatch(owner, name, ref, paths, maxSize)
}
func BlameFile(owner, name, ref, file string) (*Blame, error) {
	return CurrentBackend().BlameFile(owner, name, ref, file)
}
func RawCommit(owner, name, sha string) ([]byte, error) {
	return CurrentBackend().RawCommit(owner, name, sha)
}
func RawCommits(owner, name string, shas []string) map[string][]byte {
	return CurrentBackend().RawCommits(owner, name, shas)
}
func Commits(owner, name, ref string, limit, offset int, query string) ([]Commit, error) {
	return CurrentBackend().Commits(owner, name, ref, limit, offset, query)
}

// CommitInfo 读取单个提交的元数据。
func CommitInfo(owner, name, sha string) (*Commit, error) {
	return CurrentBackend().CommitInfo(owner, name, sha)
}
func LastCommit(owner, name, ref, path string) (*Commit, error) {
	return CurrentBackend().LastCommit(owner, name, ref, path)
}
func CommitDiff(owner, name, sha string) ([]DiffFile, string, error) {
	return CurrentBackend().CommitDiff(owner, name, sha)
}
func DiffStats(owner, name, base, head string) ([]DiffFile, error) {
	return CurrentBackend().DiffStats(owner, name, base, head)
}
func DiffPatch(owner, name, base, head string) (string, error) {
	return CurrentBackend().DiffPatch(owner, name, base, head)
}
func SearchWith(ctx context.Context, owner, name, query string, opts SearchOpts) ([]SearchHit, error) {
	return CurrentBackend().SearchWith(ctx, owner, name, query, opts)
}

func WriteCommit(owner, name, branch, message, author string, changes []FileChange) (string, error) {
	return CurrentBackend().WriteCommit(owner, name, branch, message, author, changes)
}
func RevSHA(owner, name, rev string) (string, error) {
	return CurrentBackend().RevSHA(owner, name, rev)
}
func CanFastForward(owner, name, target, source string) bool {
	return CurrentBackend().CanFastForward(owner, name, target, source)
}
func MergeCheck(owner, name, target, source string) (mergeable, conflicted bool) {
	return CurrentBackend().MergeCheck(owner, name, target, source)
}
func MergeFastForward(owner, name, target, source string) (string, error) {
	return CurrentBackend().MergeFastForward(owner, name, target, source)
}
func MergeNonFF(owner, name, target, source, message, committer, method string) (string, error) {
	return CurrentBackend().MergeNonFF(owner, name, target, source, message, committer, method)
}
func MergeRebase(owner, name, target, source, committer string) (string, error) {
	return CurrentBackend().MergeRebase(owner, name, target, source, committer)
}
func RevertCommit(owner, name, branch, sha, message, committer string) (string, error) {
	return CurrentBackend().RevertCommit(owner, name, branch, sha, message, committer)
}
func ApplyPatchSeries(owner, name, base string, patchData []byte) (branch, sha string, err error) {
	return CurrentBackend().ApplyPatchSeries(owner, name, base, patchData)
}

// GitOut runs a raw git command through the active backend.
func GitOut(dir string, args ...string) (string, error) { return CurrentBackend().GitOut(dir, args...) }
