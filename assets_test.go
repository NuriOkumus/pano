package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssets(t *testing.T) {
	app, err := newApp(serveOpts{addr: "127.0.0.1:7878", claudeDir: t.TempDir(), cfgPath: filepath.Join(t.TempDir(), "none.json"), office: "off"})
	must(t, err)
	h := app.handler()
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "http://127.0.0.1:7878"+path, nil))
		return rec
	}
	for path, ct := range map[string]string{"/assets/app.js": "text/javascript", "/assets/sky.js": "text/javascript", "/assets/app.css": "text/css", "/assets/pano.svg": "image/svg+xml"} {
		if rec := get(path); rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), ct) || rec.Body.Len() == 0 {
			t.Errorf("%s: %d %q", path, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
	for _, path := range []string{"/assets/index.html", "/assets/main.go", "/assets/nope.js"} {
		if rec := get(path); rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", path, rec.Code)
		}
	}
	page := get("/").Body.String()
	if !strings.Contains(page, `src="/assets/app.js"`) || strings.Contains(page, "__PANO_TOKEN__") || !strings.Contains(page, app.token) {
		t.Error("the page loads app.js and carries this start's token")
	}
}
