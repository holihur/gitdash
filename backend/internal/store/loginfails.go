package store

import "time"

// 登录失败限速的持久化实现（原内存 map 重启/多实例后失效，且无限增长）。

type loginFailRow struct {
	Key   string `gorm:"primaryKey;size:255"` // username|ip
	Count int    `gorm:"not null"`
	Until string `gorm:"not null;index"`
}

func (loginFailRow) TableName() string { return "login_fails" }

// RateBlocked 判断 key 是否已被限流（窗口内失败次数 >= maxFails）。
func (s *Store) RateBlocked(key string, maxFails int) (bool, error) {
	var r loginFailRow
	if err := s.db.Where("`key` = ?", key).First(&r).Error; err != nil {
		return false, nil //nolint:nilerr // 无记录 = 未限流，查询失败按未限流处理
	}
	if until, e := time.Parse(time.RFC3339, r.Until); e == nil && time.Now().After(until) {
		_ = s.db.Delete(&loginFailRow{}, r.Key).Error // 窗口已过，惰性清理
		return false, nil
	}
	return r.Count >= maxFails, nil
}

// RateFail 记录一次失败：窗口内累加；窗口过期则重新开始计时。
func (s *Store) RateFail(key string, window time.Duration) error {
	var r loginFailRow
	err := s.db.Where("`key` = ?", key).First(&r).Error
	if err == nil {
		if until, e := time.Parse(time.RFC3339, r.Until); e == nil && time.Now().After(until) {
			r.Count, r.Until = 0, ""
		}
	}
	r.Key = key
	r.Count++
	if r.Until == "" {
		r.Until = time.Now().Add(window).UTC().Format(time.RFC3339)
	}
	return s.db.Save(&r).Error
}

// CleanupLoginFails 清理过期（Until 已超 olderThan 窗口）的限流行，返回删除行数。
// Until 以 RFC3339 UTC 字符串存储，字典序比较与 SQLite/PG 文本排序一致。
func (s *Store) CleanupLoginFails(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan).UTC().Format(time.RFC3339)
	res := s.db.Where("until < ?", cutoff).Delete(&loginFailRow{})
	return res.RowsAffected, res.Error
}

// RateReset 成功登录后清除失败记录。
func (s *Store) RateReset(key string) error {
	return s.db.Where("`key` = ?", key).Delete(&loginFailRow{}).Error
}
