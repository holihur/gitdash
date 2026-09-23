package codesearch

import (
	"os"
	"path/filepath"
)

// 「待重建」标记：post-receive hook 在默认分支 push 时**同步**写一个空文件，
// 检索侧在标记存在时回退到实时 grep，从而保证「push 后立即可搜到新内容」——
// 索引本身是异步（增量）重建的，在此之前不能把旧索引当作就绪。
// 重建完成（Index）或仓库删除（Forget）时清除标记。
const dirtySubDir = "dirty"

func dirtyDir(dataDir string) string { return filepath.Join(dataDir, IndexSubDir, dirtySubDir) }

func dirtyFileName(owner, name string) string { return owner + "__" + name }

// MarkDirty 标记某仓库的默认分支索引待重建（由 post-receive hook 调用）。
// best-effort：失败时静默（最坏情况是短暂返回旧索引，由后续重建收敛）。
func MarkDirty(dataDir, owner, name string) {
	if owner == "" || name == "" {
		return
	}
	dir := dirtyDir(dataDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	f, err := os.Create(filepath.Join(dir, dirtyFileName(owner, name)))
	if err != nil {
		return
	}
	_ = f.Close()
}

func (b *Bleve) dirtyMarker(owner, name string) string {
	return filepath.Join(b.dir, dirtySubDir, dirtyFileName(owner, name))
}

func (b *Bleve) isDirty(owner, name string) bool {
	_, err := os.Stat(b.dirtyMarker(owner, name))
	return err == nil
}

func (b *Bleve) clearDirty(owner, name string) {
	_ = os.Remove(b.dirtyMarker(owner, name))
}
