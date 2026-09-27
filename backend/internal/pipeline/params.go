package pipeline

import (
	"fmt"
	"strings"
)

// readParams 解析 params 块：缩进 2 的 `- name: x` 列表，字段位于缩进 4。
// 支持的字段：name / description / type / default / required / options。
func readParams(lines []string, start int, cfg *Config) (int, error) {
	i := start
	cur := -1
	for i < len(lines) {
		line := lines[i]
		if blankOrComment(line) {
			i++
			continue
		}
		ind := indentOf(line)
		if ind < 2 {
			break // 回到顶层
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") && ind == 2 {
			if len(cfg.Params) >= MaxParams {
				return i, fmt.Errorf("line %d: too many params (max %d)", i+1, MaxParams)
			}
			cfg.Params = append(cfg.Params, Param{})
			cur = len(cfg.Params) - 1
			first := strings.TrimSpace(stripComment(trimmed[2:]))
			if first != "" {
				if err := assignParamField(&cfg.Params[cur], first, i+1); err != nil {
					return i, err
				}
			}
			i++
			continue
		}
		if cur < 0 {
			return i, fmt.Errorf("line %d: unexpected indentation before param", i+1)
		}
		key, val, err := splitKeyValue(line, i+1)
		if err != nil {
			return i, err
		}
		switch key {
		case "name":
			cfg.Params[cur].Name = val
			i++
		case "description":
			cfg.Params[cur].Description = val
			i++
		case "type":
			cfg.Params[cur].Type = val
			i++
		case "default":
			cfg.Params[cur].Default = val
			i++
		case "required":
			b, berr := parseBoolValue(val, i+1)
			if berr != nil {
				return i, berr
			}
			cfg.Params[cur].Required = b
			i++
		case "options":
			opts, next, oerr := readParamOptions(lines, i, val)
			if oerr != nil {
				return i, oerr
			}
			cfg.Params[cur].Options = opts
			i = next
		default:
			return i, fmt.Errorf("line %d: unknown param key %q (allowed: name, description, type, default, required, options)", i+1, key)
		}
	}
	return i, nil
}

// assignParamField 解析参数首行内联的 key/value（如 `- name: version`）。
func assignParamField(p *Param, s string, lineNo int) error {
	key, val, err := splitKeyValue(s, lineNo)
	if err != nil {
		return err
	}
	switch key {
	case "name":
		p.Name = val
	case "description":
		p.Description = val
	case "type":
		p.Type = val
	case "default":
		p.Default = val
	case "required":
		b, berr := parseBoolValue(val, lineNo)
		if berr != nil {
			return berr
		}
		p.Required = b
	default:
		return fmt.Errorf("line %d: unknown param key %q (allowed: name, description, type, default, required, options)", lineNo, key)
	}
	return nil
}

// readParamOptions 解析 options：内联 `[a, b]` 或缩进 6 的块列表。
func readParamOptions(lines []string, i int, val string) ([]string, int, error) {
	if strings.HasPrefix(val, "[") {
		if !strings.HasSuffix(val, "]") {
			return nil, i, fmt.Errorf("line %d: invalid inline options list", i+1)
		}
		inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(val, "["), "]"))
		if inner == "" {
			return nil, i + 1, nil
		}
		out := []string{}
		for _, item := range strings.Split(inner, ",") {
			item = strings.TrimSpace(unquote(strings.TrimSpace(item)))
			if item == "" {
				return nil, i, fmt.Errorf("line %d: empty option", i+1)
			}
			out = append(out, item)
		}
		return out, i + 1, nil
	}
	if val != "" {
		return nil, i, fmt.Errorf("line %d: options takes a list ([a, b] or indented \"- item\")", i+1)
	}
	out := []string{}
	j := i + 1
	for j < len(lines) {
		line := lines[j]
		if blankOrComment(line) {
			j++
			continue
		}
		if indentOf(line) <= 4 {
			break
		}
		trimmed := strings.TrimSpace(stripComment(line))
		if !strings.HasPrefix(trimmed, "- ") {
			return nil, j, fmt.Errorf("line %d: expected option list item", j+1)
		}
		item := strings.TrimSpace(unquote(strings.TrimSpace(trimmed[2:])))
		if item == "" {
			return nil, j, fmt.Errorf("line %d: empty option", j+1)
		}
		out = append(out, item)
		j++
	}
	return out, j, nil
}

// parseBoolValue 解析 required 字段的布尔值。
func parseBoolValue(val string, lineNo int) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "true", "yes", "on", "1":
		return true, nil
	case "false", "no", "off", "0", "":
		return false, nil
	default:
		return false, fmt.Errorf("line %d: expected a boolean, got %q", lineNo, val)
	}
}

// validateParams 校验 params 块：名称合法且唯一、类型有效、choice 必须有选项、默认值合法。
func validateParams(params []Param) error {
	if len(params) > MaxParams {
		return fmt.Errorf("too many params (max %d)", MaxParams)
	}
	seen := map[string]bool{}
	for idx, p := range params {
		if !envKeyRe.MatchString(p.Name) {
			return fmt.Errorf("param %d: invalid name %q (must match [A-Za-z_][A-Za-z0-9_]*)", idx+1, p.Name)
		}
		if seen[p.Name] {
			return fmt.Errorf("duplicate param %q", p.Name)
		}
		seen[p.Name] = true
		if len(p.Description) > 500 {
			return fmt.Errorf("param %q: description too long", p.Name)
		}
		if len(p.Default) > 4096 {
			return fmt.Errorf("param %q: default too long", p.Name)
		}
		if len(p.Options) > 50 {
			return fmt.Errorf("param %q: too many options (max 50)", p.Name)
		}
		switch p.Type {
		case "", "string":
		case "boolean":
			if p.Default != "" && p.Default != "true" && p.Default != "false" {
				return fmt.Errorf("param %q: boolean default must be \"true\" or \"false\"", p.Name)
			}
		case "choice":
			if len(p.Options) == 0 {
				return fmt.Errorf("param %q: choice requires a non-empty options list", p.Name)
			}
			if p.Default != "" && !containsString(p.Options, p.Default) {
				return fmt.Errorf("param %q: default %q is not one of the options", p.Name, p.Default)
			}
		default:
			return fmt.Errorf("param %q: unknown type %q (allowed: string, choice, boolean)", p.Name, p.Type)
		}
	}
	return nil
}

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

// ResolveInputs 按声明参数解析手动/dispatch 传入的 inputs：套用默认值、校验必填与取值。
// cfg 必须声明了至少一个参数（未声明时由调用方原样使用 inputs）。
func ResolveInputs(cfg *Config, provided map[string]string) (map[string]string, error) {
	if cfg == nil || len(cfg.Params) == 0 {
		return provided, nil
	}
	out := map[string]string{}
	for _, p := range cfg.Params {
		v, ok := provided[p.Name]
		v = strings.TrimSpace(v)
		if !ok || v == "" {
			if p.Default != "" {
				out[p.Name] = p.Default
				continue
			}
			if p.Required {
				return nil, fmt.Errorf("missing required parameter %q", p.Name)
			}
			continue
		}
		if len(v) > 4096 {
			return nil, fmt.Errorf("parameter %q is too long", p.Name)
		}
		switch p.Type {
		case "boolean":
			if v != "true" && v != "false" {
				return nil, fmt.Errorf("parameter %q must be \"true\" or \"false\"", p.Name)
			}
		case "choice":
			if !containsString(p.Options, v) {
				return nil, fmt.Errorf("parameter %q must be one of: %s", p.Name, strings.Join(p.Options, ", "))
			}
		}
		out[p.Name] = v
	}
	return out, nil
}
