package gitsvc

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// TestMain 拦截 pre-receive hook 调用（hook 以 `测试二进制 pre-receive owner repo`
// 方式执行本二进制；必须放在 *_test.go 中才会被 go test 识别)。测试环境无保护规则，
// 校验放行即可（CheckBranchProtection 在库不可用时也放行）。
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
	os.Exit(m.Run())
}
