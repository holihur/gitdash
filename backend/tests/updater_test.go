package tests

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"gitdash/backend/internal/updater"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.1.0", "v0.1.0", 0},
		{"0.1.0", "v0.1.0", 0},
		{"v0.2.0", "v0.1.9", 1},
		{"v0.1.10", "v0.1.9", 1},
		{"v1.0.0", "v0.9.9", 1},
		{"v0.1", "v0.1.0", 0},
		{"v1.0.0-rc1", "v0.9.9", 1},
	}
	for _, c := range cases {
		if got := updater.CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("archive bytes")
	sum := sha256.Sum256(data)
	hexSum := hex.EncodeToString(sum[:])
	name := "gitdash_0.1.0_linux_amd64.tar.gz"
	sums := fmt.Sprintf("%s  %s\n%s  other.tar.gz\n", hexSum, name, strings.Repeat("a", 64))

	if err := updater.VerifyChecksum(data, []byte(sums), name); err != nil {
		t.Fatalf("verify ok: %v", err)
	}
	if err := updater.VerifyChecksum([]byte("tampered"), []byte(sums), name); err == nil {
		t.Fatal("tampered data accepted")
	}
	if err := updater.VerifyChecksum(data, []byte(strings.Repeat("b", 64)+"  "+name+"\n"), name); err == nil {
		t.Fatal("wrong checksum accepted")
	}
	if err := updater.VerifyChecksum(data, []byte(hexSum+"  unknown-file\n"), name); err == nil {
		t.Fatal("missing filename accepted")
	}
}

func TestExtractBinary(t *testing.T) {
	// tar.gz
	var tarBuf bytes.Buffer
	gz := gzip.NewWriter(&tarBuf)
	tw := tar.NewWriter(gz)
	mustTar(t, tw, "gitdash", "TARBIN")
	mustTar(t, tw, "README.md", "readme")
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	bin, err := updater.ExtractBinary(tarBuf.Bytes(), "linux", "gitdash")
	if err != nil || string(bin) != "TARBIN" {
		t.Fatalf("tar extract = %q, %v", bin, err)
	}
	if _, err := updater.ExtractBinary(tarBuf.Bytes(), "linux", "gitdash.exe"); err == nil {
		t.Fatal("expected missing binary error")
	}

	// zip
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	mustZip(t, zw, "gitdash.exe", "ZIPBIN")
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	bin, err = updater.ExtractBinary(zipBuf.Bytes(), "windows", "gitdash.exe")
	if err != nil || string(bin) != "ZIPBIN" {
		t.Fatalf("zip extract = %q, %v", bin, err)
	}
}

func mustTar(t *testing.T, tw *tar.Writer, name, content string) {
	t.Helper()
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
}

func mustZip(t *testing.T, zw *zip.Writer, name, content string) {
	t.Helper()
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
}
