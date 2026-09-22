package store

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// 仓库部署密钥（deploy key）：绑定单个仓库的 SSH 公钥，权限为 read 或 write。
//
// SSH 握手时仅凭公钥指纹定位身份，因此 fingerprint 全局唯一（同一把钥匙不能
// 绑定多个仓库）。命中的身份用合成字符串表示（见 DeployIdentity），用户名语法
// 不含 ':'，不会与真实用户冲突；SSH 网关据此把访问限制到绑定仓库。
const (
	DeployPermissionRead  = "read"
	DeployPermissionWrite = "write"

	deployIdentityPrefix = "deploy:"
)

func deployKeyFromRow(r deployKeyRow) DeployKey {
	return DeployKey{
		ID: r.ID, Owner: r.Owner, Repo: r.Repo, Name: r.Name,
		PublicKey: r.PublicKey, Fingerprint: r.Fingerprint,
		Permission: r.Permission, CreatedAt: r.CreatedAt,
	}
}

// DeployIdentity 构造 deploy key 的合成登录身份："deploy:<owner>/<repo>:<rw>"。
func DeployIdentity(owner, repo string, canWrite bool) string {
	rw := "r"
	if canWrite {
		rw = "w"
	}
	return deployIdentityPrefix + owner + "/" + repo + ":" + rw
}

// ParseDeployIdentity 解析合成身份；非 deploy 身份返回 ok=false。
func ParseDeployIdentity(identity string) (owner, repo string, canWrite, ok bool) {
	if len(identity) <= len(deployIdentityPrefix) || identity[:len(deployIdentityPrefix)] != deployIdentityPrefix {
		return "", "", false, false
	}
	rest := identity[len(deployIdentityPrefix):]
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 {
		return "", "", false, false
	}
	owner = rest[:slash]
	rest = rest[slash+1:]
	colon := strings.LastIndexByte(rest, ':')
	if colon <= 0 {
		return "", "", false, false
	}
	repo = rest[:colon]
	switch rest[colon+1:] {
	case "r":
		canWrite = false
	case "w":
		canWrite = true
	default:
		return "", "", false, false
	}
	return owner, repo, canWrite, true
}

// CreateDeployKey 新增一把仓库部署密钥；指纹已存在时返回 ErrExists。
func (s *Store) CreateDeployKey(owner, repo, name, publicKey, fingerprint, permission string) (DeployKey, error) {
	if permission != DeployPermissionWrite {
		permission = DeployPermissionRead
	}
	row := deployKeyRow{
		Owner: owner, Repo: repo, Name: name, PublicKey: publicKey,
		Fingerprint: fingerprint, Permission: permission, CreatedAt: now(),
	}
	if err := s.db.Create(&row).Error; err != nil {
		if isUniqueErr(err) {
			return DeployKey{}, ErrExists
		}
		return DeployKey{}, err
	}
	return deployKeyFromRow(row), nil
}

// ListDeployKeys 返回仓库的全部部署密钥（不返回私钥——本就没有）。
func (s *Store) ListDeployKeys(owner, repo string) ([]DeployKey, error) {
	var rows []deployKeyRow
	if err := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []DeployKey{}
	for _, r := range rows {
		out = append(out, deployKeyFromRow(r))
	}
	return out, nil
}

// DeleteDeployKey 删除仓库的一把部署密钥；不存在返回 ErrNotFound。
func (s *Store) DeleteDeployKey(owner, repo string, id int64) error {
	res := s.db.Where("id = ? AND owner = ? AND repo = ?", id, owner, repo).Delete(&deployKeyRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteDeployKeysByRepo 删除仓库的全部部署密钥（仓库删除时级联清理）。
func (s *Store) DeleteDeployKeysByRepo(owner, repo string) error {
	return s.db.Where("owner = ? AND repo = ?", owner, repo).Delete(&deployKeyRow{}).Error
}

// matchDeployKey 按指纹匹配部署密钥。
func (s *Store) matchDeployKey(fingerprint string) (DeployKey, bool, error) {
	var row deployKeyRow
	err := s.db.Where("fingerprint = ?", fingerprint).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DeployKey{}, false, nil
	}
	if err != nil {
		return DeployKey{}, false, err
	}
	return deployKeyFromRow(row), true, nil
}
