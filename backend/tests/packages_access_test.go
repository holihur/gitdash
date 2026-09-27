package tests

import (
	"net/http"
	"testing"
)

// TestPackageAuditIDORAndDockerEnumeration 覆盖安全审计 M1.1/M1.2：
// 包审计日志不得跨租户读取；私有 Docker 镜像不得被非命名空间成员枚举。
func TestPackageAuditIDORAndDockerEnumeration(t *testing.T) {
	hs, st := startAPISeed(t, nil)
	env := &Env{t: t, BaseURL: hs.URL}
	victim := register(t, env, "pvictim", "password-1")
	other := register(t, env, "pother", "password-1")

	// M1.1：审计日志仅命名空间发布者可读；其他登录用户 403，匿名 401。
	other.mustFail("GET", "/packages/pvictim/audit", nil, http.StatusForbidden)
	victim.mustStatus("GET", "/packages/pvictim/audit", nil, http.StatusOK)
	anon, err := http.Get(hs.URL + "/api/packages/pvictim/audit")
	if err != nil {
		t.Fatal(err)
	}
	_ = anon.Body.Close()
	if anon.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous audit = %d, want 401", anon.StatusCode)
	}

	// M1.2：私有 registry 镜像仅命名空间成员可枚举。
	if err := st.PutRegistryManifest("pvictim", "secret-img", "latest",
		"application/vnd.oci.image.manifest.v1+json", "sha256:abc", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	other.mustFail("GET", "/packages/pvictim/docker", nil, http.StatusForbidden)
	victim.mustStatus("GET", "/packages/pvictim/docker", nil, http.StatusOK)
	anonDocker, err := http.Get(hs.URL + "/api/packages/pvictim/docker")
	if err != nil {
		t.Fatal(err)
	}
	_ = anonDocker.Body.Close()
	if anonDocker.StatusCode != http.StatusForbidden {
		t.Fatalf("anonymous docker list = %d, want 403", anonDocker.StatusCode)
	}
}
