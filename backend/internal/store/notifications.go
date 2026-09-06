package store

// Notification 用户通知（公开 DTO，字段与 JSON 形状保持不变）。
type Notification struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`   // issue | pull
	Action    string `json:"action"` // opened | closed | reopened | merged
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	Number    int64  `json:"number"`
	Title     string `json:"title"`
	Actor     string `json:"actor"`
	Read      bool   `json:"read"`
	CreatedAt string `json:"created_at"`
}

// AddNotification 写入单条通知。
func (s *Store) AddNotification(username, kind, action, owner, repo string, number int64, title, actor string) error {
	row := notificationRow{
		Username: username, Kind: kind, Action: action,
		Owner: owner, Repo: repo, Number: number, Title: title, Actor: actor,
		Read: false, CreatedAt: now(),
	}
	return s.db.Create(&row).Error
}

// AddNotifications 批量写入通知（一次 Create 多行，避免逐人写入）。
func (s *Store) AddNotifications(usernames []string, kind, action, owner, repo string, number int64, title, actor string) error {
	if len(usernames) == 0 {
		return nil
	}
	ts := now()
	rows := make([]notificationRow, 0, len(usernames))
	for _, u := range usernames {
		rows = append(rows, notificationRow{
			Username: u, Kind: kind, Action: action,
			Owner: owner, Repo: repo, Number: number, Title: title, Actor: actor,
			Read: false, CreatedAt: ts,
		})
	}
	return s.db.Create(&rows).Error
}

// ListNotifications 分页返回某用户收件箱（最新在前）；limit<=0 表示不限制。
func (s *Store) ListNotifications(username string, limit, offset int) ([]Notification, error) {
	q := s.db.Where("username = ?", username).Order("id DESC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	var rows []notificationRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Notification, 0, len(rows))
	for _, r := range rows {
		out = append(out, Notification{
			ID: r.ID, Kind: r.Kind, Action: r.Action,
			Owner: r.Owner, Repo: r.Repo, Number: r.Number,
			Title: r.Title, Actor: r.Actor, Read: r.Read, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

// CountNotifications 某用户收件箱通知总数。
func (s *Store) CountNotifications(username string) (int, error) {
	var n int64
	err := s.db.Model(&notificationRow{}).Where("username = ?", username).Count(&n).Error
	return int(n), err
}

// UnreadNotifications 未读通知数。
func (s *Store) UnreadNotifications(username string) (int, error) {
	var n int64
	err := s.db.Model(&notificationRow{}).Where("username = ? AND read = ?", username, false).
		Count(&n).Error
	return int(n), err
}

// MarkNotificationRead 标记单条已读（不存在返回 ErrNotFound）。
func (s *Store) MarkNotificationRead(username string, id int64) error {
	res := s.db.Model(&notificationRow{}).
		Where("id = ? AND username = ?", id, username).
		Update("read", true)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkAllNotificationsRead 全部标记已读。
func (s *Store) MarkAllNotificationsRead(username string) error {
	return s.db.Model(&notificationRow{}).
		Where("username = ? AND read = ?", username, false).
		Update("read", true).Error
}

// DeleteNotification 删除单条通知（不存在返回 ErrNotFound）。
func (s *Store) DeleteNotification(username string, id int64) error {
	res := s.db.Where("id = ? AND username = ?", id, username).Delete(&notificationRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- 通知接收者计算（站内通知与邮件通知共用） ----

// EmailTarget 开启邮件通知的用户（接收者计算结果的投影）。
type EmailTarget struct {
	Username string
	Email    string
}

// NotifyRecipients 计算某仓库动态的通知接收者：
//   - 显式 watch 该仓库的用户
//   - 个人仓库的所有者 / 组织仓库的全部成员
//   - 不包含 actor 本人
func (s *Store) NotifyRecipients(owner, repo, actor string) []string {
	seen := map[string]bool{}
	if watchers, err := s.WatchingUsers(owner, repo); err == nil {
		for _, u := range watchers {
			seen[u] = true
		}
	}
	if s.IsOrg(owner) {
		if members, err := s.OrgMembers(owner); err == nil {
			for _, m := range members {
				seen[m.Username] = true
			}
		}
	} else if owner != "" {
		seen[owner] = true
	}
	delete(seen, actor)
	users := make([]string, 0, len(seen))
	for u := range seen {
		if u != "" {
			users = append(users, u)
		}
	}
	return users
}

// EmailTargets 返回已开启邮件通知且邮箱非空的用户。
func (s *Store) EmailTargets(usernames []string) []EmailTarget {
	if len(usernames) == 0 {
		return nil
	}
	var rows []userRow
	if err := s.db.Where("username IN ? AND email <> '' AND notify_email = ?", usernames, true).
		Find(&rows).Error; err != nil {
		return nil
	}
	out := make([]EmailTarget, 0, len(rows))
	for _, r := range rows {
		out = append(out, EmailTarget{Username: r.Username, Email: r.Email})
	}
	return out
}
