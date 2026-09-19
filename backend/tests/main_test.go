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

// TestMain 拦截 pre-receive / post-receive hook 调用：仓库 hooks 里的脚本会以
// `测试二进制 <subcommand> owner repo` 方式调用（os.Executable() 即本测试二进制），
// 这里转给对应的 gitsvc 函数。
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
	if len(os.Args) > 1 && os.Args[1] == "post-receive" && len(os.Args) >= 4 {
		owner, repo := os.Args[2], os.Args[3]
		// 子进程未跑过 gitsvc.Init，补上以定位 spool 目录
		if d := os.Getenv("GITDASH_DATA"); d != "" {
			_ = gitsvc.Init(d)
		}
		user := os.Getenv("GITDASH_USER")
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			f := strings.Fields(sc.Text())
			if len(f) == 3 {
				_ = gitsvc.WritePushEvent(owner, repo, f[0], f[1], f[2], user)
			}
		}
		os.Exit(0)
	}
	// 集成测试默认关闭“注册/建用户自动建同名仓库”，保持用例对仓库数量的确定性。
	// 需要验证该行为的用例用 t.Setenv("GITDASH_PROFILE_REPO", "1") 覆盖（见 profile_repo_test.go）。
	_ = os.Setenv("GITDASH_PROFILE_REPO", "0")
	os.Exit(m.Run())
}
