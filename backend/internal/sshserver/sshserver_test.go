package sshserver

import (
	"encoding/binary"
	"testing"
)

func TestSplitCommandLine(t *testing.T) {
	prog, args, err := splitCommandLine(`git-upload-pack 'demo.git'`)
	if err != nil || prog != "git-upload-pack" || len(args) != 1 || args[0] != "demo.git" {
		t.Errorf("got %q %q %v", prog, args, err)
	}

	prog, args, err = splitCommandLine(`git-receive-pack "my repo.git"`)
	if err != nil || prog != "git-receive-pack" || len(args) != 1 || args[0] != "my repo.git" {
		t.Errorf("got %q %q %v", prog, args, err)
	}

	prog, args, err = splitCommandLine("git-upload-archive 'a.git'")
	if err != nil || prog != "git-upload-archive" || len(args) != 1 || args[0] != "a.git" {
		t.Errorf("got %q %q %v", prog, args, err)
	}

	if _, _, err := splitCommandLine(`git-upload-pack 'unterminated`); err == nil {
		t.Error("expected error for unterminated quote")
	}
	if _, _, err := splitCommandLine(""); err == nil {
		t.Error("expected error for empty command")
	}
}

func TestParseCommand(t *testing.T) {
	build := func(cmd string) []byte {
		buf := make([]byte, 4+len(cmd))
		binary.BigEndian.PutUint32(buf[:4], uint32(len(cmd)))
		copy(buf[4:], cmd)
		return buf
	}
	if got := parseCommand(build("git-upload-pack '/x.git'")); got != "git-upload-pack '/x.git'" {
		t.Errorf("parseCommand = %q", got)
	}
	if got := parseCommand([]byte{0, 0, 0, 99, 'x'}); got != "" {
		t.Errorf("parseCommand truncated = %q", got)
	}
}

func TestParseEnvPayload(t *testing.T) {
	build := func(name, value string) []byte {
		buf := make([]byte, 0, 4+len(name)+4+len(value))
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(name)))
		buf = append(buf, n[:]...)
		buf = append(buf, name...)
		var v [4]byte
		binary.BigEndian.PutUint32(v[:], uint32(len(value)))
		buf = append(buf, v[:]...)
		buf = append(buf, value...)
		return buf
	}
	name, value, ok := parseEnvPayload(build("GIT_PROTOCOL", "version=2"))
	if !ok || name != "GIT_PROTOCOL" || value != "version=2" {
		t.Errorf("got %q %q %v", name, value, ok)
	}
	if _, _, ok := parseEnvPayload([]byte{0, 0}); ok {
		t.Error("short payload should fail")
	}
}

func TestRepoNameRegex(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"demo.git", "demo"},
		{"/demo.git", "demo"},
		{"demo", "demo"},
		{"/demo/", "demo"},
		{"nested/repo.git", ""},
		{"../evil", ""},
		{"", ""},
	}
	for _, c := range cases {
		m := repoNameRe.FindStringSubmatch(c.in)
		got := ""
		if m != nil {
			got = m[1]
		}
		if got != c.want {
			t.Errorf("repoNameRe(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
