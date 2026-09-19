package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm/clause"
)

// ---- Docker / OCI 私有注册表 ----
//
// blob 复用包注册表的内容寻址磁盘存储（data/packages-blobs/<sha2>/<sha>），
// manifest 内容较小，直接存 registry_manifests 表；tag 与 digest 各占一行 reference。

const RegistryDigestPrefix = "sha256:"

// RegistryManifest manifest 元数据（不含内容）。
type RegistryManifest struct {
	ID        int64  `json:"id"`
	Owner     string `json:"owner"`
	Image     string `json:"image"`
	Reference string `json:"reference"`
	MediaType string `json:"media_type"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
}

type registryManifestRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Owner     string `gorm:"not null;uniqueIndex:uq_registry_manifest;size:255"`
	Image     string `gorm:"not null;uniqueIndex:uq_registry_manifest;size:512"`
	Reference string `gorm:"not null;uniqueIndex:uq_registry_manifest;size:255"`
	MediaType string `gorm:"not null;default:'';size:255"`
	Digest    string `gorm:"not null;size:71"`
	Size      int64  `gorm:"not null;default:0"`
	Content   []byte `gorm:"not null"`
	CreatedAt string `gorm:"not null"`
}

func (registryManifestRow) TableName() string { return "registry_manifests" }

// registryBlobAccessRow 记录命名空间对某 blob 的访问权（仅在实际上传成功时授予）。
// 拉取 blob 时必须命中本表，防止凭 digest 跨命名空间读取私有层（安全评审 §3.2）。
type registryBlobAccessRow struct {
	Owner  string `gorm:"primaryKey;size:255"`
	Digest string `gorm:"primaryKey;size:71"`
}

func (registryBlobAccessRow) TableName() string { return "registry_blob_access" }

func toRegistryManifest(r registryManifestRow) RegistryManifest {
	return RegistryManifest{
		ID: r.ID, Owner: r.Owner, Image: r.Image, Reference: r.Reference,
		MediaType: r.MediaType, Digest: r.Digest, Size: r.Size, CreatedAt: r.CreatedAt,
	}
}

func digestHex(digest string) (string, bool) {
	sha := strings.TrimPrefix(digest, RegistryDigestPrefix)
	if len(sha) != 64 {
		return "", false
	}
	for _, c := range sha {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", false
		}
	}
	return sha, true
}

// SaveRegistryBlobFromFile 校验临时文件 sha256 与 digest 一致后，rename 进内容寻址 blob 存储。
// 已存在同内容 blob 时删除临时文件。成功后临时文件被移走或删除。
func (s *Store) SaveRegistryBlobFromFile(tmpPath, digest string) error {
	sha, ok := digestHex(digest)
	if !ok {
		return errors.New("invalid digest")
	}
	f, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, copyErr := io.Copy(h, f)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if hex.EncodeToString(h.Sum(nil)) != sha {
		_ = os.Remove(tmpPath)
		return errors.New("digest mismatch")
	}
	dst := blobFile(sha)
	if dst == "" {
		_ = os.Remove(tmpPath)
		return errors.New("blob store not configured")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		return os.Remove(tmpPath)
	}
	return os.Rename(tmpPath, dst)
}

// RegistryBlobPath 返回 blob 的磁盘路径（存在时）。
func (s *Store) RegistryBlobPath(digest string) (string, bool) {
	sha, ok := digestHex(digest)
	if !ok {
		return "", false
	}
	p := blobFile(sha)
	if p == "" {
		return "", false
	}
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return "", false
	}
	return p, true
}

// RegistryBlobExists 判断 blob 是否已存在。
func (s *Store) RegistryBlobExists(digest string) bool {
	_, ok := s.RegistryBlobPath(digest)
	return ok
}

// GrantRegistryBlobAccess 记录命名空间对 blob 的访问权（上传成功后调用）。
func (s *Store) GrantRegistryBlobAccess(owner, digest string) error {
	if _, ok := digestHex(digest); !ok {
		return errors.New("invalid digest")
	}
	row := registryBlobAccessRow{Owner: owner, Digest: digest}
	return s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

// RegistryBlobAccessible 判断命名空间是否可读取该 blob。
func (s *Store) RegistryBlobAccessible(owner, digest string) bool {
	if _, ok := digestHex(digest); !ok {
		return false
	}
	var n int64
	if err := s.db.Model(&registryBlobAccessRow{}).
		Where("owner = ? AND digest = ?", owner, digest).Count(&n).Error; err != nil {
		return false
	}
	return n > 0
}

// PutRegistryManifest 写入 / 覆盖某 reference 的 manifest。
func (s *Store) PutRegistryManifest(owner, image, reference, mediaType, digest string, content []byte) error {
	row := registryManifestRow{
		Owner: owner, Image: image, Reference: reference, MediaType: mediaType,
		Digest: digest, Size: int64(len(content)), Content: content, CreatedAt: now(),
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "owner"}, {Name: "image"}, {Name: "reference"}},
		DoUpdates: clause.AssignmentColumns([]string{"media_type", "digest", "size", "content", "created_at"}),
	}).Create(&row).Error
}

// GetRegistryManifest 读取 manifest 内容；不存在返回 ErrNotFound。
func (s *Store) GetRegistryManifest(owner, image, reference string) (RegistryManifest, []byte, error) {
	var row registryManifestRow
	err := s.db.Where("owner = ? AND image = ? AND reference = ?", owner, image, reference).First(&row).Error
	if err != nil {
		return RegistryManifest{}, nil, notFoundErr(err)
	}
	return toRegistryManifest(row), row.Content, nil
}

// DeleteRegistryManifest 删除 tag 或（按 digest）删除整个镜像的全部引用。
func (s *Store) DeleteRegistryManifest(owner, image, reference string) error {
	q := s.db.Where("owner = ? AND image = ?", owner, image)
	if _, isDigest := digestHex(reference); isDigest {
		q = q.Where("reference = ? OR digest = ?", reference, reference)
	} else {
		q = q.Where("reference = ?", reference)
	}
	res := q.Delete(&registryManifestRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ListRegistryTags 列出镜像的全部 tag（不含 digest 引用）。
func (s *Store) ListRegistryTags(owner, image string) ([]string, error) {
	var refs []string
	err := s.db.Model(&registryManifestRow{}).
		Where("owner = ? AND image = ? AND reference NOT LIKE ?", owner, image, RegistryDigestPrefix+"%").
		Order("reference").Pluck("reference", &refs).Error
	return refs, err
}

// ListRegistryImages 列出命名空间下的镜像名。
func (s *Store) ListRegistryImages(owner string) ([]string, error) {
	var images []string
	err := s.db.Model(&registryManifestRow{}).Where("owner = ?", owner).
		Distinct("image").Order("image").Pluck("image", &images).Error
	return images, err
}

// ListRegistryCatalog 列出全部 <namespace>/<image>（_catalog 用）。
func (s *Store) ListRegistryCatalog() ([]string, error) {
	var rows []struct{ Owner, Image string }
	if err := s.db.Model(&registryManifestRow{}).
		Select("owner, image").Distinct("owner", "image").Order("owner, image").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Owner+"/"+r.Image)
	}
	return out, nil
}

// manifestReferencedDigests 解析 manifest JSON，返回 config 与 layers 的 digest。
func manifestReferencedDigests(content []byte) []string {
	var m struct {
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
		Layers []struct {
			Digest string `json:"digest"`
		} `json:"layers"`
	}
	if json.Unmarshal(content, &m) != nil {
		return nil
	}
	out := make([]string, 0, len(m.Layers)+1)
	if m.Config.Digest != "" {
		out = append(out, m.Config.Digest)
	}
	for _, l := range m.Layers {
		if l.Digest != "" {
			out = append(out, l.Digest)
		}
	}
	return out
}

// BackfillRegistryBlobAccess 在升级到访问控制后，根据既有 manifest 为其 owner
// 补授 blob 访问权（仅当访问表为空时执行），避免存量镜像拉取失效。
func (s *Store) BackfillRegistryBlobAccess() error {
	var n int64
	if err := s.db.Model(&registryBlobAccessRow{}).Count(&n).Error; err != nil || n > 0 {
		return err
	}
	var rows []registryManifestRow
	if err := s.db.Select("owner, content").Find(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		for _, d := range manifestReferencedDigests(r.Content) {
			row := registryBlobAccessRow{Owner: r.Owner, Digest: d}
			if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
