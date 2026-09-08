package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"

	"gitdash/backend/internal/store"
)

// ---- cargo ----

// cargoConfig cargo registry config
//
//	@Summary     cargo config
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Router      /packages/cargo/{owner}/config.json [get]
func (a *API) cargoConfig(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	base := baseURL(r)
	writeJSON(w, http.StatusOK, map[string]string{
		"dl":  base + "/api/packages/cargo/" + owner + "/dl",
		"api": base + "/api/packages/cargo/" + owner,
	})
}

// cargoPublish cargo publish
//
//	@Summary     cargo 发布
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Success     201
//	@Router      /packages/cargo/{owner}/api/v1/crates/new [put]
func (a *API) cargoPublish(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	body, ok := limitBody(w, r)
	if !ok {
		return
	}
	// cargo publish：第一行 JSON 元数据，随后是 .crate tarball
	idx := bytes.IndexByte(body, '\n')
	if idx < 0 {
		writeCode(w, http.StatusBadRequest, "invalid_cargo_payload", "missing metadata line")
		return
	}
	var meta struct {
		Name     string `json:"name"`
		Vers     string `json:"vers"`
		Checksum string `json:"cksum"`
	}
	if err := json.Unmarshal(body[:idx], &meta); err != nil || meta.Name == "" || meta.Vers == "" {
		writeCode(w, http.StatusBadRequest, "invalid_cargo_payload", "bad metadata JSON")
		return
	}
	content := body[idx+1:]
	if meta.Checksum != "" {
		sum := sha256.Sum256(content)
		if hex.EncodeToString(sum[:]) != meta.Checksum {
			writeCode(w, http.StatusBadRequest, "cksum_mismatch", "crate checksum mismatch")
			return
		}
	}
	depsJSON, featuresJSON := cargoIndexMeta(content)
	p := &store.Package{Owner: owner, Repo: strings.TrimPrefix(r.Header.Get("X-Gitdash-Repo"), "/"),
		Type: "cargo", Name: meta.Name, Version: meta.Vers, Filename: meta.Name + "-" + meta.Vers + ".crate",
		Uploader: pkgUser(r), Meta: `{"deps":` + depsJSON + `,"features":` + featuresJSON + `}`}
	user := pkgUser(r)
	if !a.canPublishPackage(owner, user) {
		pkgForbidden(w)
		return
	}
	if err := a.store.CreatePackage(p, content); err != nil {
		if errors.Is(err, store.ErrExists) {
			writeCode(w, http.StatusConflict, "package_exists", "package version already exists")
			return
		}
		internalError(w, err)
		return
	}
	_ = a.store.AddPackageAudit(owner, "cargo", meta.Name, meta.Vers, "publish", user)
	writeJSON(w, http.StatusCreated, p)
}

// cargoIndexMeta 解析 .crate（gzip tar）内的 Cargo.toml，产出稀疏索引所需
// 的 deps 与 features JSON片段。解析失败返回空数组/空对象（不影响发布）。
func cargoIndexMeta(crate []byte) (depsJSON, featuresJSON string) {
	deps, features := []cargoDep{}, map[string][]string{}
	gz, err := gzip.NewReader(bytes.NewReader(crate))
	if err != nil {
		return "[]", "{}"
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	prefix := ""
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		if prefix == "" && strings.HasSuffix(hdr.Name, "/Cargo.toml") && strings.Count(hdr.Name, "/") == 1 {
			prefix = hdr.Name
			b, err := io.ReadAll(io.LimitReader(tr, 4<<20))
			if err == nil {
				deps, features = parseCargoToml(string(b))
			}
			break
		}
	}
	_ = prefix
	db, _ := json.Marshal(deps)
	fb, _ := json.Marshal(features)
	return string(db), string(fb)
}

type cargoDep struct {
	Name            string   `json:"name"`
	Req             string   `json:"req"`
	Kind            string   `json:"kind,omitempty"` // dev / build；normal 省略
	Optional        bool     `json:"optional,omitempty"`
	DefaultFeatures bool     `json:"default_features"`
	Features        []string `json:"features,omitempty"`
	Package         string   `json:"package,omitempty"`
}

var cargoTomlKeyRe = regexp.MustCompile(`^([A-Za-z0-9_.-]+)\s*=\s*(.+)$`)

