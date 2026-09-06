package store

import "errors"

func commentToDTO(r commentRow) Comment {
	return Comment(r)
}

// CreateComment 在 issue/PR（kind: "issue"|"pull"）下新增一条评论。
func (s *Store) CreateComment(owner, repo, kind string, number int64, author, body string) (Comment, error) {
	if kind != "issue" && kind != "pull" {
		return Comment{}, errors.New("invalid kind")
	}
	if body == "" {
		return Comment{}, errors.New("empty body")
	}
	r := commentRow{
		Owner: owner, Repo: repo, Kind: kind, Number: number,
		Author: author, Body: body,
		CreatedAt: now(), UpdatedAt: now(),
	}
	if err := s.db.Create(&r).Error; err != nil {
		return Comment{}, err
	}
	return commentToDTO(r), nil
}

// ListComments 按 ID 升序列出评论（最早的在前）；limit<=0 表示不限制。
func (s *Store) ListComments(owner, repo, kind string, number int64, limit, offset int) ([]Comment, error) {
	if kind != "issue" && kind != "pull" {
		return nil, errors.New("invalid kind")
	}
	q := s.db.Model(&commentRow{}).
		Where("owner = ? AND repo = ? AND kind = ? AND number = ?", owner, repo, kind, number).
		Order("id ASC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	var rows []commentRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	comments := []Comment{}
	for _, r := range rows {
		comments = append(comments, commentToDTO(r))
	}
	return comments, nil
}

// GetComment 按 ID 取单条评论（用于权限校验）。
func (s *Store) GetComment(owner, repo string, id int64) (Comment, error) {
	var r commentRow
	if err := s.db.Where("owner = ? AND repo = ? AND id = ?", owner, repo, id).
		First(&r).Error; err != nil {
		return Comment{}, notFoundErr(err)
	}
	return commentToDTO(r), nil
}

// DeleteComment 删除评论（返回是否存在）。
func (s *Store) DeleteComment(owner, repo string, id int64) error {
	res := s.db.Where("owner = ? AND repo = ? AND id = ?", owner, repo, id).
		Delete(&commentRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
