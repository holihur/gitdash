package store

import (
	"crypto/rand"
	"encoding/hex"
)

// Upload 是 Markdown 编辑器上传的图片 / 附件元数据（不含内容）。
type Upload struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	URL         string `json:"url"`
	CreatedAt   string `json:"created_at"`
}

func toUpload(r uploadRow) Upload {
	return Upload{
		Key:         r.Key,
		Name:        r.Name,
		ContentType: r.ContentType,
		Size:        r.Size,
		URL:         "/api/uploads/" + r.Key,
		CreatedAt:   r.CreatedAt,
	}
}

func randomUploadKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateUpload 保存一个上传文件并返回其元数据（URL 为 /api/uploads/{key}）。
func (s *Store) CreateUpload(name, contentType string, data []byte, uploader string) (Upload, error) {
	key, err := randomUploadKey()
	if err != nil {
		return Upload{}, err
	}
	row := uploadRow{
		Key:         key,
		Name:        name,
		ContentType: contentType,
		Size:        int64(len(data)),
		Uploader:    uploader,
		Data:        data,
		CreatedAt:   now(),
	}
	if err := s.db.Create(&row).Error; err != nil {
		return Upload{}, err
	}
	return toUpload(row), nil
}

// GetUploadData 按 key 返回文件内容与 Content-Type；不存在返回 ErrNotFound。
func (s *Store) GetUploadData(key string) ([]byte, string, error) {
	var row uploadRow
	if err := s.db.Select("data", "content_type").Where("key = ?", key).First(&row).Error; err != nil {
		return nil, "", notFoundErr(err)
	}
	ct := row.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	return row.Data, ct, nil
}
