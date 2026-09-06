package tests

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitdash/backend/internal/gitsvc"
)

// TestMain 拦截 pre-receive hook 调用：仓库 hooks 里的 pre-receive 会以
// `测试二进制 pre-receive owner repo` 方式调用（os.Executable() 即本测试二进制），
// 这里转给 gitsvc.CheckBranchProtection 执行分支保护校验。
// 注意：TestMain 必须放在 *_test.go 文件里才会被 go test 识别
// (放在普通 .go 文件里会被当作未引用函数被裁掉，hook 退化成跑整个测试套件）。
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "pre-receive" && len(os.Args) >= 4 {
		owner, repo := os.Args[2], os.Args[3]
		var refs []gitsvc.PushRef
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			f := strings.Fields(sc.Text())
			if len(f) == 3 {
				refs = append(refs, gitsvc.PushRef{Old: f[0], New: f[1], Ref: f[2]})
			}
		}
		dbPath := os.Getenv("GITDASH_DB")
		if dbPath == "" {
			dbPath = filepath.Join(os.Getenv("GITDASH_DATA"), "test.db")
		}
		// 子进程未跑过 gitsvc.Init，补上以便 RepoPath/merge-base 用绝对路径
		if d := os.Getenv("GITDASH_DATA"); d != "" {
			_ = gitsvc.Init(d)
		}
		if err := gitsvc.CheckBranchProtection(dbPath, owner, repo, refs); err != nil {
			fmt.Fprintln(os.Stderr, "gitdash:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