// parseCargoToml 极简 TOML 子集解析（仅依赖与特性段）。
func parseCargoToml(src string) ([]cargoDep, map[string][]string) {
	var deps []cargoDep
	features := map[string][]string{}
	section := ""
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.Trim(line, "[]")
			continue
		}
		m := cargoTomlKeyRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key, val := m[1], strings.TrimSpace(m[2])
		switch section {
		case "features":
			if arr, ok := parseTomlArray(val); ok {
				if arr == nil {
					arr = []string{}
				}
				features[key] = arr
			}
		case "dependencies", "dev-dependencies", "build-dependencies":
			kind := ""
			switch section {
			case "dev-dependencies":
				kind = "dev"
			case "build-dependencies":
				kind = "build"
			}
			d := cargoDep{Name: key, DefaultFeatures: true, Kind: kind}
			if strings.HasPrefix(val, "{") {
				// TOML inline table 的 bare key 转成 JSON 引号键再解析
				jsonish := regexp.MustCompile(`([{,]\s*)([A-Za-z0-9_.-]+)\s*=`).
					ReplaceAllString(val, `$1"$2":`)
				jsonish = strings.TrimSuffix(jsonish, "}") + "}"
				// TOML 尾逗号清理
				jsonish = regexp.MustCompile(`,\s*}`).ReplaceAllString(jsonish, "}")
				var tbl map[string]any
				if err := json.Unmarshal([]byte(jsonish), &tbl); err != nil {
					continue
				}
				if v, ok := tbl["package"].(string); ok {
					d.Package = v
				}
				if v, ok := tbl["version"].(string); ok {
					d.Req = v
				} else if _, has := tbl["version"]; !has {
					if _, ok := tbl["path"]; ok {
						d.Req = "*"
					}
				}
				if v, ok := tbl["optional"].(bool); ok {
					d.Optional = v
				}
				if v, ok := tbl["default-features"].(bool); ok {
					d.DefaultFeatures = v
				}
				if v, ok := tbl["features"].([]any); ok {
					for _, f := range v {
						if s, ok := f.(string); ok {
							d.Features = append(d.Features, s)
						}
					}
				}
			} else {
				d.Req = strings.Trim(val, `"`)
			}
			deps = append(deps, d)
		}
	}
	return deps, features
}

// parseTomlArray 解析 ["a", "b"] 形式的数组字面量。
func parseTomlArray(s string) ([]string, bool) {
	if !strings.HasPrefix(s, "[") {
		return nil, false
	}
	end := strings.LastIndex(s, "]")
	if end < 0 {
		return nil, false
	}
	inner := s[1:end]
	var out []string
	for _, part := range strings.Split(inner, ",") {
		part = strings.TrimSpace(part)
		part = strings.Split(part, "#")[0] // 去注释
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.HasPrefix(part, `"`) || !strings.HasSuffix(part, `"`) {
			return nil, false
		}
		out = append(out, strings.Trim(part, `"`))
	}
	return out, true
}

// cargoIndex cargo 稀疏索引
//
//	@Summary     cargo 索引
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Param       rest  path string true "稀疏索引路径（最后一段为 crate 名）"
//	@Router      /packages/cargo/{owner}/index/{rest} [get]
func (a *API) cargoIndex(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	rest := r.PathValue("rest")
	crate := path.Base(rest) // 稀疏索引路径最后一段即 crate 名
	if !a.canReadPackage(owner, "cargo", crate, pkgUser(r)) {
		pkgForbidden(w)
		return
	}
	pkgs, _ := a.store.ListPackageVersions(owner, "cargo", crate)
	if len(pkgs) == 0 {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	var lines []string
	for _, p := range pkgs {
		yanked := "false"
		if p.Yanked {
			yanked = "true"
		}
		deps, features := "[]", "{}"
		if p.Meta != "" {
			var m struct {
				Deps     json.RawMessage     `json:"deps"`
				Features map[string][]string `json:"features"`
			}
			if json.Unmarshal([]byte(p.Meta), &m) == nil && len(m.Deps) > 0 {
				deps = string(m.Deps)
			}
			if m.Features != nil {
				if fb, err := json.Marshal(m.Features); err == nil {
					features = string(fb)
				}
			}
		}
		lines = append(lines, fmt.Sprintf(
			`{"name":%q,"vers":%q,"deps":%s,"cksum":%q,"features":%s,"yanked":%s,"link":null}`,
			p.Name, p.Version, deps, p.Checksum, features, yanked))
	}
	sort.Strings(lines)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for _, l := range lines {
		_, _ = w.Write([]byte(l + "\n"))
	}
}

// cargoYank cargo yank / unyank
//
//	@Summary     cargo yank / unyank
//	@Tags        packages
//	@Param       owner   path string true "用户或组织"
//	@Param       crate   path string true "crate 名"
//	@Param       version path string true "版本"
//	@Success     200
//	@Router      /packages/cargo/{owner}/api/v1/crates/{crate}/{version}/yank [delete]
func (a *API) cargoYank(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	crate := r.PathValue("crate")
	version := r.PathValue("version")
	user := pkgUser(r)
	if !a.canPublishPackage(owner, user) {
		pkgForbidden(w)
		return
	}
	yanked := r.Method != http.MethodPut
	if err := a.store.SetPackageYanked(owner, "cargo", crate, version, yanked); err != nil {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	action := "unyank"
	if yanked {
		action = "yank"
	}
	_ = a.store.AddPackageAudit(owner, "cargo", crate, version, action, user)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "warnings": map[string]any{}})
}

func (a *API) cargoDownload(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	crate := r.PathValue("crate")
	version := r.PathValue("version")
	filename := r.PathValue("filename")
	p, content, err := a.store.GetPackageFile(owner, "cargo", crate, version, filename)
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	servePackageBytes(w, p, content, "application/octet-stream")
}
