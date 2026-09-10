package main

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/glebarez/sqlite" // 注册 database/sql 驱动 "sqlite"（VACUUM INTO 一致性快照）
)

// backup 命令行：打包数据目录（SQLite 用 VACUUM INTO 生成一致快照）。
func backup(args []string) error {
	dataDir := getenv("GITDASH_DATA", "./data")
	out := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--out":
			if i+1 < len(args) {
				out = args[i+1]
				i++
			}
		}
	}
	if out == "" {
		out = fmt.Sprintf("gitdash-backup-%s.tar.gz", time.Now().UTC().Format("20060102-150405"))
	}
	if err := writeBackup(dataDir, out); err != nil {
		return err
	}
	fmt.Printf("backup written: %s\n", out)
	if os.Getenv("GITDASH_DB") != "" && strings.HasPrefix(os.Getenv("GITDASH_DB"), "postgres") {
		fmt.Println("note: PostgreSQL database is NOT included; run pg_dump separately.")
	}
	return nil
}

// writeBackup 把 dataDir 打包为 out（tar.gz）。SQLite 主库用 VACUUM INTO 生成一致副本。
func writeBackup(dataDir, out string) error {
	fi, err := os.Stat(dataDir)
	if err != nil || !fi.IsDir() {
		return fmt.Errorf("data dir %q not found", dataDir)
	}
	// SQLite 快照
	var snapPath string
	if os.Getenv("GITDASH_DB") == "" {
		src := filepath.Join(dataDir, "gitdash.db")
		if _, err := os.Stat(src); err == nil {
			tmp, err := os.CreateTemp("", "gitdash-snap-*.db")
			if err != nil {
				return err
			}
			snapPath = tmp.Name()
			_ = tmp.Close()
			_ = os.Remove(snapPath) // VACUUM INTO 要求目标不存在
			defer func() { _ = os.Remove(snapPath) }()
			db, err := sql.Open("sqlite", src)
			if err != nil {
				return fmt.Errorf("open sqlite: %w", err)
			}
			defer func() { _ = db.Close() }()
			if _, err := db.Exec("VACUUM INTO ?", snapPath); err != nil {
				return fmt.Errorf("sqlite snapshot: %w", err)
			}
		}
	}

	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	skip := map[string]bool{"gitdash.db": true, "gitdash.db-wal": true, "gitdash.db-shm": true}
	walkErr := filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dataDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if info.IsDir() {
			return tw.WriteHeader(&tar.Header{Name: rel + "/", Typeflag: tar.TypeDir, Mode: 0o755, ModTime: info.ModTime()})
		}
		if skip[rel] {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil // 跳过符号链接/设备等
		}
		if err := writeTarFile(tw, path, rel, info); err != nil {
			return err
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	// 写入一致的数据库快照
	if snapPath != "" {
		info, err := os.Stat(snapPath)
		if err != nil {
			return err
		}
		if err := writeTarFile(tw, snapPath, "gitdash.db", info); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func writeTarFile(tw *tar.Writer, path, name string, info os.FileInfo) error {
	hdr := &tar.Header{Name: name, Mode: int64(info.Mode().Perm()), Size: info.Size(), ModTime: info.ModTime(), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	_, err = io.Copy(tw, src)
	return err
}

// restore 命令行：从归档恢复到数据目录。
func restore(archive string, args []string) error {
	dataDir := getenv("GITDASH_DATA", "./data")
	force := false
	for _, a := range args {
		if a == "--force" || a == "-f" {
			force = true
		}
	}
	if err := restoreArchive(archive, dataDir, force); err != nil {
		return err
	}
	fmt.Printf("restored %s -> %s\n", archive, dataDir)
	return nil
}

// restoreArchive 解包归档到 dataDir。拒绝路径穿越；非空数据目录需 force。
func restoreArchive(archive, dataDir string, force bool) error {
	if _, err := os.Stat(archive); err != nil {
		return err
	}
	if !force {
		for _, marker := range []string{"gitdash.db", "repos"} {
			if _, err := os.Stat(filepath.Join(dataDir, marker)); err == nil {
				return fmt.Errorf("data dir %q is not empty (use --force to overwrite)", dataDir)
			}
		}
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		target, err := safeArchivePath(dataDir, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			dst, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(dst, tr); err != nil { // #nosec G110 -- 归档为本地可信文件
				_ = dst.Close()
				return err
			}
			_ = dst.Close()
		default:
			// 忽略符号链接等特殊条目，避免恢复出越界链接
		}
	}
	return nil
}

// safeArchivePath 解析归档条目路径，拒绝绝对路径与 ../ 穿越。
func safeArchivePath(base, name string) (string, error) {
	clean := filepath.Clean(name)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe path %q in archive", name)
	}
	return filepath.Join(base, clean), nil
}
