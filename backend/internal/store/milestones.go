package store

import "fmt"

func (s *Store) CreateMilestone(owner, repo, title, description string) (Milestone, error) {
	m := Milestone{Owner: owner, Repo: repo, Title: title, Description: description, State: "open", CreatedAt: now()}
	res, err := s.db.Exec(`INSERT INTO milestones (owner, repo, title, description, state, created_at) VALUES (?,?,?,?,'open',?)`,
		owner, repo, title, description, m.CreatedAt)
	if err != nil {
		return m, err
	}
	m.ID, _ = res.LastInsertId()
	return m, nil
}

func (s *Store) ListMilestones(owner, repo string) ([]Milestone, error) {
	rows, err := s.db.Query(`SELECT m.id, m.owner, m.repo, m.title, m.description, m.state, m.created_at,
		COALESCE(SUM(CASE WHEN i.state = 'open' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN i.state = 'closed' THEN 1 ELSE 0 END), 0)
		FROM milestones m LEFT JOIN issues i ON i.milestone_id = m.id AND i.owner = m.owner AND i.repo = m.repo
		WHERE m.owner = ? AND m.repo = ? GROUP BY m.id ORDER BY m.title`, owner, repo)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Milestone{}
	for rows.Next() {
		var m Milestone
		if err := rows.Scan(&m.ID, &m.Owner, &m.Repo, &m.Title, &m.Description, &m.State, &m.CreatedAt,
			&m.OpenIssues, &m.ClosedIssues); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) UpdateMilestone(owner, repo string, id int64, title, description, state string) (Milestone, error) {
	if title != "" {
		if _, err := s.db.Exec(`UPDATE milestones SET title = ? WHERE id = ? AND owner = ? AND repo = ?`,
			title, id, owner, repo); err != nil {
			return Milestone{}, err
		}
	}
	if description != "" {
		if _, err := s.db.Exec(`UPDATE milestones SET description = ? WHERE id = ? AND owner = ? AND repo = ?`,
			description, id, owner, repo); err != nil {
			return Milestone{}, err
		}
	}
	if state != "" {
		if _, err := s.db.Exec(`UPDATE milestones SET state = ? WHERE id = ? AND owner = ? AND repo = ?`,
			state, id, owner, repo); err != nil {
			return Milestone{}, err
		}
	}
	res, err := s.db.Exec(`UPDATE milestones SET title = title WHERE id = ? AND owner = ? AND repo = ?`, id, owner, repo)
	if err != nil {
		return Milestone{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 && title == "" && description == "" && state == "" {
		return Milestone{}, ErrNotFound
	}
	// 重新读取
	list, err := s.ListMilestones(owner, repo)
	if err != nil {
		return Milestone{}, err
	}
	for _, m := range list {
		if m.ID == id {
			return m, nil
		}
	}
	return Milestone{}, ErrNotFound
}

func (s *Store) DeleteMilestone(owner, repo string, id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE issues SET milestone_id = NULL WHERE owner = ? AND repo = ? AND milestone_id = ?`,
		owner, repo, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM milestones WHERE id = ? AND owner = ? AND repo = ?`, id, owner, repo)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// SetIssueMilestone 设置/清除 issue 里程碑（返回是否属于仓库校验）。
func (s *Store) SetIssueMilestone(owner, repo string, number, milestoneID int64) error {
	var issueID int64
	err := s.db.QueryRow(`SELECT id FROM issues WHERE owner = ? AND repo = ? AND number = ?`, owner, repo, number).Scan(&issueID)
	if err != nil {
		return ErrNotFound
	}
	if milestoneID == 0 {
		_, err := s.db.Exec(`UPDATE issues SET milestone_id = NULL WHERE id = ?`, issueID)
		return err
	}
	var one int
	if err := s.db.QueryRow(`SELECT 1 FROM milestones WHERE id = ? AND owner = ? AND repo = ?`, milestoneID, owner, repo).Scan(&one); err != nil {
		return fmt.Errorf("milestone does not belong to this repository")
	}
	_, err = s.db.Exec(`UPDATE issues SET milestone_id = ? WHERE id = ?`, milestoneID, issueID)
	return err
}

// IssueMilestones 返回 issue number -> 所属里程碑（精简字段）。
func (s *Store) IssueMilestones(owner, repo string, numbers []int64) (map[int64]Milestone, error) {
	out := map[int64]Milestone{}
	if len(numbers) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(`SELECT i.number, m.id, m.title, m.state FROM issues i
		JOIN milestones m ON m.id = i.milestone_id
		WHERE i.owner = ? AND i.repo = ? AND i.milestone_id IS NOT NULL`, owner, repo)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var num, id int64
		var title, state string
		if err := rows.Scan(&num, &id, &title, &state); err != nil {
			return nil, err
		}
		out[num] = Milestone{ID: id, Title: title, State: state}
	}
	return out, rows.Err()
}

// ---- admin & settings & oauth ----
