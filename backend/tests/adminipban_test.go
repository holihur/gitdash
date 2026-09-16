package tests

// adminipban_test.go 管理端 IP / CIDR 黑名单黑盒测试。

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// rawDo 发起带自定义 X-Forwarded-For 与 admin cookie 的请求。
func ipBanDo(t *testing.T, env *Env, method, path, cookie, xff, body string) *http.Response {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, env.BaseURL+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", "gitdash_admin="+cookie)
	}
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func adminIPBanCall(t *testing.T, env *Env, cookie, method, path, body string) (int, map[string]any) {
	t.Helper()
	res := ipBanDo(t, env, method, "/api"+path, cookie, "", body)
	defer func() { _ = res.Body.Close() }()
	var v map[string]any
	_ = json.NewDecoder(res.Body).Decode(&v)
	return res.StatusCode, v
}

func TestAdminIPBlacklist(t *testing.T) {
	env := start(t)
	// 让测试客户端可信任 X-Forwarded-For（httptest 的 RemoteAddr 为回环）。
	t.Setenv("GITDASH_TRUSTED_PROXIES", "127.0.0.1")
	tok := bootstrapAdmin(t, env)

	// 初始为空
	if code, _ := adminIPBanCall(t, env, tok, "GET", "/admin/ip-bans", ""); code != 200 {
		t.Fatalf("list = %d, want 200", code)
	}

	// 新增
	code, body := adminIPBanCall(t, env, tok, "POST", "/admin/ip-bans",
		`{"cidr":"203.0.113.0/24","note":"abuse"}`)
	if code != 201 {
		t.Fatalf("add = %d (%v), want 201", code, body)
	}
	if body["cidr"] != "203.0.113.0/24" || body["created_by"] != "admin" {
		t.Fatalf("unexpected body: %v", body)
	}
	id := int64(body["id"].(float64))
	idPath := "/admin/ip-bans/" + strconv.FormatInt(id, 10)

	// 黑名单内的请求被全局拦截
	res := ipBanDo(t, env, "GET", "/api/instance", "", "203.0.113.5", "")
	_ = res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatalf("banned ip request = %d, want 403", res.StatusCode)
	}

	// 黑名单外的请求放行
	res = ipBanDo(t, env, "GET", "/api/instance", "", "198.51.100.1", "")
	_ = res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("allowed ip request = %d, want 200", res.StatusCode)
	}

	// 重复条目 → 409
	if code, _ := adminIPBanCall(t, env, tok, "POST", "/admin/ip-bans",
		`{"cidr":"203.0.113.42/24"}`); code != 409 {
		t.Fatalf("duplicate = %d, want 409", code)
	}
	// 非法输入 → 400
	if code, _ := adminIPBanCall(t, env, tok, "POST", "/admin/ip-bans",
		`{"cidr":"nonsense"}`); code != 400 {
		t.Fatalf("invalid = %d, want 400", code)
	}
	// 封禁自身当前 IP → 400，避免管理员把自己锁在门外
	if code, _ := adminIPBanCall(t, env, tok, "POST", "/admin/ip-bans",
		`{"cidr":"127.0.0.1/32"}`); code != 400 {
		t.Fatalf("self ban = %d, want 400", code)
	}

	// 删除后恢复放行
	if code, _ := adminIPBanCall(t, env, tok, "DELETE", idPath, ""); code != 204 {
		t.Fatalf("delete = %d, want 204", code)
	}
	res = ipBanDo(t, env, "GET", "/api/instance", "", "203.0.113.5", "")
	_ = res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("after delete = %d, want 200", res.StatusCode)
	}
	// 再次删除 → 404
	if code, _ := adminIPBanCall(t, env, tok, "DELETE", idPath, ""); code != 404 {
		t.Fatalf("delete again = %d, want 404", code)
	}
}
