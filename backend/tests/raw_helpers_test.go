package tests

import (
	"io"
	"net/http"
	"strconv"
	"testing"
)

func rawGet(t *testing.T, c *Client, path string) string {
	t.Helper()
	body, _ := rawGetPaged(t, c, path)
	return body
}

// rawGetPaged 返回响应体与 X-Total-Count 响应头（列表分页断言用）。
func rawGetPaged(t *testing.T, c *Client, path string) (string, int) {
	t.Helper()
	req, _ := http.NewRequest("GET", c.env.BaseURL+"/api"+path, nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	total, _ := strconv.Atoi(resp.Header.Get("X-Total-Count"))
	return string(b), total
}
