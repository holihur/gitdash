package sshserver

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"gitdash/backend/internal/authz"
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/store"
)

// 匹配 "demo.git"（owner=空，解析为当前登录用户）或 "alice/demo.git"
var repoPathRe = regexp.MustCompile(`^/?([\w.-]+?)(?:/([\w.-]+?))?(?:\.git)?/?$`)

var gitCommands = map[string]string{
	"git-upload-pack":    "upload-pack",
	"git-receive-pack":   "receive-pack",
	"git-upload-archive": "upload-archive",
}

const (
	// sshHandshakeTimeout 限制单连接完成 SSH 握手的时间，防慢速握手占资源。
	sshHandshakeTimeout = 20 * time.Second
	// maxSSHConns 并发连接上限，防止大批半开连接耗光 fd/goroutine。
	maxSSHConns = 512
	// maxSSHEnvEntries 单会话接受的 env 请求上限，防止内存被无限累积。
	maxSSHEnvEntries = 32
)

type Server struct {
	authz    authz.Authorizer
	reposDir string
	config   *ssh.ServerConfig
}

// NewServer 创建 SSH git 服务（进程内直连数据库，供 main 与测试使用）。
func NewServer(st *store.Store, reposDir, dataDir string) (*Server, error) {
	return NewServerWithAuthorizer(authz.NewStore(st), reposDir, dataDir)
}

// NewServerWithAuthorizer 用任意授权器创建 SSH git 服务：
// 传入 authz.NewStore(st) 即同机直连数据库；传入 authz.RemoteAuthorizer
// 则可通过授权面 gRPC 在独立机器上运行。
func NewServerWithAuthorizer(az authz.Authorizer, reposDir, dataDir string) (*Server, error) {
	signer, err := loadOrGenerateHostKey(filepath.Join(dataDir, "ssh_host_ed25519_key"))
	if err != nil {
		return nil, fmt.Errorf("host key: %w", err)
	}
	cfg := buildConfig(az)
	cfg.AddHostKey(signer)
	return &Server{authz: az, reposDir: reposDir, config: cfg}, nil
}

func Serve(addr string, st *store.Store, reposDir, dataDir string) error {
	return ServeWithAuthorizer(addr, authz.NewStore(st), reposDir, dataDir)
}

// ServeWithAuthorizer 用给定授权器在 addr 上提供 SSH git 服务（阻塞）。
func ServeWithAuthorizer(addr string, az authz.Authorizer, reposDir, dataDir string) error {
	s, err := NewServerWithAuthorizer(az, reposDir, dataDir)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.ServeOn(listener)
}

// ServeOn 在给定 listener 上运行（测试可注入随机端口）。
func (s *Server) ServeOn(ln net.Listener) error {
	sem := make(chan struct{}, maxSSHConns)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			logx.Infof("ssh accept: %v", err)
			continue
		}
		select {
		case sem <- struct{}{}:
		default:
			logx.Infof("ssh: too many connections, rejecting %s", conn.RemoteAddr())
			_ = conn.Close()
			continue
		}
		go func(c net.Conn) {
			defer func() { <-sem }()
			s.handleConn(c, s.config)
		}(conn)
	}
}

func buildConfig(az authz.Authorizer) *ssh.ServerConfig {
	return &ssh.ServerConfig{
		ServerVersion: "SSH-2.0-gitdash",
		PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			username, ok, reason, err := az.AuthorizePublicKey(context.Background(), key.Type(), key.Marshal())
			if err != nil {
				logx.Infof("ssh: authorize key for %s: %v", meta.User(), err)
				return nil, fmt.Errorf("authorization unavailable")
			}
			if !ok {
				logx.Infof("ssh: rejected %s, %s (%s)", meta.User(), reason, ssh.FingerprintSHA256(key))
				return nil, fmt.Errorf("%s", reason)
			}
			logx.Infof("ssh: %s authenticated as user %q with key %s", meta.User(), username, ssh.FingerprintSHA256(key))
			return &ssh.Permissions{Extensions: map[string]string{"username": username}}, nil
		},
	}
}

