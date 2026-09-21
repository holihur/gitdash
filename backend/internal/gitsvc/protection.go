package gitsvc

import (
	"context"
	"fmt"
	"strings"

	"gitdash/backend/internal/authz"
	"gitdash/backend/internal/store"
)

// ---- 分支保护（pre-receive 校验）----

// PushRef pre-receive stdin 的一行：oldrev newrev refname。
type PushRef struct {
	Old string
	New string
	Ref string
}

// CheckBranchProtection 按 DB 中的分支保护规则校验一批 push 引用更新。
// dbPath 为数据库连接（生产 = GITDASH_DB 或 dataDir/gitdash.db；测试注入）。
// 返回错误即拒绝整个 push（pre-receive 非零退出）。
// 仅保护 refs/heads/*；无规则 / 库不可用时放行，避免保护逻辑故障阻断 push。
func CheckBranchProtection(dbPath, owner, repo string, refs []PushRef) error {
	if !ValidName(owner) || !ValidName(repo) {
		return nil
	}
	st, err := store.OpenDSN(dbPath)
	if err != nil {
		// 库不可用放行：保护逻辑故障不应阻断 push（fail-open）
		return nil //nolint:nilerr // intentional fail-open
	}
	for _, p := range refs {
		if !strings.HasPrefix(p.Ref, "refs/heads/") {
			continue
		}
		branch := strings.TrimPrefix(p.Ref, "refs/heads/")
		if !ValidRef(branch) {
			continue
		}
		prot, err := st.GetBranchProtection(owner, repo, branch)
		if err != nil {
			continue // 无保护规则
		}
		zero := strings.Repeat("0", len(p.Old))
		switch {
		case p.New == zero: // 删除分支
			if prot.BlockDeletion {
				return fmt.Errorf("branch %q is protected: deletion is not allowed", branch)
			}
		case p.Old != zero: // 更新：非快进（force push）
			if !prot.BlockForcePush {
				continue
			}
			// old 不是 new 的祖先 = 非快进
			if _, err := gitOut(repoPath(owner, repo), "merge-base", "--is-ancestor", p.Old, p.New); err != nil {
				return fmt.Errorf("branch %q is protected: force push is not allowed", branch)
			}
		}
	}
	return nil
}

// BranchProtectionAuthorizer 是 CheckBranchProtectionWithAuthorizer 所需的最小接口；
// authz.Authorizer（本地/远端）均满足。
type BranchProtectionAuthorizer interface {
	BranchProtection(ctx context.Context, owner, repo, branch string) (authz.BranchProtectionRule, error)
}

// CheckBranchProtectionWithAuthorizer 与 CheckBranchProtection 语义一致，但保护规则
// 来自授权面（BranchProtectionAuthorizer），供独立 SSH 机上的 pre-receive hook 使用：
// 无需访问数据库。授权面不可用时放行（沿用 fail-open 语义），避免保护逻辑故障阻断 push。
func CheckBranchProtectionWithAuthorizer(ctx context.Context, az BranchProtectionAuthorizer, owner, repo string, refs []PushRef) error {
	if !ValidName(owner) || !ValidName(repo) {
		return nil
	}
	for _, p := range refs {
		if !strings.HasPrefix(p.Ref, "refs/heads/") {
			continue
		}
		branch := strings.TrimPrefix(p.Ref, "refs/heads/")
		if !ValidRef(branch) {
			continue
		}
		rule, err := az.BranchProtection(ctx, owner, repo, branch)
		if err != nil {
			// 授权面不可用放行：保护逻辑故障不应阻断 push（fail-open）。
			return nil //nolint:nilerr // intentional fail-open
		}
		if !rule.Protected {
			continue
		}
		zero := strings.Repeat("0", len(p.Old))
		switch {
		case p.New == zero: // 删除分支
			if rule.BlockDeletion {
				return fmt.Errorf("branch %q is protected: deletion is not allowed", branch)
			}
		case p.Old != zero: // 更新：非快进（force push）
			if !rule.BlockForcePush {
				continue
			}
			// old 不是 new 的祖先 = 非快进
			if _, err := gitOut(repoPath(owner, repo), "merge-base", "--is-ancestor", p.Old, p.New); err != nil {
				return fmt.Errorf("branch %q is protected: force push is not allowed", branch)
			}
		}
	}
	return nil
}
