package api

import (
	"net/http"
	"strings"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/pipeline"
)

// 本文件只负责流水线的「定义/配置」：开关、文件发现与可视化图。
// 运行相关（触发/取消/重跑/查询）见 pipeline_runs.go。

// getPipeline 获取仓库流水线开关状态。
//
//	@Summary     获取流水线配置
//	@Tags        pipeline
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {object} map[string]any
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pipeline [get]
func (a *API) getPipeline(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	p, err := a.store.GetPipeline(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	// 列出默认分支上的流水线文件（多文件支持）
	files := []string{}
	if hb, herr := gitsvc.HeadBranch(owner, name); herr == nil && hb != "" {
		files = pipeline.DiscoverFiles(owner, name, hb)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": p.Enabled,
		"file":    pipeline.FileName,
		"files":   files,
	})
}

// getPipelineGraph 返回流水线的可视化图（步骤 DAG 节点/边），供前端布局渲染。
//
//	@Summary     流水线可视化图
//	@Tags        pipeline
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       ref   query string false "分支或 tag（默认仓库默认分支）"
//	@Success     200 {object} pipeline.Graph
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pipeline/graph [get]
func (a *API) getPipelineGraph(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	if ref == "" {
		if hb, err := gitsvc.HeadBranch(owner, name); err == nil {
			ref = hb
		}
	}
	if ref == "" {
		writeCode(w, http.StatusBadRequest, "ref_required", "repository has no default branch")
		return
	}
	file := strings.TrimSpace(r.URL.Query().Get("file"))
	if file == "" {
		if files := pipeline.DiscoverFiles(owner, name, ref); len(files) > 0 {
			file = files[0]
		}
	}
	if file == "" {
		file = pipeline.FileName
	}
	blob, err := gitsvc.ReadBlob(owner, name, ref, file)
	if err != nil || blob.Encoding != "utf-8" || strings.TrimSpace(blob.Content) == "" {
		writeCode(w, http.StatusNotFound, "pipeline_not_found", "no "+file+" at "+ref)
		return
	}
	cfg, perr := pipeline.Parse([]byte(blob.Content))
	if perr != nil {
		writeCode(w, http.StatusBadRequest, "pipeline_invalid", perr.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ref":     ref,
		"file":    file,
		"image":   cfg.Image,
		"timeout": cfg.Timeout.String(),
		"graph":   cfg.Graph(),
	})
}

// setPipeline 启用/禁用流水线（仅 owner）。
//
//	@Summary     设置流水线开关
//	@Tags        pipeline
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body setPipelineReq true "{enabled: bool}"
//	@Success     200 {object} map[string]any
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pipeline [put]
func (a *API) setPipeline(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "maintain")
	if !ok {
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if err := a.store.SetPipeline(owner, name, in.Enabled); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": in.Enabled})
}
