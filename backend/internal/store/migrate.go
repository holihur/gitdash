package store

// migrate 使用 GORM AutoMigrate 保证 schema（SQLite / PostgreSQL 均支持）。
// 已存在的旧 SQLite 库会自动补齐缺失列；不再支持无 owner 字段的 legacy schema。
func (s *Store) migrate() error {
	// 旧版（无 owner 字段）schema 直接重置，v0.2 起仓库归属用户
	if s.db.Migrator().HasTable("repos") && !s.db.Migrator().HasColumn(&repoRow{}, "Owner") {
		if err := s.db.Migrator().DropTable("repos", "ssh_keys"); err != nil {
			return err
		}
	}
	if err := s.db.AutoMigrate(
		&userRow{},
		&sessionRow{},
		&repoRow{},
		&repoTopicRow{},
		&sshKeyRow{},
		&gpgKeyRow{},
		&patRow{},
		&issueRow{},
		&commentRow{},
		&repoLabelRow{},
		&issueLabelRow{},
		&milestoneRow{},
		&projectRow{},
		&projectColumnRow{},
		&projectSwimlaneRow{},
		&projectCardRow{},
		&collabRow{},
		&orgRow{},
		&orgMemberRow{},
		&webhookRow{},
		&webhookDeliveryRow{},
		&adminUserRow{},
		&adminSessionRow{},
		&settingRow{},
		&userOAuthRow{},
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
		&pipelineCfgRow{},
		&pipelineRunRow{},
		&pipelineScheduleRow{},
		&repoEnvVarRow{},
		&releaseRow{},
		&releaseAssetRow{},
		&packageRow{},
		&packageTagRow{},
		&packageAuditRow{},
		&registryManifestRow{},
		&runnerRow{},
		&runnerTokenRow{},
		&byokKeyRow{},
		&copilotSessionRow{},
		&userAvatarRow{},
		&oauthAppRow{},
		&oauthGrantRow{},
		&oauthDeviceGrantRow{},
		&incomingWebhookRow{},
	); err != nil {
		return err
	}
	// 邮箱唯一性（部分唯一索引：空串表示未设置，允许多个）
	if err := s.db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users (email) WHERE email <> ''").Error; err != nil {
		return err
	}
	// 引导第一方 OAuth 客户端（gitdash-cli 设备流，公开客户端无 secret）。
	return s.ensureFirstPartyOAuthApp()
}
