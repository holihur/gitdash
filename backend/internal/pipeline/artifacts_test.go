package pipeline

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestParseArtifactsBlock(t *testing.T) {
	cfg, err := Parse([]byte("artifacts:\n  paths:\n    - dist\n    - bin/app\nsteps:\n  - run: x\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cfg.ArtifactPaths) != 2 || cfg.ArtifactPaths[0] != "dist" || cfg.ArtifactPaths[1] != "bin/app" {
		t.Fatalf("paths = %v", cfg.ArtifactPaths)
	}
}

func TestParseArtifactsRejects(t *testing.T) {
	if _, err := Parse([]byte("artifacts:\n  paths: [a]\nsteps:\n  - run: x\n")); err == nil {
		t.Fatal("inline artifacts paths should fail")
	}
	if _, err := Parse([]byte("artifacts:\nsteps:\n  - run: x\n")); err == nil {
		t.Fatal("artifacts without paths should fail")
	}
	if _, err := Parse([]byte("artifacts:\n  paths:\n    - /etc/x\nsteps:\n  - run: x\n")); err == nil {
		t.Fatal("absolute artifact path should fail")
	}
}

func TestArtifactArchiveRoundTrip(t *testing.T) {
	if err := Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}

	job := RunJob{Owner: "alice", Repo: "r", RunID: 7}
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "dist", "app"), []byte("BIN"), 0o644); err != nil {
		t.Fatal(err)
	}

	var log bytes.Buffer
	collectArtifacts(&Config{ArtifactPaths: []string{"dist"}}, job, work, &log)
	if !HasArtifacts("alice", "r", 7) {
		t.Fatalf("expected artifacts; log=%s", log.String())
	}
	files, err := ListArtifacts("alice", "r", 7)
	if err != nil || len(files) != 1 || files[0].Path != "dist/app" {
		t.Fatalf("files=%v err=%v", files, err)
	}

	var buf bytes.Buffer
	if err := WriteArtifactArchive("alice", "r", 7, &buf); err != nil {
		t.Fatalf("archive: %v", err)
	}
	gr, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gr)
	found := false
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Name == "dist/app" {
			b, _ := io.ReadAll(tr)
			if string(b) != "BIN" {
				t.Fatalf("content = %q", b)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("dist/app missing from archive")
	}
}

func TestRemoteArtifactArchiveRoundTrip(t *testing.T) {
	if err := Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "out", "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := BuildArtifactArchive(&Config{ArtifactPaths: []string{"out"}}, work)
	if err != nil || len(data) == 0 {
		t.Fatalf("build: err=%v len=%d", err, len(data))
	}
	if err := ExtractArtifactArchive("alice", "r", 9, bytes.NewReader(data)); err != nil {
		t.Fatalf("extract: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(ArtifactDir("alice", "r", 9), "out", "a.txt"))
	if err != nil || string(b) != "hello" {
		t.Fatalf("content=%q err=%v", b, err)
	}
	// 未匹配到路径时返回 nil 归档
	if d, err := BuildArtifactArchive(&Config{ArtifactPaths: []string{"nope"}}, work); err != nil || d != nil {
		t.Fatalf("expected nil archive, len=%d err=%v", len(d), err)
	}
}

func TestPipelineArtifactsEndToEnd(t *testing.T) {
	dir := t.TempDir()
	st := openPipelineStore(t, dir)
	SetHostAllowed(true)
	t.Cleanup(func() { SetHostAllowed(false) })

	sha := seedRepoFiles(t, "alice", "art", map[string]string{
		".gitdash.yml": "artifacts:\n  paths:\n    - out\nsteps:\n  - name: build\n    run: mkdir -p out && echo hello > out/hello.txt\n",
	})
	if err := st.SetPipeline("alice", "art", true); err != nil {
		t.Fatal(err)
	}
	run, err := Trigger(st, TriggerOpts{Owner: "alice", Repo: "art", SHA: sha, Ref: "main", By: "tester", Event: "manual", Force: true})
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	waitRunStatus(t, st, "alice", "art", run.ID, "success")
	if !HasArtifacts("alice", "art", run.ID) {
		t.Fatal("expected artifacts after successful run")
	}
	files, err := ListArtifacts("alice", "art", run.ID)
	if err != nil || len(files) != 1 || files[0].Path != "out/hello.txt" {
		t.Fatalf("files = %v err=%v", files, err)
	}
}
