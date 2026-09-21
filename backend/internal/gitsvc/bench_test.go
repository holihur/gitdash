package gitsvc

import (
	"fmt"
	"strings"
	"testing"
)

// benchRepo 构造一个带若干次提交与一个较大文本文件的 bare 仓库（setup 不计入计时）。
func benchRepo(b *testing.B) (owner, name string) {
	b.Helper()
	if err := Init(b.TempDir()); err != nil {
		b.Fatalf("init: %v", err)
	}
	owner, name = "bench", "repo"
	if err := CreateBare(owner, name); err != nil {
		b.Fatalf("create bare: %v", err)
	}
	if err := InitTemplate(owner, name); err != nil {
		b.Fatalf("init template: %v", err)
	}
	var lines []string
	for i := 0; i < 40; i++ {
		lines = append(lines, fmt.Sprintf("line %03d: value %d", i, i))
		action := "create"
		if i > 0 {
			action = "update"
		}
		if _, err := WriteCommit(owner, name, "main",
			fmt.Sprintf("commit %d", i), "bench",
			[]FileChange{{Path: "big.txt", Action: action, Content: strings.Join(lines, "\n") + "\n"}}); err != nil {
			b.Fatalf("write commit %d: %v", i, err)
		}
	}
	return owner, name
}

func BenchmarkCommits(b *testing.B) {
	owner, name := benchRepo(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Commits(owner, name, "main", 50, 0, ""); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBlameFile(b *testing.B) {
	owner, name := benchRepo(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := BlameFile(owner, name, "main", "big.txt"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTree(b *testing.B) {
	owner, name := benchRepo(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Tree(owner, name, "main", ""); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadBlob(b *testing.B) {
	owner, name := benchRepo(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ReadBlob(owner, name, "main", "big.txt"); err != nil {
			b.Fatal(err)
		}
	}
}
