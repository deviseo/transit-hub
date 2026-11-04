package httpserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestStaticHandlerCachesHashedAssetsAndRevalidatesTheIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app-abc123.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "favicon.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := staticHandler(dir)

	cases := []struct {
		path        string
		status      int
		cache       string
		bodyIsIndex bool
	}{
		{path: "/", status: http.StatusOK, cache: "no-cache", bodyIsIndex: true},
		{path: "/admin/dashboard", status: http.StatusOK, cache: "no-cache", bodyIsIndex: true},
		{path: "/assets/app-abc123.js", status: http.StatusOK, cache: "public, max-age=31536000, immutable"},
		// 旧版本的脚本已经不在了：404，不能回退成 index.html。
		{path: "/assets/app-old999.js", status: http.StatusNotFound},
		{path: "/favicon.svg", status: http.StatusOK},
	}
	for _, tc := range cases {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if recorder.Code != tc.status {
			t.Fatalf("%s: status %d, want %d", tc.path, recorder.Code, tc.status)
		}
		if got := recorder.Header().Get("Cache-Control"); got != tc.cache {
			t.Fatalf("%s: Cache-Control %q, want %q", tc.path, got, tc.cache)
		}
		if tc.bodyIsIndex && recorder.Body.String() != "<!doctype html>" {
			t.Fatalf("%s: expected index.html, got %q", tc.path, recorder.Body.String())
		}
	}
}
