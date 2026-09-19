package gitsvc

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// TestMain 拦截 pre-receive / post-receive hook 调用（hook 以
// `测试二进制 <subcommand> owner repo` 方式执行本二进制；必须放在 *_test.go 中
// 才会被 go test 识别)。测试环境无保护规则，pre-receive 校验放行即可；
// post-receive 写入 push 事件 spool。
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "pre-receive" && len(os.Args) >= 4 {
		var refs []PushRef
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			f := strings.Fields(sc.Text())
			if len(f) == 3 {
				refs = append(refs, PushRef{Old: f[0], New: f[1], Ref: f[2]})
			}
		}
		_ = CheckBranchProtection(os.Getenv("GITDASH_DB"), os.Args[2], os.Args[3], refs)
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "post-receive" && len(os.Args) >= 4 {
		// 子进程未跑过 Init，补上以定位 spool 目录
		if d := os.Getenv("GITDASH_DATA"); d != "" {
			_ = Init(d)
		}
		user := os.Getenv("GITDASH_USER")
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			f := strings.Fields(sc.Text())
			if len(f) == 3 {
				_ = WritePushEvent(os.Args[2], os.Args[3], f[0], f[1], f[2], user)
			}
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