func (s *Server) handleConn(conn net.Conn, config *ssh.ServerConfig) {
	if host, _, err := net.SplitHostPort(conn.RemoteAddr().String()); err == nil {
		banned, berr := s.authz.IsIPBanned(context.Background(), host)
		if berr != nil {
			// 授权面不可用：失败关闭，避免误放行黑名单来源。
			logx.Infof("ssh: ip ban check failed for %s: %v", host, berr)
			_ = conn.Close()
			return
		}
		if banned {
			logx.Infof("ssh: rejected blacklisted ip %s", host)
			_ = conn.Close()
			return
		}
	}
	_ = conn.SetDeadline(time.Now().Add(sshHandshakeTimeout))
	sconn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		logx.Infof("ssh handshake from %s: %v", conn.RemoteAddr(), err)
		_ = conn.Close()
		return
	}
	_ = conn.SetDeadline(time.Time{})
	defer func() { _ = sconn.Close() }()
	go ssh.DiscardRequests(reqs)

	username := sconn.Permissions.Extensions["username"]
	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			_ = newCh.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}
		ch, requests, err := newCh.Accept()
		if err != nil {
			continue
		}
		go s.handleSession(ch, requests, username)
	}
}

func (s *Server) handleSession(ch ssh.Channel, requests <-chan *ssh.Request, username string) {
	defer func() { _ = ch.Close() }()

	var env []string
	for req := range requests {
		switch req.Type {
		case "env":
			// 只放行 git 协议协商/本地化所需的少量变量；其余（GIT_TRACE*、
			// GIT_CONFIG_*、LD_* 等）一律丢弃，避免通过 SSH env 请求向 git 子进程
			// 注入任意文件写入（GIT_TRACE）或改写配置（GIT_CONFIG_*）。
			if name, value, ok := parseEnvPayload(req.Payload); ok && allowedSSHEnv(name) {
				if len(env) < maxSSHEnvEntries {
					env = append(env, name+"="+value)
				}
			}
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
		case "exec":
			cmdline := parseCommand(req.Payload)
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
			s.runGit(ch, env, cmdline, username)
			return
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

func (s *Server) runGit(ch ssh.Channel, env []string, cmdline, username string) {
	deny := func(msg string) {
		_, _ = fmt.Fprintf(ch.Stderr(), "gitdash: %s\n", msg)
		sendExit(ch, 1)
	}
	if username == "" {
		deny("authentication required")
		return
	}

	prog, args, err := splitCommandLine(cmdline)
	sub, known := gitCommands[prog]
	if err != nil || !known {
		deny("only git-upload-pack / git-receive-pack / git-upload-archive are allowed")
		return
	}
	if len(args) != 1 {
		deny("expected exactly one repository argument")
		return
	}

	m := repoPathRe.FindStringSubmatch(args[0])
	if m == nil {
		deny(fmt.Sprintf("invalid repository path %q", args[0]))
		return
	}
	first, second := m[1], m[2]
	single := second == ""

	var owner, name string
	// deploy key：身份为合成字符串（store.DeployIdentity），只能访问绑定仓库。
	if deployOwner, deployRepo, deployWrite, isDeploy := store.ParseDeployIdentity(username); isDeploy {
		if single {
			owner, name = deployOwner, first
		} else {
			owner, name = first, second
		}
		if owner != deployOwner || name != deployRepo {
			deny(fmt.Sprintf("deploy key is not authorized for %q", args[0]))
			return
		}
		if sub == "receive-pack" && !deployWrite {
			deny("deploy key is read-only")
			return
		}
	} else {
		if single {
			// 单段路径（demo.git）解析为当前登录用户自己的仓库
			owner, name = username, first
		} else {
			owner, name = first, second
		}
		if !validToken(owner) || !validToken(name) {
			deny(fmt.Sprintf("invalid repository path %q", args[0]))
			return
		}
		// 权限：push（receive-pack）需 write，clone/fetch/archive 需 read；所有者恒有全部权限
		if sub == "receive-pack" {
			allowed, err := s.authz.CanWrite(context.Background(), owner, name, username)
			if err != nil {
				logx.Infof("ssh: CanWrite(%s/%s, %s): %v", owner, name, username, err)
				deny("authorization unavailable")
				return
			}
			if !allowed {
				deny(fmt.Sprintf("repository %q not found or not accessible by %q", args[0], username))
				return
			}
		} else {
			allowed, err := s.authz.CanRead(context.Background(), owner, name, username)
			if err != nil {
				logx.Infof("ssh: CanRead(%s/%s, %s): %v", owner, name, username, err)
				deny("authorization unavailable")
				return
			}
			if !allowed {
				deny(fmt.Sprintf("repository %q not found or not accessible by %q", args[0], username))
				return
			}
		}
	}

	repoPath := filepath.Join(s.reposDir, owner, name+".git")
	if fi, err := os.Stat(repoPath); err != nil || !fi.IsDir() {
		deny(fmt.Sprintf("repository %q not found", args[0]))
		return
	}

	// 为 receive-pack 显式传入 push 体积上限，覆盖未写入仓库配置的历史仓库。
	gitArgs := []string{}
	if sub == "receive-pack" {
		if lim := gitsvc.MaxPushBytes(); lim > 0 {
			gitArgs = append(gitArgs, "-c", "receive.maxInputSize="+strconv.FormatInt(lim, 10))
		}
	}
	gitArgs = append(gitArgs, sub, repoPath)
	cmd := exec.Command("git", gitArgs...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Env = append(cmd.Env, "GITDASH_USER="+username) // post-receive hook 记录 pusher
	cmd.Stderr = ch.Stderr()

	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		sendExit(ch, 1)
		return
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		sendExit(ch, 1)
		return
	}
	cmd.Stdin = stdinR
	cmd.Stdout = stdoutW

	if err := cmd.Start(); err != nil {
		_, _ = fmt.Fprintf(ch.Stderr(), "gitdash: %v\n", err)
		sendExit(ch, 1)
		return
	}
	_ = stdinR.Close()
	_ = stdoutW.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(stdinW, ch)
		_ = stdinW.Close()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(ch, stdoutR)
		_ = ch.CloseWrite()
	}()

	err = cmd.Wait()
	wg.Wait()
	_ = stdoutR.Close()
	if sub == "receive-pack" {
		// push 改变了分支/标签集合，失效 15s TTL 缓存
		gitsvc.InvalidateRefs(owner, name)
	}

	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			code = 1
			logx.Infof("ssh exec %s %s/%s: %v", sub, owner, name, err)
		}
	}
	sendExit(ch, code)
}

