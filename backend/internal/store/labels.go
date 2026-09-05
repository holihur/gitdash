package store

import (
	"database/sql"
	"errors"
	"fmt"
)

func (s *Store) CreateLabel(owner, repo, name, color string) (Label, error) {
	l := Label{Owner: owner, Repo: repo, Name: name, Color: color, CreatedAt: now()}
	res, err := s.db.Exec(`INSERT INTO repo_labels (owner, repo, name, color, created_at) VALUES (?,?,?,?,?)`,
		owner, repo, name, color, l.CreatedAt)
	if err != nil {
		if isUniqueErr(err) {
			return l, ErrExists
		}
		return l, err
	}
	l.ID, _ = res.LastInsertId()
	return l, nil
}

func (s *Store) ListLabels(owner, repo string) ([]Label, error) {
	rows, err := s.db.Query(`SELECT id, owner, repo, name, color, created_at
		FROM repo_labels WHERE owner = ? AND repo = ? ORDER BY name`, owner, repo)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Label{}
	for rows.Next() {
		var l Label
		if err := rows.Scan(&l.ID, &l.Owner, &l.Repo, &l.Name, &l.Color, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) UpdateLabel(owner, repo string, id int64, name, color string) (Label, error) {
	res, err := s.db.Exec(`UPDATE repo_labels SET name = ?, color = ? WHERE id = ? AND owner = ? AND repo = ?`,
		name, color, id, owner, repo)
	if err != nil {
		return Label{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Label{}, ErrNotFound
	}
	rows, err := s.db.Query(`SELECT id, owner, repo, name, color, created_at FROM repo_labels WHERE id = ?`, id)
	if err != nil {
		return Label{}, err
	}
	defer func() { _ = rows.Close() }()
	var l Label
	if rows.Next() {
		_ = rows.Scan(&l.ID, &l.Owner, &l.Repo, &l.Name, &l.Color, &l.CreatedAt)
	}
	return l, rows.Err()
}

func (s *Store) DeleteLabel(owner, repo string, id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM issue_labels WHERE label_id = ? AND issue_id IN
		(SELECT id FROM issues WHERE owner = ? AND repo = ?)`, id, owner, repo); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM repo_labels WHERE id = ? AND owner = ? AND repo = ?`, id, owner, repo)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// SetIssueLabels 全量替换 issue 标签（校验标签属于该仓库）。
func (s *Store) SetIssueLabels(owner, repo string, number int64, labelIDs []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var issueID int64
	err = tx.QueryRow(`SELECT id FROM issues WHERE owner = ? AND repo = ? AND number = ?`, owner, repo, number).Scan(&issueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	for _, id := range labelIDs {
		var one int
		if err := tx.QueryRow(`SELECT 1 FROM repo_labels WHERE id = ? AND owner = ? AND repo = ?`, id, owner, repo).Scan(&one); err != nil {
			return fmt.Errorf("label %d does not belong to this repository", id)
		}
	}
	if _, err := tx.Exec(`DELETE FROM issue_labels WHERE issue_id = ?`, issueID); err != nil {
		return err
	}
	for _, id := range labelIDs {
		if _, err := tx.Exec(`INSERT INTO issue_labels (issue_id, label_id) VALUES (?, ?)`, issueID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// IssueLabels 返回若干 issue（number）的标签映射。
func (s *Store) IssueLabels(owner, repo string, numbers []int64) (map[int64][]Label, error) {
	out := map[int64][]Label{}
	if len(numbers) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(`SELECT i.number, l.id, l.name, l.color FROM issue_labels il
		JOIN issues i ON i.id = il.issue_id
		JOIN repo_labels l ON l.id = il.label_id
		WHERE i.owner = ? AND i.repo = ?`, owner, repo)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var num, id int64
		var name, color string
		if err := rows.Scan(&num, &id, &name, &color); err != nil {
			return nil, err
		}
		out[num] = append(out[num], Label{ID: id, Name: name, Color: color})
	}
	return out, rows.Err()
}
