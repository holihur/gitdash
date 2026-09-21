package tests

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func registryReq(t *testing.T, method, url, user, pass string, body io.Reader, contentType string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

func closeBody(t *testing.T, resp *http.Response) {
	t.Helper()
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func TestDockerRegistryPushPull(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	m := alice.mustStatus("POST", "/tokens", map[string]any{"name": "docker"}, 201)
	tok, _ := m["token"].(string)
	if tok == "" {
		t.Fatal("empty PAT")
	}
	base := env.BaseURL

	// 未认证：返回 401 + Basic challenge（docker login 依赖）
	resp := registryReq(t, "GET", base+"/v2/", "", "", nil, "")
	if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("WWW-Authenticate") == "" {
		t.Fatalf("/v2/ unauth = %d challenge=%q", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
	}
	closeBody(t, resp)

	// 认证后 /v2/ → 200
	resp = registryReq(t, "GET", base+"/v2/", "alice", tok, nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/v2/ auth = %d", resp.StatusCode)
	}
	closeBody(t, resp)

	// 上传 blob：POST 开 session → PATCH 追加 → PUT 完成
	blob := []byte("hello docker layer")
	sum := sha256.Sum256(blob)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	resp = registryReq(t, "POST", base+"/v2/alice/demo/blobs/uploads/", "alice", tok, nil, "")
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("upload start = %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	closeBody(t, resp)
	if loc == "" {
		t.Fatal("upload start: empty Location")
	}

	resp = registryReq(t, "PATCH", base+loc, "alice", tok, bytes.NewReader(blob), "application/octet-stream")
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("upload patch = %d", resp.StatusCode)
	}
	closeBody(t, resp)

	resp = registryReq(t, "PUT", base+loc+"?digest="+digest, "alice", tok, nil, "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload put = %d", resp.StatusCode)
	}
	closeBody(t, resp)

	// HEAD blob
	resp = registryReq(t, "HEAD", base+"/v2/alice/demo/blobs/"+digest, "alice", tok, nil, "")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Length") != "18" {
		t.Fatalf("blob head = %d len=%q", resp.StatusCode, resp.Header.Get("Content-Length"))
	}
	closeBody(t, resp)

	// GET blob
	resp = registryReq(t, "GET", base+"/v2/alice/demo/blobs/"+digest, "alice", tok, nil, "")
	got, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Equal(got, blob) {
		t.Fatalf("blob get = %d body=%q", resp.StatusCode, got)
	}

	// PUT / GET manifest（按 tag）
	manifest := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.image.config.v1+json","size":2,"digest":"` + digest + `"},"layers":[{"mediaType":"application/vnd.oci.image.layer.v1.tar","size":18,"digest":"` + digest + `"}]}`)
	mt := "application/vnd.oci.image.manifest.v1+json"
	resp = registryReq(t, "PUT", base+"/v2/alice/demo/manifests/latest", "alice", tok, bytes.NewReader(manifest), mt)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("manifest put = %d", resp.StatusCode)
	}
	mdig := resp.Header.Get("Docker-Content-Digest")
	closeBody(t, resp)
	if mdig == "" {
		t.Fatal("manifest put: empty Docker-Content-Digest")
	}

	resp = registryReq(t, "GET", base+"/v2/alice/demo/manifests/latest", "alice", tok, nil, "")
	gotM, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Equal(gotM, manifest) ||
		resp.Header.Get("Content-Type") != mt || resp.Header.Get("Docker-Content-Digest") != mdig {
		t.Fatalf("manifest get = %d ct=%q digest=%q", resp.StatusCode, resp.Header.Get("Content-Type"), resp.Header.Get("Docker-Content-Digest"))
	}

	// 按 digest 也能取到
	resp = registryReq(t, "GET", base+"/v2/alice/demo/manifests/"+mdig, "alice", tok, nil, "")
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("manifest get by digest = %d", resp.StatusCode)
	}

	// tags list
	resp = registryReq(t, "GET", base+"/v2/alice/demo/tags/list", "alice", tok, nil, "")
	var tags struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&tags)
	_ = resp.Body.Close()
	if len(tags.Tags) != 1 || tags.Tags[0] != "latest" {
		t.Fatalf("tags = %+v", tags)
	}

	// 非成员无权访问 alice 命名空间
	bob := register(t, env, "bobby", "bobby-pass-1234")
	bm := bob.mustStatus("POST", "/tokens", map[string]any{"name": "docker"}, 201)
	btok, _ := bm["token"].(string)
	resp = registryReq(t, "GET", base+"/v2/alice/demo/manifests/latest", "bobby", btok, nil, "")
	closeBody(t, resp)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-namespace access = %d, want 403", resp.StatusCode)
	}
}

