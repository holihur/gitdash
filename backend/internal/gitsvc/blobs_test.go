package gitsvc

import "testing"

func TestListTreeFilesAndReadBlobsBatch(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := CreateBare("alice", "src"); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteCommit("alice", "src", "main", "init", "alice", []FileChange{
		{Path: "a.txt", Action: "create", Content: "hello\nworld\n"},
		{Path: "src/b.go", Action: "create", Content: "package b\n"},
		{Path: "bin.dat", Action: "create", Content: "a\x00b"},
	}); err != nil {
		t.Fatal(err)
	}

	files, err := ListTreeFiles("alice", "src", "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("ListTreeFiles = %+v, want 3 entries", files)
	}
	// 按路径升序：a.txt, bin.dat, src/b.go
	wantOrder := []string{"a.txt", "bin.dat", "src/b.go"}
	for i, f := range files {
		if f.Path != wantOrder[i] {
			t.Fatalf("files[%d].Path = %q, want %q (%+v)", i, f.Path, wantOrder[i], files)
		}
	}
	if files[0].Size != int64(len("hello\nworld\n")) {
		t.Fatalf("a.txt size = %d", files[0].Size)
	}

	got, err := ReadBlobsBatch("alice", "src", "main", []string{"a.txt", "src/b.go", "missing.txt"}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if string(got["a.txt"]) != "hello\nworld\n" {
		t.Fatalf("a.txt = %q", got["a.txt"])
	}
	if string(got["src/b.go"]) != "package b\n" {
		t.Fatalf("src/b.go = %q", got["src/b.go"])
	}
	if _, ok := got["missing.txt"]; ok {
		t.Fatal("missing.txt should be absent")
	}
}

// 超过 maxSize 的对象应被跳过，但流不能错位（后续对象仍能正确读取）。
func TestReadBlobsBatchSkipsOversized(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := CreateBare("alice", "src"); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteCommit("alice", "src", "main", "init", "alice", []FileChange{
		{Path: "big.txt", Action: "create", Content: "0123456789abcdef"},
		{Path: "small.txt", Action: "create", Content: "ok"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadBlobsBatch("alice", "src", "main", []string{"big.txt", "small.txt"}, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["big.txt"]; ok {
		t.Fatal("big.txt over maxSize should be skipped")
	}
	if string(got["small.txt"]) != "ok" {
		t.Fatalf("small.txt = %q (stream desynced?)", got["small.txt"])
	}
}
