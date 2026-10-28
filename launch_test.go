package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseCommand(t *testing.T) {
	cases := []struct {
		args []string
		cmd  string
		rest []string
	}{
		{nil, "open", nil},
		{[]string{"-demo"}, "open", []string{"-demo"}},
		{[]string{"serve", "-addr", "127.0.0.1:9000"}, "serve", []string{"-addr", "127.0.0.1:9000"}},
		{[]string{"stop"}, "stop", []string{}},
		{[]string{"install-desktop"}, "install-desktop", []string{}},
	}
	for _, c := range cases {
		cmd, rest, err := parseCommand(c.args)
		if err != nil || cmd != c.cmd || len(rest) != len(c.rest) || (len(rest) > 0 && !reflect.DeepEqual(rest, c.rest)) {
			t.Errorf("parseCommand(%q) = %q %q %v, want %q %q", c.args, cmd, rest, err, c.cmd, c.rest)
		}
	}
	if _, _, err := parseCommand([]string{"bogus"}); err == nil {
		t.Error("an unknown subcommand must be an error")
	}
}

func TestFindOfficeDir(t *testing.T) {
	home := t.TempDir()
	if got := findOfficeDir("", home); got != "" {
		t.Fatalf("no clone anywhere: got %q, want empty", got)
	}
	clone := filepath.Join(home, "Projects", "agent-office-3d")
	must(t, os.MkdirAll(filepath.Join(clone, "scripts"), 0o755))
	must(t, os.WriteFile(filepath.Join(clone, "scripts", "start.sh"), []byte("#!/bin/sh\n"), 0o644))
	if got := findOfficeDir("", home); got != clone {
		t.Errorf("default place: got %q, want %q", got, clone)
	}

	mine := filepath.Join(home, "elsewhere", "office")
	must(t, os.MkdirAll(filepath.Join(mine, "scripts"), 0o755))
	must(t, os.WriteFile(filepath.Join(mine, "scripts", "start.sh"), []byte("#!/bin/sh\n"), 0o644))
	if got := findOfficeDir("~/elsewhere/office", home); got != mine {
		t.Errorf("configured folder wins (with ~): got %q, want %q", got, mine)
	}
	if got := findOfficeDir(filepath.Join(home, "missing"), home); got != "" {
		t.Errorf("a configured folder without scripts/start.sh is ignored, got %q", got)
	}
}

func TestBrowserCommand(t *testing.T) {
	const u = "http://127.0.0.1:7878/"
	only := func(name string) func(string) (string, error) {
		return func(f string) (string, error) {
			if f == name {
				return "/usr/bin/" + f, nil
			}
			return "", errors.New("not found")
		}
	}
	if got := browserCommand(u, "linux", only("google-chrome-stable")); !reflect.DeepEqual(got, []string{"/usr/bin/google-chrome-stable", "--app=" + u}) {
		t.Errorf("chrome installed: got %q", got)
	}
	if got := browserCommand(u, "linux", only("chromium")); !reflect.DeepEqual(got, []string{"/usr/bin/chromium", "--app=" + u}) {
		t.Errorf("chromium installed: got %q", got)
	}
	if got := browserCommand(u, "linux", only("none")); !reflect.DeepEqual(got, []string{"xdg-open", u}) {
		t.Errorf("no chromium-family browser on linux: got %q", got)
	}
	if got := browserCommand(u, "darwin", only("none")); !reflect.DeepEqual(got, []string{"open", u}) {
		t.Errorf("macOS fallback: got %q", got)
	}
}

func TestDesktopEntry(t *testing.T) {
	e := desktopEntry("/home/a b/.local/bin/pano")
	for _, want := range []string{"[Desktop Entry]\n", "Type=Application\n", "Name=pano\n", "Icon=pano\n", "Exec=\"/home/a b/.local/bin/pano\"\n"} {
		if !strings.Contains(e, want) {
			t.Errorf("desktop entry lacks %q:\n%s", want, e)
		}
	}
	if !strings.Contains(desktopEntry("/usr/bin/pano"), "Exec=/usr/bin/pano\n") {
		t.Error("a path without spaces is not quoted")
	}
}

func TestHealthNeedsNoTokenButAPIStillDoes(t *testing.T) {
	app, err := newApp(serveOpts{addr: "127.0.0.1:7878", claudeDir: t.TempDir(), cfgPath: filepath.Join(t.TempDir(), "none.json"), office: "off"})
	must(t, err)
	h := app.handler()

	req := httptest.NewRequest("GET", "http://127.0.0.1:7878/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body struct {
		OK      bool   `json:"ok"`
		App     string `json:"app"`
		Version string `json:"version"`
		PID     int    `json:"pid"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil || !body.OK || body.App != "pano" || body.PID != os.Getpid() {
		t.Fatalf("/health = %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "http://127.0.0.1:7878/api/meta", nil))
	if rec.Code != http.StatusForbidden {
		t.Errorf("/api/meta without token = %d, want 403", rec.Code)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "http://evil.example/health", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("/health with a foreign Host = %d, want 403", rec.Code)
	}
}

func TestPanoHealthClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"app":"pano","version":"0.1.0","pid":4242}`))
	}))
	defer srv.Close()
	if pid, ok := panoHealth(strings.TrimPrefix(srv.URL, "http://")); !ok || pid != 4242 {
		t.Errorf("panoHealth = %d %v, want 4242 true", pid, ok)
	}

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"app":"something-else"}`))
	}))
	defer other.Close()
	if _, ok := panoHealth(strings.TrimPrefix(other.URL, "http://")); ok {
		t.Error("another app on the port is not pano")
	}
	if _, ok := panoHealth("127.0.0.1:1"); ok {
		t.Error("nothing listening is not up")
	}
}
