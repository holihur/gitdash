package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"gitdash/backend/internal/store"
)

func newLangAPI(t *testing.T) (*API, *store.Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	return New(s, "test"), s
}

func decodeLanguages(t *testing.T, rr *httptest.ResponseRecorder) ([]string, map[string]string, map[string]string) {
	t.Helper()
	var out struct {
		Languages []string          `json:"languages"`
		Defaults  map[string]string `json:"defaults"`
		Colors    map[string]string `json:"colors"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, rr.Body.String())
	}
	return out.Languages, out.Defaults, out.Colors
}

func TestLanguagesEndpointListsKnownLanguages(t *testing.T) {
	a, _ := newLangAPI(t)
	rr := httptest.NewRecorder()
	a.languages(rr, httptest.NewRequest("GET", "/api/languages", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	langs, defaults, colors := decodeLanguages(t, rr)
	if len(langs) < 20 {
		t.Fatalf("languages look truncated: %v", langs)
	}
	found := false
	for _, l := range langs {
		if l == "Go" {
			found = true
		}
	}
	if !found {
		t.Errorf("Go missing from %v", langs)
	}
	// 默认配色覆盖全部已知语言，Go 为固定色
	if len(defaults) != len(langs) {
		t.Errorf("defaults count = %d, want %d", len(defaults), len(langs))
	}
	if defaults["Go"] != "#00ADD8" {
		t.Errorf("Go default = %q, want #00ADD8", defaults["Go"])
	}
	if len(colors) != 0 {
		t.Errorf("expected no overrides, got %v", colors)
	}
}

func TestAdminSaveLanguageColors(t *testing.T) {
	a, s := newLangAPI(t)

	// 合法保存：键/值去空格、颜色小写归一
	body, _ := json.Marshal(map[string]any{
		"colors": map[string]string{"Go": "#123ABC", "C++": "#F34B7D"},
	})
	rr := httptest.NewRecorder()
	a.adminSaveLanguageColors(rr, httptest.NewRequest("POST", "/api/admin/language-colors", bytes.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("save status = %d: %s", rr.Code, rr.Body.String())
	}

	// store 中已落库，公开接口可见
	rr = httptest.NewRecorder()
	a.languages(rr, httptest.NewRequest("GET", "/api/languages", nil))
	_, _, colors := decodeLanguages(t, rr)
	if colors["Go"] != "#123abc" || colors["C++"] != "#f34b7d" {
		t.Fatalf("colors = %v", colors)
	}

	// 非法颜色 → 400，且不覆盖已有配置
	body, _ = json.Marshal(map[string]any{"colors": map[string]string{"Go": "red"}})
	rr = httptest.NewRecorder()
	a.adminSaveLanguageColors(rr, httptest.NewRequest("POST", "/api/admin/language-colors", bytes.NewReader(body)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid color status = %d, want 400", rr.Code)
	}
	if got := s.GetSetting(languageColorsKey); got == "" {
		t.Fatal("invalid save should not clear existing setting")
	}

	// 空 map 清空覆盖
	body, _ = json.Marshal(map[string]any{"colors": map[string]string{}})
	rr = httptest.NewRecorder()
	a.adminSaveLanguageColors(rr, httptest.NewRequest("POST", "/api/admin/language-colors", bytes.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("clear status = %d", rr.Code)
	}
	if a.languageColorOverrides()["Go"] != "" {
		t.Fatalf("overrides not cleared: %v", a.languageColorOverrides())
	}
}
