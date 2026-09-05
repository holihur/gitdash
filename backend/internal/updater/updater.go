// Package updater 实现 gitdash 的自更新：
// 查询 GitHub 最新 release -> 下载对应平台压缩包 -> SHA256 校验 -> 原子替换当前二进制。
package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const DefaultRepo = "holihur/gitdash"

const maxBinarySize = 512 << 20 // 512MB

var httpClient = &http.Client{Timeout: 15 * time.Minute}

// Repo 返回 release 来源仓库，可用 GITDASH_UPDATE_REPO 覆盖（fork / 测试）。
func Repo() string {
	if v := strings.TrimSpace(os.Getenv("GITDASH_UPDATE_REPO")); v != "" {
		return v
	}
	return DefaultRepo
}

type Release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// FetchLatest 查询最新 release。
func FetchLatest(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", Repo())
	data, err := httpGet(ctx, url)
	if err != nil {
		return nil, err
	}
	var rel Release
	if err := json.Unmarshal(data, &rel); err != nil {
		return nil, err
	}
	if rel.TagName == "" {
		return nil, errors.New("release tag_name 为空")
	}
	return &rel, nil
}

// SelfUpdate 检查并应用更新，返回 (新版本, 是否发生更新, 错误)。
// currentVersion 为 "dev" 时无条件更新为最新 release。
func SelfUpdate(ctx context.Context, currentVersion string) (string, bool, error) {
	rel, err := FetchLatest(ctx)
	if err != nil {
		return "", false, err
	}
	if currentVersion != "dev" && CompareVersions(currentVersion, rel.TagName) >= 0 {
		return rel.TagName, false, nil
	}
	if err := apply(ctx, rel); err != nil {
		return "", false, err
	}
	return rel.TagName, true, nil
}

func apply(ctx context.Context, rel *Release) error {
	ver := strings.TrimPrefix(rel.TagName, "v")
	goos, goarch := runtime.GOOS, runtime.GOARCH

	archiveName := fmt.Sprintf("gitdash_%s_%s_%s.tar.gz", ver, goos, goarch)
	binaryName := "gitdash"
	if goos == "windows" {
		archiveName = fmt.Sprintf("gitdash_%s_%s_%s.zip", ver, goos, goarch)
		binaryName = "gitdash.exe"
	}

	var archiveURL, sumsURL string
	for _, a := range rel.Assets {
		switch a.Name {
		case archiveName:
			archiveURL = a.BrowserDownloadURL
		case "checksums.txt":
			sumsURL = a.BrowserDownloadURL
		}
	}
	if archiveURL == "" {
		return fmt.Errorf("release %s 没有 %s 构建产物", rel.TagName, goos+"/"+goarch)
	}
	if sumsURL == "" {
		return fmt.Errorf("release %s 缺少 checksums.txt", rel.TagName)
	}

	fmt.Printf("下载 %s\n", archiveURL)
	archive, err := httpGet(ctx, archiveURL)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	sums, err := httpGet(ctx, sumsURL)
	if err != nil {
		return fmt.Errorf("下载 checksums.txt 失败: %w", err)
	}
	if err := VerifyChecksum(archive, sums, archiveName); err != nil {
		return fmt.Errorf("校验失败: %w", err)
	}
	bin, err := ExtractBinary(archive, goos, binaryName)
	if err != nil {
		return err
	}

	target, err := os.Executable()
	if err != nil {
		return fmt.Errorf("定位当前二进制失败: %w", err)
	}
	realPath, err := filepath.EvalSymlinks(target)
	if err != nil {
		realPath = target
	}
	dir := filepath.Dir(realPath)
	tmpPath := fmt.Sprintf("%s.new-%d", realPath, os.Getpid())
	if err := os.WriteFile(tmpPath, bin, 0o755); err != nil {
		return fmt.Errorf("写入 %s 失败（可能需要 sudo）: %w", dir, err)
	}
	// Windows 上无法覆盖运行中的 exe，先把旧文件挪走
	if goos == "windows" {
		_ = os.Remove(realPath + ".old")
		_ = os.Rename(realPath, realPath+".old")
	}
	if err := os.Rename(tmpPath, realPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("替换二进制失败: %w", err)
	}
	return nil
}

func ExtractBinary(archive []byte, goos, binaryName string) ([]byte, error) {
	if goos == "windows" {
		return extractFromZip(archive, binaryName)
	}
	return extractFromTarGz(archive, binaryName)
}

func extractFromTarGz(archive []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag == tar.TypeReg && path.Base(hdr.Name) == name {
			data, err := io.ReadAll(io.LimitReader(tr, maxBinarySize))
			if err != nil {
				return nil, err
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("压缩包中未找到 %s", name)
}

func extractFromZip(archive []byte, name string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if path.Base(f.Name) == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer func() { _ = rc.Close() }()
			return io.ReadAll(io.LimitReader(rc, maxBinarySize))
		}
	}
	return nil, fmt.Errorf("压缩包中未找到 %s", name)
}

func VerifyChecksum(archive, sums []byte, filename string) error {
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == filename {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("checksums.txt 中没有 %s", filename)
	}
	sum := sha256.Sum256(archive)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), want) {
		return errors.New("sha256 不匹配")
	}
	return nil
}

func httpGet(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "gitdash-updater")
		req.Header.Set("Accept", "application/vnd.github+json")
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxBinarySize))
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
			continue
		}
		return data, nil
	}
	return nil, lastErr
}

// CompareVersions 语义化版本比较（忽略 v 前缀与 -next 之类后缀）。
func CompareVersions(a, b string) int {
	pa, pb := parseVersion(a), parseVersion(b)
	for i := range pa {
		if pa[i] != pb[i] {
			if pa[i] > pb[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}

func parseVersion(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out [3]int
	for i, p := range strings.SplitN(v, ".", 3) {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			continue
		}
		out[i] = n
		if i == 2 {
			break
		}
	}
	return out
}
