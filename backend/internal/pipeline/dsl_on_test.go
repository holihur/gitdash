package pipeline

import "testing"

func TestParseOnAndSchedule(t *testing.T) {
	src := `image: alpine
on: [push, pull_request, schedule, workflow_dispatch]
schedule:
  - "0 2 * * *"
  - "*/5 * * * *"
env:
  - FOO=bar
steps:
  - run: echo hi
`
	cfg, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, e := range []string{"push", "pull_request", "schedule", "workflow_dispatch"} {
		if !cfg.Triggers(e) {
			t.Errorf("trigger %q should be enabled", e)
		}
	}
	if !cfg.ScheduleEnabled() {
		t.Error("ScheduleEnabled should be true")
	}
	if !cfg.DispatchEnabled() {
		t.Error("DispatchEnabled should be true")
	}
	if len(cfg.Schedule) != 2 || cfg.Schedule[0] != "0 2 * * *" || cfg.Schedule[1] != "*/5 * * * *" {
		t.Errorf("schedule = %#v", cfg.Schedule)
	}
}

func TestParseOnBlockList(t *testing.T) {
	src := `on:
  - pull_request
  - schedule
schedule:
  - "0 0 * * 0"
steps:
  - run: x
`
	cfg, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.Triggers("pull_request") || cfg.Triggers("push") {
		t.Errorf("on = %#v", cfg.On)
	}
}

func TestParseDefaultTriggerIsPushOnly(t *testing.T) {
	cfg, err := Parse([]byte("image: alpine\nsteps:\n  - run: x\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.Triggers("push") {
		t.Error("push should be enabled by default")
	}
	if cfg.Triggers("pull_request") || cfg.Triggers("workflow_dispatch") {
		t.Error("only push should be enabled by default")
	}
	if cfg.ScheduleEnabled() || cfg.DispatchEnabled() {
		t.Error("schedule/dispatch should be off by default")
	}
}

func TestParseTriggerErrors(t *testing.T) {
	cases := map[string]string{
		"unknown trigger":              "on: [push, bogus]\nsteps:\n  - run: x\n",
		"duplicate trigger":            "on: [push, push]\nsteps:\n  - run: x\n",
		"schedule without on":          "schedule:\n  - \"0 2 * * *\"\nsteps:\n  - run: x\n",
		"on schedule without schedule": "on: [schedule]\nsteps:\n  - run: x\n",
		"bad cron":                     "on: [schedule]\nschedule:\n  - \"not a cron\"\nsteps:\n  - run: x\n",
		"too many schedules":           "on: [schedule]\nschedule:\n  - \"0 1 * * *\"\n  - \"0 2 * * *\"\n  - \"0 3 * * *\"\n  - \"0 4 * * *\"\n  - \"0 5 * * *\"\n  - \"0 6 * * *\"\nsteps:\n  - run: x\n",
	}
	for name, src := range cases {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
