package store

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gitdash/backend/internal/envx"
	"gitdash/backend/internal/logx"
)

// currentSchemaVersion 是当前代码期望的 schema 版本。新增结构性迁移时递增，
// 并在 migrate 中按 from→to 处理。
const currentSchemaVersion = 1

// schemaMetaRow 记录数据库 schema 版本（单行，ID 恒为 1）。
type schemaMetaRow struct {
	ID        uint `gorm:"primarykey"`
	Version   int
	UpdatedAt time.Time
}

// migrate 使用 GORM AutoMigrate 保证 schema（SQLite / PostgreSQL 均支持）。
// 已存在的旧 SQLite 库会自动补齐缺失列；不再支持无 owner 字段的 legacy schema。
//
// 安全审计 A6：迁移记录 schema_version；任何破坏性迁移（drop table/index）都
// 必须经 GITDASH_ALLOW_DESTRUCTIVE_MIGRATION=1 显式确认，否则拒绝启动，避免
// 在歧义/半迁移状态下不可恢复地删数据；GITDASH_MIGRATE_DRY_RUN=1 只打印计划。
func (s *Store) migrate() error {
	if err := s.db.AutoMigrate(&schemaMetaRow{}); err != nil {
		return err
	}
	from, err := s.schemaVersion()
	if err != nil {
		return err
	}
	if from > currentSchemaVersion {
		return fmt.Errorf("数据库 schema 版本 %d 高于当前二进制支持的 %d，请升级 gitdash 后再启动", from, currentSchemaVersion)
	}
	if err := s.runDestructiveLegacyMigrations(); err != nil {
		return err
	}
	if err := s.db.AutoMigrate(
		&userRow{},
		&sessionRow{},
		&webauthnCredentialRow{},
		&repoRow{},
		&repoCounterRow{},
		&repoTopicRow{},
		&repoPinRow{},
		&repoCommitRuleRow{},
		&repoLanguageRow{},
		&repoLanguageMetaRow{},
		&sshKeyRow{},
		&deployKeyRow{},
		&gpgKeyRow{},
		&patRow{},
		&issueRow{},
		&issueAssigneeRow{},
		&issueSubscriberRow{},
		&issueEventRow{},
		&commentRow{},
		&repoLabelRow{},
		&issueLabelRow{},
		&milestoneRow{},
		&projectRow{},
		&projectColumnRow{},
		&projectSwimlaneRow{},
		&projectCardRow{},
		&projectCardAssigneeRow{},
		&projectCardLabelRow{},
		&collabRow{},
		&orgRow{},
		&orgMemberRow{},
		&orgFollowRow{},
		&orgTeamRow{},
		&orgTeamMemberRow{},
		&repoTeamGrantRow{},
		&badgeRow{},
		&badgeImageRow{},
		&badgeGrantRow{},
		&badgeDisplayRow{},
		&webhookRow{},
		&webhookDeliveryRow{},
		&adminUserRow{},
		&adminSessionRow{},
		&settingRow{},
		&userOAuthRow{},
		&linkedAccountRow{},
		&starRow{},
		&watchRow{},
		&followRow{},
		&notificationRow{},
		&loginFailRow{},
		&forkRow{},
		&importRow{},
		&mirrorRow{},
		&pullRequestRow{},
		&pullReviewRow{},
		&branchProtectionRow{},
		&mergeQueueRow{},
		&refNoteRow{},
		&pipelineCfgRow{},
		&pipelineRunRow{},
		&pipelineScheduleRow{},
		&repoEnvVarRow{},
		&repoSecretRow{},
		&releaseRow{},
		&releaseAssetRow{},
		&packageRow{},
		&packageTagRow{},
		&packageAuditRow{},
		&registryManifestRow{},
		&registryBlobAccessRow{},
		&runnerRow{},
		&runnerTokenRow{},
		&byokKeyRow{},
		&copilotSessionRow{},
		&userAvatarRow{},
		&uploadRow{},
		&userCoverRow{},
		&orgCoverRow{},
		&oauthAppRow{},
		&oauthGrantRow{},
		&oauthDeviceGrantRow{},
		&incomingWebhookRow{},
		&ipBanRow{},
		&reservedNameRow{},
	); err != nil {
		return err
	}
	// 升级到 blob 访问控制后，为存量 manifest 补授访问权（仅表为空时执行）。
	if err := s.BackfillRegistryBlobAccess(); err != nil {
		return err
	}
	// 邮箱唯一性（部分唯一索引：空串表示未设置，允许多个）
	if err := s.db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users (email) WHERE email <> ''").Error; err != nil {
		return err
	}
	// 引导第一方 OAuth 客户端（gitdash-cli 设备流，公开客户端无 secret）。
	if err := s.ensureFirstPartyOAuthApp(); err != nil {
		return err
	}
	return s.setSchemaVersion(currentSchemaVersion)
}

