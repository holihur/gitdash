package copilot

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
)

// testBYOK 从仓库根目录的 assets.md 读取测试密钥；
// 不存在时跳过（t.Skip），存在返回 (apiKey, baseURL, model)。
func testBYOK(t *testing.T) (string, string, string) {
	t.Helper()
	candidates := []string{"../../../assets.md", "../../assets.md", "assets.md"}
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var key, base, model string
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "LLM_APIKEY="):
				key = strings.Trim(strings.TrimPrefix(line, "LLM_APIKEY="), `"'`)
			case strings.HasPrefix(line, "LLM_BASE_URL="):
				base = strings.Trim(strings.TrimPrefix(line, "LLM_BASE_URL="), `"'`)
			case strings.HasPrefix(line, "LLM_MODEL="):
				model = strings.Trim(strings.TrimPrefix(line, "LLM_MODEL="), `"'`)
			}
		}
		if key != "" {
			return key, base, model
		}
	}
	t.Skip("no BYOK test key in assets.md (LLM_APIKEY=...); skipping")
	return "", "", ""
}

// TestStartWithRealKey 有测试密钥 + docker 时执行真实编排：
// 创建仓库/用户/BYOK 密钥 → Start 容器 → 校验注入的环境变量与工作区 → Stop/清理。
func TestStartWithRealKey(t *testing.T) {
	apiKey, baseURL, model := testBYOK(t)
	if err := dockerAvailable(); err != nil {
		t.Skipf("docker not available: %v", err)
	}

	dataDir := t.TempDir()
	if err := gitsvc.Init(dataDir); err != nil {
		t.Fatal(err)
	}
	if err := Init(dataDir); err != nil {
		t.Fatal(err)
	}
	owner, repo := "alice", "copilot-it"
	if err := gitsvc.CreateBare(owner, repo); err != nil {
		t.Fatal(err)
	}
	if err := gitsvc.InitTemplate(owner, repo); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(filepath.Join(dataDir, "gitdash.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser(owner, "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	byok, err := st.CreateByokKey(owner, "test", "anthropic", apiKey, baseURL, model)
	if err != nil {
		t.Fatal(err)
	}

	session, err := st.CreateCopilotSession(owner, repo, owner, byok.ID, "alpine:3.19",
		"echo injected env", "echo base=$LLM_BASE_URL; echo model=$LLM_MODEL; echo keylen=${#LLM_APIKEY}; ls /workspace")
	if err != nil {
		t.Fatal(err)
	}

	m := NewManager(st)
	t.Cleanup(func() {
		_ = m.Stop(context.Background(), session)
		_ = m.RemoveWorkspace(session)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := m.Start(ctx, session); err != nil {
		t.Fatalf("start: %v", err)
	}

	// 轮询等容器结束（echo 命令立即退出）
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if s, _ := m.DockerStatus(ctx, session); s == "exited" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	logs, err := m.Logs(ctx, session)
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	if baseURL != "" && !strings.Contains(logs, baseURL) {
		t.Fatalf("logs missing LLM_BASE_URL: %s", logs)
	}
	if model != "" && !strings.Contains(logs, model) {
		t.Fatalf("logs missing LLM_MODEL: %s", logs)
	}
	if !strings.Contains(logs, "keylen="+strconv.Itoa(len(apiKey))) {
		t.Fatalf("logs missing LLM_APIKEY length: %s", logs)
	}
	if !strings.Contains(logs, "README.md") {
		t.Fatalf("workspace not mounted: %s", logs)
	}
}
