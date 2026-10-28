// pano: a local dashboard for Claude Code. Reads ~/.claude (transcripts,
// skills, CLAUDE.md files), shows usage, cache state, skills and memory, runs
// skills in the background with `claude -p`, and embeds agent-office-3d.
//
//	pano                  # start in the background if needed, open the window
//	pano stop             # stop pano (and agent-office-3d)
//	pano serve            # run the server in the foreground (systemd)
//	pano install-desktop  # add pano to the app menu
//	pano -demo            # synthetic data, for a preview
//	pano -addr 127.0.0.1:9000 -claude-dir ~/.claude
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

//go:embed web/index.html web/app.js web/app.css web/sky.js web/pano.svg web/fonts
var webFS embed.FS

// assetTypes are the only files /assets/ serves.
var assetTypes = map[string]string{
	"app.js":   "text/javascript; charset=utf-8",
	"sky.js":   "text/javascript; charset=utf-8",
	"app.css":  "text/css; charset=utf-8",
	"pano.svg": "image/svg+xml",
}

const version = "0.1.0"

type App struct {
	cfg       Config
	claudeDir string
	store     *Store
	runner    *Runner
	demo      bool
	token     string
	hosts     map[string]bool
	origin    string
	officeURL string
	page      []byte
}

type serveOpts struct {
	addr, claudeDir, cfgPath, office string
	demo                             bool
}

func main() {
	cmd, rest, err := parseCommand(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "pano:", err)
		os.Exit(2)
	}
	o := parseFlags(cmd, rest)
	switch cmd {
	case "serve":
		serve(o)
	case "stop":
		os.Exit(stopPano(o))
	case "install-desktop":
		os.Exit(installDesktop())
	default:
		os.Exit(openPano(o, rest))
	}
}

func parseFlags(cmd string, args []string) serveOpts {
	home, _ := os.UserHomeDir()
	var o serveOpts
	fs := flag.NewFlagSet("pano "+cmd, flag.ExitOnError)
	fs.StringVar(&o.addr, "addr", "127.0.0.1:7878", "listen address (loopback only)")
	fs.StringVar(&o.claudeDir, "claude-dir", envOr("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude")), "Claude Code config folder")
	fs.StringVar(&o.cfgPath, "config", filepath.Join(configDir(), "config.json"), "pano config file")
	fs.StringVar(&o.office, "office", "http://127.0.0.1:7847", "agent-office-3d URL to embed (\"off\" to hide)")
	fs.BoolVar(&o.demo, "demo", false, "serve synthetic data instead of ~/.claude")
	_ = fs.Parse(args)
	return o
}

func serve(o serveOpts) {
	app, err := newApp(o)
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Addr: o.addr, Handler: app.handler(), ReadHeaderTimeout: 5 * time.Second}
	mode := "~/.claude: " + o.claudeDir
	if o.demo {
		mode = "DEMO data"
	}
	log.Printf("pano %s on http://%s (%s)", version, o.addr, mode)
	log.Fatal(srv.ListenAndServe())
}

func newApp(o serveOpts) (*App, error) {
	host, port, err := net.SplitHostPort(o.addr)
	if err != nil {
		return nil, fmt.Errorf("bad -addr: %v", err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, fmt.Errorf("pano only listens on loopback (127.0.0.1 / ::1 / localhost); got %q", host)
	}

	app := &App{
		cfg:       loadConfig(o.cfgPath),
		claudeDir: o.claudeDir,
		store:     NewStore(o.claudeDir),
		demo:      o.demo,
		token:     randomHex(16),
		hosts:     map[string]bool{"127.0.0.1:" + port: true, "localhost:" + port: true, "[::1]:" + port: true},
		origin:    "http://" + net.JoinHostPort(host, port),
	}
	if o.office != "off" && o.office != "" {
		u, err := url.Parse(o.office)
		if err != nil || u.Scheme != "http" || !isLoopbackHost(u.Hostname()) {
			return nil, fmt.Errorf("-office must be an http URL on loopback, got %q", o.office)
		}
		app.officeURL = strings.TrimRight(u.String(), "/")
	}
	app.runner = NewRunner(app.cfg, app.demo)
	page, _ := webFS.ReadFile("web/index.html")
	app.page = bytes.Replace(page, []byte("__PANO_TOKEN__"), []byte(app.token), 1)
	return app, nil
}

func (a *App) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.index)
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("GET /fonts/{name}", a.font)
	mux.HandleFunc("GET /assets/{name}", a.asset)
	mux.HandleFunc("GET /api/overview", a.api(a.overview))
	mux.HandleFunc("GET /api/sessions", a.api(a.sessions))
	mux.HandleFunc("GET /api/skills", a.api(a.skills))
	mux.HandleFunc("GET /api/memory", a.api(a.memory))
	mux.HandleFunc("GET /api/runs", a.api(a.runs))
	mux.HandleFunc("POST /api/run", a.api(a.startRun))
	mux.HandleFunc("GET /api/office", a.api(a.office))
	mux.HandleFunc("POST /api/office/start", a.api(a.officeStart))
	mux.HandleFunc("GET /api/meta", a.api(a.meta))
	return a.guard(mux)
}

