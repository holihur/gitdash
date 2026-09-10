// Package pipeline 实现仓库流水线 MVP：
// push 事件触发，读取仓库根目录的 .gitdash.yml（自定义 YAML DSL 子集），
// 在 Docker 容器中按步骤执行并记录日志。
//
// DSL 语法（受支持的 YAML 子集）：
//
//	image: alpine:3.19      # 可选：每步运行所用镜像（Docker 沙箱执行）；
//	                        # 省略时直接在宿主 sh 中执行（需服务端/agent 允许 host 执行）
//	timeout: 10m            # 可选：单步超时（默认 10m，上限 1h）
//	env:                    # 可选：注入容器的环境变量
//	  - CGO_ENABLED=0
//	steps:                  # 必填：1..20 个步骤单元（并行子步骤各自计数），顺序执行，任一失败即终止
//	  - name: build
//	    run: go build ./...
//	  - name: gate         # 条件步骤：when 不满足时整组跳过（计为已完成）
//	    when: branch == main
//	    run: go test ./...
//	  - name: checks       # parallel：子步骤并发执行，任一失败该组失败
//	    parallel:
//	      - name: lint-js
//	        run: npm lint
//	      - name: lint-go
//	        when: event == push
//	        run: |
//	          go vet ./...
//
// 条件表达式（when）支持的变量：branch（refs/heads/ 之后的短名）、tag（refs/tags/ 之后的短名）、
// ref（完整 ref）、event（push | manual）。运算符：==、!=、&&、||、!，支持括号与空字符串字面量。
package pipeline

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cel.dev/cel-go/cel"
)

// MaxStepTimeout 单步最大时长。
const (
	MaxStepTimeout = time.Hour
	MaxSteps       = 20
	MaxEnvVars     = 20
	MaxRunLength   = 8 << 10
)

// DefaultStepTimeout 单步默认超时，可被 GITDASH_PIPELINE_DEFAULT_TIMEOUT 覆盖
// （Init 时读取，便于测试/受限环境缩短）。
var DefaultStepTimeout = 10 * time.Minute

// Step 流水线中的一个步骤。有 run 则为普通步骤；有 parallel 则为并行组（二者互斥）。
type Step struct {
	Name string
	Run  string
	// When 为 CEL 条件表达式原文；whenCond 是 parse 阶段编译好的求值程序。
	// 不满足时跳过该步骤（或整组）。
	When     string
	whenCond cel.Program
	Parallel []Step // 可选：并发子步骤（不允许嵌套 parallel）
}

// Config 解析后的流水线配置。
type Config struct {
	Image   string
	Timeout time.Duration
	Env     []string
	Volumes []string // 额外挂载卷（host:container），沙箱会校验禁止挂载 docker socket
	RunsOn  []string // 可选：目标 runner 标签；非空时派发给远程 agent，否则本地 docker
	Steps   []Step
}

// dockerSockPath 宿主 docker socket 路径（禁止挂载进流水线容器）。
const dockerSockPath = "/var/run/docker.sock"