// schemaVersion 读取已记录的 schema 版本；空库（从未迁移）返回 0。
func (s *Store) schemaVersion() (int, error) {
	var row schemaMetaRow
	err := s.db.Order("id").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return row.Version, nil
}

// setSchemaVersion 写入 schema 版本（upsert 单行）。
func (s *Store) setSchemaVersion(v int) error {
	return s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"version", "updated_at"})}).
		Create(&schemaMetaRow{ID: 1, Version: v}).Error
}

// runDestructiveLegacyMigrations 探测历史遗留的破坏性迁移，并据此决定：
// 干跑时仅打印；未显式 opt-in 时拒绝启动；否则执行。
func (s *Store) runDestructiveLegacyMigrations() error {
	var planned []string
	// 旧版（无 owner 字段）schema 直接重置，v0.2 起仓库归属用户。
	if s.db.Migrator().HasTable("repos") && !s.db.Migrator().HasColumn(&repoRow{}, "Owner") {
		planned = append(planned, "drop tables repos, ssh_keys（legacy schema 无 owner 字段）")
	}
	// pipeline_schedules 旧主键为 (owner, repo, expr)；多流水线需纳入 file，
	// AutoMigrate 不会修改既有主键，故缺列时重建该表。
	if s.db.Migrator().HasTable("pipeline_schedules") && !s.db.Migrator().HasColumn(&pipelineScheduleRow{}, "File") {
		planned = append(planned, "drop table pipeline_schedules（legacy 主键缺 file）")
	}
	// incoming_webhooks 旧版对 (owner, repo) 有唯一约束；多 token 需去掉该索引。
	if s.db.Migrator().HasTable("incoming_webhooks") && s.db.Migrator().HasIndex(&incomingWebhookRow{}, "uq_incoming") {
		planned = append(planned, "drop index uq_incoming on incoming_webhooks")
	}
	if len(planned) == 0 {
		return nil
	}
	summary := strings.Join(planned, "; ")
	if envx.Bool("GITDASH_MIGRATE_DRY_RUN", false) {
		return fmt.Errorf("migration dry-run: 待执行的破坏性迁移: %s；请先 `gitdash backup`，再以 GITDASH_ALLOW_DESTRUCTIVE_MIGRATION=1 启动", summary)
	}
	if !envx.Bool("GITDASH_ALLOW_DESTRUCTIVE_MIGRATION", false) {
		return fmt.Errorf("拒绝执行破坏性数据库迁移: %s；请先 `gitdash backup` 备份，确认后设置 GITDASH_ALLOW_DESTRUCTIVE_MIGRATION=1 再启动", summary)
	}
	logx.Warnf("migration: 执行破坏性迁移: %s", summary)
	if s.db.Migrator().HasTable("repos") && !s.db.Migrator().HasColumn(&repoRow{}, "Owner") {
		if err := s.db.Migrator().DropTable("repos", "ssh_keys"); err != nil {
			return err
		}
	}
	if s.db.Migrator().HasTable("pipeline_schedules") && !s.db.Migrator().HasColumn(&pipelineScheduleRow{}, "File") {
		if err := s.db.Migrator().DropTable("pipeline_schedules"); err != nil {
			return err
		}
	}
	if s.db.Migrator().HasTable("incoming_webhooks") && s.db.Migrator().HasIndex(&incomingWebhookRow{}, "uq_incoming") {
		if err := s.db.Migrator().DropIndex(&incomingWebhookRow{}, "uq_incoming"); err != nil {
			return err
		}
	}
	return nil
}
