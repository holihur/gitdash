package store

import "gorm.io/gorm"

// RefNoteKind 备注关联的引用类型。
type RefNoteKind = string

const (
	RefNoteBranch RefNoteKind = "branch"
	RefNoteTag    RefNoteKind = "tag"
)

// RefNotes 批量读取仓库所有分支/标签备注，返回 "kind/name" -> note。
func (s *Store) RefNotes(owner, repo string) (map[string]string, error) {
	var rows []refNoteRow
	if err := s.db.Where("owner = ? AND repo = ?", owner, repo).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Kind+"/"+r.Name] = r.Note
	}
	return out, nil
}

// SetRefNote 设置（新增/覆盖/清空）分支或标签备注。note 为空时删除记录。
func (s *Store) SetRefNote(owner, repo, kind, name, note string) error {
	if note == "" {
		return s.db.Where("owner = ? AND repo = ? AND kind = ? AND name = ?", owner, repo, kind, name).
			Delete(&refNoteRow{}).Error
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var existing refNoteRow
		err := tx.Where("owner = ? AND repo = ? AND kind = ? AND name = ?", owner, repo, kind, name).
			First(&existing).Error
		if err == nil {
			return tx.Model(&existing).Update("note", note).Error
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		return tx.Create(&refNoteRow{
			Owner: owner, Repo: repo, Kind: kind, Name: name,
			Note: note, UpdatedAt: now(),
		}).Error
	})
}

// DeleteRefNote 删除某引用备注（分支/标签删除时清理）。
func (s *Store) DeleteRefNote(owner, repo, kind, name string) error {
	return s.db.Where("owner = ? AND repo = ? AND kind = ? AND name = ?", owner, repo, kind, name).
		Delete(&refNoteRow{}).Error
}