var (
	imageRe    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._:/-]{0,127}$`)
	stepNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	envKeyRe   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Parse 解析 DSL 文本（拒绝制表符缩进，未知顶层键报错）。
func Parse(data []byte) (*Config, error) {
	cfg := &Config{Timeout: DefaultStepTimeout}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")

	i := 0
	for i < len(lines) {
		line := lines[i]
		if strings.ContainsRune(expandTabs(line), '\t') {
			return nil, fmt.Errorf("line %d: tabs are not allowed for indentation", i+1)
		}
		if blankOrComment(line) {
			i++
			continue
		}
		ind := indentOf(line)
		if ind > 0 {
			return nil, fmt.Errorf("line %d: unexpected indentation", i+1)
		}
		key, val, err := splitKeyValue(line, i+1)
		if err != nil {
			return nil, err
		}
		switch key {
		case "image":
			if val == "" {
				return nil, fmt.Errorf("line %d: image is required", i+1)
			}
			cfg.Image = val
			i++
		case "timeout":
			d, err := parseTimeout(val, i+1)
			if err != nil {
				return nil, err
			}
			cfg.Timeout = d
			i++
		case "env":
			var err error
			i, err = readListItems(lines, i+1, func(item string, lineNo int) error {
				eq := strings.IndexByte(item, '=')
				if eq <= 0 {
					return fmt.Errorf("line %d: env entries must be KEY=VALUE", lineNo)
				}
				k, v := item[:eq], unquote(item[eq+1:])
				if !envKeyRe.MatchString(k) {
					return fmt.Errorf("line %d: invalid env key %q", lineNo, k)
				}
				if len(item) > 4096 {
					return fmt.Errorf("line %d: env value too long", lineNo)
				}
				cfg.Env = append(cfg.Env, k+"="+v)
				return nil
			})
			if err != nil {
				return nil, err
			}
		case "volumes":
			var err error
			i, err = readListItems(lines, i+1, func(item string, lineNo int) error {
				if len(item) > 4096 || strings.Count(item, ":") < 1 {
					return fmt.Errorf("line %d: volume entries must be host:container[:mode]", lineNo)
				}
				cfg.Volumes = append(cfg.Volumes, item)
				return nil
			})
			if err != nil {
				return nil, err
			}
		case "runs-on":
			// 支持两种写法：内联 `runs-on: [a, b]` 或块列表 `- a`
			if strings.HasPrefix(val, "[") {
				if !strings.HasSuffix(val, "]") {
					return nil, fmt.Errorf("line %d: invalid runs-on list", i+1)
				}
				for _, item := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(val, "["), "]"), ",") {
					item = strings.TrimSpace(unquote(strings.TrimSpace(item)))
					if item == "" || len(item) > 32 {
						return nil, fmt.Errorf("line %d: invalid label in runs-on", i+1)
					}
					cfg.RunsOn = append(cfg.RunsOn, item)
				}
				i++
				continue
			}
			var err error
			i, err = readListItems(lines, i+1, func(item string, lineNo int) error {
				if item == "" || len(item) > 32 {
					return fmt.Errorf("line %d: invalid label %q", lineNo, item)
				}
				cfg.RunsOn = append(cfg.RunsOn, item)
				return nil
			})
			if err != nil {
				return nil, err
			}
		case "steps":
			var err error
			i, cfg.Steps, err = readSteps(lines, i+1, 2)
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("line %d: unknown key %q (allowed: image, timeout, env, volumes, runs-on, steps)", i+1, key)
		}
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Image != "" && !imageRe.MatchString(c.Image) {
		return fmt.Errorf("invalid image %q", c.Image)
	}
	if len(c.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}
	if len(c.Steps) > MaxSteps {
		return fmt.Errorf("too many steps (max %d)", MaxSteps)
	}
	if units := c.UnitCount(); units > MaxSteps {
		return fmt.Errorf("too many steps (max %d, got %d)", MaxSteps, units)
	}
	if len(c.Env) > MaxEnvVars {
		return fmt.Errorf("too many env vars (max %d)", MaxEnvVars)
	}
	for i, s := range c.Steps {
		if err := validateStep("step", i+1, s); err != nil {
			return err
		}
	}
	for i, v := range c.Volumes {
		// 沙箱硬性限制：禁止把宿主 docker socket 挂进容器（等于交出宿主 root）
		if strings.Contains(v, dockerSockPath) {
			return fmt.Errorf("volume %d: mounting %s is not allowed", i+1, dockerSockPath)
		}
	}
	return nil
}

// validateStep 递归校验步骤：name、脚本体、并行子步骤（不允许嵌套 parallel）与超长限制。
func validateStep(kind string, n int, s Step) error {
	if s.Name == "" {
		return fmt.Errorf("%s %d: name is required", kind, n)
	}
	switch {
	case s.Parallel != nil && strings.TrimSpace(s.Run) != "":
		return fmt.Errorf("%s %d (%s): run and parallel are mutually exclusive", kind, n, s.Name)
	case s.Parallel != nil:
		if kind == "sub-step" {
			return fmt.Errorf("sub-step %d (%s): nested parallel groups are not allowed", n, s.Name)
		}
		for i, sub := range s.Parallel {
			if err := validateStep("sub-step", i+1, sub); err != nil {
				return err
			}
		}
	default:
		if strings.TrimSpace(s.Run) == "" {
			return fmt.Errorf("%s %d (%s): run is required", kind, n, s.Name)
		}
	}
	if len(s.Run) > MaxRunLength {
		return fmt.Errorf("%s %d (%s): run script too long", kind, n, s.Name)
	}
	return nil
}

// UnitCount 进度计数的步骤单元数量：普通步骤计 1，并行组按子步骤逐个计数。
func (c *Config) UnitCount() int {
	n := 0
	for _, s := range c.Steps {
		n += stepUnitCount(s)
	}
	return n
}

func stepUnitCount(s Step) int {
	if len(s.Parallel) > 0 {
		n := 0
		for _, sub := range s.Parallel {
			n += stepUnitCount(sub)
		}
		if n == 0 {
			return 1
		}
		return n
	}
	return 1
}

// parseTimeout 解析 Go duration（上限 MaxStepTimeout，下限 1s）。
func parseTimeout(val string, lineNo int) (time.Duration, error) {
	if val == "" {
		return DefaultStepTimeout, nil
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return 0, fmt.Errorf("line %d: invalid timeout %q", lineNo, val)
	}
	if d < time.Second {
		d = time.Second
	}
	if d > MaxStepTimeout {
		d = MaxStepTimeout
	}
	return d, nil
}

// readListItems 消费缩进的 `- item` 列表，返回新的下标。
func readListItems(lines []string, i int, add func(item string, lineNo int) error) (int, error) {
	if i < len(lines) {
		line := lines[i]
		if blankOrComment(line) || indentOf(line) < 2 || !strings.HasPrefix(strings.TrimSpace(line), "- ") {
			return i, fmt.Errorf("line %d: expected list items under key (indent 2, prefix \"- \")", i+1)
		}
	}
	for i < len(lines) {
		line := lines[i]
		if blankOrComment(line) {
			i++
			continue
		}
		ind := indentOf(line)
		if ind < 2 {
			break // 回到顶层键
		}
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- ") {
			return i, fmt.Errorf("line %d: unexpected content in list", i+1)
		}
		item := strings.TrimSpace(unquote(stripComment(trimmed[2:])))
		if err := add(item, i+1); err != nil {
			return i, err
		}
		i++
	}
	return i, nil
}

// readSteps 消费一段列表：`- ` 项目落在 listInd 缩进，字段行落在更深缩进。
// 顶层调用 listInd=2；步骤内 parallel 的子步骤递归调用，listInd=parallel 键的缩进+2。
func readSteps(lines []string, i, listInd int) (int, []Step, error) {
	steps := []Step{}
	if i >= len(lines) || blankOrComment(lines[i]) || indentOf(lines[i]) < listInd ||
		!strings.HasPrefix(strings.TrimSpace(lines[i]), "- ") {
		return i, steps, fmt.Errorf("line %d: expected at least one step (indent %d, prefix \"- \")", i+1, listInd)
	}
	cur := -1
	for i < len(lines) {
		line := lines[i]
		if blankOrComment(line) {
			i++
			continue
		}
		ind := indentOf(line)
		if ind < listInd {
			break // 回到上层
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") && ind == listInd {
			steps = append(steps, Step{})
			cur = len(steps) - 1
			first := strings.TrimSpace(stripComment(trimmed[2:]))
			if first != "" {
				if err := assignStepField(&steps[cur], first, i+1); err != nil {
					return i, steps, err
				}
			}
			i++
			continue
		}
		// 步骤字段（name/run/when/parallel）或 run 的多行块
		if cur < 0 {
			return i, steps, fmt.Errorf("line %d: unexpected indentation before step", i+1)
		}
		key, val, err := splitKeyValue(line, i+1)
		if err != nil {
			return i, steps, err
		}
		switch key {
		case "name":
			if val == "" {
				return i, steps, fmt.Errorf("line %d: step name cannot be empty", i+1)
			}
			steps[cur].Name = val
			i++
		case "run":
			if val == "|" || val == "|-" {
				var block string
				i, block, err = readBlockScalar(lines, i+1, ind)
				if err != nil {
					return i, steps, err
				}
				steps[cur].Run = block
				continue
			}
			steps[cur].Run = val
			i++
		case "when":
			prog, perr := compileWhen(val, i+1)
			if perr != nil {
				return i, steps, perr
			}
			steps[cur].When = val
			steps[cur].whenCond = prog
			i++
		case "parallel":
			if val != "" {
				return i, steps, fmt.Errorf("line %d: parallel takes a list of sub-steps", i+1)
			}
			j, subs, serr := readSteps(lines, i+1, ind+2)
			if serr != nil {
				return i, steps, fmt.Errorf("parallel (line %d): %w", i+1, serr)
			}
			steps[cur].Parallel = subs
			i = j
		default:
			return i, steps, fmt.Errorf("line %d: unknown step key %q (allowed: name, run, when, parallel)", i+1, key)
		}
	}
	// 默认步骤名（并行组的子步骤跟在父名后，避免并列日志里无法区分）
	for j := range steps {
		if steps[j].Name == "" {
			steps[j].Name = "step-" + strconv.Itoa(j+1)
		}
		for k := range steps[j].Parallel {
			if steps[j].Parallel[k].Name == "" {
				steps[j].Parallel[k].Name = steps[j].Name + "-" + strconv.Itoa(k+1)
			}
		}
		if !stepNameRe.MatchString(steps[j].Name) {
			return i, steps, fmt.Errorf("invalid step name %q", steps[j].Name)
		}
		for _, sub := range steps[j].Parallel {
			if !stepNameRe.MatchString(sub.Name) {
				return i, steps, fmt.Errorf("invalid step name %q", sub.Name)
			}
		}
	}
	return i, steps, nil
}

// readBlockScalar 读取多行块（`run: |`），去掉公共缩进与尾部空行。
func readBlockScalar(lines []string, i, keyIndent int) (int, string, error) {
	var raw []string
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			raw = append(raw, "")
			i++
			continue
		}
		if indentOf(line) <= keyIndent {
			break
		}
		raw = append(raw, line)
		i++
	}
	for len(raw) > 0 && strings.TrimSpace(raw[len(raw)-1]) == "" {
		raw = raw[:len(raw)-1]
	}
	min := -1
	for _, l := range raw {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if ind := indentOf(l); min < 0 || ind < min {
			min = ind
		}
	}
	if min < 0 {
		return i, "", nil
	}
	out := make([]string, len(raw))
	for j, l := range raw {
		if strings.TrimSpace(l) == "" {
			continue
		}
		out[j] = l[min:]
	}
	return i, strings.Join(out, "\n"), nil
}

// assignStepField 解析步骤首行内联的 key/value（如 `- name: build`，破折号已剥离）。
func assignStepField(step *Step, s string, lineNo int) error {
	key, val, err := splitKeyValue(s, lineNo)
	if err != nil {
		return err
	}
	switch key {
	case "name":
		if val == "" {
			return fmt.Errorf("line %d: step name cannot be empty", lineNo)
		}
		step.Name = val
	case "run":
		step.Run = val
	case "when":
		if val == "" {
			return fmt.Errorf("line %d: when requires a condition expression", lineNo)
		}
		prog, perr := compileWhen(val, lineNo)
		if perr != nil {
			return perr
		}
		step.When = val
		step.whenCond = prog
	case "parallel":
		return fmt.Errorf("line %d: parallel takes a list of sub-steps (not inline)", lineNo)
	default:
		return fmt.Errorf("line %d: unknown step key %q (allowed: name, run, when, parallel)", lineNo, key)
	}
	return nil
}

func expandTabs(s string) string {
	if strings.ContainsRune(s, '\t') {
		return strings.ReplaceAll(s, "\t", "    ")
	}
	return s
}

func indentOf(line string) int {
	n := 0
	for _, r := range line {
		if r == ' ' {
			n++
			continue
		}
		break
	}
	return n
}

func blankOrComment(line string) bool {
	t := strings.TrimSpace(line)
	return t == "" || strings.HasPrefix(t, "#")
}

// stripComment 去掉未加引号值中的行内注释（“空格+#”之后的部分）。
func stripComment(s string) string {
	inSingle, inDouble := false, false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble && i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
				return s[:i]
			}
		}
	}
	return s
}

// splitKeyValue 按第一个冒号拆 key/value，并去掉 value 引号与行内注释。
func splitKeyValue(line string, lineNo int) (key, val string, err error) {
	trimmed := strings.TrimSpace(line)
	ci := strings.IndexByte(trimmed, ':')
	if ci <= 0 {
		return "", "", fmt.Errorf("line %d: expected `key: value`", lineNo)
	}
	key = strings.TrimSpace(trimmed[:ci])
	if strings.ContainsAny(key, " \t") {
		return "", "", fmt.Errorf("line %d: invalid key %q", lineNo, key)
	}
	val = strings.TrimSpace(unquote(strings.TrimSpace(stripComment(trimmed[ci+1:]))))
	return key, val, nil
}

// unquote 去除成对的单/双引号。
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