// validToken 校验 owner/name 片段（含 username），必须是字母或数字开头，防止路径穿越与参数注入
func validToken(s string) bool {
	if s == "" || s[0] == '-' || s[0] == '.' {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.':
		default:
			return false
		}
	}
	return true
}

func sendExit(ch ssh.Channel, code int) {
	_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(code)}))
	_ = ch.Close()
}

func parseCommand(payload []byte) string {
	if len(payload) < 4 {
		return ""
	}
	n := binary.BigEndian.Uint32(payload[:4])
	if int(n)+4 > len(payload) {
		return ""
	}
	return string(payload[4 : 4+n])
}

func parseEnvPayload(payload []byte) (string, string, bool) {
	if len(payload) < 4 {
		return "", "", false
	}
	n := int(binary.BigEndian.Uint32(payload[:4]))
	payload = payload[4:]
	if n > len(payload) || len(payload)-n < 4 {
		return "", "", false
	}
	name := string(payload[:n])
	payload = payload[n:]
	v := int(binary.BigEndian.Uint32(payload[:4]))
	payload = payload[4:]
	if v > len(payload) {
		return "", "", false
	}
	return name, string(payload[:v]), true
}

// allowedSSHEnv 仅放行 git 协议协商所需的 GIT_PROTOCOL（v2）与本地化变量，
// 其余全部丢弃（尤其是 GIT_TRACE*/GIT_CONFIG_*/LD_*）。
func allowedSSHEnv(name string) bool {
	switch name {
	case "GIT_PROTOCOL", "LANG":
		return true
	}
	return strings.HasPrefix(name, "LC_")
}

// splitCommandLine parses something like: git-upload-pack 'alice/demo.git'
func splitCommandLine(cmdline string) (string, []string, error) {
	var fields []string
	var cur strings.Builder
	var inQuote rune
	hasField := false
	for _, r := range cmdline {
		switch {
		case inQuote != 0:
			if r == inQuote {
				inQuote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			inQuote = r
			hasField = true
		case r == ' ' || r == '\t':
			if hasField {
				fields = append(fields, cur.String())
				cur.Reset()
				hasField = false
			}
		default:
			cur.WriteRune(r)
			hasField = true
		}
	}
	if hasField {
		fields = append(fields, cur.String())
	}
	if inQuote != 0 {
		return "", nil, errors.New("unterminated quote")
	}
	if len(fields) == 0 {
		return "", nil, errors.New("empty command")
	}
	return fields[0], fields[1:], nil
}

func loadOrGenerateHostKey(path string) (ssh.Signer, error) {
	if data, err := os.ReadFile(path); err == nil {
		return ssh.ParsePrivateKey(data)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	block, err := ssh.MarshalPrivateKey(priv, "gitdash host key")
	if err != nil {
		return nil, err
	}
	pemBytes := pem.EncodeToMemory(block)
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(pemBytes)
}
