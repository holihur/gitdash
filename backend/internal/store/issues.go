package store

import (
	"database/sql"
	"errors"
)

func (s *Store) getIssue(owner, repo string, number int64) (Issue, error) {
	var it Issue
	var ca sql.NullString
	err := s.db.QueryRow(`SELECT id, owner, repo, number, title, body, state, author, created_at, updated_at, closed_at
		FROM issues WHERE owner = ? AND repo = ? AND number = ?`, owner, repo, number).
		Scan(&it.ID, &it.Owner, &it.Repo, &it.Number, &it.Title, &it.Body, &it.State, &it.Author,
			&it.CreatedAt, &it.UpdatedAt, &ca)
	if errors.Is(err, sql.ErrNoRows) {
		return it, ErrNotFound
	}
	if ca.Valid {
		v := ca.String
		it.ClosedAt = &v
	}
	return it, err
}

func (s *Store) CreateIssue(owner, repo, author, title, body string) (Issue, error) {
	var it Issue
	var err error
	now := now()
	// 号码在同一仓库内递增；并发冲突时重试（UNIQUE(owner, repo, number)）
	for attempt := 0; attempt < 5; attempt++ {
		res, e := s.db.Exec(`INSERT INTO issues (owner, repo, number, title, body, state, author, created_at, updated_at)
			VALUES (?, ?, (SELECT COALESCE(MAX(number), 0) + 1 FROM issues WHERE owner = ? AND repo = ?), ?, ?, 'open', ?, ?, ?)`,
			owner, repo, owner, repo, title, body, author, now, now)
		if e == nil {
			it.ID, _ = res.LastInsertId()
			it = Issue{ID: it.ID, Owner: owner, Repo: repo, Title: title, Body: body,
				State: "open", Author: author, CreatedAt: now, UpdatedAt: now}
			if err := s.db.QueryRow(`SELECT number FROM issues WHERE id = ?`, it.ID).Scan(&it.Number); err != nil {
				return it, err
			}
			return it, nil
		}
		if !isUniqueErr(e) {
			return it, e
		}
		err = e
	}
	return it, err
}

func (s *Store) ListIssues(owner, repo string) ([]Issue, error) {
	rows, err := s.db.Query(`SELECT id, owner, repo, number, title, body, state, author, created_at, updated_at, closed_at
		FROM issues WHERE owner = ? AND repo = ? ORDER BY (state = 'open') DESC, number DESC`, owner, repo)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	issues := []Issue{}
	for rows.Next() {
		var it Issue
		var ca sql.NullString
		if err := rows.Scan(&it.ID, &it.Owner, &it.Repo, &it.Number, &it.Title, &it.Body, &it.State,
			&it.Author, &it.CreatedAt, &it.UpdatedAt, &ca); err != nil {
			return nil, err
		}
		if ca.Valid {
			v := ca.String
			it.ClosedAt = &v
		}
		issues = append(issues, it)
	}
	return issues, rows.Err()
}

func (s *Store) SetIssueState(owner, repo string, number int64, state string) (Issue, error) {
	if state != "open" && state != "closed" {
		return Issue{}, errors.New("invalid state")
	}
	now := now()
	var closedAt any
	if state == "closed" {
		closedAt = now
	}
	res, err := s.db.Exec(`UPDATE issues SET state = ?, updated_at = ?, closed_at = ?
		WHERE owner = ? AND repo = ? AND number = ?`, state, now, closedAt, owner, repo, number)
	if err != nil {
		return Issue{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Issue{}, ErrNotFound
	}
	return s.getIssue(owner, repo, number)
}

// ---- collaborators ----