// guard blocks DNS rebinding (Host must be ours) and sets a strict CSP.
func (a *App) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.hosts[r.Host] {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		frame := "'none'"
		if a.officeURL != "" {
			frame = a.officeURL
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-src "+frame+"; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// api requires the per-start token (only our own page has it) and, when the
// browser sends one, our own Origin. A foreign page can do neither.
func (a *App) api(h func(*http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Pano-Token") != a.token {
			http.Error(w, "missing token", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && !a.hosts[strings.TrimPrefix(o, "http://")] {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		v, err := h(r)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err != nil {
			code := http.StatusBadRequest
			if errors.Is(err, errBusy) {
				code = http.StatusTooManyRequests
			}
			if errors.Is(err, errOfficeMissing) {
				code = http.StatusNotFound
			}
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(v)
	}
}

func (a *App) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(a.page)
}

// health tells the launcher that pano runs here and under which PID. It needs
// no token and carries no data.
func (a *App) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "app": "pano", "version": version, "pid": os.Getpid()})
}

func (a *App) font(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !strings.HasSuffix(name, ".woff2") || strings.ContainsAny(name, "/\\") {
		http.NotFound(w, r)
		return
	}
	b, err := webFS.ReadFile("web/fonts/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "font/woff2")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(b)
}

func (a *App) asset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ct, ok := assetTypes[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	b, err := webFS.ReadFile("web/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ct)
	_, _ = w.Write(b)
}

func (a *App) files() map[string]*fileData {
	if a.demo {
		return demoFiles(time.Now())
	}
	return a.store.Refresh()
}

func (a *App) projects(files map[string]*fileData) []string {
	if a.demo {
		return demoProjectsList()
	}
	return knownProjects(files)
}

func (a *App) skillList(files map[string]*fileData) []Skill {
	usage := skillUsage(files)
	if a.demo {
		return demoSkills(usage)
	}
	return scanSkills(a.claudeDir, a.projects(files), usage)
}

func (a *App) overview(r *http.Request) (any, error) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 1 || days > 365 {
		days = 30
	}
	ov := buildOverview(a.files(), a.cfg, days, time.Now())
	ov.Demo = a.demo
	return ov, nil
}

func (a *App) sessions(r *http.Request) (any, error) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 12
	}
	return buildSessions(a.files(), a.cfg, limit, time.Now()), nil
}

func (a *App) skills(r *http.Request) (any, error) {
	return a.skillList(a.files()), nil
}

func (a *App) memory(r *http.Request) (any, error) {
	if a.demo {
		return demoMemory(), nil
	}
	return scanMemory(a.claudeDir, a.projects(a.files())), nil
}

func (a *App) runs(r *http.Request) (any, error) { return a.runner.List(), nil }

func (a *App) startRun(r *http.Request) (any, error) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return nil, errors.New("JSON only")
	}
	var req RunRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		return nil, errors.New("bad JSON")
	}
	files := a.files()
	return a.runner.Start(req, a.skillList(files), a.projects(files))
}

type officeState struct {
	URL     string `json:"url"`
	Up      bool   `json:"up"`
	Version string `json:"version,omitempty"`
}

// office asks agent-office-3d's /health from the server side; a browser page
// cannot read it cross-origin.
func (a *App) office(r *http.Request) (any, error) {
	st := officeState{URL: a.officeURL}
	if a.officeURL != "" {
		st.Up, st.Version = officeHealth(a.officeURL)
	}
	return st, nil
}

var (
	errOfficeMissing = errors.New("agent-office-3d folder not found")
	ansiRe           = regexp.MustCompile("\x1b\\[[0-9;]*m")
)

// officeStart runs agent-office-3d's own start script; pano starts nothing
// else. The script's output goes back to the page.
func (a *App) officeStart(r *http.Request) (any, error) {
	if a.officeURL == "" {
		return nil, errors.New("the office panel is turned off (-office off)")
	}
	if a.demo {
		return map[string]any{"ok": true, "output": "demo"}, nil
	}
	home, _ := os.UserHomeDir()
	dir := findOfficeDir(a.cfg.OfficeDir, home)
	if dir == "" {
		return nil, errOfficeMissing
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", filepath.Join(dir, "scripts", "start.sh"), "--no-browser")
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.CombinedOutput()
	text := clip(ansiRe.ReplaceAllString(strings.TrimSpace(string(out)), ""), 2000)
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return nil, errors.New(text)
	}
	return map[string]any{"ok": true, "output": text}, nil
}

func officeHealth(base string) (up bool, version string) {
	c := http.Client{Timeout: 700 * time.Millisecond}
	resp, err := c.Get(strings.TrimRight(base, "/") + "/health")
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()
	var h struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&h) == nil && h.OK {
		return true, h.Version
	}
	return false, ""
}

func (a *App) meta(r *http.Request) (any, error) {
	files := a.files()
	proj := a.projects(files)
	return map[string]any{
		"version":  version,
		"demo":     a.demo,
		"models":   a.cfg.Models,
		"projects": proj,
		"ttl":      a.cfg.CacheTTLMinutes,
		"prices":   a.cfg.Prices,
		"user":     userName(),
	}, nil
}

func userName() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "you"
}

func isLoopbackHost(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("no randomness: %v", err))
	}
	return hex.EncodeToString(b)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
