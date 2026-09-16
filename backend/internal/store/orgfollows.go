package store

// ---- org_follows（用户关注组织） ----

// FollowOrg 建立 follower -> org 的关注关系；重复关注返回 ErrExists。
func (s *Store) FollowOrg(follower, org string) error {
	row := orgFollowRow{Follower: follower, Org: org, CreatedAt: now()}
	if err := s.db.Create(&row).Error; err != nil {
		if isUniqueErr(err) {
			return ErrExists
		}
		return err
	}
	return nil
}

// UnfollowOrg 取消关注（幂等）。
func (s *Store) UnfollowOrg(follower, org string) error {
	return s.db.Where("follower = ? AND org = ?", follower, org).Delete(&orgFollowRow{}).Error
}

// IsFollowingOrg 判断 follower 是否关注了 org。
func (s *Store) IsFollowingOrg(follower, org string) bool {
	var n int64
	err := s.db.Model(&orgFollowRow{}).Where("follower = ? AND org = ?", follower, org).
		Limit(1).Count(&n).Error
	return err == nil && n > 0
}

// OrgFollowerCount 组织粉丝数。
func (s *Store) OrgFollowerCount(org string) (int64, error) {
	var n int64
	err := s.db.Model(&orgFollowRow{}).Where("org = ?", org).Count(&n).Error
	return n, err
}

// ListOrgFollowers 关注 org 的用户列表（最新关注在前）。
func (s *Store) ListOrgFollowers(org string) ([]UserSummary, error) {
	var out []UserSummary
	err := s.db.Table("users").
		Select("users.username, users.created_at").
		Joins("JOIN org_follows f ON f.follower = users.username").
		Where("f.org = ?", org).
		Order("f.created_at DESC").
		Scan(&out).Error
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []UserSummary{}
	}
	return out, nil
}
