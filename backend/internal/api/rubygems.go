package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"regexp"
)

// ---- rubygems ----

var gemNameRe = regexp.MustCompile(`(?m)^\s*name:\s*(\S+)`)

// 真实版本行为 "  version: 1.0.0"（首字符为数字）；跳过 "version: !ruby/object:..." 行
var gemVersionRe = regexp.MustCompile(`(?m)^\s*version:\s*([0-9][^\s]*)`)

func gemInfoFromGem(gem []byte) (name, version string) {
	// gem 是未压缩 tar，内含 metadata.gz（gzip 的 YAML gemspec）
	tr := tar.NewReader(bytes.NewReader(gem))
	for {
		hdr, err := tr.Next()
		if err != nil {
			return "", ""
		}
		if hdr.Name == "metadata.gz" {
			gz, err := gzip.NewReader(tr)
			if err != nil {
				return "", ""
			}
			defer func() { _ = gz.Close() }()
			b, err := io.ReadAll(io.LimitReader(gz, 1<<20))
			if err != nil {
				return "", ""
			}
			m := gemNameRe.FindSubmatch(b)
			v := gemVersionRe.FindSubmatch(b)
			if m != nil {
				name = string(m[1])
			}
			if v != nil {
				version = string(v[1])
			}
			return name, version
		}
	}
}

// gemPush gem push
//
//	@Summary     rubygems 发布
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Success     201
//	@Router      /packages/rubygems/{owner}/api/v1/gems [post]
func (a *API) gemPush(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	body, ok := limitBody(w, r)
	if !ok {
		return
	}
	name, version := gemInfoFromGem(body)
	if name == "" || version == "" {
		writeCode(w, http.StatusBadRequest, "invalid_gem", "cannot parse gem metadata")
		return
	}
	a.savePackage(w, r, owner, "rubygems", name, version, name+"-"+version+".gem", body)
}

func (a *API) gemDownload(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	filename := r.PathValue("filename")
	names, _ := a.store.ListAllPackageNames(owner, "rubygems")
	for _, n := range names {
		if !a.canReadPackage(owner, "rubygems", n, pkgUser(r)) {
			continue
		}
		ps, _ := a.store.ListPackageVersions(owner, "rubygems", n)
		for _, p := range ps {
			if p.Filename == filename {
				_, content, err := a.store.GetPackageFile(owner, "rubygems", p.Name, p.Version, filename)
				if err != nil {
					writeCode(w, http.StatusNotFound, "not_found", "not found")
					return
				}
				servePackageBytes(w, p, content, "application/octet-stream")
				return
			}
		}
	}
	writeCode(w, http.StatusNotFound, "not_found", "not found")
}
