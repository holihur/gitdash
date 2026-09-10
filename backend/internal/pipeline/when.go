package pipeline

import (
	"fmt"
	"strings"
	"sync"

	"cel.dev/cel-go/cel"
)

// 条件表达式（when）使用 CEL（github.com/google/cel-go）求值：
//
//	when: branch == "main" && (event == "push" || tag.startsWith("v"))
//	when: tag.matches(r'v\d+\.\d+')
//
// 变量：branch（refs/heads/ 之后的短名，手动触发为短分支名）、tag（refs/tags/ 之后的短名，
// 非 tag 为空）、ref（完整 ref）、event（push | manual）、sha、repo（owner/name）。
// when 不满足时步骤（或整组）跳过并计为完成；表达式必须返回 bool。

// MaxWhenLength 条件表达式长度上限（防 DoS）。
const MaxWhenLength = 512

// whenEnv CEL 环境缓存在 sync.Once 中，仅声明白名单变量。
var (
	whenEnvOnce sync.Once
	whenEnv     *cel.Env
)

func celWhenEnv() *cel.Env {
	whenEnvOnce.Do(func() {
		whenEnv, _ = cel.NewEnv(
			cel.Variable("branch", cel.StringType),
			cel.Variable("tag", cel.StringType),
			cel.Variable("ref", cel.StringType),
			cel.Variable("event", cel.StringType),
			cel.Variable("sha", cel.StringType),
			cel.Variable("repo", cel.StringType),
		)
	})
	return whenEnv
}

// WhenCtx 条件求值上下文。
type WhenCtx struct {
	Ref   string // 完整 ref（refs/heads/main、refs/tags/v1），手动触发为短分支名
	Event string // push | pull_request | schedule | workflow_dispatch | manual
	Sha   string // 对应提交
	Repo  string // owner/name
}

// whenVars 基于上下文构造 CEL 变量值。
func (c WhenCtx) whenVars() map[string]any {
	branch := ""
	if after, ok := strings.CutPrefix(c.Ref, "refs/heads/"); ok {
		branch = after
	} else if !strings.HasPrefix(c.Ref, "refs/") {
		branch = c.Ref // 手动触发为短分支名
	}
	tag := ""
	if after, ok := strings.CutPrefix(c.Ref, "refs/tags/"); ok {
		tag = after
	}
	return map[string]any{
		"branch": branch,
		"tag":    tag,
		"ref":    c.Ref,
		"event":  c.Event,
		"sha":    c.Sha,
		"repo":   c.Repo,
	}
}

// compileWhen 编译条件表达式为可复用求值程序（DSL 解析时执行一次；
// 类型与语法错误在解析阶段直接拒绝整条流水线）。
func compileWhen(expr string, lineNo int) (cel.Program, error) {
	if len(expr) == 0 {
		return nil, fmt.Errorf("line %d: when requires a condition expression", lineNo)
	}
	if len(expr) > MaxWhenLength {
		return nil, fmt.Errorf("line %d: when expression too long (max %d)", lineNo, MaxWhenLength)
	}
	ast, iss := celWhenEnv().Compile(expr)
	if iss.Err() != nil {
		return nil, fmt.Errorf("line %d: when: %w", lineNo, iss.Err())
	}
	if !ast.OutputType().IsExactType(cel.BoolType) {
		return nil, fmt.Errorf("line %d: when: expression must be boolean", lineNo)
	}
	prg, err := celWhenEnv().Program(ast)
	if err != nil {
		return nil, fmt.Errorf("line %d: when: %w", lineNo, err)
	}
	return prg, nil
}

// evalWhen 求值；错误时按跳过处理（解析期已校验类型，正常不会发生）。
func (c WhenCtx) evalWhen(prg cel.Program) (bool, error) {
	out, _, err := prg.Eval(c.whenVars())
	if err != nil {
		return false, err
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("when: non-bool result")
	}
	return b, nil
}
