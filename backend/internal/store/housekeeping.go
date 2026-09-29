package store

import (
	"time"

	"gitdash/backend/internal/logx"
)

// CleanupExpired 清理各类过期/临时数据行：登录失败限流、webhook 投递记录、
// OAuth state/grant、设备码、MFA challenge、WebAuthn session、会话、已读通知。
//
// 幂等：重复执行与多实例并发都安全；由周期性后台任务（asynq.Scheduler）调用，
// 不再在每个 API 进程里各起一个 time.Sleep 循环。
func (s *Store) CleanupExpired(now time.Time) {
	log := func(name string, n int64, err error) {
		switch {
		case err != nil:
			logx.Infof("%s cleanup: %v", name, err)
		case n > 0:
			logx.Infof("%s cleanup: removed %d rows", name, n)
		}
	}
	nowStr := now.UTC().Format(time.RFC3339)

	n, err := s.CleanupLoginFails(24 * time.Hour)
	log("login-fails", n, err)
	n, err = s.PruneDeliveries(now.Add(-7 * 24 * time.Hour).Format(time.RFC3339))
	log("webhook-deliveries", n, err)
	n, err = s.PruneOAuthStates(nowStr)
	log("oauth-state", n, err)
	n, err = s.PruneOAuthGrants(nowStr)
	log("oauth-grant", n, err)
	n, err = s.PruneDeviceGrants(nowStr)
	log("oauth-device-grant", n, err)
	n, err = s.PruneMFAChallenges(nowStr)
	log("mfa-challenge", n, err)
	n, err = s.PruneWebAuthnSessions(nowStr)
	log("webauthn-session", n, err)
	n, err = s.PruneSessions()
	log("session", n, err)
	n, err = s.PruneNotifications(90 * 24 * time.Hour)
	log("notification", n, err)
}
