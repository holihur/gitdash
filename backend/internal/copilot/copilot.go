// Package copilot 实现 BYOK copilot 会话的 Docker 编排：
// 每个会话运行在独立的 Docker 容器里，工作区为仓库的只读克隆副本，
// 用户自带的 LLM 密钥（BYOK）以环境变量注入容器。
package copilot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
)

// ErrDockerMissing docker 不可用。
var ErrDockerMissing = errors.New("docker not available")

// ErrByokMissing 会话没有可用的 BYOK 密钥。
var ErrByokMissing = errors.New("byok key not configured")

// ErrImageRequired 未提供镜像且服务端未配置默认镜像。
var ErrImageRequired = errors.New("image is required (set GITDASH_COPILOT_IMAGE or provide image)")

var workspacesDir string

// Init 设置 copilot 工作区根目录（main 启动时调用）。
func Init(dataDir string) error {
	workspacesDir = filepath.Join(dataDir, "copilots")
	return os.MkdirAll(workspacesDir, 0o755)
}

// Manager 编排 copilot 会话的 Docker 生命周期。
type Manager struct {
	st *store.Store
}

// NewManager 创建 Manager。
func NewManager(st *store.Store) *Manager { return &Manager{st: st} }

// DefaultImage 服务端默认镜像（GITDASH_COPILOT_IMAGE），未配置返回空串。
func DefaultImage() string { return strings.TrimSpace(os.Getenv("GITDASH_COPILOT_IMAGE")) }

// ContainerName 会话容器名（由 id 派生，确定且唯一）。
func ContainerName(id int64) string { return fmt.Sprintf("gitdash-copilot-%d", id) }

// WorkspacePath 会话工作区目录（仓库克隆副本）。
func WorkspacePath(owner, repo string, id int64) string {
	return filepath.Join(workspacesDir, owner, repo, fmt.Sprintf("ws-%d", id))
}

// Start 启动会话容器：确保工作区克隆 → 注入 BYOK 密钥 → docker run -d。
func (m *Manager) Start(ctx context.Context, session store.CopilotSession) error {
	if err := dockerAvailable(); err != nil {
		return err
	}
	secret, err := m.st.GetByokSecret(session.CreatedBy, session.ByokID)
	if err != nil || secret.APIKey == "" {
		return ErrByokMissing
	}
	image := strings.TrimSpace(session.Image)
	if image == "" {
		image = strings.TrimSpace(os.Getenv("GITDASH_COPILOT_IMAGE"))
	}
	if image == "" {
		return ErrImageRequired
	}

	ws := WorkspacePath(session.Owner, session.Repo, session.ID)
	ref, err := m.ensureWorkspace(session, ws)
	if err != nil {
		return err
	}

	// 幂等：先清掉同名残留容器
	_ = dockerRm(ContainerName(session.ID))

	args := []string{
		"run", "-d",
		"--name", ContainerName(session.ID),
		"--workdir", "/workspace",
		"-v", ws + ":/workspace",
		"--network", copilotNetwork(),
		"--memory", "512m",
		"--memory-swap", "512m",
		"--cpus", "1.0",
		"--pids-limit", "128",
		"--security-opt", "no-new-privileges",
		"--cap-drop", "ALL",
		"-e", "GITDASH=true",
		"-e", "GITDASH_COPILOT_ID=" + fmt.Sprint(session.ID),
		"-e", "GITDASH_OWNER=" + session.Owner,
		"-e", "GITDASH_REPO=" + session.Repo,
		"-e", "GITDASH_REF=" + ref,
		"-e", "GITDASH_TASK=" + session.Prompt,
		"-e", "LLM_APIKEY=" + secret.APIKey,
	}
	if secret.BaseURL != "" {
		args = append(args, "-e", "LLM_BASE_URL="+secret.BaseURL)
	}
	if secret.Model != "" {
		args = append(args, "-e", "LLM_MODEL="+secret.Model)
	}
	args = append(args, image)
	if cmd := strings.TrimSpace(session.Command); cmd != "" {
		args = append(args, "sh", "-ec", cmd)
	}

	log.Printf("copilot audit: START session=%d repo=%s/%s image=%s byok=%d time=%s",
		session.ID, session.Owner, session.Repo, image, session.ByokID, time.Now().UTC().Format(time.RFC3339))
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Stop 停止并删除会话容器（保留工作区，便于再次启动）。
func (m *Manager) Stop(ctx context.Context, session store.CopilotSession) error {
	name := ContainerName(session.ID)
	log.Printf("copilot audit: STOP session=%d repo=%s/%s time=%s",
		session.ID, session.Owner, session.Repo, time.Now().UTC().Format(time.RFC3339))
	if err := dockerRmContext(ctx, name); err != nil {
		// 容器不存在视为已停止
		if strings.Contains(err.Error(), "No such container") {
			return nil
		}
		return err
	}
	return nil
}

// Logs 读取容器日志（含已退出容器的日志；截断到最近 500 行）。
func (m *Manager) Logs(ctx context.Context, session store.CopilotSession) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", "500", ContainerName(session.ID))
	out, err := cmd.CombinedOutput()
	// 容器不存在时返回空日志
	if err != nil && strings.Contains(string(out), "No such container") {
		return "", nil
	}
	return string(out), nil
}

// DockerStatus 返回容器运行状态（docker inspect）。
func (m *Manager) DockerStatus(ctx context.Context, session store.CopilotSession) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Status}}", ContainerName(session.ID))
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "No such object") {
			return "exited", nil
		}
		return "", err
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return "exited", nil
	}
	return s, nil
}

// RemoveWorkspace 删除会话工作区（删除会话时调用）。
func (m *Manager) RemoveWorkspace(session store.CopilotSession) error {
	if workspacesDir == "" {
		return nil
	}
	return os.RemoveAll(WorkspacePath(session.Owner, session.Repo, session.ID))
}

// ensureWorkspace 确保工作区存在（首次克隆）；返回默认分支短名。
func (m *Manager) ensureWorkspace(session store.CopilotSession, ws string) (string, error) {
	if fi, err := os.Stat(ws); err == nil && fi.IsDir() {
		return worktreeRef(ws), nil
	}
	if err := os.MkdirAll(filepath.Dir(ws), 0o755); err != nil {
		return "", fmt.Errorf("create workspace dir: %w", err)
	}
	if out, err := gitsvc.GitOut("", "clone", "--quiet", gitsvc.RepoPath(session.Owner, session.Repo), ws); err != nil {
		return "", fmt.Errorf("clone repo: %w: %s", err, strings.TrimSpace(out))
	}
	return worktreeRef(ws), nil
}

// worktreeRef 返回工作区当前检出的分支短名（clone 后即默认分支）。
func worktreeRef(ws string) string {
	out, err := gitsvc.GitOut(ws, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func dockerRm(name string) error {
	return dockerRmContext(context.Background(), name)
}

func dockerRmContext(ctx context.Context, name string) error {
	out, err := exec.CommandContext(ctx, "docker", "rm", "-f", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func copilotNetwork() string {
	if n := strings.TrimSpace(os.Getenv("GITDASH_COPILOT_NETWORK")); n != "" {
		return n
	}
	return "bridge"
}

func dockerAvailable() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return ErrDockerMissing
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", ErrDockerMissing, strings.TrimSpace(string(out)))
	}
	return nil
}