// TestDockerRegistryBlobCrossNamespaceDenied 覆盖安全评审 §3.2：知道 digest 的
// 攻击者不能通过自己的命名空间 URL 读取其它命名空间的私有 blob。
func TestDockerRegistryBlobCrossNamespaceDenied(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bobby", "bob-pass-123")
	_ = bob

	atok := alice.mustStatus("POST", "/tokens", map[string]any{"name": "alice-docker"}, 201)["token"].(string)
	btok := bob.mustStatus("POST", "/tokens", map[string]any{"name": "bob-docker"}, 201)["token"].(string)

	blob := []byte("alice-private-layer")
	sum := sha256.Sum256(blob)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	// alice 上传 blob
	start := registryReq(t, "POST", env.BaseURL+"/v2/alice/demo/blobs/uploads/", "alice", atok, nil, "")
	loc := start.Header.Get("Location")
	closeBody(t, start)
	patch := registryReq(t, "PATCH", env.BaseURL+loc, "alice", atok, bytes.NewReader(blob), "application/octet-stream")
	if patch.StatusCode != http.StatusAccepted {
		t.Fatalf("alice blob patch = %d", patch.StatusCode)
	}
	closeBody(t, patch)
	put := registryReq(t, "PUT", env.BaseURL+loc+"?digest="+digest, "alice", atok, nil, "")
	if put.StatusCode != http.StatusCreated {
		t.Fatalf("alice blob put = %d", put.StatusCode)
	}
	closeBody(t, put)

	// 本人可读
	got := registryReq(t, "GET", env.BaseURL+"/v2/alice/demo/blobs/"+digest, "alice", atok, nil, "")
	if got.StatusCode != http.StatusOK {
		t.Fatalf("alice blob get = %d", got.StatusCode)
	}
	closeBody(t, got)

	// 攻击者用**自己的**命名空间路径 + alice 的 digest → 必须 404
	for _, method := range []string{"GET", "HEAD"} {
		r := registryReq(t, method, env.BaseURL+"/v2/bobby/anything/blobs/"+digest, "bobby", btok, nil, "")
		if r.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-namespace %s blob = %d, want 404", method, r.StatusCode)
		}
		closeBody(t, r)
	}
}

func TestRegistryBlobSizeLimit(t *testing.T) {
	t.Setenv("GITDASH_MAX_REGISTRY_BLOB_BYTES", "16")
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	m := alice.mustStatus("POST", "/tokens", map[string]any{"name": "docker"}, 201)
	tok, _ := m["token"].(string)
	base := env.BaseURL

	resp := registryReq(t, "POST", base+"/v2/alice/demo/blobs/uploads/", "alice", tok, nil, "")
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("upload start = %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	closeBody(t, resp)

	// 超过 16 字节的 blob 应被拒绝（413）而不是无限写入磁盘。
	resp = registryReq(t, "PATCH", base+loc, "alice", tok, bytes.NewReader(bytes.Repeat([]byte("x"), 64)), "application/octet-stream")
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized patch = %d, want 413", resp.StatusCode)
	}
	closeBody(t, resp)
}
