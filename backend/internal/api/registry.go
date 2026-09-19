package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/store"
)

// Docker / OCI 私有注册表（Distribution spec）。
//
// 路由统一挂在 /v2/ 下，按路径分派；认证复用 Basic（用户名 + PAT，见 resolveUser），
// 未认证时返回 WWW-Authenticate: Basic 以便 docker login 重试。
// blob 走内容寻址磁盘存储，manifest 存库；命名空间即用户/组织，仅其成员可推拉。

const (
	registryManifestMax = 4 << 20 // manifest 上限 4MB
	registryUploadTTL   = time.Hour
)

var registryNameRe = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*$`)

type registryUploadSession struct {
	mu      sync.Mutex
	ns      string
	image   string
	path    string
	size    int64
	created time.Time
}

var registryUploads sync.Map // uuid -> *registryUploadSession

func writeRegistryError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{
		"errors": []map[string]string{{"code": code, "message": msg}},
	})
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// registryAuth 注册表认证：加入 Docker-Distribution 头，未认证返回 Basic challenge。
func (a *API) registryAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
		username, scopes, isPAT := a.resolveUser(r)
		if username == "" {
			w.Header().Set("WWW-Authenticate", `Basic realm="gitdash"`)
			writeRegistryError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
			return
		}
		if isPAT && !patAllowed("/api/repos", scopes) {
			writeRegistryError(w, http.StatusForbidden, "DENIED", "token does not have repo scope")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUser{}, username)
		next(w, r.WithContext(ctx))
	}
}

// parseRegistryPath 解析 /v2/ 下的操作。kind ∈ version|catalog|tags|manifest|blob|upload-start|upload。
func parseRegistryPath(p string) (kind, name, ref string, ok bool) {
	if p == "/v2/" || p == "/v2" {
		return "version", "", "", true
	}
	rest := strings.TrimPrefix(p, "/v2/")
	switch {
	case rest == "_catalog":
		return "catalog", "", "", true
	case strings.HasSuffix(rest, "/tags/list"):
		return "tags", strings.TrimSuffix(rest, "/tags/list"), "", true
	case strings.Contains(rest, "/manifests/"):
		i := strings.Index(rest, "/manifests/")
		if i <= 0 {
			return "", "", "", false
		}
		return "manifest", rest[:i], rest[i+len("/manifests/"):], true
	case strings.Contains(rest, "/blobs/uploads/"):
		i := strings.Index(rest, "/blobs/uploads/")
		if i <= 0 {
			return "", "", "", false
		}
		uid := strings.Trim(rest[i+len("/blobs/uploads/"):], "/")
		if uid == "" {
			return "upload-start", rest[:i], "", true
		}
		return "upload", rest[:i], uid, true
	case strings.Contains(rest, "/blobs/"):
		i := strings.Index(rest, "/blobs/")
		if i <= 0 {
			return "", "", "", false
		}
		return "blob", rest[:i], rest[i+len("/blobs/"):], true
	}
	return "", "", "", false
}

func splitRegistryName(name string) (ns, image string) {
	i := strings.IndexByte(name, '/')
	if i <= 0 || i == len(name)-1 {
		return "", ""
	}
	return name[:i], name[i+1:]
}

// registryAllowed 命名空间成员（本人或组织成员）才可推拉。
func (a *API) registryAllowed(ns, user string) bool {
	if ns == user {
		return true
	}
	return a.store.IsOrg(ns) && a.store.OrgRole(ns, user) != ""
}

func (a *API) registryHandler(w http.ResponseWriter, r *http.Request) {
	kind, name, ref, ok := parseRegistryPath(r.URL.Path)
	if !ok {
		writeRegistryError(w, http.StatusNotFound, "NAME_UNKNOWN", "unknown registry path")
		return
	}
	user := userFrom(r)

	switch kind {
	case "version":
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	case "catalog":
		a.registryCatalog(w, user)
	case "tags":
		a.registryTags(w, name, user)
	case "manifest":
		a.registryManifest(w, r, name, ref, user)
	case "blob":
		a.registryBlob(w, r, name, ref, user)
	case "upload-start":
		a.registryUploadStart(w, r, name, user)
	case "upload":
		a.registryUploadChunk(w, r, name, ref, user)
	}
}

// registryCatalog 只返回调用者可访问的命名空间（本人或所在组织），避免任意
// 登录用户枚举全实例的私有镜像名（安全评审 §3.2）。
func (a *API) registryCatalog(w http.ResponseWriter, user string) {
	repos, err := a.store.ListRegistryCatalog()
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]string, 0, len(repos))
	for _, r := range repos {
		ns, _, ok := strings.Cut(r, "/")
		if !ok {
			continue
		}
		if a.registryAllowed(ns, user) {
			out = append(out, r)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"repositories": out})
}

func (a *API) registryTags(w http.ResponseWriter, name, user string) {
	ns, image := splitRegistryName(name)
	if ns == "" || !a.registryAllowed(ns, user) {
		writeRegistryError(w, http.StatusNotFound, "NAME_UNKNOWN", "repository not found")
		return
	}
	tags, err := a.store.ListRegistryTags(ns, image)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "tags": tags})
}

func (a *API) checkRegistryRepo(w http.ResponseWriter, name, user string) (ns, image string, ok bool) {
	ns, image = splitRegistryName(name)
	if ns == "" || !registryNameRe.MatchString(name) {
		writeRegistryError(w, http.StatusNotFound, "NAME_UNKNOWN", "invalid repository name")
		return "", "", false
	}
	if !a.registryAllowed(ns, user) {
		writeRegistryError(w, http.StatusForbidden, "DENIED", "no access to this namespace")
		return "", "", false
	}
	return ns, image, true
}

func (a *API) registryManifest(w http.ResponseWriter, r *http.Request, name, ref, user string) {
	ns, image, ok := a.checkRegistryRepo(w, name, user)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		m, content, err := a.store.GetRegistryManifest(ns, image, ref)
		if err != nil {
			writeRegistryError(w, http.StatusNotFound, "MANIFEST_UNKNOWN", "manifest unknown")
			return
		}
		w.Header().Set("Content-Type", m.MediaType)
		w.Header().Set("Docker-Content-Digest", m.Digest)
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	case http.MethodPut:
		body, err := io.ReadAll(io.LimitReader(r.Body, registryManifestMax+1))
		if err != nil {
			writeRegistryError(w, http.StatusBadRequest, "MANIFEST_INVALID", "read body: "+err.Error())
			return
		}
		if len(body) > registryManifestMax {
			writeRegistryError(w, http.StatusRequestEntityTooLarge, "MANIFEST_INVALID", "manifest too large")
			return
		}
		if len(body) == 0 {
			writeRegistryError(w, http.StatusBadRequest, "MANIFEST_INVALID", "empty manifest")
			return
		}
		mediaType := r.Header.Get("Content-Type")
		if mediaType == "" {
			mediaType = "application/vnd.oci.image.manifest.v1+json"
		}
		sum := sha256.Sum256(body)
		digest := store.RegistryDigestPrefix + hex.EncodeToString(sum[:])
		if _, isDigest := strings.CutPrefix(ref, store.RegistryDigestPrefix); isDigest && ref != digest {
			writeRegistryError(w, http.StatusBadRequest, "DIGEST_INVALID", "manifest digest mismatch")
			return
		}
		if err := a.store.PutRegistryManifest(ns, image, ref, mediaType, digest, body); err != nil {
			internalError(w, err)
			return
		}
		if ref != digest {
			if err := a.store.PutRegistryManifest(ns, image, digest, mediaType, digest, body); err != nil {
				internalError(w, err)
				return
			}
		}
		logx.Infof("registry audit: PUSH manifest %s/%s ref=%s digest=%s by=%s", ns, image, ref, digest, user)
		w.Header().Set("Docker-Content-Digest", digest)
		w.Header().Set("Location", "/v2/"+name+"/manifests/"+digest)
		w.WriteHeader(http.StatusCreated)
	case http.MethodDelete:
		if err := a.store.DeleteRegistryManifest(ns, image, ref); err != nil {
			writeRegistryError(w, http.StatusNotFound, "MANIFEST_UNKNOWN", "manifest unknown")
			return
		}
		logx.Infof("registry audit: DELETE manifest %s/%s ref=%s by=%s", ns, image, ref, user)
		w.WriteHeader(http.StatusAccepted)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *API) registryBlob(w http.ResponseWriter, r *http.Request, name, digest, user string) {
	if _, _, ok := a.checkRegistryRepo(w, name, user); !ok {
		return
	}
	path, exists := a.store.RegistryBlobPath(digest)
	if !exists {
		writeRegistryError(w, http.StatusNotFound, "BLOB_UNKNOWN", "blob unknown")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeRegistryError(w, http.StatusNotFound, "BLOB_UNKNOWN", "blob unknown")
		return
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		internalError(w, err)
		return
	}
	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	http.ServeContent(w, r, "", st.ModTime(), f)
}

func pruneRegistryUploads() {
	now := time.Now()
	registryUploads.Range(func(k, v any) bool {
		s := v.(*registryUploadSession)
		if now.Sub(s.created) > registryUploadTTL {
			_ = os.Remove(s.path)
			registryUploads.Delete(k)
		}
		return true
	})
}

func (a *API) registryUploadStart(w http.ResponseWriter, r *http.Request, name, user string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ns, image, ok := a.checkRegistryRepo(w, name, user)
	if !ok {
		return
	}
	f, err := os.CreateTemp("", "gitdash-registry-upload-*")
	if err != nil {
		internalError(w, err)
		return
	}
	_ = f.Close()
	uid := randomHex(16)
	registryUploads.Store(uid, &registryUploadSession{ns: ns, image: image, path: f.Name(), created: time.Now()})
	pruneRegistryUploads()
	w.Header().Set("Location", "/v2/"+name+"/blobs/uploads/"+uid)
	w.Header().Set("Docker-Upload-UUID", uid)
	w.Header().Set("Range", "0-0")
	w.WriteHeader(http.StatusAccepted)
}

func (a *API) registryUploadChunk(w http.ResponseWriter, r *http.Request, name, uid, user string) {
	ns, image, ok := a.checkRegistryRepo(w, name, user)
	if !ok {
		return
	}
	v, found := registryUploads.Load(uid)
	if !found {
		writeRegistryError(w, http.StatusNotFound, "BLOB_UPLOAD_UNKNOWN", "upload not found")
		return
	}
	sess := v.(*registryUploadSession)
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.ns != ns || sess.image != image {
		writeRegistryError(w, http.StatusNotFound, "BLOB_UPLOAD_UNKNOWN", "upload not found")
		return
	}

	switch r.Method {
	case http.MethodPatch, http.MethodPut:
		f, err := os.OpenFile(sess.path, os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			internalError(w, err)
			return
		}
		n, copyErr := io.Copy(f, r.Body)
		closeErr := f.Close()
		if copyErr != nil {
			writeRegistryError(w, http.StatusBadRequest, "BLOB_UPLOAD_INVALID", copyErr.Error())
			return
		}
		if closeErr != nil {
			internalError(w, closeErr)
			return
		}
		sess.size += n

		if r.Method == http.MethodPatch {
			w.Header().Set("Location", "/v2/"+name+"/blobs/uploads/"+uid)
			w.Header().Set("Docker-Upload-UUID", uid)
			w.Header().Set("Range", fmt.Sprintf("0-%d", sess.size-1))
			w.WriteHeader(http.StatusAccepted)
			return
		}
		// PUT：完成上传并校验 digest
		digest := r.URL.Query().Get("digest")
		if digest == "" {
			writeRegistryError(w, http.StatusBadRequest, "DIGEST_INVALID", "digest query parameter required")
			return
		}
		if err := a.store.SaveRegistryBlobFromFile(sess.path, digest); err != nil {
			registryUploads.Delete(uid)
			writeRegistryError(w, http.StatusBadRequest, "DIGEST_INVALID", err.Error())
			return
		}
		registryUploads.Delete(uid)
		logx.Infof("registry audit: PUSH blob %s/%s digest=%s size=%d by=%s", ns, image, digest, sess.size, user)
		w.Header().Set("Docker-Content-Digest", digest)
		w.Header().Set("Location", "/v2/"+name+"/blobs/"+digest)
		w.WriteHeader(http.StatusCreated)
	case http.MethodDelete:
		_ = os.Remove(sess.path)
		registryUploads.Delete(uid)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
