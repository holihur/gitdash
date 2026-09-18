package store

// 管理面板的用户管理：列表 / 级联删除（admin_users 与普通 users 是两套账号体系）。

import (
	"strings"

	"gorm.io/gorm"
)

// AdminListUsers 列出普通用户：q 为用户名模糊过滤（不区分大小写），limit/offset 分页。
// 统一用 LOWER(username) LIKE：SQLite 的 LIKE 默认对 ASCII 不区分大小写，而 Postgres 区分，
// 因此显式 LOWER 才能保证两个后端行为一致。
// 返回 (users, 总数, error)。
func (s *Store) AdminListUsers(q string, limit, offset int) ([]User, int, error) {
	db := s.db.Model(&userRow{})
	if q = strings.ToLower(strings.TrimSpace(q)); q != "" {
		db = db.Where("LOWER(username) LIKE ?", "%"+q+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []userRow
	if err := db.Order("id ASC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	users := make([]User, 0, len(rows))
	for _, r := range rows {
		users = append(users, User{ID: r.ID, Username: r.Username, Email: r.Email, CreatedAt: r.CreatedAt, Banned: r.Banned})
	}
	return users, int(total), nil
}

// AdminDeleteUser 删除用户及其全部归属数据（管理端入口，保留原函数名）。
func (s *Store) AdminDeleteUser(username string) error {
	return s.DeleteUserAccount(username)
}

// DeleteUserAccount 彻底删除用户及其所有个人数据，使账号在实例上完全消失：
//   - 名下仓库及其全部关联（issue/PR/评论/标签/项目/流水线/发布/话题/包/镜像/copilot 等）
//   - 会话、SSH/GPG 公钥、PAT、OAuth 应用与授权、头像、BYOK 密钥、runner
//   - star/watch/关注/组织成员/协作者/通知、配额覆盖、限速与验证码记录
//
// 被遗忘权：他人仓库中该用户留下的内容保留以维持协作历史，但作者身份匿名化为
// deleted-user。用户不存在返回 ErrNotFound。
//
// 注意：git 仓库对象与流水线日志存放在磁盘，需由 API 层另行清理。
func (s *Store) DeleteUserAccount(username string) error {
	var u userRow
	if err := s.db.Where("username = ?", username).First(&u).Error; err != nil {
		return notFoundErr(err)
	}
	// 名下仓库的完整级联在下方单个事务内完成（含 DeleteRepo 未覆盖的子表）
	return s.db.Transaction(func(tx *gorm.DB) error {
		// --- 先清理按父表 ID 关联的子表（须在父表删除前执行）---
		subDeletes := []struct {
			query string
			args  []any
		}{
			{"DELETE FROM issue_labels WHERE issue_id IN (SELECT id FROM issues WHERE owner = ?) OR label_id IN (SELECT id FROM repo_labels WHERE owner = ?)", []any{username, username}},
			{"DELETE FROM webhook_deliveries WHERE hook_id IN (SELECT id FROM webhooks WHERE owner = ?)", []any{username}},
			{"DELETE FROM project_columns WHERE project_id IN (SELECT id FROM projects WHERE owner = ?)", []any{username}},
			{"DELETE FROM project_swimlanes WHERE project_id IN (SELECT id FROM projects WHERE owner = ?)", []any{username}},
			{"DELETE FROM project_cards WHERE project_id IN (SELECT id FROM projects WHERE owner = ?)", []any{username}},
		}
		for _, d := range subDeletes {
			if err := tx.Exec(d.query, d.args...).Error; err != nil {
				return err
			}
		}
		// --- 按 owner 归属的全部仓库关联数据（补齐 DeleteRepo 未覆盖的表）---
		ownerTables := []any{
			&repoCounterRow{}, &repoTopicRow{}, &issueRow{}, &commentRow{}, &repoLabelRow{}, &milestoneRow{},
			&projectRow{}, &collabRow{}, &webhookRow{}, &starRow{}, &watchRow{},
			&notificationRow{}, &forkRow{}, &importRow{}, &mirrorRow{}, &pullRequestRow{},
			&pullReviewRow{}, &branchProtectionRow{}, &pipelineCfgRow{}, &pipelineRunRow{},
			&pipelineScheduleRow{}, &repoEnvVarRow{}, &releaseRow{}, &releaseAssetRow{},
			&incomingWebhookRow{}, &packageRow{}, &packageTagRow{}, &packageAuditRow{},
			&registryManifestRow{}, &copilotSessionRow{},
		}
		for _, m := range ownerTables {
			if err := tx.Where("owner = ?", username).Delete(m).Error; err != nil {
				return err
			}
		}
		// 被 fork 自该用户仓库的记录（owner 可能是他人）
		if err := tx.Where("source_owner = ?", username).Delete(&forkRow{}).Error; err != nil {
			return err
		}
		if err := tx.Where("owner = ?", username).Delete(&repoRow{}).Error; err != nil {
			return err
		}

		// --- 按 user_id 关联的归属数据 ---
		for _, m := range []any{&sessionRow{}, &sshKeyRow{}, &gpgKeyRow{}, &patRow{}, &userOAuthRow{}} {
			if err := tx.Where("user_id = ?", u.ID).Delete(m).Error; err != nil {
				return err
			}
		}
		// --- 按用户名关联的归属数据 ---
		for _, m := range []any{&starRow{}, &watchRow{}, &notificationRow{}, &orgMemberRow{}, &collabRow{}, &byokKeyRow{}, &userAvatarRow{}} {
			if err := tx.Where("username = ?", username).Delete(m).Error; err != nil {
				return err
			}
		}
		// 关注关系（作为关注者或被关注者）
		if err := tx.Where("follower = ? OR followee = ?", username, username).Delete(&followRow{}).Error; err != nil {
			return err
		}
		// 用户注册的 OAuth 应用及其授权码 / 签发的 access token
		var appIDs []int64
		if err := tx.Model(&oauthAppRow{}).Where("user_id = ?", u.ID).Pluck("id", &appIDs).Error; err != nil {
			return err
		}
		if len(appIDs) > 0 {
			// 分块删除，避免 app 数量过多时超出绑定参数上限。
			for _, part := range chunkInt64s(appIDs, 0) {
				if err := tx.Where("app_id IN ?", part).Delete(&oauthGrantRow{}).Error; err != nil {
					return err
				}
				if err := tx.Where("oauth_app_id IN ?", part).Delete(&patRow{}).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Where("user_id = ?", u.ID).Delete(&oauthGrantRow{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", u.ID).Delete(&oauthDeviceGrantRow{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", u.ID).Delete(&oauthAppRow{}).Error; err != nil {
			return err
		}
		// 归属该用户的 runner 与注册 token（scope 形如 user:<username>）
		scope := "user:" + username
		for _, m := range []any{&runnerRow{}, &runnerTokenRow{}} {
			if err := tx.Where("scope = ?", scope).Delete(m).Error; err != nil {
				return err
			}
		}
		// 配额覆盖（key: quota:user:<lower>）
		if err := tx.Where("\"key\" = ?", quotaUserPrefix+strings.ToLower(username)).Delete(&settingRow{}).Error; err != nil {
			return err
		}
		// 邮箱验证码 / MFA 一次性验证码（key: email_mfa:<purpose>:<username>）
		for _, key := range []string{"email_mfa:enroll:" + username, "email_mfa:disable:" + username} {
			if err := tx.Where("\"key\" = ?", key).Delete(&settingRow{}).Error; err != nil {
				return err
			}
		}
		// 登录限速记录（key 形如 "username|ip"；双引号引用兼容 SQLite/PG）
		if err := tx.Where("\"key\" LIKE ?", username+"|%").Delete(&loginFailRow{}).Error; err != nil {
			return err
		}
		// 被遗忘权：他人仓库中该用户的作者身份匿名化（内容保留以维持协作历史）
		const ghost = "deleted-user"
		anon := []struct {
			model any
			cond  string
			field string
		}{
			{&issueRow{}, "author = ?", "author"},
			{&commentRow{}, "author = ?", "author"},
			{&pullRequestRow{}, "author = ?", "author"},
			{&pullRequestRow{}, "merged_by = ?", "merged_by"},
			{&pullReviewRow{}, "reviewer = ?", "reviewer"},
			{&releaseRow{}, "author = ?", "author"},
			{&pipelineRunRow{}, "trigger_by = ?", "trigger_by"},
			{&notificationRow{}, "actor = ?", "actor"},
			{&packageRow{}, "uploader = ?", "uploader"},
			{&packageAuditRow{}, "actor = ?", "actor"},
		}
		for _, a := range anon {
			if err := tx.Model(a.model).Where(a.cond, username).Update(a.field, ghost).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&userRow{}, u.ID).Error
	})
}
