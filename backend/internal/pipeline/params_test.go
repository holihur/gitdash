package pipeline

import (
	"strings"
	"testing"
)

func TestParseParams(t *testing.T) {
	dsl := `
on: [workflow_dispatch]
params:
  - name: version
    description: Release version
    required: true
  - name: environment
    type: choice
    options: [staging, production]
    default: staging
  - name: dry_run
    type: boolean
    default: "true"
steps:
  - name: build
    run: echo build
`
	cfg, err := Parse([]byte(dsl))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cfg.Params) != 3 {
		t.Fatalf("params = %+v", cfg.Params)
	}
	if cfg.Params[1].Type != "choice" || len(cfg.Params[1].Options) != 2 || cfg.Params[1].Default != "staging" {
		t.Fatalf("choice param = %+v", cfg.Params[1])
	}
	if !cfg.Params[0].Required || cfg.Params[2].Type != "boolean" {
		t.Fatalf("params = %+v", cfg.Params)
	}
}

func TestParseParamsBlockOptions(t *testing.T) {
	dsl := `
params:
  - name: env
    type: choice
    options:
      - dev
      - prod
steps:
  - name: build
    run: echo hi
`
	cfg, err := Parse([]byte(dsl))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cfg.Params) != 1 || strings.Join(cfg.Params[0].Options, ",") != "dev,prod" {
		t.Fatalf("options = %+v", cfg.Params)
	}
}

func TestParseParamsInvalid(t *testing.T) {
	cases := map[string]string{
		"bad name":              "params:\n  - name: 1bad\n    description: x\nsteps:\n  - name: s\n    run: x\n",
		"duplicate":             "params:\n  - name: a\n  - name: a\nsteps:\n  - name: s\n    run: x\n",
		"unknown type":          "params:\n  - name: a\n    type: number\nsteps:\n  - name: s\n    run: x\n",
		"choice no options":     "params:\n  - name: a\n    type: choice\nsteps:\n  - name: s\n    run: x\n",
		"default not in choice": "params:\n  - name: a\n    type: choice\n    options: [x, y]\n    default: z\nsteps:\n  - name: s\n    run: x\n",
	}
	for name, dsl := range cases {
		if _, err := Parse([]byte(dsl)); err == nil {
			t.Errorf("%s: expected parse error", name)
		}
	}
}

func TestResolveInputs(t *testing.T) {
	cfg := &Config{Params: []Param{
		{Name: "version", Required: true},
		{Name: "environment", Type: "choice", Options: []string{"staging", "production"}, Default: "staging"},
		{Name: "dry_run", Type: "boolean", Default: "true"},
	}}

	// 缺少必填
	if _, err := ResolveInputs(cfg, map[string]string{}); err == nil {
		t.Fatal("expected missing required parameter error")
	}

	got, err := ResolveInputs(cfg, map[string]string{"version": "1.2.3"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got["version"] != "1.2.3" || got["environment"] != "staging" || got["dry_run"] != "true" {
		t.Fatalf("resolved = %v", got)
	}

	// 非法选项
	if _, err := ResolveInputs(cfg, map[string]string{"version": "1", "environment": "nope"}); err == nil {
		t.Fatal("expected invalid choice error")
	}
	// 非法布尔
	if _, err := ResolveInputs(cfg, map[string]string{"version": "1", "dry_run": "maybe"}); err == nil {
		t.Fatal("expected invalid boolean error")
	}
	// 未知键被忽略
	got, err = ResolveInputs(cfg, map[string]string{"version": "2", "unknown": "x"})
	if err != nil || got["unknown"] != "" {
		t.Fatalf("unknown key should be ignored: %v, %v", got, err)
	}
}

func TestResolveInputsNoParams(t *testing.T) {
	in := map[string]string{"anything": "goes"}
	got, err := ResolveInputs(&Config{}, in)
	if err != nil || got["anything"] != "goes" {
		t.Fatalf("no-params passthrough = %v, %v", got, err)
	}
}
