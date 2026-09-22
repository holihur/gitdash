package gitsvc

import (
	"fmt"
	"regexp"
	"strings"
)

// CommitIdentityRule 提交身份校验规则：校验 push 引入提交的作者/提交者
// 姓名与邮箱格式。空 pattern 表示该项不校验。
//
// pattern 为 Go 正则（RE2），按“整体匹配”生效（内部自动加 ^(?:...)$）。
type CommitIdentityRule struct {
	NamePattern  string
	EmailPattern string
	nameRe       *regexp.Regexp
	emailRe      *regexp.Regexp
}

// CompileCommitIdentityRule 编译规则；非法正则返回错误。
func CompileCommitIdentityRule(namePattern, emailPattern string) (CommitIdentityRule, error) {
	rule := CommitIdentityRule{NamePattern: namePattern, EmailPattern: emailPattern}
	if s := strings.TrimSpace(namePattern); s != "" {
		re, err := regexp.Compile("^(?:" + s + ")$")
		if err != nil {
			return rule, fmt.Errorf("invalid name pattern: %w", err)
		}
		rule.nameRe = re
	}
	if s := strings.TrimSpace(emailPattern); s != "" {
		re, err := regexp.Compile("^(?:" + s + ")$")
		if err != nil {
			return rule, fmt.Errorf("invalid email pattern: %w", err)
		}
		rule.emailRe = re
	}
	return rule, nil
}

// Active 报告规则是否包含任何校验项。
func (r CommitIdentityRule) Active() bool { return r.nameRe != nil || r.emailRe != nil }

type commitIdentity struct {
	sha            string
	authorName     string
	authorEmail    string
	committerName  string
	committerEmail string
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func (r CommitIdentityRule) check(c commitIdentity) error {
	if r.nameRe != nil {
		if !r.nameRe.MatchString(c.authorName) {
			return fmt.Errorf("commit %s: author name %q does not match the required format", shortSHA(c.sha), c.authorName)
		}
		if !r.nameRe.MatchString(c.committerName) {
			return fmt.Errorf("commit %s: committer name %q does not match the required format", shortSHA(c.sha), c.committerName)
		}
	}
	if r.emailRe != nil {
		if !r.emailRe.MatchString(c.authorEmail) {
			return fmt.Errorf("commit %s: author email %q does not match the required format", shortSHA(c.sha), c.authorEmail)
		}
		if !r.emailRe.MatchString(c.committerEmail) {
			return fmt.Errorf("commit %s: committer email %q does not match the required format", shortSHA(c.sha), c.committerEmail)
		}
	}
	return nil
}

// CheckCommitIdentity 校验一批 push 引用所引入提交的作者/提交者姓名与邮箱。
// 返回错误即拒绝整个 push。无有效规则或无法读取提交时放行（不阻断正常 push）。
func CheckCommitIdentity(owner, repo string, refs []PushRef, rule CommitIdentityRule) error {
	if !rule.Active() {
		return nil
	}
	if !ValidName(owner) || !ValidName(repo) {
		return nil
	}
	for _, p := range refs {
		if !strings.HasPrefix(p.Ref, "refs/heads/") && !strings.HasPrefix(p.Ref, "refs/tags/") {
			continue
		}
		zero := strings.Repeat("0", len(p.Old))
		if p.New == zero {
			continue // 删除引用：无新提交
		}
		args := []string{"log", "--format=%H%x1f%an%x1f%ae%x1f%cn%x1f%ce%x1e"}
		if p.Old != "" && p.Old != zero {
			args = append(args, p.Old+".."+p.New)
		} else {
			// 新引用：只校验本次引入（不沿现有引用可达）的提交。
			args = append(args, p.New, "--not", "--all")
		}
		out, err := gitOut(repoPath(owner, repo), args...)
		if err != nil {
			return nil //nolint:nilerr // 读取失败放行，避免阻断正常 push
		}
		for _, rec := range strings.Split(out, "\x1e") {
			rec = strings.TrimPrefix(rec, "\n")
			if strings.TrimSpace(rec) == "" {
				continue
			}
			f := strings.Split(rec, "\x1f")
			if len(f) < 5 {
				continue
			}
			c := commitIdentity{
				sha: f[0], authorName: f[1], authorEmail: f[2],
				committerName: f[3], committerEmail: f[4],
			}
			if err := rule.check(c); err != nil {
				return err
			}
		}
	}
	return nil
}
