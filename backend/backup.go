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
	"sort"
	"strconv"
	"strings"
	"time"

	"gitdash/backend/internal/logx"

	_ "github.com/glebarez/sqlite" // 注册 database/sql 驱动 "sqlite"（VACUUM INTO 一致性快照）
)

// backupOptions 是 `gitdash backup` 的解析结果。
type backupOptions struct {
	dataDir string
	out     string // 完整输出文件路径（-o/--out）
	dir     string // 输出目录（-d/--dir）；未指定 out 时按目录+时间戳命名
	keep    int    // 保留份数（-k/--keep，>=1）；-1 表示不清理
	list    bool   // -l/--list：仅列出已有备份
}

// backup 命令行：打包数据目录（SQLite 用 VACUUM INTO 生成一致快照）。
func backup(args []string) error {
	opts := backupOptions{dataDir: getenv("GITDASH_DATA", "./data"), keep: -1}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--out":
			if i+1 >= len(args) {
				return fmt.Errorf("--out needs a value")
			}
			opts.out = args[i+1]
			i++
		case "-d", "--dir":
			if i+1 >= len(args) {
				return fmt.Errorf("--dir needs a value")
			}
			opts.dir = args[i+1]
			i++
		case "-k", "--keep":
			if i+1 >= len(args) {
				return fmt.Errorf("--keep needs a value")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 1 {
				return fmt.Errorf("--keep must be an integer >= 1")
			}
			opts.keep = n
			i++
		case "-l", "--list":
			opts.list = true
		default:
			return fmt.Errorf("unknown flag %q", args[i])
		}
	}

	dir := opts.dir
	if opts.out != "" {
		dir = filepath.Dir(opts.out)
	}
	if dir == "" {
		dir = "."
	}
	if opts.list {
		return listBackups(dir)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}
	out := opts.out
	if out == "" {
		out = filepath.Join(dir, fmt.Sprintf("gitdash-backup-%s.tar.gz", time.Now().UTC().Format("20060102-150405")))
	}
	if err := writeBackup(opts.dataDir, out); err != nil {
		return err
	}
	fmt.Printf("backup written: %s\n", out)
	if opts.keep >= 1 {
		removed, err := pruneBackups(dir, opts.keep)
		if err != nil {
			return err
		}
		fmt.Printf("kept newest %d backup(s) in %s", opts.keep, dir)
		if removed > 0 {
			fmt.Printf(", pruned %d old", removed)
		}
		fmt.Println()
	}
	if os.Getenv("GITDASH_DB") != "" && strings.HasPrefix(os.Getenv("GITDASH_DB"), "postgres") {
		fmt.Println("note: PostgreSQL database is NOT included; run pg_dump separately.")
	}
	return nil
}

// backupGlob 匹配内置命令与脚本生成的备份文件名。
const backupGlob = "gitdash-backup-*.tar.gz"

// listBackups 按修改时间从新到旧列出目录中的备份归档。
func listBackups(dir string) error {
	files, err := backupFiles(dir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Printf("no backups in %s\n", dir)
		return nil
	}
	for _, fi := range files {
		fmt.Printf("%s  %8s  %s\n", fi.ModTime().UTC().Format(time.RFC3339), humanSize(fi.Size()), filepath.Join(dir, fi.Name()))
	}
	return nil
}

// backupFiles 返回目录中匹配备份命名的文件，按修改时间从新到旧排序。
func backupFiles(dir string) ([]os.FileInfo, error) {
	matches, err := filepath.Glob(filepath.Join(dir, backupGlob))
	if err != nil {
		return nil, err
	}
	files := make([]os.FileInfo, 0, len(matches))
	for _, m := range matches {
		fi, err := os.Stat(m)
		if err != nil || fi.IsDir() {
			continue
		}
		files = append(files, fi)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ModTime().After(files[j].ModTime()) })
	return files, nil
}

