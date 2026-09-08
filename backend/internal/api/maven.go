package api

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gitdash/backend/internal/store"
)

// ---- maven ----

// mavenQualOrder maven 版本修饰符排序（越小越旧；release 空串比一切修饰符新）。
// 参考 Maven ComparableVersion：alpha < beta < milestone < rc = cr < snapshot < release < sp。
var mavenQualOrder = map[string]int{
	"alpha": 1, "a": 1, "beta": 2, "b": 2, "milestone": 3, "m": 3,
	"rc": 4, "cr": 4, "snapshot": 5, "": 6, "final": 6, "ga": 6, "sp": 7,
}

// mavenVersionTokens 把版本串按 ./- 切成段（小写；连续分隔符产生空段=release）。
func mavenVersionTokens(v string) []string {
	return strings.FieldsFunc(strings.ToLower(v), func(r rune) bool { return r == '.' || r == '-' })
}

// mavenTokenCmp 比较单个版本段：数字段 > 修饰符段；数字按数值；修饰符按 maven 次序。
func mavenTokenCmp(x, y string) int {
	xn, xe := strconv.Atoi(x)
	yn, ye := strconv.Atoi(y)
	switch {
	case xe == nil && ye == nil:
		return xn - yn
	case xe == nil: // 数字（release 级别）大于修饰符
		return 1
	case ye == nil:
		return -1
	}
	xo, yo := mavenQualOrder[x], mavenQualOrder[y]
	if xo != yo {
		return xo - yo
	}
	return strings.Compare(x, y)
}

// mavenVersionCmp 语义化比较 maven 版本（major.minor.patch + 修饰符）。
func mavenVersionCmp(a, b string) int {
	as, bs := mavenVersionTokens(a), mavenVersionTokens(b)
	for i := 0; i < len(as) || i < len(bs); i++ {
		x, y := "", ""
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		if c := mavenTokenCmp(x, y); c != 0 {
			return c
		}
	}
	return 0
}

func mavenSortVersions(versions []string) {
	sort.Slice(versions, func(i, j int) bool { return mavenVersionCmp(versions[i], versions[j]) < 0 })
}

// mavenUpload maven/gradle 制品上传（路径式 {group}/{artifact}/{version}/{filename}）
//
//	@Summary     maven 上传
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Param       rest  path string true "{group}/{artifact}/{version}/{filename}"
//	@Success     201
//	@Router      /packages/maven/{owner}/{rest} [put]
func (a *API) mavenUpload(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	rest := strings.TrimPrefix(r.URL.Path, "/api/packages/maven/"+owner+"/")
	segments := strings.Split(strings.Trim(rest, "/"), "/")
	if len(segments) < 2 {
		writeCode(w, http.StatusBadRequest, "invalid_path", "expected {group...}/{artifact}/{version}/{filename}")
		return
	}
	body, ok := limitBody(w, r)
	if !ok {
		return
	}
	filename := segments[len(segments)-1]
	if len(segments) == 2 {
		// 元数据文件（maven-metadata.xml 等）挂在 artifact 层
		a.savePackage(w, r, owner, "maven", segments[0], "_", filename, body)
		return
	}
	version := segments[len(segments)-2]
	name := strings.Join(segments[:len(segments)-2], "/")
	a.savePackage(w, r, owner, "maven", name, version, filename, body)
}

