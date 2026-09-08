package store

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"gorm.io/gorm"
)

// ---- 私有包注册表 ----
//
// 元数据存 packages 表；文件内容走内容寻址磁盘存储（data/packages-blobs/<sha2>/<sha>），
// 旧库中仍存在 Content 列的行回退读库。审计记录存 package_audits 表。

// Package 包文件元数据（Content/BlobPath 仅内部使用）。
type Package struct {
	ID        int64  `json:"id"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"` // 可选：关联仓库（跟随其可见性），空 = 不关联
	Type      string `json:"type"` // npm | composer | pypi | rubygems | go | cargo | maven
	Name      string `json:"name"`
	Version   string `json:"version"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	Checksum  string `json:"checksum"` // sha256 hex
	Downloads int64  `json:"downloads"`
	Yanked    bool   `json:"yanked"`
	Uploader  string `json:"uploader"`
	CreatedAt string `json:"created_at"`
}

type packageRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Owner     string `gorm:"not null;uniqueIndex:uq_package;size:255"`
	Repo      string `gorm:"not null;default:'';size:255"`
	Type      string `gorm:"not null;uniqueIndex:uq_package;size:16"`
	Name      string `gorm:"not null;uniqueIndex:uq_package;size:512"`
	Version   string `gorm:"not null;uniqueIndex:uq_package;size:255"`
	Filename  string `gorm:"not null;uniqueIndex:uq_package;size:512"`
	Size      int64  `gorm:"not null;default:0"`
	Checksum  string `gorm:"not null;default:'';size:64"`
	Downloads int64  `gorm:"not null;default:0"`
	Yanked    bool   `gorm:"not null;default:false"`
	Uploader  string `gorm:"not null;size:255"`
	BlobPath  string `gorm:"column:blob_path;not null;default:'';size:512"`
	Content   []byte `gorm:"not null;default:''"`
	CreatedAt string `gorm:"not null"`
}

func (packageRow) TableName() string { return "packages" }

// PackageTag 包级 dist-tag（npm）。
type PackageTag struct {
	ID      int64  `json:"id"`
	Owner   string `json:"owner"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Tag     string `json:"tag"`
	Version string `json:"version"`
}

type packageTagRow struct {
	ID      int64  `gorm:"primaryKey;autoIncrement"`
	Owner   string `gorm:"not null;uniqueIndex:uq_package_tag;size:255"`
	Type    string `gorm:"not null;uniqueIndex:uq_package_tag;size:16"`
	Name    string `gorm:"not null;uniqueIndex:uq_package_tag;size:512"`
	Tag     string `gorm:"not null;uniqueIndex:uq_package_tag;size:64"`
	Version string `gorm:"not null;size:255"`
}

func (packageTagRow) TableName() string { return "package_tags" }

