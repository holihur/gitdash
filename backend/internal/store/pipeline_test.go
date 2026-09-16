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
	if _, ok, err := st.GetScheduleLastFired("alice", "demo", ".gitdash.yml", "0 2 * * *"); err != nil || ok {
		t.Fatalf("expected no record, got ok=%v err=%v", ok, err)
	}
	claimed, err := st.ClaimSchedule("alice", "demo", ".gitdash.yml", "0 2 * * *", "2026-01-01T02:00:00Z")
	if err != nil || !claimed {
		t.Fatalf("first claim should succeed: claimed=%v err=%v", claimed, err)
	}
	// 同一时间再次 claim 失败（已认领）
	if claimed, err := st.ClaimSchedule("alice", "demo", ".gitdash.yml", "0 2 * * *", "2026-01-01T02:00:00Z"); err != nil || claimed {
		t.Fatalf("duplicate claim should fail: claimed=%v err=%v", claimed, err)
	}
	// 更晚的时间可再次 claim
	if claimed, err := st.ClaimSchedule("alice", "demo", ".gitdash.yml", "0 2 * * *", "2026-01-02T02:00:00Z"); err != nil || !claimed {
		t.Fatalf("later claim should succeed: claimed=%v err=%v", claimed, err)
	}
	got, ok, err := st.GetScheduleLastFired("alice", "demo", ".gitdash.yml", "0 2 * * *")
	if err != nil || !ok || got != "2026-01-02T02:00:00Z" {
		t.Fatalf("last fired = %q ok=%v err=%v", got, ok, err)
	}
}

// TestClaimSchedulePerFile 同一仓库、同一 cron、不同文件之间互不干扰。
func TestClaimSchedulePerFile(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	at := "2026-01-01T02:00:00Z"
	if claimed, err := st.ClaimSchedule("a", "r", ".gitdash/a.yml", "0 2 * * *", at); err != nil || !claimed {
		t.Fatalf("file a first claim: %v %v", claimed, err)
	}
	if claimed, err := st.ClaimSchedule("a", "r", ".gitdash/b.yml", "0 2 * * *", at); err != nil || !claimed {
		t.Fatalf("file b claim should not collide: %v %v", claimed, err)
	}
	if claimed, _ := st.ClaimSchedule("a", "r", ".gitdash/a.yml", "0 2 * * *", at); claimed {
		t.Fatal("same file/cron should be deduplicated")
	}
}

func TestAggregatePipelineStatus(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, ok, err := st.AggregatePipelineStatusForSHA("a", "r", "sha"); err != nil || ok {
		t.Fatalf("no runs: ok=%v err=%v", ok, err)
	}
	mk := func(file, status string) {
		t.Helper()
		run, err := st.CreatePipelineRun("a", "r", file, "sha", "main", "u", "push", nil, 1)
		if err != nil {
			t.Fatal(err)
		}
		switch status {
		case "running":
			_ = st.StartPipelineRun(run.ID)
		default:
			_ = st.FinishPipelineRun(run.ID, status, "")
		}
	}
	mk(".gitdash/ci.yml", "success")
	mk(".gitdash.yml", "failed")
	ci, ok, err := st.AggregatePipelineStatusForSHA("a", "r", "sha")
	if err != nil || !ok || ci.Status != "failed" {
		t.Fatalf("success+failed = %+v ok=%v err=%v", ci, ok, err)
	}
	// running 优先于 failed
	mk(".gitdash/deploy.yml", "running")
	if ci, _, _ := st.AggregatePipelineStatusForSHA("a", "r", "sha"); ci.Status != "running" {
		t.Fatalf("running aggregate = %+v", ci)
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
