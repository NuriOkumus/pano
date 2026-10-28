package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestOfficeStart(t *testing.T) {
	office := t.TempDir()
	writeFile(t, filepath.Join(office, "scripts", "start.sh"), "echo \"started $1\"\n")
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	must(t, os.WriteFile(cfgPath, []byte(`{"office_dir":`+strconv.Quote(office)+`}`), 0o644))
	app, err := newApp(serveOpts{addr: "127.0.0.1:7878", claudeDir: t.TempDir(), cfgPath: cfgPath, office: "http://127.0.0.1:7847"})
	must(t, err)
	h := app.handler()
	start := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "http://127.0.0.1:7878/api/office/start", nil)
		if token != "" {
			req.Header.Set("X-Pano-Token", token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := start(""); rec.Code != http.StatusForbidden {
		t.Fatalf("without the token: %d", rec.Code)
	}
	if rec := start(app.token); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "started --no-browser") {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	app.cfg.OfficeDir = filepath.Join(office, "missing")
	if rec := start(app.token); rec.Code != http.StatusNotFound {
		t.Errorf("no office folder: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRunBodyCap(t *testing.T) {
	app, err := newApp(serveOpts{addr: "127.0.0.1:7878", claudeDir: t.TempDir(), cfgPath: filepath.Join(t.TempDir(), "config.json"), demo: true})
	must(t, err)
	body := `{"model":"haiku","prompt":"` + strings.Repeat("a", 100) + `","pad":"` + strings.Repeat("b", 20000) + `"}`
	req := httptest.NewRequest("POST", "http://127.0.0.1:7878/api/run", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pano-Token", app.token)
	rec := httptest.NewRecorder()
	app.handler().ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "bad JSON") {
		t.Fatalf("a 20 kB body was cut off: %d %s", rec.Code, rec.Body.String())
	}
}
