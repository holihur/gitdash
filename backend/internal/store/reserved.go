package store

import "strings"

// IsReservedName 报告某个用户名是否在保留名单中（大小写不敏感）。
func (s *Store) IsReservedName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	var n int64
	if err := s.db.Model(&reservedNameRow{}).Where("name = ?", name).Count(&n).Error; err != nil {
		return false
	}
	return n > 0
}

// ListReservedNames 返回全部保留用户名（按名称升序）。
func (s *Store) ListReservedNames() ([]string, error) {
	var rows []reservedNameRow
	if err := s.db.Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Name)
	}
	return out, nil
}

// AddReservedName 新增一个保留用户名；已存在返回 ErrExists。
func (s *Store) AddReservedName(name, by string) error {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ErrNotFound
	}
	row := reservedNameRow{Name: name, CreatedBy: by, CreatedAt: now()}
	if err := s.db.Create(&row).Error; err != nil {
		if isUniqueErr(err) {
			return ErrExists
		}
		return err
	}
	return nil
}

// DeleteReservedName 移除一个保留用户名；不存在返回 ErrNotFound。
func (s *Store) DeleteReservedName(name string) error {
	name = strings.ToLower(strings.TrimSpace(name))
	res := s.db.Where("name = ?", name).Delete(&reservedNameRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
