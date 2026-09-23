package codesearch

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
)

// 增量索引清单：记录某仓库每个已索引文件的 blob SHA。
// 下一次重建时据此前算出「新增 / 变更 / 删除」集合，只重建变更文件。
// 以 gzip JSON sidecar 存放，避免污染主索引与 meta.json。
type manifest map[string]string

func (b *Bleve) manifestDir() string { return filepath.Join(b.dir, "manifests") }

// manifestPath owner/name 已由 validRepoKey 保证不含 `/`，可用作文件名。
func (b *Bleve) manifestPath(owner, name string) string {
	return filepath.Join(b.manifestDir(), owner+"__"+name+".json.gz")
}

// loadManifest 读取仓库清单；不存在或损坏时返回 nil（触发全量重建）。
func (b *Bleve) loadManifest(owner, name string) map[string]string {
	f, err := os.Open(b.manifestPath(owner, name))
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil
	}
	defer func() { _ = zr.Close() }()
	m := manifest{}
	if err := json.NewDecoder(zr).Decode(&m); err != nil {
		return nil
	}
	return m
}

// saveManifest 原子写入仓库清单（临时文件 + rename）。
func (b *Bleve) saveManifest(owner, name string, m map[string]string) error {
	if err := os.MkdirAll(b.manifestDir(), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(b.manifestDir(), name+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	zw := gzip.NewWriter(tmp)
	encErr := json.NewEncoder(zw).Encode(m)
	if encErr == nil {
		encErr = zw.Close()
	} else {
		_ = zw.Close()
	}
	if werr := tmp.Close(); encErr == nil {
		encErr = werr
	}
	if encErr != nil {
		_ = os.Remove(tmpName)
		return encErr
	}
	return os.Rename(tmpName, b.manifestPath(owner, name))
}

// deleteManifest 移除仓库清单（仓库删除时调用）。
func (b *Bleve) deleteManifest(owner, name string) {
	_ = os.Remove(b.manifestPath(owner, name))
}
