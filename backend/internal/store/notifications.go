package store

import (
	"fmt"
	"strings"
)

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

func (s *Store) AddNotification(username, kind, action, owner, repo string, number int64, title, actor string) error {
	_, err := s.db.Exec(`INSERT INTO notifications (username, kind, action, owner, repo, number, title, actor, read, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?)`, username, kind, action, owner, repo, number, title, actor, now())
	return err
}

// AddNotifications 单条多行 INSERT 通知多个收件人（O(1) 次往返，避免逐人写入）。
func (s *Store) AddNotifications(usernames []string, kind, action, owner, repo string, number int64, title, actor string) error {
	if len(usernames) == 0 {
		return nil
	}
	var sb strings.Builder
	args := make([]any, 0, len(usernames)*9)
	for i, u := range usernames {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(?, ?, ?, ?, ?, ?, ?, ?, 0, ?)")
		args = append(args, u, kind, action, owner, repo, number, title, actor, now())
	}
	_, err := s.db.Exec(`INSERT INTO notifications (username, kind, action, owner, repo, number, title, actor, read, created_at)
		VALUES `+sb.String(), args...)
	if err != nil && strings.Contains(err.Error(), "too many SQL variables") {
		// SQLite 变量上限（999）兜底：分批写入
		for _, u := range usernames {
			if e := s.AddNotification(u, kind, action, owner, repo, number, title, actor); e != nil {
				return e
			}
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("notify batch: %w", err)
	}
	return nil
}

// ListNotifications 返回某用户收件箱（最新在前，最多 200 条）。
func (s *Store) ListNotifications(username string) ([]Notification, error) {
	rows, err := s.db.Query(`SELECT id, kind, action, owner, repo, number, title, actor, read, created_at
		FROM notifications WHERE username = ? ORDER BY id DESC LIMIT 200`, username)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		var read int
		if err := rows.Scan(&n.ID, &n.Kind, &n.Action, &n.Owner, &n.Repo, &n.Number, &n.Title, &n.Actor,
			&read, &n.CreatedAt); err != nil {
			return nil, err
		}
		n.Read = read != 0
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) UnreadNotifications(username string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE username = ? AND read = 0`, username).Scan(&n)
	return n, err
}

func (s *Store) MarkNotificationRead(username string, id int64) error {
	res, err := s.db.Exec(`UPDATE notifications SET read = 1 WHERE id = ? AND username = ?`, id, username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) MarkAllNotificationsRead(username string) error {
	_, err := s.db.Exec(`UPDATE notifications SET read = 1 WHERE username = ? AND read = 0`, username)
	return err
}

func (s *Store) DeleteNotification(username string, id int64) error {
	res, err := s.db.Exec(`DELETE FROM notifications WHERE id = ? AND username = ?`, id, username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- forks ----