// PackageAudit 包操作审计记录。
type PackageAudit struct {
	ID        int64  `json:"id"`
	Owner     string `json:"owner"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Action    string `json:"action"` // publish | delete | yank | unyank
	Actor     string `json:"actor"`
	CreatedAt string `json:"created_at"`
}

type packageAuditRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Owner     string `gorm:"not null;index:idx_package_audit;size:255"`
	Type      string `gorm:"not null;index:idx_package_audit;size:16"`
	Name      string `gorm:"not null;index:idx_package_audit;size:512"`
	Version   string `gorm:"not null;default:'';size:255"`
	Action    string `gorm:"not null;size:16"`
	Actor     string `gorm:"not null;size:255"`
	CreatedAt string `gorm:"not null"`
}

func (packageAuditRow) TableName() string { return "package_audits" }

// ---- 内容寻址 blob 存储 ----

var pkgBlobDir string

func init() { SetBlobDir("") }

// SetBlobDir 设置包文件 blob 存储目录（main 启动时调用一次）。
func SetBlobDir(dir string) {
	if dir != "" {
		pkgBlobDir = dir
	}
}

func blobFile(sha string) string {
	if pkgBlobDir == "" || sha == "" {
		return ""
	}
	return filepath.Join(pkgBlobDir, sha[:2], sha)
}

func storeBlob(content []byte) (blobPath, sha string, err error) {
	sum := sha256.Sum256(content)
	sha = hex.EncodeToString(sum[:])
	p := blobFile(sha)
	if p == "" {
		return "", sha, nil
	}
	if _, err := os.Stat(p); err == nil {
		return p, sha, nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", sha, err
	}
	if err := os.WriteFile(p, content, 0o644); err != nil {
		return "", sha, err
	}
	return p, sha, nil
}

func readBlob(row packageRow) ([]byte, error) {
	if row.BlobPath != "" {
		b, err := os.ReadFile(row.BlobPath)
		if err == nil {
			return b, nil
		}
	}
	return row.Content, nil
}

func packageFromRow(row packageRow) Package {
	return Package{
		ID: row.ID, Owner: row.Owner, Repo: row.Repo, Type: row.Type, Name: row.Name,
		Version: row.Version, Filename: row.Filename, Size: row.Size, Checksum: row.Checksum,
		Downloads: row.Downloads, Yanked: row.Yanked, Uploader: row.Uploader, CreatedAt: row.CreatedAt,
	}
}

// CreatePackage 新增包文件；同 (owner,type,name,version,filename) 重复返回 ErrExists。
// 内容写入内容寻址 blob 存储，DB 只保留元数据（sha256 校验和 + blob 路径）。
func (s *Store) CreatePackage(p *Package, content []byte) error {
	blobPath, sha, err := storeBlob(content)
	if err != nil {
		return err
	}
	row := packageRow{
		Owner: p.Owner, Repo: p.Repo, Type: p.Type, Name: p.Name, Version: p.Version,
		Filename: p.Filename, Size: int64(len(content)), Checksum: sha, Uploader: p.Uploader,
		BlobPath: blobPath, Content: content, CreatedAt: now(),
	}
	if blobPath != "" {
		row.Content = []byte{} // 内容已落盘，DB 只存元数据
	}
	if err := s.db.Create(&row).Error; err != nil {
		if isUniqueErr(err) {
			return ErrExists
		}
		return err
	}
	*p = packageFromRow(row)
	return nil
}

// GetPackageFile 读取包文件内容并递增下载计数；不存在返回 ErrNotFound。
func (s *Store) GetPackageFile(owner, typ, name, version, filename string) (Package, []byte, error) {
	var row packageRow
	err := s.db.Where("owner = ? AND type = ? AND name = ? AND version = ? AND filename = ?",
		owner, typ, name, version, filename).First(&row).Error
	if err != nil {
		return Package{}, nil, notFoundErr(err)
	}
	content, err := readBlob(row)
	if err != nil {
		return Package{}, nil, err
	}
	_ = s.db.Model(&packageRow{}).Where("id = ?", row.ID).
		UpdateColumn("downloads", gorm.Expr("downloads + 1")).Error
	row.Downloads++
	return packageFromRow(row), content, nil
}

// GetPackageVersion 读取某版本任意一个文件（通常每版本只有一个包文件）。
func (s *Store) GetPackageVersion(owner, typ, name, version string) (Package, []byte, error) {
	var row packageRow
	err := s.db.Where("owner = ? AND type = ? AND name = ? AND version = ?",
		owner, typ, name, version).Order("id").First(&row).Error
	if err != nil {
		return Package{}, nil, notFoundErr(err)
	}
	content, err := readBlob(row)
	if err != nil {
		return Package{}, nil, err
	}
	return packageFromRow(row), content, nil
}

// ListPackageVersions 列出某包全部版本文件（按创建时间降序）。
func (s *Store) ListPackageVersions(owner, typ, name string) ([]Package, error) {
	var rows []packageRow
	err := s.db.Where("owner = ? AND type = ? AND name = ?", owner, typ, name).
		Order("created_at DESC, id ASC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]Package, 0, len(rows))
	for _, r := range rows {
		out = append(out, packageFromRow(r))
	}
	return out, nil
}

// ListPackages 列出命名空间下某类型的包（按 name 去重，取最新一条展示）。
func (s *Store) ListPackages(owner, typ string, limit, offset int) ([]Package, int, error) {
	var total int64
	q := s.db.Model(&packageRow{}).Where("owner = ? AND type = ?", owner, typ)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []packageRow
	if err := q.Order("name ASC, created_at DESC, id DESC").Limit(limit).Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]Package, 0, len(rows))
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.Name] {
			continue
		}
		seen[r.Name] = true
		out = append(out, packageFromRow(r))
	}
	return out, int(total), nil
}

// ListAllPackageNames 列出命名空间下某类型的全部去重包名（simple index / composer 用）。
func (s *Store) ListAllPackageNames(owner, typ string) ([]string, error) {
	var names []string
	err := s.db.Model(&packageRow{}).Where("owner = ? AND type = ?", owner, typ).
		Distinct("name").Order("name").Pluck("name", &names).Error
	return names, err
}

// DeletePackage 删除整个包（全版本）。
func (s *Store) DeletePackage(owner, typ, name string) error {
	res := s.db.Where("owner = ? AND type = ? AND name = ?", owner, typ, name).Delete(&packageRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	_ = s.db.Where("owner = ? AND type = ? AND name = ?", owner, typ, name).Delete(&packageTagRow{}).Error
	return nil
}

// DeletePackageVersion 删除单个版本。
func (s *Store) DeletePackageVersion(owner, typ, name, version string) error {
	res := s.db.Where("owner = ? AND type = ? AND name = ? AND version = ?", owner, typ, name, version).
		Delete(&packageRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetPackageYanked 标记 / 取消标记某版本 yanked（cargo）。
func (s *Store) SetPackageYanked(owner, typ, name, version string, yanked bool) error {
	res := s.db.Model(&packageRow{}).
		Where("owner = ? AND type = ? AND name = ? AND version = ?", owner, typ, name, version).
		Update("yanked", yanked)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetPackageTag 设置包级 tag（npm dist-tags）；tag == "" 删除该 tag。
func (s *Store) SetPackageTag(owner, typ, name, tag, version string) error {
	if tag == "" {
		return nil
	}
	if version == "" {
		return s.db.Where("owner = ? AND type = ? AND name = ? AND tag = ?", owner, typ, name, tag).
			Delete(&packageTagRow{}).Error
	}
	row := packageTagRow{Owner: owner, Type: typ, Name: name, Tag: tag, Version: version}
	return s.db.Where("owner = ? AND type = ? AND name = ? AND tag = ?", owner, typ, name, tag).
		Assign(row).FirstOrCreate(&row).Error
}

// ListPackageTags 列出包的全部 tag。
func (s *Store) ListPackageTags(owner, typ, name string) (map[string]string, error) {
	var rows []packageTagRow
	err := s.db.Where("owner = ? AND type = ? AND name = ?", owner, typ, name).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	tags := map[string]string{}
	for _, r := range rows {
		tags[r.Tag] = r.Version
	}
	return tags, nil
}

// AddPackageAudit 记录包操作审计。
func (s *Store) AddPackageAudit(owner, typ, name, version, action, actor string) error {
	row := packageAuditRow{Owner: owner, Type: typ, Name: name, Version: version,
		Action: action, Actor: actor, CreatedAt: now()}
	return s.db.Create(&row).Error
}

// ListPackageAudit 分页列出命名空间（可按包过滤）的审计记录。
func (s *Store) ListPackageAudit(owner, typ, name string, limit, offset int) ([]PackageAudit, int, error) {
	q := s.db.Model(&packageAuditRow{}).Where("owner = ?", owner)
	if typ != "" {
		q = q.Where("type = ?", typ)
	}
	if name != "" {
		q = q.Where("name = ?", name)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []packageAuditRow
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]PackageAudit, 0, len(rows))
	for _, r := range rows {
		out = append(out, PackageAudit(r))
	}
	return out, int(total), nil
}
