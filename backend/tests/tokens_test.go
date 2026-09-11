package tests

import (
	"net/http"
	"testing"
	"time"
)

// doWithIP 发送带指定 X-Forwarded-For 头的请求（回环是受信反代，测试可控制 clientIP）。
func doWithIP(t *testing.T, env *Env, token, xff, path string) int {
	t.Helper()
	req, err := http.NewRequest("GET", env.BaseURL+"/api"+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func TestPATCreateValidation(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	// 非法 CIDR
	alice.mustFail("POST", "/tokens",
		map[string]any{"name": "bad", "scopes": []string{"repo"}, "cidrs": []string{"not-an-ip"}}, 400)
	// 非法过期时间
	alice.mustFail("POST", "/tokens",
		map[string]any{"name": "bad", "scopes": []string{"repo"}, "expires_at": "yesterday"}, 400)
	// 过期时间在过去
	alice.mustFail("POST", "/tokens",
		map[string]any{"name": "bad", "scopes": []string{"repo"},
			"expires_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)}, 400)
	// 合法：CIDR + 未来过期时间
	m := alice.mustStatus("POST", "/tokens",
		map[string]any{"name": "ci", "scopes": []string{"repo"},
			"cidrs": []string{" 192.0.2.0/24 ", "192.0.2.0/24", "2001:db8::1"},
			"expires_at": time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)}, 201)
	if m["token"] == nil || m["expires_at"] == nil || m["cidrs"] == nil {
		t.Fatalf("created response missing fields: %v", m)
	}
	cidrs, _ := m["cidrs"].([]any)
	if len(cidrs) != 2 { // 去重合并后应为 2
		t.Fatalf("expected 2 deduped cidrs, got %v", cidrs)
	}
}

func TestPATExpiryBlocked(t *testing.T) {
	env := start(t)
	_ = register(t, env, "alice", "alice-pass-123")
	uid, err := env.Store.UserID("alice")
	if err != nil {
		t.Fatal(err)
	}
	// 直接创建两个 PAT：一个永不过期，一个已过期
	liveTok, _, err := env.Store.CreatePAT(uid, "live", "repo", "", "")
	if err != nil {
		t.Fatal(err)
	}
	expiredTok, _, err := env.Store.CreatePAT(uid, "expired", "repo", "",
		time.Now().Add(-time.Hour).UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	if code := doWithIP(t, env, liveTok, "", "/tokens"); code != http.StatusOK {
		t.Fatalf("live pat = %d, want 200", code)
	}
	if code := doWithIP(t, env, expiredTok, "", "/tokens"); code != http.StatusUnauthorized {
		t.Fatalf("expired pat = %d, want 401", code)
	}
}

func TestPATCIDRRestriction(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	// 仅允许文档保留网段 192.0.2.0/24
	m := alice.mustStatus("POST", "/tokens",
		map[string]any{"name": "restricted", "scopes": []string{"repo"}, "cidrs": []string{"192.0.2.0/24"}}, 201)
	tok, _ := m["token"].(string)
	if tok == "" {
		t.Fatal("created token empty")
	}
	// 默认来源（回环）不在白名单 → 拒绝
	if code := doWithIP(t, env, tok, "", "/tokens"); code != http.StatusUnauthorized {
		t.Fatalf("default source = %d, want 401", code)
	}
	// 伪造 192.0.2.10（回环为受信反代，XFF 生效）→ 放行
	if code := doWithIP(t, env, tok, "192.0.2.10", "/tokens"); code != http.StatusOK {
		t.Fatalf("allowed source = %d, want 200", code)
	}
	// 同一网段欢迎其他 IP → 放行
	if code := doWithIP(t, env, tok, "192.0.2.200", "/tokens"); code != http.StatusOK {
		t.Fatalf("allowed source 2 = %d, want 200", code)
	}
}

func TestPATCIDRWithIPOnly(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	// 只允许单个 IP（主机掩码）
	m := alice.mustStatus("POST", "/tokens",
		map[string]any{"name": "single", "scopes": []string{"repo"}, "cidrs": []string{"192.0.2.5"}}, 201)
	tok, _ := m["token"].(string)
	if tok == "" {
		t.Fatal("created token empty")
	}
	for _, xff := range []string{"192.0.2.5", "192.0.2.6"} {
		want := http.StatusOK
		if xff == "192.0.2.6" {
			want = http.StatusUnauthorized
		}
		if code := doWithIP(t, env, tok, xff, "/tokens"); code != want {
			t.Fatalf("xff %s = %d, want %d", xff, code, want)
		}
	}
}

func TestPATAdminLoginDetection(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	// 创建受限来源的 PAT（发起自回环会被 IP 白名单拒绝）
	m := alice.mustStatus("POST", "/tokens",
		map[string]any{"name": "restricted", "scopes": []string{"repo"}, "cidrs": []string{"192.0.2.0/24"}}, 201)
	patTok, _ := m["token"].(string)
	if patTok == "" {
		t.Fatal("created token empty")
	}
	// admin/login 检测 PAT 时跳过 IP 白名单：即使来源被拒，仍识别为 PAT → 403
	req, _ := http.NewRequest(http.MethodPost, env.BaseURL+"/api/admin/login", nil)
	req.Header.Set("Authorization", "Bearer "+patTok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("admin/login with restricted pat = %d, want 403", resp.StatusCode)
	}
	// 普通受限 PAT 用于非 admin 端点：来源被拒 → 401
	if code := doWithIP(t, env, patTok, "", "/tokens"); code != http.StatusUnauthorized {
		t.Fatalf("restricted pat from loopback = %d, want 401", code)
	}

	// 无关令牌不应被识别为 PAT：来源被拒时不会误报 pat_not_allowed
	req, _ = http.NewRequest(http.MethodPost, env.BaseURL+"/api/admin/login", nil)
	req.Header.Set("Authorization", "Bearer deadbeef-junk-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("admin/login with junk bearer = %d, want 404 (admin disabled)", resp.StatusCode)
	}
}