package store

import "time"

// 数据保留策略（GDPR Art. 5(1)(e) storage limitation）：
// 过期会话与已读旧通知由 main 的后台清理循环定期删除。

// PruneSessions 删除已过期会话，返回删除行数。
func (s *Store) PruneSessions() (int64, error) {
	res := s.db.Where("expires_at < ?", now()).Delete(&sessionRow{})
	return res.RowsAffected, res.Error
}

// PruneNotifications 删除超过保留期（默认 90 天）的已读通知，返回删除行数。
func (s *Store) PruneNotifications(retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention).UTC().Format(time.RFC3339)
	res := s.db.Where("read = ? AND created_at < ?", true, cutoff).Delete(&notificationRow{})
	return res.RowsAffected, res.Error
}
