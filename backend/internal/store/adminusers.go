package store

// 管理面板的用户管理：列表 / 级联删除（admin_users 与普通 users 是两套账号体系）。

import (
	"gorm.io/gorm"
)

// AdminListUsers 列出普通用户：q 为用户名模糊过滤（LIKE %q%），limit/offset 分页。
// 返回 (users, 总数, error)。
func (s *Store) AdminListUsers(q string, limit, offset int) ([]User, int, error) {
	db := s.db.Model(&userRow{})
	if q != "" {
		db = db.Where("username LIKE ?", "%"+q+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []userRow
	if err := db.Order("id ASC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	users := make([]User, 0, len(rows))
	for _, r := range rows {
		users = append(users, User{ID: r.ID, Username: r.Username, Email: r.Email, CreatedAt: r.CreatedAt})
	}
	return users, int(total), nil
}

// AdminDeleteUser 删除用户及其归属数据（会话 / SSH / GPG / PAT / OAuth 绑定 /
// stars / watches / 通知 / 组织成员 / 协作者），先级联删除其名下仓库。
// 用户不存在返回 ErrNotFound。
func (s *Store) AdminDeleteUser(username string) error {
	var u userRow
	if err := s.db.Where("username = ?", username).First(&u).Error; err != nil {
		return notFoundErr(err)
	}
	// 名下仓库走 DeleteRepo 的完整级联（issues/labels/mirrors/pipeline 等）
	var names []string
	if err := s.db.Model(&repoRow{}).Where("owner = ?", username).Pluck("name", &names).Error; err != nil {
		return err
	}
	for _, name := range names {
		if err := s.DeleteRepo(username, name); err != nil {
			return err
		}
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 按 user_id 关联的归属数据
		for _, m := range []any{&sessionRow{}, &sshKeyRow{}, &gpgKeyRow{}, &patRow{}, &userOAuthRow{}} {
			if err := tx.Where("user_id = ?", u.ID).Delete(m).Error; err != nil {
				return err
			}
		}
		// 按用户名关联的归属数据
		for _, m := range []any{&starRow{}, &watchRow{}, &notificationRow{}, &orgMemberRow{}, &collabRow{}} {
			if err := tx.Where("username = ?", username).Delete(m).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&userRow{}, u.ID).Error
	})
}
