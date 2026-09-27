package store

import (
	"path/filepath"
	"testing"

	gormsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// legacyPipelineScheduleTable 构造一个缺 file 列的旧 pipeline_schedules 表，
// 用于触发破坏性迁移路径。
func legacyPipelineSchedules(t *testing.T, path string) {
	t.Helper()
	db, err := gorm.Open(gormsqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE pipeline_schedules (owner text, repo text, expr text)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE incoming_webhooks (id integer primary key, owner text, repo text)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX uq_incoming ON incoming_webhooks (owner, repo)").Error; err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

func TestMigrateRefusesDestructiveWithoutOptIn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacyPipelineSchedules(t, path)

	t.Setenv("GITDASH_ALLOW_DESTRUCTIVE_MIGRATION", "")
	t.Setenv("GITDASH_MIGRATE_DRY_RUN", "")
	if _, err := Open(path); err == nil {
		t.Fatal("expected destructive migration to be refused without opt-in")
	}

	t.Setenv("GITDASH_MIGRATE_DRY_RUN", "1")
	if _, err := Open(path); err == nil {
		t.Fatal("expected dry-run to be refused")
	}
	t.Setenv("GITDASH_MIGRATE_DRY_RUN", "")

	t.Setenv("GITDASH_ALLOW_DESTRUCTIVE_MIGRATION", "1")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("opt-in migration failed: %v", err)
	}
	if v, err := st.schemaVersion(); err != nil || v != currentSchemaVersion {
		t.Fatalf("schema version = %d, %v; want %d", v, err, currentSchemaVersion)
	}
	if !st.db.Migrator().HasTable("pipeline_schedules") || !st.db.Migrator().HasColumn(&pipelineScheduleRow{}, "File") {
		t.Fatal("pipeline_schedules not rebuilt with file column")
	}
	if st.db.Migrator().HasIndex(&incomingWebhookRow{}, "uq_incoming") {
		t.Fatal("legacy uq_incoming index not dropped")
	}
}

func TestMigrateIdempotentRecordsVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if v, err := st.schemaVersion(); err != nil || v != currentSchemaVersion {
		t.Fatalf("schema version = %d, %v; want %d", v, err, currentSchemaVersion)
	}
	if sqlDB, err := st.db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	// 再次打开同一库不应报错，版本保持。
	st2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	if v, err := st2.schemaVersion(); err != nil || v != currentSchemaVersion {
		t.Fatalf("reopened schema version = %d, %v; want %d", v, err, currentSchemaVersion)
	}
}