// pruneBackups 只保留目录中最新的 keep 份备份，删除其余，返回删除数量。
func pruneBackups(dir string, keep int) (int, error) {
	if keep < 1 {
		keep = 1
	}
	files, err := backupFiles(dir)
	if err != nil {
		return 0, err
	}
	removed := 0
	for i, fi := range files {
		if i < keep {
			continue
		}
		if err := os.Remove(filepath.Join(dir, fi.Name())); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// humanSize 把字节数格式化为人读字符串（如 1.5MB）。
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// --- 自动备份（serve 模式） -------------------------------------------------

// backupDirEnv 返回自动备份输出目录；为空表示关闭自动备份。
func backupDirEnv() string { return strings.TrimSpace(os.Getenv("GITDASH_BACKUP_DIR")) }

// backupIntervalEnv 返回自动备份间隔（默认 24h，最小 1 分钟）。
func backupIntervalEnv() time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(os.Getenv("GITDASH_BACKUP_INTERVAL")))
	if err != nil || d < time.Minute {
		return 24 * time.Hour
	}
	return d
}

// backupKeepEnv 返回自动备份保留份数（默认 14）。
func backupKeepEnv() int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("GITDASH_BACKUP_KEEP")))
	if err != nil || n < 1 {
		return 14
	}
	return n
}

// autoBackupLoop 按固定间隔生成备份并保留最近 keep 份，直到进程退出。
func autoBackupLoop(dataDir, dir string, interval time.Duration, keep int) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logx.Errorf("backup: create dir %s: %v", dir, err)
		return
	}
	for {
		out := filepath.Join(dir, fmt.Sprintf("gitdash-backup-%s.tar.gz", time.Now().UTC().Format("20060102-150405")))
		if err := writeBackup(dataDir, out); err != nil {
			logx.Infof("backup: %v", err)
		} else {
			removed, perr := pruneBackups(dir, keep)
			if perr != nil {
				logx.Infof("backup prune: %v", perr)
			}
			logx.Infof("backup written: %s (kept newest %d, pruned %d)", out, keep, removed)
		}
		time.Sleep(interval)
	}
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
	dryRun := false
	for _, a := range args {
		switch a {
		case "--force", "-f":
			force = true
		case "--dry-run", "-n":
			dryRun = true
		default:
			return fmt.Errorf("unknown flag %q", a)
		}
	}
	if dryRun {
		m, err := verifyArchive(archive)
		if err != nil {
			return err
		}
		fmt.Printf("archive OK: %d file(s), %s, %d repo dir(s), db=%v, ssh_host_key=%v\n", m.Files, humanSize(m.Bytes), m.Repos, m.HasDB, m.HasSSHKey)
		fmt.Printf("would restore into %s (force=%v)\n", dataDir, force)
		return nil
	}
	if err := restoreArchive(archive, dataDir, force); err != nil {
		return err
	}
	fmt.Printf("restored %s -> %s\n", archive, dataDir)
	return nil
}

// archiveManifest 汇总归档内容，用于 `restore --dry-run` 校验。
type archiveManifest struct {
	Files     int
	Bytes     int64
	Repos     int
	HasDB     bool
	HasSSHKey bool
}

// verifyArchive 只读校验归档：gzip/tar 可解、路径安全，并统计内容。
func verifyArchive(archive string) (archiveManifest, error) {
	var m archiveManifest
	f, err := os.Open(archive)
	if err != nil {
		return m, err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return m, fmt.Errorf("not a gzip archive: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return m, fmt.Errorf("corrupt tar: %w", err)
		}
		if _, err := safeArchivePath("/", hdr.Name); err != nil {
			return m, err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if strings.HasSuffix(hdr.Name, ".git/") {
				m.Repos++
			}
		case tar.TypeReg:
			m.Files++
			m.Bytes += hdr.Size
			if filepath.Base(hdr.Name) == "gitdash.db" {
				m.HasDB = true
			}
			if filepath.Base(hdr.Name) == "ssh_host_ed25519_key" {
				m.HasSSHKey = true
			}
		}
	}
	return m, nil
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
