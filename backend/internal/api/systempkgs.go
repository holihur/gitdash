package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"gitdash/backend/internal/store"
)

// 系统包仓库（apt / yum / apk / brew / snap）。
//
// 与语言生态不同，系统仓库需要生成索引文件供原生客户端（apt / dnf / apk / brew /
// snapd）拉取。这里采用「发布时由客户端提交元数据」的方式：publish 接收
// multipart 的 meta(JSON) + file，服务端据此生成标准索引，无需解析二进制包格式，
// 也不引入额外依赖。未签名的索引适用于 `[trusted=yes]` / `gpgcheck=0` /
// `--allow-untrusted` 场景（自托管常见做法）。

var systemPkgTypeList = []string{"apt", "yum", "apk", "brew", "snap"}

var systemPkgTypes = map[string]bool{
	"apt": true, "yum": true, "apk": true, "brew": true, "snap": true,
}

// systemPkgMeta 发布时客户端提交的包元数据（各类型共用，字段按需填写）。
type systemPkgMeta struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Arch        string `json:"arch"`
	Description string `json:"description"`
	Summary     string `json:"summary"`
	Depends     string `json:"depends"`
	Provides    string `json:"provides"`
	Section     string `json:"section"`
	Priority    string `json:"priority"`
	Maintainer  string `json:"maintainer"`
	Homepage    string `json:"homepage"`
	License     string `json:"license"`
	Release     string `json:"release"`
	Epoch       int    `json:"epoch"`
	Installed   int64  `json:"installed_size"`
	// 发布时服务端回填，供索引生成使用。
	MD5    string `json:"md5,omitempty"`
	SHA1   string `json:"sha1,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

func decodeSystemMeta(raw string) systemPkgMeta {
	var m systemPkgMeta
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &m)
	}
	return m
}

// publishSystemPackage 发布系统包制品（multipart: meta(JSON) + file）。
func (a *API) publishSystemPackage(w http.ResponseWriter, r *http.Request, typ string) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	if !systemPkgTypes[typ] {
		writeCode(w, http.StatusBadRequest, "invalid_type", "unknown system package type")
		return
	}
	user := pkgUser(r)
	if !a.canPublishPackage(owner, user) {
		pkgForbidden(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPackageSize+(4<<20))
	if err := r.ParseMultipartForm(maxPackageSize + (4 << 20)); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_multipart", "invalid multipart form")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeCode(w, http.StatusBadRequest, "file_required", "multipart field 'file' is required")
		return
	}
	defer func() { _ = file.Close() }()

	meta := decodeSystemMeta(r.FormValue("meta"))
	filename := path.Base(hdr.Filename)
	if filename == "" || filename == "." || filename == "/" {
		writeCode(w, http.StatusBadRequest, "filename_required", "file name is required")
		return
	}
	if meta.Name == "" {
		meta.Name = strings.TrimSuffix(strings.TrimSuffix(filename, ".deb"), ".rpm")
	}
	if meta.Version == "" {
		meta.Version = "0"
	}

	tmp, err := os.CreateTemp("", "gitdash-syspkg-*")
	if err != nil {
		internalError(w, err)
		return
	}
	md5h, sha1h := md5.New(), sha1.New()
	n, err := io.Copy(io.MultiWriter(tmp, md5h, sha1h), io.LimitReader(file, maxPackageSize+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		internalError(w, err)
		return
	}
	if n > maxPackageSize {
		_ = os.Remove(tmp.Name())
		writeCode(w, http.StatusBadRequest, "package_too_large", "package too large (max 64MB)")
		return
	}
	meta.MD5 = hex.EncodeToString(md5h.Sum(nil))
	meta.SHA1 = hex.EncodeToString(sha1h.Sum(nil))
	metaRaw, _ := json.Marshal(meta)

	p := &store.Package{
		Owner: owner, Repo: strings.TrimPrefix(r.Header.Get("X-Gitdash-Repo"), "/"),
		Type: typ, Name: repo, Version: meta.Version, Filename: filename,
		Uploader: user, Meta: string(metaRaw),
	}
	if err := a.store.CreatePackageFromFile(p, tmp.Name()); err != nil {
		if errors.Is(err, store.ErrExists) {
			writeCode(w, http.StatusConflict, "package_exists", "package version already exists")
			return
		}
		internalError(w, err)
		return
	}
	_ = a.store.AddPackageAudit(owner, typ, repo, meta.Version, "publish", user)
	writeJSON(w, http.StatusCreated, p)
}

// systemRepoPackages 读取仓库制品并做读权限校验（ok=false 时已写出响应）。
func (a *API) systemRepoPackages(w http.ResponseWriter, r *http.Request, typ string) (owner, repo string, ok bool) {
	owner = r.PathValue("owner")
	repo = r.PathValue("repo")
	if !a.canReadPackage(owner, typ, repo, pkgUser(r)) {
		writeNotFound(w, "repository")
		return "", "", false
	}
	return owner, repo, true
}

// findRepoFile 在仓库制品中按文件名查找。
func findRepoFile(pkgs []store.Package, filename string) (store.Package, bool) {
	for _, p := range pkgs {
		if p.Filename == filename {
			return p, true
		}
	}
	return store.Package{}, false
}

func serveSysPkgDownload(w http.ResponseWriter, a *API, owner, repo, filename string, pkgs []store.Package) {
	p, found := findRepoFile(pkgs, filename)
	if !found {
		writeNotFound(w, "file")
		return
	}
	_, content, err := a.store.GetPackageFile(owner, p.Type, repo, p.Version, filename)
	if err != nil {
		writeNotFound(w, "file")
		return
	}
	servePackageBytes(w, p, content, "application/octet-stream")
}

// ---- apt (Debian) ----

func aptArchs(pkgs []store.Package) []string {
	set := map[string]bool{}
	for _, p := range pkgs {
		m := decodeSystemMeta(p.Meta)
		if m.Arch != "" && m.Arch != "all" {
			set[m.Arch] = true
		}
	}
	if len(set) == 0 {
		set["all"] = true
	}
	out := make([]string, 0, len(set))
	for a := range set {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

func aptPackagesText(owner, repo string, pkgs []store.Package, arch string) string {
	var b strings.Builder
	for _, p := range pkgs {
		m := decodeSystemMeta(p.Meta)
		if m.Arch != "" && m.Arch != "all" && m.Arch != arch {
			continue
		}
		rel := "pool/" + url.PathEscape(p.Filename)
		if m.Name == "" {
			m.Name = repo
		}
		fmt.Fprintf(&b, "Package: %s\n", m.Name)
		fmt.Fprintf(&b, "Version: %s\n", p.Version)
		fmt.Fprintf(&b, "Architecture: %s\n", valueOr(arch, "all"))
		if m.Maintainer != "" {
			fmt.Fprintf(&b, "Maintainer: %s\n", m.Maintainer)
		}
		if m.Section != "" {
			fmt.Fprintf(&b, "Section: %s\n", m.Section)
		}
		if m.Priority != "" {
			fmt.Fprintf(&b, "Priority: %s\n", m.Priority)
		}
		if m.Depends != "" {
			fmt.Fprintf(&b, "Depends: %s\n", m.Depends)
		}
		if m.Provides != "" {
			fmt.Fprintf(&b, "Provides: %s\n", m.Provides)
		}
		if m.Homepage != "" {
			fmt.Fprintf(&b, "Homepage: %s\n", m.Homepage)
		}
		fmt.Fprintf(&b, "Filename: %s\n", rel)
		fmt.Fprintf(&b, "Size: %d\n", p.Size)
		if m.MD5 != "" {
			fmt.Fprintf(&b, "MD5sum: %s\n", m.MD5)
		}
		if m.SHA1 != "" {
			fmt.Fprintf(&b, "SHA1: %s\n", m.SHA1)
		}
		fmt.Fprintf(&b, "SHA256: %s\n", p.Checksum)
		desc := m.Description
		if desc == "" {
			desc = m.Summary
		}
		if desc != "" {
			fmt.Fprintf(&b, "Description: %s\n", strings.ReplaceAll(desc, "\n", "\n "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (a *API) aptPackages(w http.ResponseWriter, r *http.Request) {
	owner, repo, ok := a.systemRepoPackages(w, r, "apt")
	if !ok {
		return
	}
	pkgs, err := a.store.ListPackageRepoFiles(owner, "apt", repo)
	if err != nil {
		internalError(w, err)
		return
	}
	arch := strings.TrimPrefix(r.PathValue("arch"), "binary-")
	body := aptPackagesText(owner, repo, pkgs, arch)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if strings.HasSuffix(r.URL.Path, ".gz") {
		w.Header().Set("Content-Type", "application/gzip")
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		_, _ = gz.Write([]byte(body))
		_ = gz.Close()
		_, _ = w.Write(buf.Bytes())
		return
	}
	_, _ = w.Write([]byte(body))
}

func (a *API) aptRelease(w http.ResponseWriter, r *http.Request) {
	owner, repo, ok := a.systemRepoPackages(w, r, "apt")
	if !ok {
		return
	}
	pkgs, err := a.store.ListPackageRepoFiles(owner, "apt", repo)
	if err != nil {
		internalError(w, err)
		return
	}
	dist := r.PathValue("dist")
	component := r.PathValue("component")
	archs := aptArchs(pkgs)
	var b strings.Builder
	fmt.Fprintf(&b, "Origin: %s\n", owner)
	fmt.Fprintf(&b, "Label: %s\n", repo)
	fmt.Fprintf(&b, "Suite: %s\n", dist)
	fmt.Fprintf(&b, "Codename: %s\n", dist)
	fmt.Fprintf(&b, "Components: %s\n", component)
	fmt.Fprintf(&b, "Architectures: %s\n", strings.Join(archs, " "))
	fmt.Fprintf(&b, "Date: %s\n", time.Now().UTC().Format(time.RFC1123))
	fmt.Fprintf(&b, "Description: gitdash apt repository %s/%s\n", owner, repo)
	b.WriteString("MD5Sum:\n")
	for _, ar := range archs {
		data := []byte(aptPackagesText(owner, repo, pkgs, ar))
		p := fmt.Sprintf("%s/binary-%s/Packages", component, ar)
		fmt.Fprintf(&b, " %s %d %s\n", md5Hex(data), len(data), p)
	}
	b.WriteString("SHA256:\n")
	for _, ar := range archs {
		data := []byte(aptPackagesText(owner, repo, pkgs, ar))
		p := fmt.Sprintf("%s/binary-%s/Packages", component, ar)
		fmt.Fprintf(&b, " %s %d %s\n", sha256Hex(data), len(data), p)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

func (a *API) aptPool(w http.ResponseWriter, r *http.Request) {
	owner, repo, ok := a.systemRepoPackages(w, r, "apt")
	if !ok {
		return
	}
	files, err := a.store.ListPackageRepoFiles(owner, "apt", repo)
	if err != nil {
		internalError(w, err)
		return
	}
	serveSysPkgDownload(w, a, owner, repo, r.PathValue("filename"), files)
}

// ---- yum (RPM) ----

type yumPrimary struct {
	XMLName  xml.Name     `xml:"metadata"`
	Xmlns    string       `xml:"xmlns,attr"`
	Rpm      string       `xml:"xmlns:rpm,attr"`
	Packages int          `xml:"packages,attr"`
	Package  []yumPackage `xml:"package"`
}

type yumPackage struct {
	Type     string      `xml:"type,attr"`
	Name     string      `xml:"name"`
	Arch     string      `xml:"arch"`
	Version  yumVersion  `xml:"version"`
	Checksum yumChecksum `xml:"checksum"`
	Summary  string      `xml:"summary"`
	Desc     string      `xml:"description"`
	Size     yumSize     `xml:"size"`
	Location yumLocation `xml:"location"`
	Format   yumFormat   `xml:"format"`
}

type yumVersion struct {
	Epoch string `xml:"epoch,attr"`
	Ver   string `xml:"ver,attr"`
	Rel   string `xml:"rel,attr"`
}
type yumChecksum struct {
	Type  string `xml:"type,attr"`
	Pkgid string `xml:"pkgid,attr"`
	Value string `xml:",chardata"`
}
type yumSize struct {
	Package   int64 `xml:"package,attr"`
	Installed int64 `xml:"installed,attr"`
	Archive   int64 `xml:"archive,attr"`
}
type yumLocation struct {
	Href string `xml:"href,attr"`
}
type yumFormat struct {
	License  string     `xml:"rpm:license"`
	Provides yumEntries `xml:"rpm:provides"`
	Requires yumEntries `xml:"rpm:requires"`
}
type yumEntries struct {
	Entry []yumEntry `xml:"rpm:entry"`
}
type yumEntry struct {
	Name string `xml:"name,attr"`
}

func yumPrimaryXML(pkgs []store.Package) []byte {
	doc := yumPrimary{Xmlns: "http://linux.duke.edu/metadata/common", Rpm: "http://linux.duke.edu/metadata/rpm"}
	for _, p := range pkgs {
		m := decodeSystemMeta(p.Meta)
		name := m.Name
		if name == "" {
			name = p.Filename
		}
		arch := valueOr(m.Arch, "noarch")
		desc := m.Description
		if desc == "" {
			desc = m.Summary
		}
		pkg := yumPackage{
			Type: "rpm", Name: name, Arch: arch,
			Version:  yumVersion{Epoch: fmt.Sprintf("%d", m.Epoch), Ver: p.Version, Rel: valueOr(m.Release, "1")},
			Checksum: yumChecksum{Type: "sha256", Pkgid: "YES", Value: p.Checksum},
			Summary:  m.Summary, Desc: desc,
			Size:     yumSize{Package: p.Size, Installed: m.Installed, Archive: p.Size},
			Location: yumLocation{Href: p.Filename},
			Format:   yumFormat{License: m.License},
		}
		if m.Provides != "" {
			for _, e := range strings.Fields(m.Provides) {
				pkg.Format.Provides.Entry = append(pkg.Format.Provides.Entry, yumEntry{Name: e})
			}
		}
		if m.Depends != "" {
			for _, e := range strings.Fields(m.Depends) {
				pkg.Format.Requires.Entry = append(pkg.Format.Requires.Entry, yumEntry{Name: e})
			}
		}
		doc.Package = append(doc.Package, pkg)
	}
	doc.Packages = len(doc.Package)
	body, _ := xml.MarshalIndent(doc, "", "  ")
	return append([]byte(xml.Header), body...)
}

func (a *API) yumRepodata(w http.ResponseWriter, r *http.Request) {
	owner, repo, ok := a.systemRepoPackages(w, r, "yum")
	if !ok {
		return
	}
	pkgs, err := a.store.ListPackageRepoFiles(owner, "yum", repo)
	if err != nil {
		internalError(w, err)
		return
	}
	file := r.PathValue("file")
	if file == "repomd.xml" {
		primary := gzipBytes(yumPrimaryXML(pkgs))
		var b strings.Builder
		b.WriteString(xml.Header)
		b.WriteString(`<repomd xmlns="http://linux.duke.edu/metadata/repo">`)
		fmt.Fprintf(&b, "<revision>%d</revision>", time.Now().Unix())
		b.WriteString(`<data type="primary">`)
		fmt.Fprintf(&b, `<checksum type="sha256">%s</checksum>`, sha256Hex(primary))
		b.WriteString(`<location href="repodata/primary.xml.gz"/>`)
		fmt.Fprintf(&b, "<timestamp>%d</timestamp>", time.Now().Unix())
		fmt.Fprintf(&b, "<size>%d</size>", len(primary))
		fmt.Fprintf(&b, `<open-checksum type="sha256">%s</open-checksum>`, sha256Hex(yumPrimaryXML(pkgs)))
		b.WriteString(`</data></repomd>`)
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		_, _ = w.Write([]byte(b.String()))
		return
	}
	if file == "primary.xml.gz" {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(gzipBytes(yumPrimaryXML(pkgs)))
		return
	}
	writeNotFound(w, "repodata")
}

func (a *API) yumDownload(w http.ResponseWriter, r *http.Request) {
	owner, repo, ok := a.systemRepoPackages(w, r, "yum")
	if !ok {
		return
	}
	pkgs, err := a.store.ListPackageRepoFiles(owner, "yum", repo)
	if err != nil {
		internalError(w, err)
		return
	}
	serveSysPkgDownload(w, a, owner, repo, r.PathValue("filename"), pkgs)
}

// ---- apk (Alpine) ----

func apkIndexText(pkgs []store.Package) string {
	var b strings.Builder
	for _, p := range pkgs {
		m := decodeSystemMeta(p.Meta)
		sha, _ := hex.DecodeString(m.SHA1)
		fmt.Fprintf(&b, "C:Q1%s\n", base64.StdEncoding.EncodeToString(sha))
		fmt.Fprintf(&b, "P:%s\n", valueOr(m.Name, p.Filename))
		fmt.Fprintf(&b, "V:%s\n", p.Version)
		fmt.Fprintf(&b, "A:%s\n", valueOr(m.Arch, "noarch"))
		fmt.Fprintf(&b, "S:%d\n", p.Size)
		fmt.Fprintf(&b, "I:%d\n", m.Installed)
		if m.Description != "" {
			fmt.Fprintf(&b, "T:%s\n", m.Description)
		}
		if m.Homepage != "" {
			fmt.Fprintf(&b, "U:%s\n", m.Homepage)
		}
		if m.License != "" {
			fmt.Fprintf(&b, "L:%s\n", m.License)
		}
		if m.Maintainer != "" {
			fmt.Fprintf(&b, "m:%s\n", m.Maintainer)
		}
		if m.Depends != "" {
			fmt.Fprintf(&b, "D:%s\n", m.Depends)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (a *API) apkIndex(w http.ResponseWriter, r *http.Request) {
	owner, repo, ok := a.systemRepoPackages(w, r, "apk")
	if !ok {
		return
	}
	all, err := a.store.ListPackageRepoFiles(owner, "apk", repo)
	if err != nil {
		internalError(w, err)
		return
	}
	arch := r.PathValue("arch")
	filtered := make([]store.Package, 0, len(all))
	for _, p := range all {
		m := decodeSystemMeta(p.Meta)
		if m.Arch == "" || m.Arch == arch {
			filtered = append(filtered, p)
		}
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	content := []byte(apkIndexText(filtered))
	_ = tw.WriteHeader(&tar.Header{Name: "APKINDEX", Mode: 0o644, Size: int64(len(content)), ModTime: time.Now()})
	_, _ = tw.Write(content)
	_ = tw.Close()
	_ = gz.Close()
	w.Header().Set("Content-Type", "application/gzip")
	_, _ = w.Write(buf.Bytes())
}

func (a *API) apkDownload(w http.ResponseWriter, r *http.Request) {
	owner, repo, ok := a.systemRepoPackages(w, r, "apk")
	if !ok {
		return
	}
	pkgs, err := a.store.ListPackageRepoFiles(owner, "apk", repo)
	if err != nil {
		internalError(w, err)
		return
	}
	serveSysPkgDownload(w, a, owner, repo, r.PathValue("filename"), pkgs)
}

// ---- brew (Homebrew tap) ----

func (a *API) brewFormula(w http.ResponseWriter, r *http.Request) {
	owner, tap, ok := a.systemRepoPackages(w, r, "brew")
	if !ok {
		return
	}
	pkgs, err := a.store.ListPackageRepoFiles(owner, "brew", tap)
	if err != nil {
		internalError(w, err)
		return
	}
	name := strings.TrimSuffix(r.PathValue("name"), ".json")
	var match *store.Package
	for i := range pkgs {
		m := decodeSystemMeta(pkgs[i].Meta)
		if m.Name == name || strings.HasPrefix(pkgs[i].Filename, name) {
			match = &pkgs[i]
			break
		}
	}
	if match == nil {
		writeNotFound(w, "formula")
		return
	}
	m := decodeSystemMeta(match.Meta)
	base := externalBaseURL(r)
	fileURL := base + "/api/packages/brew/" + owner + "/" + tap + "/bottles/" + url.PathEscape(match.Filename)
	formula := map[string]any{
		"name":      valueOr(m.Name, name),
		"full_name": owner + "/" + tap + "/" + valueOr(m.Name, name),
		"tap":       owner + "/" + tap,
		"versions":  map[string]string{"stable": match.Version},
		"urls": map[string]any{
			"stable": map[string]string{"url": fileURL, "sha256": match.Checksum},
		},
		"bottle": map[string]any{
			"stable": map[string]any{
				"files": map[string]any{
					valueOr(m.Arch, "all"): map[string]string{"url": fileURL, "sha256": match.Checksum},
				},
			},
		},
		"desc":     m.Description,
		"homepage": m.Homepage,
		"license":  m.License,
	}
	writeJSON(w, http.StatusOK, formula)
}

func (a *API) brewBottle(w http.ResponseWriter, r *http.Request) {
	owner, tap, ok := a.systemRepoPackages(w, r, "brew")
	if !ok {
		return
	}
	pkgs, err := a.store.ListPackageRepoFiles(owner, "brew", tap)
	if err != nil {
		internalError(w, err)
		return
	}
	serveSysPkgDownload(w, a, owner, tap, r.PathValue("filename"), pkgs)
}

// ---- snap ----

func (a *API) snapIndex(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.systemRepoPackages(w, r, "snap")
	if !ok {
		return
	}
	pkgs, err := a.store.ListPackageRepoFiles(owner, "snap", name)
	if err != nil {
		internalError(w, err)
		return
	}
	base := externalBaseURL(r)
	out := make([]map[string]any, 0, len(pkgs))
	for _, p := range pkgs {
		m := decodeSystemMeta(p.Meta)
		out = append(out, map[string]any{
			"name":     valueOr(m.Name, name),
			"version":  p.Version,
			"arch":     m.Arch,
			"revision": p.ID,
			"size":     p.Size,
			"sha256":   p.Checksum,
			"url":      base + "/api/packages/snap/" + owner + "/" + name + "/download/" + url.PathEscape(p.Filename),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"snaps": out})
}

func (a *API) snapDownload(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.systemRepoPackages(w, r, "snap")
	if !ok {
		return
	}
	pkgs, err := a.store.ListPackageRepoFiles(owner, "snap", name)
	if err != nil {
		internalError(w, err)
		return
	}
	serveSysPkgDownload(w, a, owner, name, r.PathValue("filename"), pkgs)
}

// ---- helpers ----

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func md5Hex(b []byte) string {
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func gzipBytes(b []byte) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write(b)
	_ = gz.Close()
	return buf.Bytes()
}

// externalBaseURL 由请求推导对外 base URL（反代场景优先 X-Forwarded-Proto/Host）。
func externalBaseURL(r *http.Request) string {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "http"
		if r.TLS != nil {
			proto = "https"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return proto + "://" + host
}
