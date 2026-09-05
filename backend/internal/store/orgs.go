package store

import "errors"

func (s *Store) IsOrg(name string) bool {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM orgs WHERE name = ?`, name).Scan(&one)
	return err == nil
}

func (s *Store) CreateOrg(name, display, creator string) (Org, error) {
	if _, err := s.GetByUsername(name); err == nil {
		return Org{}, ErrExists // 用户名占用
	}
	o := Org{Name: name, Display: display, CreatedAt: now()}
	tx, err := s.db.Begin()
	if err != nil {
		return o, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`INSERT INTO orgs (name, display, created_at) VALUES (?, ?, ?)`, name, display, o.CreatedAt)
	if err != nil {
		if isUniqueErr(err) {
			return o, ErrExists
		}
		return o, err
	}
	o.ID, _ = res.LastInsertId()
	if _, err := tx.Exec(`INSERT INTO org_members (org, username, role, created_at) VALUES (?, ?, 'owner', ?)`,
		name, creator, now()); err != nil {
		return o, err
	}
	return o, tx.Commit()
}

func (s *Store) ListMyOrgs(username string) ([]Org, error) {
	rows, err := s.db.Query(`SELECT o.id, o.name, o.display, o.created_at FROM orgs o
		JOIN org_members m ON m.org = o.name WHERE m.username = ? ORDER BY o.name`, username)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Org{}
	for rows.Next() {
		var o Org
		if err := rows.Scan(&o.ID, &o.Name, &o.Display, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) OrgRole(org, username string) string {
	var role string
	err := s.db.QueryRow(`SELECT role FROM org_members WHERE org = ? AND username = ?`, org, username).Scan(&role)
	if err != nil {
		return ""
	}
	return role
}

func (s *Store) OrgMembers(org string) ([]OrgMember, error) {
	rows, err := s.db.Query(`SELECT org, username, role FROM org_members WHERE org = ? ORDER BY username`, org)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []OrgMember{}
	for rows.Next() {
		var m OrgMember
		if err := rows.Scan(&m.Org, &m.Username, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) AddOrgMember(org, username, role string) error {
	_, err := s.db.Exec(`INSERT INTO org_members (org, username, role, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(org, username) DO UPDATE SET role = excluded.role`, org, username, role, now())
	return err
}

func (s *Store) RemoveOrgMember(org, username string) error {
	res, err := s.db.Exec(`DELETE FROM org_members WHERE org = ? AND username = ?`, org, username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteOrg(org string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var cnt int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM repos WHERE owner = ?`, org).Scan(&cnt); err != nil {
		return err
	}
	if cnt > 0 {
		return errors.New("org not empty")
	}
	if _, err := tx.Exec(`DELETE FROM org_members WHERE org = ?`, org); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM orgs WHERE name = ?`, org); err != nil {
		return err
	}
	return tx.Commit()
}
