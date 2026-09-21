package gitsvc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gitdash/backend/internal/authz"
)

// fakeProtAuthorizer 只实现分支保护查询，用于校验 hook 端逻辑。
type fakeProtAuthorizer struct {
	rule authz.BranchProtectionRule
	err  error
}

func (f fakeProtAuthorizer) BranchProtection(context.Context, string, string, string) (authz.BranchProtectionRule, error) {
	return f.rule, f.err
}

func TestCheckBranchProtectionWithAuthorizer(t *testing.T) {
	ctx := context.Background()
	zero := strings.Repeat("0", 40)
	deletion := []PushRef{{Old: strings.Repeat("a", 40), New: zero, Ref: "refs/heads/main"}}
	update := []PushRef{{Old: strings.Repeat("a", 40), New: strings.Repeat("b", 40), Ref: "refs/heads/main"}}

	// 无保护规则 → 放行
	if err := CheckBranchProtectionWithAuthorizer(ctx, fakeProtAuthorizer{}, "alice", "demo", deletion); err != nil {
		t.Fatalf("no rule should allow deletion: %v", err)
	}

	// 禁止删除 → 拒绝
	blockDel := authz.BranchProtectionRule{Protected: true, BlockDeletion: true}
	if err := CheckBranchProtectionWithAuthorizer(ctx, fakeProtAuthorizer{rule: blockDel}, "alice", "demo", deletion); err == nil {
		t.Fatal("protected branch deletion should be rejected")
	}

	// 允许删除 → 放行
	allowDel := authz.BranchProtectionRule{Protected: true, BlockDeletion: false}
	if err := CheckBranchProtectionWithAuthorizer(ctx, fakeProtAuthorizer{rule: allowDel}, "alice", "demo", deletion); err != nil {
		t.Fatalf("deletion allowed when BlockDeletion=false: %v", err)
	}

	// 禁止强推：无法确认快进（仓库不存在）→ 拒绝
	blockForce := authz.BranchProtectionRule{Protected: true, BlockForcePush: true}
	if err := CheckBranchProtectionWithAuthorizer(ctx, fakeProtAuthorizer{rule: blockForce}, "alice", "demo", update); err == nil {
		t.Fatal("protected force push should be rejected")
	}

	// 允许强推 → 不做 git 检查即放行
	allowForce := authz.BranchProtectionRule{Protected: true, BlockForcePush: false}
	if err := CheckBranchProtectionWithAuthorizer(ctx, fakeProtAuthorizer{rule: allowForce}, "alice", "demo", update); err != nil {
		t.Fatalf("force push allowed when BlockForcePush=false: %v", err)
	}

	// 授权面不可用 → fail-open
	if err := CheckBranchProtectionWithAuthorizer(ctx, fakeProtAuthorizer{err: errors.New("plane down")}, "alice", "demo", deletion); err != nil {
		t.Fatalf("authorizer error should fail-open: %v", err)
	}

	// 标签不受分支保护约束
	tag := []PushRef{{Old: strings.Repeat("a", 40), New: zero, Ref: "refs/tags/v1"}}
	if err := CheckBranchProtectionWithAuthorizer(ctx, fakeProtAuthorizer{rule: blockDel}, "alice", "demo", tag); err != nil {
		t.Fatalf("tags are not branch-protected: %v", err)
	}

	// 非法 owner → 放行
	if err := CheckBranchProtectionWithAuthorizer(ctx, fakeProtAuthorizer{rule: blockDel}, "bad/owner", "demo", deletion); err != nil {
		t.Fatalf("invalid owner should allow: %v", err)
	}
}
