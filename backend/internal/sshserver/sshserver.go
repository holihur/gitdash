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

var repoNameRe = regexp.MustCompile(`^/?([\w.-]+?)(?:\.git)?/?$`)

var gitCommands = map[string]string{
	"git-upload-pack":    "upload-pack",
	"git-receive-pack":   "receive-pack",
	"git-upload-archive": "upload-archive",
}

type Server struct {
	reposDir string
}

func Serve(addr string, st *store.Store, reposDir, dataDir string) error {
	s := &Server{reposDir: reposDir}
	signer, err := loadOrGenerateHostKey(filepath.Join(dataDir, "ssh_host_ed25519_key"))
	if err != nil {
		return fmt.Errorf("host key: %w", err)
	}

	config := &ssh.ServerConfig{
		ServerVersion: "SSH-2.0-gitdash",
		PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			keys, err := st.PublicKeys()
			if err != nil {
				return nil, err
			}
			for _, line := range keys {
				parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
				if err != nil {
					continue
				}
				if parsed.Type() == key.Type() && bytes.Equal(parsed.Marshal(), key.Marshal()) {
					fp := ssh.FingerprintSHA256(key)
					log.Printf("ssh: %s authenticated with key %s", meta.User(), fp)
					return &ssh.Permissions{Extensions: map[string]string{"fingerprint": fp}}, nil
				}
			}
			log.Printf("ssh: rejected %s, unknown key %s", meta.User(), ssh.FingerprintSHA256(key))
			return nil, fmt.Errorf("unknown public key")
		},
	}
	config.AddHostKey(signer)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			log.Printf("ssh accept: %v", err)
			continue
		}
		go s.handleConn(conn, config)
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

	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			_ = newCh.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}
		ch, requests, err := newCh.Accept()
		if err != nil {
			continue
		}
		go s.handleSession(ch, requests)
	}
}

func (s *Server) handleSession(ch ssh.Channel, requests <-chan *ssh.Request) {
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
			s.runGit(ch, env, cmdline)
			return
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

func (s *Server) runGit(ch ssh.Channel, env []string, cmdline string) {
	prog, args, err := splitCommandLine(cmdline)
	sub, known := gitCommands[prog]
	if err != nil || !known {
		fmt.Fprintf(ch.Stderr(), "gitdash: only git-upload-pack / git-receive-pack / git-upload-archive are allowed\n")
		sendExit(ch, 1)
		return
	}
	if len(args) != 1 {
		fmt.Fprintf(ch.Stderr(), "gitdash: expected exactly one repository argument\n")
		sendExit(ch, 1)
		return
	}
	m := repoNameRe.FindStringSubmatch(args[0])
	if m == nil {
		fmt.Fprintf(ch.Stderr(), "gitdash: invalid repository path %q\n", args[0])
		sendExit(ch, 1)
		return
	}
	repoName := m[1]
	repoPath := filepath.Join(s.reposDir, repoName+".git")
	if fi, err := os.Stat(repoPath); err != nil || !fi.IsDir() {
		fmt.Fprintf(ch.Stderr(), "gitdash: repository %q not found\n", repoName)
		sendExit(ch, 1)
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
			log.Printf("ssh exec %s %s: %v", sub, repoName, err)
		}
	}
	sendExit(ch, code)
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

// splitCommandLine parses something like: git-upload-pack 'demo.git'
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
