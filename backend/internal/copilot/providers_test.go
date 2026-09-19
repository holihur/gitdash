package copilot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderDefaults(t *testing.T) {
	if got := EffectiveBaseURL("anthropic", ""); got != "https://api.anthropic.com" {
		t.Fatalf("anthropic base = %q", got)
	}
	if got := EffectiveModel("anthropic", ""); got != "claude-sonnet-4-5" {
		t.Fatalf("anthropic model = %q", got)
	}
	if got := EffectiveBaseURL("custom", "https://gw.example.com"); got != "https://gw.example.com" {
		t.Fatalf("explicit base = %q", got)
	}
	if got := EffectiveBaseURL("ollama", ""); got != "http://127.0.0.1:11434" {
		t.Fatalf("ollama base = %q", got)
	}
	if _, ok := Provider("openai"); ok {
		t.Fatal("openai should not be a supported provider")
	}
	if spec, ok := Provider("ollama"); !ok || spec.KeyRequired {
		t.Fatal("ollama key must be optional")
	}
}

func TestTestConnection(t *testing.T) {
	// httptest 监听回环，用 GITDASH_LLM_ALLOW_HOSTS 放行以便测试请求本身；
	// SSRF 拦截由 TestTestConnectionBlocksLoopback 单独覆盖。
	t.Setenv("GITDASH_LLM_ALLOW_HOSTS", "127.0.0.1")
	var gotPath, gotKey, gotAuth, gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotAuth = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("anthropic-version")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"content":[]}`))
	}))
	defer srv.Close()

	if err := TestConnection(context.Background(), "compatible", srv.URL, "sk-x", "m1"); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotKey != "sk-x" || gotAuth != "Bearer sk-x" {
		t.Fatalf("auth headers = %q / %q", gotKey, gotAuth)
	}
	if gotVersion == "" {
		t.Fatal("missing anthropic-version header")
	}
}

func TestTestConnectionError(t *testing.T) {
	t.Setenv("GITDASH_LLM_ALLOW_HOSTS", "127.0.0.1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"type":"error","error":{"type":"authentication_error","message":"bad key"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := TestConnection(context.Background(), "compatible", srv.URL, "sk-bad", "m1")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v, want HTTP 401", err)
	}
}

func TestTestConnectionRequiresModel(t *testing.T) {
	err := TestConnection(context.Background(), "compatible", "https://gw.example.com", "sk-x", "")
	if err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("err = %v, want model required", err)
	}
}

// TestTestConnectionBlocksLoopback 验证默认配置下 BYOK 测试连接无法探测回环地址
// （安全评审 §2.4：/me/byok/test 曾被用作服务端 SSRF 原语）。
func TestTestConnectionBlocksLoopback(t *testing.T) {
	t.Setenv("GITDASH_LLM_ALLOW_HOSTS", "")
	t.Setenv("GITDASH_SSRF_ALLOW_PRIVATE", "")
	err := TestConnection(context.Background(), "compatible", "http://127.0.0.1:8080", "sk-x", "m1")
	if !errors.Is(err, ErrProviderBaseURLBlocked) {
		t.Fatalf("err = %v, want ErrProviderBaseURLBlocked", err)
	}
}

// TestValidateBaseURLLLMAllowHosts 验证白名单可放行指定回环主机。
func TestValidateBaseURLLLMAllowHosts(t *testing.T) {
	t.Setenv("GITDASH_LLM_ALLOW_HOSTS", "127.0.0.1:11434")
	t.Setenv("GITDASH_SSRF_ALLOW_PRIVATE", "")
	if _, err := ValidateBaseURL("compatible", "http://127.0.0.1:11434"); err != nil {
		t.Fatalf("allowlisted host rejected: %v", err)
	}
	if _, err := ValidateBaseURL("compatible", "http://127.0.0.1:8080"); !errors.Is(err, ErrProviderBaseURLBlocked) {
		t.Fatalf("non-allowlisted port err = %v, want blocked", err)
	}
}

// TestValidateBaseURLRejectsBadScheme 验证非 http(s) 端点被拒绝。
func TestValidateBaseURLRejectsBadScheme(t *testing.T) {
	if _, err := ValidateBaseURL("compatible", "file:///etc/passwd"); err == nil {
		t.Fatal("file:// base_url should be rejected")
	}
}
