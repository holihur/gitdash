package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"gitdash/backend/internal/api"
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/sshserver"
	"gitdash/backend/internal/store"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func resolveStaticDir() string {
	for _, c := range []string{"./static", "../frontend/dist"} {
		if fi, err := os.Stat(filepath.Join(c, "index.html")); err == nil && !fi.IsDir() {
			abs, err := filepath.Abs(c)
			if err == nil {
				return abs
			}
		}
	}
	return ""
}

func main() {
	dataDir := getenv("GITDASH_DATA", "./data")
	httpAddr := getenv("GITDASH_HTTP_ADDR", ":8080")
	sshAddr := getenv("GITDASH_SSH_ADDR", ":2222")
	token := getenv("GITDASH_TOKEN", "dev")
	staticDir := getenv("GITDASH_STATIC", "")

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}
	if err := gitsvc.Init(dataDir); err != nil {
		log.Fatalf("init git service: %v", err)
	}

	st, err := store.Open(filepath.Join(dataDir, "gitdash.db"))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	go func() {
		log.Printf("git ssh server listening on %s", sshAddr)
		if err := sshserver.Serve(sshAddr, st, gitsvc.ReposDir(), dataDir); err != nil {
			log.Fatalf("ssh server: %v", err)
		}
	}()

	a := api.New(st, token)
	if staticDir == "" {
		staticDir = resolveStaticDir()
	}

	log.Printf("gitdash: http on %s | ssh on %s | data in %s", httpAddr, sshAddr, dataDir)
	log.Fatal(http.ListenAndServe(httpAddr, a.Handler(staticDir)))
}
