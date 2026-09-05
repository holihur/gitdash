// Package pipeline 实现仓库流水线 MVP：
// push 事件触发，读取仓库根目录的 .gitdash.yml（自定义 YAML DSL 子集），
// 在 Docker 容器中按步骤执行并记录日志。
//
// DSL 语法（受支持的 YAML 子集）：
//
//	image: alpine:3.19      # 必填：每步运行所用镜像
//	timeout: 10m            # 可选：单步超时（默认 10m，上限 1h）
//	env:                    # 可选：注入容器的环境变量
//	  - CGO_ENABLED=0
//	steps:                  # 必填：1..20 个步骤，顺序执行，任一失败即终止
//	  - name: build
//	    run: go build ./...
//	  - name: test
//	    run: |
//	      go test ./...
//	      go vet ./...
package pipeline

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DefaultStepTimeout / MaxStepTimeout 单步默认与最大时长。
const (
	DefaultStepTimeout = 10 * time.Minute
	MaxStepTimeout     = time.Hour
	MaxSteps           = 20
	MaxEnvVars         = 20
	MaxRunLength       = 8 << 10
)

// Step 流水线中的一个步骤。
type Step struct {
	Name string
	Run  string
}

// Config 解析后的流水线配置。
type Config struct {
	Image   string
	Timeout time.Duration
	Env     []string
	Steps   []Step
}

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
		case "steps":
			var err error
			i, cfg.Steps, err = readSteps(lines, i+1)
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("line %d: unknown key %q (allowed: image, timeout, env, steps)", i+1, key)
		}
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Image == "" {
		return fmt.Errorf("image is required")
	}
	if !imageRe.MatchString(c.Image) {
		return fmt.Errorf("invalid image %q", c.Image)
	}
	if len(c.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}
	if len(c.Steps) > MaxSteps {
		return fmt.Errorf("too many steps (max %d)", MaxSteps)
	}
	for i, s := range c.Steps {
		if strings.TrimSpace(s.Run) == "" {
			return fmt.Errorf("step %d (%s): run is required", i+1, s.Name)
		}
		if len(s.Run) > MaxRunLength {
			return fmt.Errorf("step %d (%s): run script too long", i+1, s.Name)
		}
	}
	return nil
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

// readSteps 消费 steps 列表；每个 `- ` 起始一个步骤，后续更深缩进的 key/value 归属当前步骤。
func readSteps(lines []string, i int) (int, []Step, error) {
	steps := []Step{}
	if i >= len(lines) || blankOrComment(lines[i]) || indentOf(lines[i]) < 2 ||
		!strings.HasPrefix(strings.TrimSpace(lines[i]), "- ") {
		return i, steps, fmt.Errorf("line %d: expected at least one step (indent 2, prefix \"- \")", i+1)
	}
	cur := -1
	for i < len(lines) {
		line := lines[i]
		if blankOrComment(line) {
			i++
			continue
		}
		ind := indentOf(line)
		if ind == 0 {
			break // 顶层键
		}
		trimmed := strings.TrimSpace(line)
		if ind >= 2 && strings.HasPrefix(trimmed, "- ") {
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
		// 步骤内 key/value（如 name/run），或 run 的多行块
		if cur < 0 {
			return i, steps, fmt.Errorf("line %d: unexpected indentation before first step", i+1)
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
		default:
			return i, steps, fmt.Errorf("line %d: unknown step key %q (allowed: name, run)", i+1, key)
		}
	}
	// 默认步骤名
	for i := range steps {
		if steps[i].Name == "" {
			steps[i].Name = "step-" + strconv.Itoa(i+1)
		}
	}
	for _, s := range steps {
		if !stepNameRe.MatchString(s.Name) {
			return i, steps, fmt.Errorf("invalid step name %q", s.Name)
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
	default:
		return fmt.Errorf("line %d: unknown step key %q (allowed: name, run)", lineNo, key)
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
