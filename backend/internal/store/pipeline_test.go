package store

import (
	"path/filepath"
	"testing"
)

func TestClaimSchedule(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// 无记录：首次 claim 成功
	if _, ok, err := st.GetScheduleLastFired("alice", "demo", "0 2 * * *"); err != nil || ok {
		t.Fatalf("expected no record, got ok=%v err=%v", ok, err)
	}
	claimed, err := st.ClaimSchedule("alice", "demo", "0 2 * * *", "2026-01-01T02:00:00Z")
	if err != nil || !claimed {
		t.Fatalf("first claim should succeed: claimed=%v err=%v", claimed, err)
	}
	// 同一时间再次 claim 失败（已认领）
	if claimed, err := st.ClaimSchedule("alice", "demo", "0 2 * * *", "2026-01-01T02:00:00Z"); err != nil || claimed {
		t.Fatalf("duplicate claim should fail: claimed=%v err=%v", claimed, err)
	}
	// 更晚的时间可再次 claim
	if claimed, err := st.ClaimSchedule("alice", "demo", "0 2 * * *", "2026-01-02T02:00:00Z"); err != nil || !claimed {
		t.Fatalf("later claim should succeed: claimed=%v err=%v", claimed, err)
	}
	got, ok, err := st.GetScheduleLastFired("alice", "demo", "0 2 * * *")
	if err != nil || !ok || got != "2026-01-02T02:00:00Z" {
		t.Fatalf("last fired = %q ok=%v err=%v", got, ok, err)
	}
}

func TestListEnabledPipelines(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.SetPipeline("alice", "on", true); err != nil {
		t.Fatalf("set on: %v", err)
	}
	if err := st.SetPipeline("alice", "off", false); err != nil {
		t.Fatalf("set off: %v", err)
	}
	got, err := st.ListEnabledPipelines()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Repo != "on" {
		t.Fatalf("enabled pipelines = %#v", got)
	}
}
