package tests

import (
	"io"
	"net/http"
	"testing"
)

func rawGet(t *testing.T, c *Client, path string) string {
	t.Helper()
	req, _ := http.NewRequest("GET", c.env.BaseURL+"/api"+path, nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}
