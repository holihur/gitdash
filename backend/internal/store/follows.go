package store

// ---- follows（用户关注） ----

// FollowUser 建立 follower -> followee 的关注关系；重复关注返回 ErrExists。
func (s *Store) FollowUser(follower, followee string) error {
	row := followRow{Follower: follower, Followee: followee, CreatedAt: now()}
	if err := s.db.Create(&row).Error; err != nil {
		if isUniqueErr(err) {
			return ErrExists
		}
		return err
	}
	return nil
}

// UnfollowUser 取消关注（幂等）。
func (s *Store) UnfollowUser(follower, followee string) error {
	return s.db.Where("follower = ? AND followee = ?", follower, followee).Delete(&followRow{}).Error
}

// IsFollowing 判断 follower 是否关注了 followee。
func (s *Store) IsFollowing(follower, followee string) bool {
	var n int64
	err := s.db.Model(&followRow{}).Where("follower = ? AND followee = ?", follower, followee).
		Limit(1).Count(&n).Error
	return err == nil && n > 0
}

// FollowCounts 返回 (followers, following)：被关注数 / 关注数。
func (s *Store) FollowCounts(username string) (followers, following int64, err error) {
	if err = s.db.Model(&followRow{}).Where("followee = ?", username).Count(&followers).Error; err != nil {
		return 0, 0, err
	}
	if err = s.db.Model(&followRow{}).Where("follower = ?", username).Count(&following).Error; err != nil {
		return 0, 0, err
	}
	return followers, following, nil
}

// ListFollowers 关注 username 的用户列表（最新关注在前）。
func (s *Store) ListFollowers(username string) ([]UserSummary, error) {
	var out []UserSummary
	err := s.db.Table("users").
		Select("users.username, users.created_at").
		Joins("JOIN user_follows f ON f.follower = users.username").
		Where("f.followee = ?", username).
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

// ListFollowing username 关注的用户列表（最新关注在前）。
func (s *Store) ListFollowing(username string) ([]UserSummary, error) {
	var out []UserSummary
	err := s.db.Table("users").
		Select("users.username, users.created_at").
		Joins("JOIN user_follows f ON f.followee = users.username").
		Where("f.follower = ?", username).
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
