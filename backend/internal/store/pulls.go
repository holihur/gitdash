package store

import (
	"database/sql"
	"errors"
)

func (s *Store) getPull(owner, repo string, number int64) (PullRequest, error) {
	var pr PullRequest
	var ma sql.NullString
	err := s.db.QueryRow(`SELECT id, owner, repo, number, title, body, source_branch, target_branch,
		base_sha, head_sha, state, author, created_at, updated_at, merged_at, merged_by
		FROM pull_requests WHERE owner = ? AND repo = ? AND number = ?`, owner, repo, number).
		Scan(&pr.ID, &pr.Owner, &pr.Repo, &pr.Number, &pr.Title, &pr.Body, &pr.SourceBranch,
			&pr.TargetBranch, &pr.BaseSHA, &pr.HeadSHA, &pr.State, &pr.Author, &pr.CreatedAt,
			&pr.UpdatedAt, &ma, &pr.MergedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return pr, ErrNotFound
	}
	if ma.Valid {
		v := ma.String
		pr.MergedAt = &v
	}
	return pr, err
}

func (s *Store) CreatePull(owner, repo, author, title, body, source, target, baseSHA, headSHA string) (PullRequest, error) {
	var pr PullRequest
	now := now()
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		res, e := s.db.Exec(`INSERT INTO pull_requests (owner, repo, number, title, body, source_branch,
			target_branch, base_sha, head_sha, state, author, created_at, updated_at)
			VALUES (?, ?, (SELECT COALESCE(MAX(number),0)+1 FROM pull_requests WHERE owner=? AND repo=?),
			?, ?, ?, ?, ?, ?, 'open', ?, ?, ?)`,
			owner, repo, owner, repo, title, body, source, target, baseSHA, headSHA, author, now, now)
		if e == nil {
			id, _ := res.LastInsertId()
			pr = PullRequest{ID: id, Owner: owner, Repo: repo, Title: title, Body: body,
				SourceBranch: source, TargetBranch: target, BaseSHA: baseSHA, HeadSHA: headSHA,
				State: "open", Author: author, CreatedAt: now, UpdatedAt: now}
			if err := s.db.QueryRow(`SELECT number FROM pull_requests WHERE id = ?`, id).Scan(&pr.Number); err != nil {
				return pr, err
			}
			return pr, nil
		}
		if !isUniqueErr(e) {
			return pr, e
		}
		err = e
	}
	return pr, err
}

// GetPullIssue 按仓库内序号取 issue（供 label/milestone 更新后回读）。
func (s *Store) GetPullIssue(owner, repo string, number int64) (Issue, error) {
	return s.getIssue(owner, repo, number)
}

func (s *Store) GetPull(owner, repo string, number int64) (PullRequest, error) {
	return s.getPull(owner, repo, number)
}

func (s *Store) ListPulls(owner, repo, state string) ([]PullRequest, error) {
	q := `SELECT id, owner, repo, number, title, body, source_branch, target_branch,
		base_sha, head_sha, state, author, created_at, updated_at, merged_at, merged_by
		FROM pull_requests WHERE owner = ? AND repo = ?`
	args := []any{owner, repo}
	if state != "" {
		q += ` AND state = ?`
		args = append(args, state)
	}
	q += ` ORDER BY (state = 'open') DESC, number DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []PullRequest{}
	for rows.Next() {
		var pr PullRequest
		var ma sql.NullString
		if err := rows.Scan(&pr.ID, &pr.Owner, &pr.Repo, &pr.Number, &pr.Title, &pr.Body,
			&pr.SourceBranch, &pr.TargetBranch, &pr.BaseSHA, &pr.HeadSHA, &pr.State, &pr.Author,
			&pr.CreatedAt, &pr.UpdatedAt, &ma, &pr.MergedBy); err != nil {
			return nil, err
		}
		if ma.Valid {
			v := ma.String
			pr.MergedAt = &v
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

// SetPullState 关闭/重开（不改变 merged 状态）。
func (s *Store) SetPullState(owner, repo string, number int64, state string) (PullRequest, error) {
	res, err := s.db.Exec(`UPDATE pull_requests SET state = ?, updated_at = ? WHERE owner = ? AND repo = ? AND number = ?`,
		state, now(), owner, repo, number)
	if err != nil {
		return PullRequest{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return PullRequest{}, ErrNotFound
	}
	return s.getPull(owner, repo, number)
}

// MarkPullMerged 记录合并结果（fast-forward 后 target 指向 headSHA）。
func (s *Store) MarkPullMerged(owner, repo string, number int64, headSHA, mergedBy string) (PullRequest, error) {
	res, err := s.db.Exec(`UPDATE pull_requests SET state = 'merged', head_sha = ?, merged_by = ?, merged_at = ?, updated_at = ?
		WHERE owner = ? AND repo = ? AND number = ?`, headSHA, mergedBy, now(), now(), owner, repo, number)
	if err != nil {
		return PullRequest{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return PullRequest{}, ErrNotFound
	}
	return s.getPull(owner, repo, number)
}

// ---- issue labels & milestones ----
