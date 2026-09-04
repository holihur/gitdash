package sshserver

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"

	"gitdash/backend/internal/store"
)

// 匹配 "demo.git"（owner=空，解析为当前登录用户）或 "alice/demo.git"
var repoPathRe = regexp.MustCompile(`^/?([\w.-]+?)(?:/([\w.-]+?))?(?:\.git)?/?$`)

var gitCommands = map[string]string{
	"git-upload-pack":    "upload-pack",
	"git-receive-pack":   "receive-pack",
	"git-upload-archive": "upload-archive",
}

type Server struct {
	reposDir string
	config   *ssh.ServerConfig
}

// NewServer 创建 SSH git 服务（供 main 与测试使用）。
func NewServer(st *store.Store, reposDir, dataDir string) (*Server, error) {
	signer, err := loadOrGenerateHostKey(filepath.Join(dataDir, "ssh_host_ed25519_key"))
	if err != nil {
		return nil, fmt.Errorf("host key: %w", err)
	}
	cfg := buildConfig(st)
	cfg.AddHostKey(signer)
	return &Server{reposDir: reposDir, config: cfg}, nil
}

func Serve(addr string, st *store.Store, reposDir, dataDir string) error {
	s, err := NewServer(st, reposDir, dataDir)
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
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			log.Printf("ssh accept: %v", err)
			continue
		}
		go s.handleConn(conn, s.config)
	}
}

func buildConfig(st *store.Store) *ssh.ServerConfig {
	return &ssh.ServerConfig{
		ServerVersion: "SSH-2.0-gitdash",
		PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			keys, err := st.PublicKeys()
			if err != nil {
				return nil, err
			}
			for _, ka := range keys {
				parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(ka.Line))
				if err != nil {
					continue
				}
				if parsed.Type() == key.Type() && bytes.Equal(parsed.Marshal(), key.Marshal()) {
					fp := ssh.FingerprintSHA256(key)
					log.Printf("ssh: %s authenticated as user %q with key %s", meta.User(), ka.Username, fp)
					return &ssh.Permissions{Extensions: map[string]string{"username": ka.Username}}, nil
				}
			}
			log.Printf("ssh: rejected %s, unknown key %s", meta.User(), ssh.FingerprintSHA256(key))
			return nil, fmt.Errorf("unknown public key")
		},
	}
}

func (s *Server) handleConn(conn net.Conn, config *ssh.ServerConfig) {
	sconn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		log.Printf("ssh handshake from %s: %v", conn.RemoteAddr(), err)
		return
	}
	defer sconn.Close()
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
	defer ch.Close()

	var env []string
	started := false
	for req := range requests {
		switch req.Type {
		case "env":
			if name, value, ok := parseEnvPayload(req.Payload); ok {
				env = append(env, name+"="+value)
			}
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
		case "exec":
			if started {
				if req.WantReply {
					_ = req.Reply(false, nil)
				}
				continue
			}
			started = true
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
		fmt.Fprintf(ch.Stderr(), "gitdash: %s\n", msg)
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
	owner, name := m[1], m[2]
	if name == "" {
		// 单段路径（demo.git）解析为当前登录用户自己的仓库
		name, owner = owner, username
	}
	if !validToken(owner) || !validToken(name) {
		deny(fmt.Sprintf("invalid repository path %q", args[0]))
		return
	}
	if owner != username {
		deny(fmt.Sprintf("repository %q not found or not accessible by %q", args[0], username))
		return
	}

	repoPath := filepath.Join(s.reposDir, owner, name+".git")
	if fi, err := os.Stat(repoPath); err != nil || !fi.IsDir() {
		deny(fmt.Sprintf("repository %q not found", args[0]))
		return
	}

	cmd := exec.Command("git", sub, repoPath)
	cmd.Env = append(os.Environ(), env...)
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
		fmt.Fprintf(ch.Stderr(), "gitdash: %v\n", err)
		sendExit(ch, 1)
		return
	}
	stdinR.Close()
	stdoutW.Close()

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
	stdoutR.Close()

	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			code = 1
			log.Printf("ssh exec %s %s/%s: %v", sub, owner, name, err)
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
