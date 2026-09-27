package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestUploadStore(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("\x89PNG\r\n\x1a\nfake")
	up, err := s.CreateUpload("pixel.png", "image/png", data, "alice")
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	if up.Key == "" || up.URL != "/api/uploads/"+up.Key || up.Size != int64(len(data)) {
		t.Fatalf("upload = %+v", up)
	}

	got, ct, err := s.GetUploadData(up.Key)
	if err != nil || ct != "image/png" || string(got) != string(data) {
		t.Fatalf("get upload = %v, %q, %v", got, ct, err)
	}
	if _, _, err := s.GetUploadData("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing upload err = %v", err)
	}
}
