package webui

import (
	"bytes"
	"embed"
	"io/fs"
)

// dist 在编译期由 goreleaser/CI 的 embed-frontend 钩子填充，
// 仓库中默认只包含 .gitkeep 占位，此时 HasAssets() 返回 false。
//
//go:embed all:dist
var embedded embed.FS

// Dist 返回以 dist/ 为根的前端静态文件系统。
func Dist() fs.FS {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil
	}
	return sub
}

// HasAssets 报告是否嵌入了真实的前端构建产物。
func HasAssets() bool {
	b, err := fs.ReadFile(embedded, "dist/index.html")
	return err == nil && !bytes.Contains(b, []byte("PLACEHOLDER"))
}
