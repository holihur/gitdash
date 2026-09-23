package gitsvc

import "context"

// cliBackend implements Backend by shelling out to the system `git` binary.
//
// The actual command construction lives in the per-area files (repo.go,
// refs.go, tree.go, ...) and is shared with the package-level wrappers through
// the unexported helper functions. This type is intentionally thin: it exists
// to satisfy Backend so alternative implementations can be swapped in.
type cliBackend struct{}

// compile-time assertion that the CLI backend satisfies the abstraction.
var _ Backend = (*cliBackend)(nil)

// NewCLI returns the default backend backed by the `git` CLI.
func NewCLI() Backend { return &cliBackend{} }

func (c *cliBackend) Init(dataDir string) error { return initDirs(dataDir) }
func (c *cliBackend) EnsureHooks() error        { return ensureHooks() }
func (c *cliBackend) ReposDir() string          { return reposDir }
func (c *cliBackend) SpoolDir() string          { return spoolDir }
func (c *cliBackend) RepoPath(owner, name string) string {
	return repoPath(owner, name)
}
func (c *cliBackend) Exists(owner, name string) bool { return repoExists(owner, name) }
func (c *cliBackend) IsEmptyRepo(owner, name string) bool {
	return isEmptyRepo(owner, name)
}
func (c *cliBackend) CreateBare(owner, name string) error { return createBare(owner, name) }
func (c *cliBackend) InitTemplate(owner, name string) error {
	return initTemplate(owner, name)
}
func (c *cliBackend) Delete(owner, name string) error { return deleteRepo(owner, name) }
func (c *cliBackend) ForkRepo(sourceOwner, sourceName, targetOwner, targetName string) error {
	return forkRepo(sourceOwner, sourceName, targetOwner, targetName)
}
func (c *cliBackend) ImportRepo(url, targetOwner, targetName, privateKey, credential string) error {
	return importRepo(url, targetOwner, targetName, privateKey, credential)
}
func (c *cliBackend) PushMirror(owner, name, url, privateKey string) error {
	return pushMirror(owner, name, url, privateKey)
}
func (c *cliBackend) RepoSize(owner, name string) (int64, error) { return repoSize(owner, name) }
func (c *cliBackend) RepoLanguages(owner, name, ref string) ([]LanguageStat, error) {
	return repoLanguages(owner, name, ref)
}
func (c *cliBackend) GC(owner, name string) (*GCResult, error) { return gc(owner, name) }

func (c *cliBackend) HeadBranch(owner, name string) (string, error) {
	return headBranch(owner, name)
}
func (c *cliBackend) Branches(owner, name string) ([]Branch, error) {
	return branches(owner, name)
}
func (c *cliBackend) Tags(owner, name string) ([]Tag, error) { return tags(owner, name) }
func (c *cliBackend) CreateRef(owner, name, kind, refName, from string) (string, error) {
	return createRef(owner, name, kind, refName, from)
}
func (c *cliBackend) DeleteRef(owner, name, kind, refName string) error {
	return deleteRef(owner, name, kind, refName)
}
func (c *cliBackend) SetHeadBranch(owner, name, branch string) error {
	return setHeadBranch(owner, name, branch)
}
func (c *cliBackend) InvalidateRefs(owner, name string) { invalidateRefs(owner, name) }

func (c *cliBackend) Tree(owner, name, ref, dir string) ([]Entry, error) {
	return tree(owner, name, ref, dir)
}
func (c *cliBackend) ListDir(owner, name, ref, dir string) ([]string, error) {
	return listDir(owner, name, ref, dir)
}
func (c *cliBackend) ReadBlob(owner, name, ref, file string) (*Blob, error) {
	return readBlob(owner, name, ref, file)
}
func (c *cliBackend) ReadRawFile(owner, name, ref, file string) ([]byte, error) {
	return readRawFile(owner, name, ref, file)
}
func (c *cliBackend) ListTreeFiles(owner, name, ref string) ([]TreeFile, error) {
	return listTreeFiles(owner, name, ref)
}
func (c *cliBackend) ReadBlobsBatch(owner, name, ref string, paths []string, maxSize int64) (map[string][]byte, error) {
	return readBlobsBatch(owner, name, ref, paths, maxSize)
}
func (c *cliBackend) BlameFile(owner, name, ref, file string) (*Blame, error) {
	return blameFile(owner, name, ref, file)
}
func (c *cliBackend) RawCommit(owner, name, sha string) ([]byte, error) {
	return rawCommit(owner, name, sha)
}
func (c *cliBackend) RawCommits(owner, name string, shas []string) map[string][]byte {
	return rawCommits(owner, name, shas)
}
func (c *cliBackend) Commits(owner, name, ref string, limit, offset int, query string) ([]Commit, error) {
	return commits(owner, name, ref, limit, offset, query)
}
func (c *cliBackend) CommitInfo(owner, name, sha string) (*Commit, error) {
	return commitInfo(owner, name, sha)
}
func (c *cliBackend) LastCommit(owner, name, ref, path string) (*Commit, error) {
	return lastCommit(owner, name, ref, path)
}
func (c *cliBackend) CommitDiff(owner, name, sha string) ([]DiffFile, string, error) {
	return commitDiff(owner, name, sha)
}
func (c *cliBackend) DiffStats(owner, name, base, head string) ([]DiffFile, error) {
	return diffStats(owner, name, base, head)
}
func (c *cliBackend) DiffPatch(owner, name, base, head string) (string, error) {
	return diffPatch(owner, name, base, head)
}
func (c *cliBackend) SearchWith(ctx context.Context, owner, name, query string, opts SearchOpts) ([]SearchHit, error) {
	return searchWith(ctx, owner, name, query, opts)
}

func (c *cliBackend) WriteCommit(owner, name, branch, message, author string, changes []FileChange) (string, error) {
	return writeCommit(owner, name, branch, message, author, changes)
}
func (c *cliBackend) RevSHA(owner, name, rev string) (string, error) {
	return revSHA(owner, name, rev)
}
func (c *cliBackend) CanFastForward(owner, name, target, source string) bool {
	return canFastForward(owner, name, target, source)
}
func (c *cliBackend) MergeCheck(owner, name, target, source string) (mergeable, conflicted bool) {
	return mergeCheck(owner, name, target, source)
}
func (c *cliBackend) MergeFastForward(owner, name, target, source string) (string, error) {
	return mergeFastForward(owner, name, target, source)
}
func (c *cliBackend) MergeNonFF(owner, name, target, source, message, committer, method string) (string, error) {
	return mergeNonFF(owner, name, target, source, message, committer, method)
}
func (c *cliBackend) MergeRebase(owner, name, target, source, committer string) (string, error) {
	return mergeRebase(owner, name, target, source, committer)
}
func (c *cliBackend) RevertCommit(owner, name, branch, sha, message, committer string) (string, error) {
	return revertCommit(owner, name, branch, sha, message, committer)
}
func (c *cliBackend) ApplyPatchSeries(owner, name, base string, patchData []byte) (branch, sha string, err error) {
	return applyPatchSeries(owner, name, base, patchData)
}
func (c *cliBackend) GitOut(dir string, args ...string) (string, error) { return gitOut(dir, args...) }
