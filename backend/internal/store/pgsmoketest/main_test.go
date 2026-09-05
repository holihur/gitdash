package pgsmoketest

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"gitdash/backend/internal/store"
)

func TestPGSmoke(t *testing.T) {
	if os.Getenv("GITDASH_DB") == "" {
		t.Skip("set GITDASH_DB to run")
	}
	s, err := store.OpenDSN(os.Getenv("GITDASH_DB"))
	if err != nil {
		panic(err)
	}
	// 幂等：清空相关表
	for _, tbl := range []string{"repo_stars", "repo_watches", "sessions", "repos", "users"} {
		if derr := s.DB().Exec("DELETE FROM " + tbl).Error; derr != nil {
			panic(derr)
		}
	}
	u, err := s.CreateUser("alice", "h")
	if err != nil {
		panic(err)
	}
	if _, err := s.CreateRepo("alice", "ci", "", false); err != nil {
		panic(err)
	}
	rr, _ := s.GetRepo("alice", "ci")
	fmt.Println("user", u.ID, u.Username, "| private =", rr.Private)
	if rr.Private {
		panic("private should be false")
	}
	if _, err := s.CreateRepo("alice", "ci", "", false); !errors.Is(err, store.ErrExists) {
		panic("want ErrExists, got " + fmt.Sprint(err))
	}
	if err := s.CreateSession("t1", u.ID); err != nil {
		panic(err)
	}
	if name, err := s.GetSession("t1"); err != nil || name != "alice" {
		panic("session")
	}
	fmt.Println("PG SMOKE OK")
}
