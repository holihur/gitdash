// Provider 预设：BYOK 密钥按"供应商"给出一组默认值（默认 base URL、认证头风格、
// 是否必须提供密钥）。agent 的 LLM 客户端只说 Anthropic Messages 兼容协议
// （POST {base}/v1/messages），因此这里的所有预设都必须指向 Anthropic 兼容端点：
//   - anthropic：官方端点；
//   - compatible：任意 Anthropic 兼容网关（LiteLLM / one-api / 自建），可转接
//     OpenAI / vLLM / Ollama 等；
//   - ollama：本地 Ollama（需其 /v1/messages 兼容端点），可不填密钥。
package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gitdash/backend/internal/ssrf"
)

// ProviderSpec 是某个 BYOK 供应商的默认配置。
type ProviderSpec struct {
	// DefaultBaseURL 是未填写 base_url 时使用的端点。
	DefaultBaseURL string
	// DefaultModel 是未填写 model 时使用的模型名。
	DefaultModel string
	// AuthStyle 传给 agent 的 LLM_AUTH_STYLE：x-api-key | bearer | both。
	AuthStyle string
	// KeyRequired 为 true 时创建密钥必须提供 api_key。
	KeyRequired bool
}

// providers 是支持的全部 BYOK 供应商预设。
var providers = map[string]ProviderSpec{
	"anthropic": {
		DefaultBaseURL: "https://api.anthropic.com",
		DefaultModel:   "claude-sonnet-4-5",
		AuthStyle:      "both",
		KeyRequired:    true,
	},
	"compatible": {
		DefaultBaseURL: "",
		DefaultModel:   "",
		AuthStyle:      "both",
		KeyRequired:    true,
	},
	"ollama": {
		DefaultBaseURL: "http://127.0.0.1:11434",
		DefaultModel:   "",
		AuthStyle:      "both",
		KeyRequired:    false,
	},
}

// Provider 返回预设；ok 为 false 表示不支持的供应商。
func Provider(name string) (ProviderSpec, bool) {
	spec, ok := providers[strings.ToLower(strings.TrimSpace(name))]
	return spec, ok
}

// ProviderNames 返回全部受支持的供应商名（用于文档/测试）。
func ProviderNames() []string {
	out := make([]string, 0, len(providers))
	for name := range providers {
		out = append(out, name)
	}
	return out
}

// EffectiveBaseURL 解析实际使用的端点：显式配置优先，否则取预设默认值。
func EffectiveBaseURL(provider, baseURL string) string {
	if v := strings.TrimSpace(baseURL); v != "" {
		return v
	}
	if spec, ok := Provider(provider); ok {
		return spec.DefaultBaseURL
	}
	return ""
}

// EffectiveModel 解析实际使用的模型：显式配置优先，否则取预设默认值。
func EffectiveModel(provider, model string) string {
	if v := strings.TrimSpace(model); v != "" {
		return v
	}
	if spec, ok := Provider(provider); ok {
		return spec.DefaultModel
	}
	return ""
}

// ErrProviderBaseURL 表示既没有显式 base_url，也没有预设默认值。
var ErrProviderBaseURL = errors.New("base_url is required for this provider")

// ErrProviderBaseURLBlocked 表示 base_url 指向被 SSRF 防护拦截的主机。
var ErrProviderBaseURLBlocked = errors.New("base_url host is not allowed")

// httpClient 使用 SSRF 防护拨号：解析后直接拨已校验的 IP，阻断对回环/私有/
// 链路本地/云元数据地址的访问，并消除 DNS 重绑定（TOCTOU）窗口。
// 运维可用 GITDASH_LLM_ALLOW_HOSTS（逗号分隔 host 或 host:port）显式为本地
// LLM 网关开白名单（仅影响 BYOK/copilot，不影响 webhook/导入 SSRF 防护）。
var httpClient = &http.Client{
	Timeout: 20 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err == nil && llmHostAllowed(host, port) {
				return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
			}
			return ssrf.DialContext(ctx, network, addr)
		},
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	},
}

// llmHostAllowed 判断 host[:port] 是否命中 GITDASH_LLM_ALLOW_HOSTS。条目可为
// `host`（任意端口）或 `host:port`（仅该端口）。每次读取，便于测试覆盖。
func llmHostAllowed(host, port string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	for _, e := range strings.Split(os.Getenv("GITDASH_LLM_ALLOW_HOSTS"), ",") {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		if e == host || (port != "" && e == host+":"+port) {
			return true
		}
	}
	return false
}

// ValidateBaseURL 校验 LLM 端点：必须是 http(s) 且主机通过 SSRF 防护
// （默认拒绝回环/私有/链路本地/云元数据；GITDASH_LLM_ALLOW_HOSTS 可放行指定
// 主机，GITDASH_SSRF_ALLOW_PRIVATE=1 可全局放开自托管内网场景）。
// 返回去掉尾部斜杠的端点。
func ValidateBaseURL(provider, raw string) (string, error) {
	base := EffectiveBaseURL(provider, raw)
	if base == "" {
		return "", ErrProviderBaseURL
	}
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("invalid base_url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("base_url must be http(s)")
	}
	if !llmHostAllowed(u.Hostname(), u.Port()) && ssrf.HostBlocked(u.Hostname()) {
		return "", ErrProviderBaseURLBlocked
	}
	return strings.TrimRight(base, "/"), nil
}

// TestConnection 向 Anthropic 兼容端点发一次最小请求，验证 provider / base_url /
// model / api_key 可用。仅返回错误，成功即 nil。
func TestConnection(ctx context.Context, provider, baseURL, apiKey, model string) error {
	m := EffectiveModel(provider, model)
	if strings.TrimSpace(m) == "" {
		return errors.New("model is required")
	}
	base, err := ValidateBaseURL(provider, baseURL)
	if err != nil {
		return err
	}
	authStyle := "both"
	if spec, ok := Provider(provider); ok && spec.AuthStyle != "" {
		authStyle = spec.AuthStyle
	}
	body, _ := json.Marshal(map[string]any{
		"model":      m,
		"max_tokens": 16,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
	})
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		base+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	switch authStyle {
	case "x-api-key":
		req.Header.Set("x-api-key", apiKey)
	case "bearer":
		req.Header.Set("authorization", "Bearer "+apiKey)
	default: // both
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("authorization", "Bearer "+apiKey)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connect %s: %w", base, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return nil
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<10))
	msg := strings.TrimSpace(string(raw))
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
}