// mavenGet maven/gradle 制品下载（同上传路径；自动生成 maven-metadata.xml 与 .sha1/.md5）
//
//	@Summary     maven 下载
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Param       rest  path string true "{group}/{artifact}/{version}/{filename}"
//	@Router      /packages/maven/{owner}/{rest} [get]
func (a *API) mavenGet(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	rest := strings.TrimPrefix(r.URL.Path, "/api/packages/maven/"+owner+"/")
	segments := strings.Split(strings.Trim(rest, "/"), "/")
	if len(segments) < 2 {
		writeCode(w, http.StatusBadRequest, "invalid_path", "path too short")
		return
	}
	filename := segments[len(segments)-1]

	// .sha1 / .md5 校验文件：对底层文件生成
	if strings.HasSuffix(filename, ".sha1") || strings.HasSuffix(filename, ".md5") {
		base := strings.TrimSuffix(filename, path.Ext(filename))
		content, ok := a.mavenLookup(owner, append(append([]string{}, segments[:len(segments)-1]...), base))
		if !ok {
			writeCode(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		var out string
		if strings.HasSuffix(filename, ".sha1") {
			s := sha1.Sum(content)
			out = hex.EncodeToString(s[:])
		} else {
			s := md5.Sum(content)
			out = hex.EncodeToString(s[:])
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(out))
		return
	}

	content, ok := a.mavenLookup(owner, segments)
	if !ok && len(segments) >= 3 {
		// SNAPSHOT 目录兜底：请求非时间戳文件名时解析为最新时间戳制品
		if p, c, resolved := a.mavenSnapshotResolve(owner, segments); resolved {
			content, ok = c, true
			w.Header().Set("X-Checksum-Sha256", p.Checksum)
		}
	}
	if !ok {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	ct := "application/octet-stream"
	switch {
	case strings.HasSuffix(filename, ".pom"), strings.HasSuffix(filename, ".xml"):
		ct = "text/xml"
	case strings.HasSuffix(filename, ".jar"):
		ct = "application/java-archive"
	}
	w.Header().Set("Content-Type", ct)
	_, _ = w.Write(content)
}

// mavenLookup 按路径段查找 maven 制品；未命中时自动生成 maven-metadata.xml。
func (a *API) mavenLookup(owner string, segments []string) ([]byte, bool) {
	filename := segments[len(segments)-1]
	var content []byte
	var err error
	if len(segments) >= 3 {
		name := strings.Join(segments[:len(segments)-2], "/")
		_, content, err = a.store.GetPackageFile(owner, "maven", name, segments[len(segments)-2], filename)
	} else {
		err = store.ErrNotFound
	}
	if err != nil {
		name := strings.Join(segments[:len(segments)-1], "/")
		_, content, err = a.store.GetPackageFile(owner, "maven", name, "_", filename)
		if err != nil {
			if filename == "maven-metadata.xml" {
				if len(segments) >= 3 {
					// 版本级 metadata：SNAPSHOT 目录未显式上传时自动生成
					if xml, ok := a.mavenSnapshotMetadata(owner, segments); ok {
						return []byte(xml), true
					}
				}
				if len(segments) >= 2 {
					if xml, ok := a.mavenAutoMetadata(owner, segments); ok {
						return []byte(xml), true
					}
				}
			}
			return nil, false
		}
	}
	return content, true
}

// mavenAutoMetadata 自动生成 artifact 级 maven-metadata.xml（版本列表来自已上传制品）。
func (a *API) mavenAutoMetadata(owner string, segments []string) (string, bool) {
	// 路径形如 {group...}/{artifact}/maven-metadata.xml → 版本挂在 {group}/{artifact} 下
	name := strings.Join(segments[:len(segments)-1], "/")
	pkgs, err := a.store.ListPackageVersions(owner, "maven", name)
	if err != nil {
		return "", false
	}
	seen := map[string]bool{}
	var versions []string
	for _, p := range pkgs {
		if p.Version != "_" && !seen[p.Version] {
			seen[p.Version] = true
			versions = append(versions, p.Version)
		}
	}
	if len(versions) == 0 {
		return "", false
	}
	mavenSortVersions(versions)
	latest := versions[len(versions)-1]
	// release 取最新非 SNAPSHOT 版本（无则退回 latest）
	release := ""
	for _, v := range versions {
		if !strings.HasSuffix(v, "-SNAPSHOT") {
			if release == "" || mavenVersionCmp(v, release) > 0 {
				release = v
			}
		}
	}
	if release == "" {
		release = latest
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "<metadata>\n  <groupId>%s</groupId>\n  <artifactId>%s</artifactId>\n",
		escapeXML(mavenGroupOf(name)), escapeXML(segments[len(segments)-2]))
	sb.WriteString("  <versioning>\n")
	fmt.Fprintf(&sb, "    <latest>%s</latest>\n    <release>%s</release>\n", latest, release)
	sb.WriteString("    <versions>\n")
	for _, v := range versions {
		fmt.Fprintf(&sb, "      <version>%s</version>\n", v)
	}
	sb.WriteString("    </versions>\n  </versioning>\n</metadata>\n")
	return sb.String(), true
}

// mavenGroupOf maven 包名（group/path/artifact）转 groupId（点分）。
func mavenGroupOf(name string) string {
	return strings.ReplaceAll(path.Dir(name), "/", ".")
}

var mavenSnapshotFileRe = regexp.MustCompile(`^(\d{8}\.\d{6})-(\d+)\.([^.]+)$`)

// mavenSnapshotMetadata 为 {group}/{artifact}/{version-SNAPSHOT}/maven-metadata.xml 自动生成
// 版本级快照元数据（timestamp/buildNumber/snapshotVersions），依据已上传的时间戳文件名。
func (a *API) mavenSnapshotMetadata(owner string, segments []string) (string, bool) {
	name := strings.Join(segments[:len(segments)-2], "/")
	ver := segments[len(segments)-2]
	if !strings.HasSuffix(ver, "-SNAPSHOT") {
		return "", false
	}
	base := strings.TrimSuffix(ver, "-SNAPSHOT")
	artifact := path.Base(name)
	prefix := artifact + "-" + base + "-"
	pkgs, err := a.store.ListPackageVersions(owner, "maven", name)
	if err != nil {
		return "", false
	}
	type snapFile struct {
		ts, build, ext string
	}
	var files []snapFile
	latestUpdated := ""
	for _, p := range pkgs {
		if p.Version != ver || !strings.HasPrefix(p.Filename, prefix) {
			continue
		}
		if m := mavenSnapshotFileRe.FindStringSubmatch(strings.TrimPrefix(p.Filename, prefix)); m != nil {
			files = append(files, snapFile{ts: m[1], build: m[2], ext: m[3]})
			if u := strings.ReplaceAll(m[1]+m[2], ".", ""); u > latestUpdated {
				latestUpdated = u
			}
		}
	}
	if len(files) == 0 {
		return "", false
	}
	// 取最大 timestamp+buildNumber
	best := files[0]
	for _, f := range files[1:] {
		if f.ts > best.ts || (f.ts == best.ts && f.build > best.build) {
			best = f
		}
	}
	updated := strings.ReplaceAll(best.ts, ".", "")
	var sb strings.Builder
	fmt.Fprintf(&sb, "<metadata>\n  <groupId>%s</groupId>\n  <artifactId>%s</artifactId>\n  <version>%s</version>\n",
		escapeXML(mavenGroupOf(name)), escapeXML(artifact), escapeXML(ver))
	sb.WriteString("  <versioning>\n")
	fmt.Fprintf(&sb, "    <snapshot>\n      <timestamp>%s</timestamp>\n      <buildNumber>%s</buildNumber>\n    </snapshot>\n", best.ts, best.build)
	fmt.Fprintf(&sb, "    <lastUpdated>%s</lastUpdated>\n", updated)
	sb.WriteString("    <snapshotVersions>\n")
	for _, f := range files {
		v := base + "-" + f.ts + "-" + f.build
		fmt.Fprintf(&sb, "      <snapshotVersion>\n        <extension>%s</extension>\n        <value>%s</value>\n        <updated>%s</updated>\n      </snapshotVersion>\n",
			escapeXML(f.ext), escapeXML(v), strings.ReplaceAll(f.ts, ".", ""))
	}
	sb.WriteString("    </snapshotVersions>\n  </versioning>\n</metadata>\n")
	return sb.String(), true
}

// mavenSnapshotResolve SNAPSHOT 目录下按请求文件名模糊解析：Maven 客户端经版本级
// metadata 取时间戳文件名；这里额外兜底 `artifact-1.0-SNAPSHOT.<ext>` 与任意时间戳变体。
func (a *API) mavenSnapshotResolve(owner string, segments []string) (store.Package, []byte, bool) {
	name := strings.Join(segments[:len(segments)-2], "/")
	ver := segments[len(segments)-2]
	filename := segments[len(segments)-1]
	if !strings.HasSuffix(ver, "-SNAPSHOT") {
		return store.Package{}, nil, false
	}
	ext := path.Ext(filename)
	prefix := path.Base(name) + "-" + strings.TrimSuffix(ver, "-SNAPSHOT") + "-"
	pkgs, err := a.store.ListPackageVersions(owner, "maven", name)
	if err != nil {
		return store.Package{}, nil, false
	}
	var best *store.Package
	var bestKey string
	for i := range pkgs {
		p := pkgs[i]
		if p.Version != ver || !strings.HasPrefix(p.Filename, prefix) || path.Ext(p.Filename) != ext {
			continue
		}
		key := ""
		if m := mavenSnapshotFileRe.FindStringSubmatch(strings.TrimPrefix(p.Filename, prefix)); m != nil {
			key = m[1] + "-" + fmt.Sprintf("%06s", m[2])
		}
		if best == nil || key >= bestKey {
			best, bestKey = &pkgs[i], key
		}
	}
	if best == nil {
		return store.Package{}, nil, false
	}
	p, content, err := a.store.GetPackageFile(owner, "maven", name, ver, best.Filename)
	if err != nil {
		return store.Package{}, nil, false
	}
	return p, content, true
}

// escapeXML 最小 XML 转义。
func escapeXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
