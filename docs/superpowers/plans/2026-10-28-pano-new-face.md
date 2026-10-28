# pano new face Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace pano's electrical-panel page with one screen that never scrolls: a live sky that follows the clock, the office in the middle, glass panels for sessions, skills and memory, and a prompt bar that starts background runs.

**Architecture:** The Go server keeps its API and security model; the backend gains installed-plugin discovery, builtin skill rows, free-prompt runs with a permission mode, an office-start endpoint and a static asset route. The page splits into `web/index.html` (skeleton), `web/app.css`, `web/sky.js` (sky layer) and `web/app.js` (data, panels, drawer, prompt bar, tabs), all embedded in the binary and loaded as ES modules from `/assets/`.

**Tech Stack:** Go 1.24 (stdlib only), plain HTML/CSS/ES modules (no build step, no external libraries), Barlow font (OFL, embedded).

**Spec:** `docs/superpowers/specs/2026-10-28-pano-new-face-design.md`

## Global Constraints

- Work on branch `new-face`; one commit per task; every commit message ends with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- The page never scrolls: `html, body { overflow: hidden }`; lists hide rows that do not fit (`fitList`), drawers and the Kayıtlar list may scroll inside themselves.
- No external requests: no CDN, no web fonts from the network; CSP stays exactly as in `main.go` (`script-src 'self' 'unsafe-inline'`, `connect-src 'self'`, `frame-src <office URL>`).
- Listen on loopback only; every `/api/` call needs `X-Pano-Token` (and our Origin when sent); `/health` and `/assets/*` need no token and carry no user data.
- UI text in Turkish; code, comments, README and docs in English.
- Background runs: `claude -p <prompt> --model <m> --output-format json --permission-mode <permission_mode>` + `extra_args`; `permission_mode` default `auto`; accepted values `acceptEdits auto bypassPermissions manual dontAsk plan`.
- Sky keyframes: exactly the table in the spec section 1 (also in `web/sky.js`, Task 6).
- Glass panel tokens: `--panel: rgba(16,14,22,.6)`, blur 14px saturate 1.2, hairline `rgba(255,255,255,.12)`, text `#f3f1ed` / `#c9c6cf` / `#9a97a3`; semantic colours warm `#ffb347`, running `#56d999`, waiting `#f4c752`, error `#ff5a5f`.
- Test command: `go test ./...` (from the repo root). Formatting: `gofmt -l .` prints nothing.

---

### Task 1: Plugin skills from installed_plugins.json

**Files:**
- Modify: `skills.go` (imports; replace the `filepath.WalkDir(pluginRoot, …)` block in `scanSkills`; add `installedPlugins`)
- Modify: `scan_test.go:41-43` (fixture: installed plugin instead of a catalogue copy)
- Create: `skills_test.go`

**Interfaces:**
- Produces: `func installedPlugins(claudeDir string) []plugin` with `type plugin struct{ name, dir string }`. Plugin skill names stay `plugin:name`, `Scope: "plugin"`, `Owner: plugin name`.

- [ ] **Step 1: Write the failing test** — create `skills_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(path), 0o755))
	must(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestInstalledPluginSkills(t *testing.T) {
	dir := t.TempDir()
	inst := filepath.Join(dir, "plugins", "cache", "official", "superpowers", "6.3.0")
	writeFile(t, filepath.Join(inst, "skills", "brainstorming", "SKILL.md"), "---\nname: brainstorming\ndescription: Ideas into designs\n---\nbody")
	writeFile(t, filepath.Join(inst, "commands", "brainstorm.md"), "---\ndescription: Start brainstorming\n---\nbody")
	// The marketplace catalogue keeps plugins that are not installed.
	writeFile(t, filepath.Join(dir, "plugins", "marketplaces", "official", "plugins", "discord", "skills", "access", "SKILL.md"), "---\nname: access\n---\n")
	writeFile(t, filepath.Join(dir, "plugins", "installed_plugins.json"),
		`{"version":2,"plugins":{"superpowers@official":[{"scope":"user","installPath":`+strconv.Quote(inst)+`,"version":"6.3.0"}]}}`)

	usage := map[string]stamp{"superpowers:brainstorming": {Count: 10, Last: time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)}}
	got := map[string]Skill{}
	for _, s := range scanSkills(dir, nil, usage) {
		got[s.Name] = s
	}
	b, ok := got["superpowers:brainstorming"]
	if !ok || b.Scope != "plugin" || b.Owner != "superpowers" || b.Uses != 10 {
		t.Fatalf("superpowers:brainstorming = %+v, all: %+v", b, got)
	}
	if c, ok := got["superpowers:brainstorm"]; !ok || c.Kind != "command" {
		t.Errorf("plugin command missing: %+v", got)
	}
	for name := range got {
		if strings.HasPrefix(name, "6.3.0:") || strings.Contains(name, "access") {
			t.Errorf("unexpected skill %q", name)
		}
	}
}
```

- [ ] **Step 2: Run it to make sure it fails**

Run: `go test -run TestInstalledPluginSkills ./...`
Expected: FAIL — `superpowers:brainstorming` missing; the old walk names it `6.3.0:brainstorming` and also lists `official:access`-style catalogue skills.

- [ ] **Step 3: Update the shared fixture** — in `scan_test.go`, replace

```go
	plug := filepath.Join(dir, "plugins", "marketplaces", "m", "tools", "skills", "lint")
	must(t, os.MkdirAll(plug, 0o755))
	must(t, os.WriteFile(filepath.Join(plug, "SKILL.md"), []byte("---\nname: lint\n---\n"), 0o644))
```

with

```go
	plug := filepath.Join(dir, "plugins", "cache", "m", "tools", "1.0.0")
	must(t, os.MkdirAll(filepath.Join(plug, "skills", "lint"), 0o755))
	must(t, os.WriteFile(filepath.Join(plug, "skills", "lint", "SKILL.md"), []byte("---\nname: lint\n---\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "plugins", "installed_plugins.json"),
		[]byte(`{"version":2,"plugins":{"tools@m":[{"scope":"user","installPath":`+strconv.Quote(plug)+`}]}}`), 0o644))
```

and add `"strconv"` to the imports of `scan_test.go`.

- [ ] **Step 4: Implement** — in `skills.go` add `"encoding/json"` to the imports and replace the whole plugin block in `scanSkills` (from the comment `// Plugins: any skills/<name>/SKILL.md …` through the closing `})` of `filepath.WalkDir`) with:

```go
	// Plugins: only the installed ones, named by installed_plugins.json. The
	// marketplace catalogue next to them lists plugins that are not installed.
	for _, pl := range installedPlugins(claudeDir) {
		for _, s := range skillsIn(filepath.Join(pl.dir, "skills"), "plugin", pl.name) {
			s.Name = pl.name + ":" + s.Name
			add(s)
		}
		for _, s := range commandsIn(filepath.Join(pl.dir, "commands"), "plugin", pl.name) {
			s.Name = pl.name + ":" + s.Name
			add(s)
		}
	}
```

and add below `scanSkills`:

```go
type plugin struct{ name, dir string }

// installedPlugins reads plugins/installed_plugins.json (version 2): one key
// per "name@marketplace", each with the folders it is installed in.
func installedPlugins(claudeDir string) []plugin {
	b, err := os.ReadFile(filepath.Join(claudeDir, "plugins", "installed_plugins.json"))
	if err != nil {
		return nil
	}
	var f struct {
		Plugins map[string][]struct {
			InstallPath string `json:"installPath"`
		} `json:"plugins"`
	}
	if json.Unmarshal(b, &f) != nil {
		return nil
	}
	keys := make([]string, 0, len(f.Plugins))
	for k := range f.Plugins {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []plugin
	for _, k := range keys {
		name, _, _ := strings.Cut(k, "@")
		for _, e := range f.Plugins[k] {
			if e.InstallPath != "" {
				out = append(out, plugin{name: name, dir: e.InstallPath})
			}
		}
	}
	return out
}
```

`io/fs` stays imported (`dirText` uses it).

- [ ] **Step 5: Run the tests**

Run: `go vet ./... && go test ./...`
Expected: PASS (including `TestSkillsAndMemory`, which still finds `tools:lint`).

- [ ] **Step 6: Commit**

```bash
gofmt -l .
git add skills.go skills_test.go scan_test.go
git commit -m "Read plugin skills from installed_plugins.json

Names come from the install key (superpowers:brainstorming, not
6.3.0:brainstorming) and the marketplace catalogue is no longer listed.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Builtin skill rows

**Files:**
- Modify: `scan.go:68-71` (`stamp`), `scan.go:187` and `scan.go:214` (callers), `scan.go:239-253` (`fileData.skill`), `scan.go:663-676` (`skillUsage`)
- Modify: `skills.go` (`scanSkills`: remember names, append builtin rows)
- Modify: `skills_test.go`

**Interfaces:**
- Consumes: `scanSkills(claudeDir string, projects []string, usage map[string]stamp) []Skill` (Task 1).
- Produces: `stamp{Count int; Last time.Time; Tool bool}`; `func (fd *fileData) skill(name string, ts time.Time, tool bool)`; skills with `Scope: "builtin"` (no `Path`, `Tokens == 0`).

- [ ] **Step 1: Write the failing tests** — append to `skills_test.go` (and add `Tool: true` to the usage in `TestInstalledPluginSkills`):

```go
func TestBuiltinSkillsKeepTheirUses(t *testing.T) {
	usage := map[string]stamp{
		"anthropic-skills:pdf": {Count: 3, Last: time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC), Tool: true},
		// Typed /clear: a Claude Code command, never called through the Skill tool.
		"clear": {Count: 9, Last: time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)},
	}
	got := map[string]Skill{}
	for _, s := range scanSkills(t.TempDir(), nil, usage) {
		got[s.Name] = s
	}
	if p, ok := got["anthropic-skills:pdf"]; !ok || p.Scope != "builtin" || p.Uses != 3 || p.Tokens != 0 || p.Path != "" {
		t.Errorf("anthropic-skills:pdf = %+v", p)
	}
	if _, ok := got["clear"]; ok {
		t.Error("a typed /clear is not a skill")
	}
}

func TestSkillToolCallsAreMarked(t *testing.T) {
	usage := skillUsage(NewStore(writeClaudeDir(t)).Refresh())
	if !usage["k8s-health"].Tool || usage["calibrate"].Tool {
		t.Errorf("k8s-health came from the Skill tool, calibrate was typed: %+v", usage)
	}
}
```

- [ ] **Step 2: Run them to make sure they fail**

Run: `go test -run 'Builtin|ToolCalls' ./...`
Expected: build FAIL — `unknown field Tool in struct literal of type stamp`.

- [ ] **Step 3: Implement** — in `scan.go`:

```go
type stamp struct {
	Count int
	Last  time.Time
	Tool  bool // called through the Skill tool at least once (not only typed as /name)
}
```

Change the two callers: `fd.skill(mm[1], ts)` → `fd.skill(mm[1], ts, false)` and `fd.skill(name, ts)` → `fd.skill(name, ts, true)`. Replace `fileData.skill`:

```go
func (fd *fileData) skill(name string, ts time.Time, tool bool) {
	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
	if name == "" {
		return
	}
	s := fd.Skills[name]
	if s == nil {
		s = &stamp{}
		fd.Skills[name] = s
	}
	s.Count++
	s.Tool = s.Tool || tool
	if ts.After(s.Last) {
		s.Last = ts
	}
}
```

In `skillUsage`, after `cur.Count += v.Count` add `cur.Tool = cur.Tool || v.Tool`.

In `skills.go` `scanSkills`: add `names := map[string]bool{}` next to `seen`, set `names[s.Name] = true` right before `out = append(out, s)` inside `add`, and insert before the final `sort.Slice`:

```go
	// Skills Claude called that have no file here: built into Claude Code or
	// served by claude.ai. They keep their use count; their size is unknown.
	var extra []string
	for name, u := range usage {
		if u.Tool && !names[name] && skillNameRe.MatchString(name) {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for _, name := range extra {
		out = append(out, Skill{Name: name, Kind: "skill", Scope: "builtin", Uses: usage[name].Count, LastUsed: usage[name].Last})
	}
```

- [ ] **Step 4: Run the tests**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -l .
git add scan.go skills.go skills_test.go
git commit -m "List builtin skills that Claude called but have no file on disk

Only Skill tool calls count, so typed commands like /clear stay out.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Permission mode and the run command line

**Files:**
- Modify: `config.go` (field, default, loading, `permissionModes`)
- Modify: `run.go` (`exec`: use `runArgs`; add `runArgs`, `hasFlag`)
- Create: `run_test.go`
- Modify: `README.md` (Config section)

**Interfaces:**
- Produces: `Config.PermissionMode string` (`json:"permission_mode"`, default `"auto"`); `func runArgs(cfg Config, prompt, model string) []string`.

- [ ] **Step 1: Write the failing tests** — create `run_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRunArgs(t *testing.T) {
	got := runArgs(defaultConfig(), "/k8s-health check", "haiku")
	want := []string{"-p", "/k8s-health check", "--model", "haiku", "--output-format", "json", "--permission-mode", "auto"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("default: %q", got)
	}

	cfg := defaultConfig()
	cfg.ExtraArgs = []string{"--permission-mode", "acceptEdits", "--verbose"}
	got = runArgs(cfg, "hi", "opus")
	want = []string{"-p", "hi", "--model", "opus", "--output-format", "json", "--permission-mode", "acceptEdits", "--verbose"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("extra_args sets the mode: %q", got)
	}

	if got := runArgs(defaultConfig(), "-rf everything", "haiku"); got[1] != " -rf everything" {
		t.Errorf("a prompt that looks like a flag must not be read as one: %q", got[1])
	}
}

func TestPermissionModeConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	for body, want := range map[string]string{`{}`: "auto", `{"permission_mode":"plan"}`: "plan", `{"permission_mode":"yolo"}`: "auto"} {
		must(t, os.WriteFile(p, []byte(body), 0o644))
		if got := loadConfig(p).PermissionMode; got != want {
			t.Errorf("%s: got %q, want %q", body, got, want)
		}
	}
}
```

- [ ] **Step 2: Run them to make sure they fail**

Run: `go test -run 'RunArgs|PermissionMode' ./...`
Expected: build FAIL — `undefined: runArgs`, `PermissionMode undefined`.

- [ ] **Step 3: Implement** — `config.go`: add to `Config` after `MaxRunMins`:

```go
	// claude --permission-mode for background runs. auto lets Claude decide
	// per action, like an interactive session in auto mode.
	PermissionMode string `json:"permission_mode"`
```

`defaultConfig()`: add `PermissionMode: "auto",`. `loadConfig`: before `cfg.OfficeDir = user.OfficeDir` add

```go
	if permissionModes[user.PermissionMode] {
		cfg.PermissionMode = user.PermissionMode
	}
```

and below `loadConfig`:

```go
// The modes claude accepts (claude --help, v2.1.296).
var permissionModes = map[string]bool{"acceptEdits": true, "auto": true, "bypassPermissions": true, "manual": true, "dontAsk": true, "plan": true}
```

`run.go`: in `exec`, replace

```go
		args := append([]string{"-p", prompt, "--model", run.Model, "--output-format", "json"}, r.cfg.ExtraArgs...)
```

with `args := runArgs(r.cfg, prompt, run.Model)` and add at the end of the file:

```go
// runArgs builds the claude command line. The permission mode comes from the
// config unless extra_args already sets one. A prompt starting with "-" gets
// a leading space so claude does not read it as a flag.
func runArgs(cfg Config, prompt, model string) []string {
	if strings.HasPrefix(prompt, "-") {
		prompt = " " + prompt
	}
	args := []string{"-p", prompt, "--model", model, "--output-format", "json"}
	if cfg.PermissionMode != "" && !hasFlag(cfg.ExtraArgs, "--permission-mode") {
		args = append(args, "--permission-mode", cfg.PermissionMode)
	}
	return append(args, cfg.ExtraArgs...)
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag || strings.HasPrefix(a, flag+"=") {
			return true
		}
	}
	return false
}
```

`README.md`: replace the `extra_args` bullet with

```markdown
- `permission_mode`: `claude --permission-mode` for background runs, `auto` by default (Claude decides
  per action, as in an interactive session in auto mode). Also `acceptEdits`, `plan`, `dontAsk`,
  `manual`, `bypassPermissions`.
- `extra_args`: added to every background run, after the permission mode. A `--permission-mode` here
  wins over `permission_mode`.
```

and add `"permission_mode": "auto",` to the JSON example above it (after `"max_run_minutes": 30,`).

- [ ] **Step 4: Run the tests**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -l .
git add config.go run.go run_test.go README.md
git commit -m "Background runs use permission mode auto by default

permission_mode in config.json changes it; extra_args can still set
its own --permission-mode.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Free-prompt runs

**Files:**
- Modify: `run.go` (`Run`, `RunRequest`, `Start`, `exec`, `save`; add `runPrompt`, `runTitle`)
- Modify: `run_test.go`

**Interfaces:**
- Consumes: `runArgs` (Task 3).
- Produces: JSON `POST /api/run` body `{"skill","note","prompt","model","cwd"}` — either `skill` (+ `note`) or `prompt`; `Run.Prompt string` (`json:"prompt,omitempty"`) holds what `claude -p` received; `func runTitle(run *Run) string`.

- [ ] **Step 1: Write the failing tests** — append to `run_test.go` (add `"strings"` to its imports):

```go
func TestFreePromptRuns(t *testing.T) {
	r := &Runner{runs: map[string]*Run{}, cfg: defaultConfig(), demo: true, slot: make(chan struct{}, 8)}
	skills := []Skill{{Name: "k8s-health"}}
	for _, bad := range []RunRequest{
		{Model: "haiku"},
		{Prompt: "   ", Model: "haiku"},
		{Prompt: strings.Repeat("a", 8001), Model: "haiku"},
		{Prompt: "/nope do it", Model: "haiku"},
		{Prompt: "/", Model: "haiku"},
	} {
		if _, err := r.Start(bad, skills, nil); err == nil {
			t.Errorf("should refuse %.40q", bad.Prompt)
		}
	}
	run, err := r.Start(RunRequest{Prompt: "summarise the README", Model: "haiku"}, skills, nil)
	if err != nil || run.Prompt != "summarise the README" || run.Skill != "" {
		t.Fatalf("free prompt: %+v %v", run, err)
	}
	run, err = r.Start(RunRequest{Skill: "k8s-health", Note: " only pods ", Model: "haiku"}, skills, nil)
	if err != nil || run.Prompt != "/k8s-health only pods" {
		t.Fatalf("skill run: %+v %v", run, err)
	}
	if _, err := r.Start(RunRequest{Prompt: "/k8s-health now", Model: "haiku"}, skills, nil); err != nil {
		t.Errorf("a prompt may start with a known skill: %v", err)
	}
}

func TestRunTitle(t *testing.T) {
	if got := runTitle(&Run{Skill: "k8s-health", Prompt: "/k8s-health x"}); got != "/k8s-health" {
		t.Errorf("skill run: %q", got)
	}
	if got := runTitle(&Run{Prompt: "line one\nline two"}); got != "line one" {
		t.Errorf("free prompt: %q", got)
	}
}
```

- [ ] **Step 2: Run them to make sure they fail**

Run: `go test -run 'FreePrompt|RunTitle' ./...`
Expected: build FAIL — `unknown field Prompt`, `undefined: runTitle`.

- [ ] **Step 3: Implement** — `run.go`:

`Run`: add after `Skill`: `Prompt   string    \`json:"prompt,omitempty"\`` (what claude -p received). `RunRequest`: add `Prompt string \`json:"prompt"\``.

In `Start`, replace the opening skill check (from `found := false` through the `return nil, errors.New("unknown skill")` block) with

```go
	prompt, err := runPrompt(req, skills)
	if err != nil {
		return nil, err
	}
```

delete the `note := strings.TrimSpace(req.Note)` block with its 2000-character check, and in the `run := &Run{…}` literal use `Skill: req.Skill, Prompt: prompt, Model: req.Model, Cwd: cwd, Note: strings.TrimSpace(req.Note),`.

In `exec`, replace

```go
	prompt := "/" + run.Skill
	if run.Note != "" {
		prompt += " " + run.Note
	}
```

with `prompt := run.Prompt`. In `save`, replace `md := "# /" + run.Skill + "\n\n" +` with `md := "# " + runTitle(run) + "\n\n" +`.

Add:

```go
// runPrompt is what claude -p gets: "/skill note" for a skill run, or the
// user's own prompt. A prompt that starts with "/" must name a known skill.
func runPrompt(req RunRequest, skills []Skill) (string, error) {
	known := func(name string) bool {
		for _, s := range skills {
			if s.Name == name {
				return true
			}
		}
		return false
	}
	if req.Skill != "" {
		if !skillNameRe.MatchString(req.Skill) || !known(req.Skill) {
			return "", errors.New("unknown skill")
		}
		note := strings.TrimSpace(req.Note)
		if len([]rune(note)) > 2000 {
			return "", errors.New("note is too long (2000 characters max)")
		}
		if note == "" {
			return "/" + req.Skill, nil
		}
		return "/" + req.Skill + " " + note, nil
	}
	p := strings.TrimSpace(req.Prompt)
	if p == "" {
		return "", errors.New("empty prompt")
	}
	if len([]rune(p)) > 8000 {
		return "", errors.New("prompt is too long (8000 characters max)")
	}
	if strings.HasPrefix(p, "/") {
		f := strings.Fields(p[1:])
		if len(f) == 0 || !known(f[0]) {
			return "", errors.New("unknown skill in the prompt")
		}
	}
	return p, nil
}

// runTitle names a run in lists and reports: the skill, or the prompt's first line.
func runTitle(run *Run) string {
	if run.Skill != "" {
		return "/" + run.Skill
	}
	first, _, _ := strings.Cut(run.Prompt, "\n")
	return clip(strings.TrimSpace(first), 80)
}
```

- [ ] **Step 4: Run the tests**

Run: `go vet ./... && go test ./...`
Expected: PASS (the old `TestRunValidation` cases are still refused).

- [ ] **Step 5: Commit**

```bash
gofmt -l .
git add run.go run_test.go
git commit -m "Accept free prompts as background runs

A run is a skill with a note or the user's own prompt (8000 characters
max); a prompt starting with / must name a known skill.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Start the office from the page

**Files:**
- Modify: `main.go` (route, `api` error codes, `officeStart`, `errOfficeMissing`, `ansiRe`; imports `context`, `os/exec`, `regexp`)
- Create: `office_test.go`

**Interfaces:**
- Consumes: `findOfficeDir(configured, home string) string` (launch.go), `Config.OfficeDir`.
- Produces: `POST /api/office/start` → `200 {"ok":true,"output":"…"}`; `404 {"error":"agent-office-3d folder not found"}` when no folder; `400 {"error":"<script output>"}` when the script fails.

- [ ] **Step 1: Write the failing test** — create `office_test.go`:

```go
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
```

- [ ] **Step 2: Run it to make sure it fails**

Run: `go test -run OfficeStart ./...`
Expected: FAIL — the start call returns 404/405 (no such route).

- [ ] **Step 3: Implement** — `main.go`: register `mux.HandleFunc("POST /api/office/start", a.api(a.officeStart))` after the `GET /api/office` line. In `api`, after the `errBusy` check add

```go
			if errors.Is(err, errOfficeMissing) {
				code = http.StatusNotFound
			}
```

and add below `officeHealth`:

```go
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
	out, err := exec.CommandContext(ctx, "sh", filepath.Join(dir, "scripts", "start.sh"), "--no-browser").CombinedOutput()
	text := clip(ansiRe.ReplaceAllString(strings.TrimSpace(string(out)), ""), 2000)
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return nil, errors.New(text)
	}
	return map[string]any{"ok": true, "output": text}, nil
}
```

Add `"context"`, `"os/exec"`, `"regexp"` to the imports.

- [ ] **Step 4: Run the tests**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -l .
git add main.go office_test.go
git commit -m "Add POST /api/office/start

Runs agent-office-3d's scripts/start.sh --no-browser from the office
folder (config office_dir or the default places); 404 when there is none.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Page shell and the live sky

**Files:**
- Modify: `main.go` (embed list, `GET /assets/{name}` route, `asset` handler, `assetTypes`)
- Replace: `web/index.html` (whole file)
- Create: `web/app.css`, `web/sky.js`, `web/app.js`
- Create: `assets_test.go`

**Interfaces:**
- Produces (Go): `GET /assets/{app.js|sky.js|app.css|pano.svg}` with fixed content types; anything else 404.
- Produces (`web/sky.js`, ES module): `export const KEYS`, `export function skyAt(hour) → {top, horizon, cloud, stars, accent, hillFar, hillNear, sun, moon}` (`sun`/`moon` = `{x, y, t}` in percent or `null`), `export function clockHour(now?)` (honours `?saat=`), `export function fmtHour(h) → "HH:MM"`, `export function startSky(root, onPaint)`; sets `--acc` on `<html>` each paint.
- Produces (`web/app.js`): helpers `$`, `h`, `svg`, `nf`, `tok`, `usd`, `pct`, `ago`, `sinceISO`, `api`, `post`, `store`, `toast`, `copy`, `fitList`, `facts`; `state`; extension points `VIEWS = {ana, kullanim, kayitlar}` (render functions), `ON_BOOT` (async init functions run once), `RENDER` (drawer renderers by kind); `switchTab(tab)`, `openDrawer(kind, item)`, `closeDrawer()`, `loadUsage()`, `renderStats()`. Later tasks insert code **above the final `boot();` line**.

- [ ] **Step 1: Write the failing test** — create `assets_test.go`:

```go
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
```

- [ ] **Step 2: Run it to make sure it fails**

Run: `go test -run TestAssets ./...`
Expected: FAIL — `/assets/app.js: 404`.

- [ ] **Step 3: Serve the assets** — `main.go`: change the embed line to

```go
//go:embed web/index.html web/app.js web/app.css web/sky.js web/pano.svg web/fonts
var webFS embed.FS

// assetTypes are the only files /assets/ serves.
var assetTypes = map[string]string{
	"app.js":   "text/javascript; charset=utf-8",
	"sky.js":   "text/javascript; charset=utf-8",
	"app.css":  "text/css; charset=utf-8",
	"pano.svg": "image/svg+xml",
}
```

register `mux.HandleFunc("GET /assets/{name}", a.asset)` after the `/fonts/` route, and add next to `font`:

```go
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
```

- [ ] **Step 4: Write `web/index.html`** (replaces the old page entirely):

```html
<!doctype html>
<html lang="tr">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="pano-token" content="__PANO_TOKEN__">
<title>pano</title>
<link rel="icon" href="/assets/pano.svg" type="image/svg+xml">
<link rel="stylesheet" href="/assets/app.css">
<script type="module" src="/assets/app.js"></script>
</head>
<body>
<div id="sky" aria-hidden="true"></div>

<header class="glass top">
  <span class="logo">pano<i>.</i></span>
  <span class="clock" id="clock">--:--</span>
  <nav class="tabs" role="tablist" aria-label="Görünüm">
    <button type="button" role="tab" data-tab="ana" aria-selected="true">Ana</button>
    <button type="button" role="tab" data-tab="kullanim" aria-selected="false">Kullanım</button>
    <button type="button" role="tab" data-tab="kayitlar" aria-selected="false">Kayıtlar</button>
  </nav>
  <span class="stats" id="stats"></span>
  <button type="button" class="jobs" id="jobs" hidden></button>
  <span class="demo" id="demo" hidden>DEMO</span>
</header>

<main id="view-ana" class="view">
  <section class="glass side left" aria-labelledby="h-sessions">
    <div class="ph"><h2 id="h-sessions">Oturumlar</h2><span id="sessions-sub"></span></div>
    <ul class="list" id="sessions"></ul>
  </section>
  <section class="stage" id="stage" aria-label="Canlı ofis"></section>
  <section class="glass side right" aria-labelledby="h-skills">
    <div class="ph"><h2 id="h-skills">Skill'ler</h2><span>kullanıma göre</span></div>
    <input class="search" id="skill-search" type="search" placeholder="skill ara…" aria-label="Skill ara">
    <ul class="list" id="skills"></ul>
    <button type="button" class="more" id="skills-all"></button>
    <div class="sep"></div>
    <div class="ph"><h2>Hafıza</h2><span id="memory-sub"></span></div>
    <ul class="list" id="memory"></ul>
    <button type="button" class="more" id="memory-all"></button>
  </section>
  <form class="glass prompt" id="prompt" autocomplete="off">
    <textarea id="prompt-text" rows="1" placeholder="Claude'a arka planda bir iş ver…  / ile skill seç" aria-label="Prompt"></textarea>
    <ul class="glass picker" id="picker" role="listbox" hidden></ul>
    <select id="prompt-cwd" aria-label="Klasör"></select>
    <select id="prompt-model" aria-label="Model"></select>
    <button class="btn go" type="submit">Gönder</button>
  </form>
</main>

<main id="view-kullanim" class="view" hidden>
  <section class="glass u-sum">
    <div class="ph"><h2>Özet</h2>
      <span class="seg" id="days"><button type="button" data-days="7">7 gün</button><button type="button" data-days="30">30 gün</button><button type="button" data-days="90">90 gün</button></span>
    </div>
    <dl class="sum" id="sum"></dl>
  </section>
  <section class="glass u-daily"><div class="ph"><h2>Günlük token, modele göre</h2><ul class="legend" id="legend"></ul></div><div class="chart" id="daily"></div></section>
  <section class="glass u-hours"><div class="ph"><h2>Saat haritası</h2></div><div class="lamps" id="lamps"></div></section>
  <section class="glass u-tools"><div class="ph"><h2>En çok kullanılan araçlar</h2></div><ul class="bars" id="tools"></ul></section>
  <section class="glass u-projects"><div class="ph"><h2>Projeler, prompt sayısına göre</h2></div><ul class="bars" id="projects"></ul></section>
  <p class="note" id="usage-note"></p>
</main>

<main id="view-kayitlar" class="view" hidden>
  <section class="glass r-list"><div class="ph"><h2>Kayıtlar</h2><span id="runs-sub"></span></div><ul class="list" id="runs"></ul></section>
  <section class="glass r-detail" id="run-detail"><p class="empty">Soldan bir iş seç.</p></section>
</main>

<aside class="glass drawer" id="drawer" role="dialog" aria-labelledby="drawer-title" hidden>
  <button type="button" class="x" id="drawer-x" aria-label="Kapat">×</button>
  <div id="drawer-body"></div>
</aside>
<div class="toasts" id="toasts" aria-live="polite"></div>
</body>
</html>
```

- [ ] **Step 5: Write `web/app.css`**:

```css
@font-face{font-family:"Barlow";font-weight:400;font-display:swap;src:url(/fonts/barlow-latin-400-normal.woff2) format("woff2");unicode-range:U+0000-00FF,U+0131,U+0152-0153,U+02BB-02BC,U+02C6,U+02DA,U+02DC,U+2000-206F,U+20AC,U+2122,U+2191,U+2193,U+2212,U+2215,U+FEFF,U+FFFD}
@font-face{font-family:"Barlow";font-weight:400;font-display:swap;src:url(/fonts/barlow-latin-ext-400-normal.woff2) format("woff2");unicode-range:U+0100-02BA,U+02BD-02C5,U+02C7-02CC,U+02CE-02D7,U+02DD-02FF,U+0304,U+0308,U+0329,U+1D00-1DBF,U+1E00-1E9F,U+1EF2-1EFF,U+2020,U+20A0-20AB,U+20AD-20C0,U+2113,U+2C60-2C7F,U+A720-A7FF}
@font-face{font-family:"Barlow";font-weight:500;font-display:swap;src:url(/fonts/barlow-latin-500-normal.woff2) format("woff2");unicode-range:U+0000-00FF,U+0131,U+0152-0153,U+02BB-02BC,U+02C6,U+02DA,U+02DC,U+2000-206F,U+20AC,U+2122,U+2191,U+2193,U+2212,U+2215,U+FEFF,U+FFFD}
@font-face{font-family:"Barlow";font-weight:500;font-display:swap;src:url(/fonts/barlow-latin-ext-500-normal.woff2) format("woff2");unicode-range:U+0100-02BA,U+02BD-02C5,U+02C7-02CC,U+02CE-02D7,U+02DD-02FF,U+0304,U+0308,U+0329,U+1D00-1DBF,U+1E00-1E9F,U+1EF2-1EFF,U+2020,U+20A0-20AB,U+20AD-20C0,U+2113,U+2C60-2C7F,U+A720-A7FF}
@font-face{font-family:"Barlow";font-weight:600;font-display:swap;src:url(/fonts/barlow-latin-600-normal.woff2) format("woff2");unicode-range:U+0000-00FF,U+0131,U+0152-0153,U+02BB-02BC,U+02C6,U+02DA,U+02DC,U+2000-206F,U+20AC,U+2122,U+2191,U+2193,U+2212,U+2215,U+FEFF,U+FFFD}
@font-face{font-family:"Barlow";font-weight:600;font-display:swap;src:url(/fonts/barlow-latin-ext-600-normal.woff2) format("woff2");unicode-range:U+0100-02BA,U+02BD-02C5,U+02C7-02CC,U+02CE-02D7,U+02DD-02FF,U+0304,U+0308,U+0329,U+1D00-1DBF,U+1E00-1E9F,U+1EF2-1EFF,U+2020,U+20A0-20AB,U+20AD-20C0,U+2113,U+2C60-2C7F,U+A720-A7FF}

:root {
  --side: clamp(270px, 23vw, 380px);
  --gap: 16px;
  --panel: rgba(16,14,22,.6);
  --panel-solid: rgba(20,18,28,.88);
  --line: rgba(255,255,255,.12);
  --ink: #f3f1ed; --dim: #c9c6cf; --faint: #9a97a3;
  --chip: rgba(255,255,255,.08); --chip-hi: rgba(255,255,255,.15);
  --acc: #ff9b4a; --acc-ink: #2a1404;
  --warm: #ffb347; --run: #56d999; --wait: #f4c752; --err: #ff5a5f; --cold: #6b6676;
  --bar: #b9a48e;
  color-scheme: dark;
}
* { box-sizing: border-box; }
html, body { height: 100%; margin: 0; overflow: hidden; }
body { font: 14px/1.4 "Barlow", system-ui, sans-serif; color: var(--ink); background: #0d1018; font-variant-numeric: tabular-nums; }
button, input, select, textarea { font: inherit; color: inherit; }
button { cursor: pointer; }
:focus-visible { outline: 2px solid var(--acc); outline-offset: 2px; }
[hidden] { display: none !important; }

/* sky */
#sky { position: fixed; inset: 0; z-index: 0; overflow: hidden; }
#sky .grad, #sky .stars, #sky .clouds { position: absolute; inset: 0; }
#sky .stars i { position: absolute; width: 2px; height: 2px; border-radius: 50%; background: #fff; animation: tw 3.5s ease-in-out infinite alternate; }
#sky .stars i.big { width: 3px; height: 3px; }
@keyframes tw { from { opacity: .35; } to { opacity: 1; } }
#sky .orb { position: absolute; border-radius: 50%; transform: translate(-50%, -50%); }
#sky .sun { width: 150px; height: 150px; background: radial-gradient(circle, #fffbe8 0 34%, rgba(255,236,170,.9) 36%, rgba(255,210,120,.35) 55%, rgba(255,190,90,0) 72%); }
#sky .moon { width: 90px; height: 90px; background: radial-gradient(circle at 40% 40%, #f4f6ff 0 30%, rgba(220,228,255,.35) 38%, rgba(200,210,255,0) 66%); }
#sky .cloud { position: absolute; left: -260px; width: 240px; height: 70px; animation: drift linear infinite; will-change: transform; }
#sky .cloud::before, #sky .cloud::after, #sky .cloud span { content: ""; position: absolute; background: var(--cloud, #fff); border-radius: 50%; filter: blur(3px); }
#sky .cloud::before { left: 30px; top: 22px; width: 180px; height: 46px; }
#sky .cloud::after { left: 70px; top: 0; width: 90px; height: 64px; }
#sky .cloud span { left: 125px; top: 12px; width: 70px; height: 50px; }
@keyframes drift { to { transform: translateX(calc(100vw + 520px)); } }
#sky .hills { position: absolute; left: 0; bottom: 0; width: 100%; height: 26vh; }
#sky.paused * { animation-play-state: paused !important; }
@media (prefers-reduced-motion: reduce) { #sky .cloud, #sky .stars i { animation: none; } }

/* glass */
.glass { background: var(--panel); border: 1px solid var(--line); border-radius: 14px; backdrop-filter: blur(14px) saturate(1.2); -webkit-backdrop-filter: blur(14px) saturate(1.2); box-shadow: 0 10px 40px rgba(0,0,0,.18); }

/* top bar */
.top { position: fixed; z-index: 10; left: 20px; right: 20px; top: 14px; height: 46px; display: flex; align-items: center; gap: 16px; padding: 0 10px 0 16px; border-radius: 12px; }
.logo { font-weight: 600; font-size: 18px; letter-spacing: -.01em; }
.logo i { color: var(--acc); font-style: normal; }
.clock { font-size: 12.5px; color: var(--dim); background: var(--chip); padding: 3px 9px; border-radius: 7px; }
.tabs { display: flex; gap: 4px; margin-left: 4px; }
.tabs button { background: none; border: 0; padding: 6px 12px; border-radius: 8px; color: var(--dim); }
.tabs button[aria-selected="true"] { background: var(--chip-hi); color: var(--ink); }
.stats { margin-left: auto; display: flex; gap: 16px; color: var(--dim); font-size: 13px; white-space: nowrap; }
.stats b { color: var(--ink); font-weight: 600; }
.jobs { display: flex; align-items: center; gap: 7px; background: var(--chip); border: 0; border-radius: 8px; padding: 5px 10px; font-size: 13px; }
.jobs::before { content: ""; width: 7px; height: 7px; border-radius: 50%; background: var(--run); box-shadow: 0 0 8px var(--run); }
.demo { font-size: 11px; letter-spacing: .12em; color: var(--acc-ink); background: var(--acc); border-radius: 6px; padding: 3px 7px; font-weight: 600; }

/* views */
.view { position: fixed; z-index: 5; left: 20px; right: 20px; top: 76px; bottom: 20px; }
#view-ana { display: grid; grid-template-columns: var(--side) minmax(0, 1fr) var(--side); grid-template-rows: minmax(0, 1fr) 56px; gap: var(--gap); }
.side { grid-row: 1; display: flex; flex-direction: column; min-height: 0; padding: 14px 12px 10px; }
.side.left { grid-column: 1; }
.side.right { grid-column: 3; }
.stage { grid-column: 2; grid-row: 1; position: relative; min-height: 0; display: flex; align-items: center; justify-content: center; }
.stage iframe { position: absolute; inset: 0; width: 100%; height: 100%; border: 0; border-radius: 14px; }
.ph { display: flex; justify-content: space-between; align-items: baseline; gap: 8px; margin: 0 4px 8px; }
.ph h2 { margin: 0; font-size: 11.5px; font-weight: 600; letter-spacing: .12em; text-transform: uppercase; color: var(--dim); }
.ph > span { font-size: 12px; color: var(--faint); }
.list { list-style: none; margin: 0; padding: 0; position: relative; flex: 1 1 auto; min-height: 0; overflow: hidden; }
.list li > button { width: 100%; text-align: left; background: none; border: 0; border-radius: 9px; padding: 7px 8px; display: grid; gap: 1px 10px; }
.list li > button:hover { background: var(--chip); }
.list li > button[aria-current="true"] { background: var(--chip-hi); }
.sess, .run { grid-template-columns: 10px minmax(0, 1fr) auto; align-items: center; }
.t, .n { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.sess .m, .run .m { grid-column: 2; font-size: 12px; color: var(--faint); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.sess .c, .run .c { grid-column: 3; grid-row: 1 / span 2; text-align: right; font-size: 13px; }
.sess .c small { display: block; color: var(--faint); font-size: 11.5px; }
.lamp { width: 8px; height: 8px; border-radius: 50%; background: var(--cold); }
.lamp.warm { background: var(--warm); box-shadow: 0 0 9px var(--warm); }
.lamp.running { background: var(--run); box-shadow: 0 0 9px var(--run); }
.lamp.error { background: var(--err); }
.search { width: 100%; margin: 0 0 6px; background: var(--chip); border: 1px solid transparent; border-radius: 8px; padding: 6px 10px; }
.search::placeholder { color: var(--faint); }
.skill { grid-template-columns: minmax(0, 1fr) 56px 30px; align-items: center; font-size: 13.5px; }
.skill .n i { font-style: normal; color: var(--faint); }
.skill .u { text-align: right; color: var(--dim); font-size: 12.5px; }
.meter { height: 4px; border-radius: 2px; background: var(--chip); overflow: hidden; }
.meter > i { display: block; height: 100%; background: var(--bar); }
.mem { grid-template-columns: minmax(0, 1fr) auto; font-size: 13.5px; }
.mem .k { color: var(--faint); font-size: 12.5px; }
.mem.bad .k::before { content: ""; display: inline-block; width: 7px; height: 7px; border-radius: 50%; background: var(--err); margin-right: 6px; }
.more { align-self: flex-start; background: none; border: 0; color: var(--acc); font-size: 13px; padding: 4px 8px; }
.sep { height: 1px; background: var(--line); margin: 8px 4px 10px; flex: none; }
#skills { flex: 3 1 0; }
#memory { flex: 2 1 0; }
.empty { color: var(--faint); font-size: 13px; padding: 6px 8px; }

/* office card, buttons */
.card { position: relative; z-index: 1; max-width: 380px; padding: 18px 20px; }
.card h3 { margin: 0 0 6px; font-size: 17px; font-weight: 600; }
.card p { margin: 0 0 12px; color: var(--dim); font-size: 13.5px; }
.card pre { white-space: pre-wrap; font: 12px/1.45 ui-monospace, monospace; color: var(--dim); background: var(--chip); border-radius: 8px; padding: 8px 10px; margin: 0 0 12px; max-height: 160px; overflow: auto; }
.btn { background: var(--acc); color: var(--acc-ink); border: 0; border-radius: 9px; padding: 7px 14px; font-weight: 600; }
.btn:disabled { opacity: .5; cursor: default; }

/* prompt bar */
.prompt { grid-column: 1 / -1; grid-row: 2; justify-self: center; width: min(760px, 100%); display: flex; align-items: center; gap: 8px; padding: 0 8px 0 18px; border-radius: 28px; position: relative; }
.prompt textarea { flex: 1; resize: none; border: 0; background: none; outline: none; padding: 16px 0; max-height: 120px; line-height: 1.4; font-size: 15px; }
.prompt textarea::placeholder { color: var(--faint); }
.prompt select { appearance: none; border: 0; background: var(--chip); border-radius: 14px; padding: 6px 12px; font-size: 13px; color: var(--dim); max-width: 160px; }
.prompt .go { border-radius: 20px; padding: 9px 18px; }
.picker { position: absolute; left: 12px; bottom: calc(100% + 8px); width: min(480px, 90%); max-height: 300px; overflow: auto; margin: 0; padding: 6px; list-style: none; background: var(--panel-solid); }
.picker li { padding: 6px 10px; border-radius: 8px; font-size: 13.5px; cursor: pointer; display: flex; justify-content: space-between; gap: 10px; }
.picker li small { color: var(--faint); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 55%; }
.picker li[aria-selected="true"] { background: var(--chip-hi); }

/* drawer */
.drawer { position: fixed; z-index: 20; top: 76px; bottom: 96px; left: calc(20px + var(--side) + var(--gap)); right: calc(20px + var(--side) + var(--gap)); padding: 18px 20px; overflow: auto; background: var(--panel-solid); }
.drawer .x { position: absolute; top: 10px; right: 10px; width: 30px; height: 30px; border: 0; border-radius: 8px; background: var(--chip); font-size: 18px; line-height: 1; }
.drawer h3, .r-detail h3 { margin: 0 40px 6px 0; font-size: 19px; font-weight: 600; word-break: break-word; }
.lead { color: var(--dim); margin: 0 0 12px; }
.facts { display: grid; grid-template-columns: max-content 1fr; gap: 6px 16px; margin: 0 0 14px; font-size: 13.5px; }
.facts dt { color: var(--faint); }
.facts dd { margin: 0; word-break: break-all; }
.actions { display: flex; gap: 8px; margin: 4px 0 14px; }
.output, .drawer pre { white-space: pre-wrap; font: 12.5px/1.5 ui-monospace, monospace; background: rgba(0,0,0,.25); border-radius: 10px; padding: 12px 14px; margin: 0 0 12px; }
.drawer .list { overflow: visible; }
.refs { list-style: none; margin: 0; padding: 0; font-size: 13px; }
.refs li { display: flex; justify-content: space-between; gap: 10px; padding: 3px 0; }
.refs li.missing { color: var(--err); }
.tags { display: flex; flex-wrap: wrap; gap: 6px; margin: 0 0 12px; padding: 0; list-style: none; }
.tags li { background: var(--chip); border-radius: 999px; padding: 3px 10px; font-size: 12.5px; color: var(--dim); }

/* toasts */
.toasts { position: fixed; z-index: 30; right: 24px; bottom: 92px; display: flex; flex-direction: column; gap: 8px; align-items: flex-end; }
.toast { background: var(--panel-solid); border: 1px solid var(--line); border-radius: 10px; padding: 9px 14px; font-size: 13.5px; box-shadow: 0 8px 24px rgba(0,0,0,.3); }
.toast.ok { border-color: rgba(86,217,153,.5); }
.toast.err { border-color: rgba(255,90,95,.6); }

/* usage */
#view-kullanim { display: grid; grid-template-columns: minmax(0, 2fr) minmax(0, 1fr); grid-template-rows: auto minmax(0, 1.2fr) minmax(0, 1fr) auto; gap: var(--gap); }
#view-kullanim > section { padding: 14px 16px; min-height: 0; display: flex; flex-direction: column; }
.u-sum { grid-column: 1 / -1; }
.u-daily { grid-column: 1; grid-row: 2; }
.u-hours { grid-column: 2; grid-row: 2; }
.u-tools { grid-column: 1; grid-row: 3; }
.u-projects { grid-column: 2; grid-row: 3; }
.sum { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 12px; margin: 0; }
.sum div { min-width: 0; }
.sum dt { font-size: 12px; color: var(--faint); }
.sum dd { margin: 2px 0 0; font-size: 24px; font-weight: 600; }
.sum dd small { display: block; font-size: 12px; font-weight: 400; color: var(--dim); }
.seg { display: flex; gap: 4px; }
.seg button { border: 0; background: var(--chip); border-radius: 7px; padding: 3px 10px; font-size: 12.5px; color: var(--dim); }
.seg button[aria-pressed="true"] { background: var(--chip-hi); color: var(--ink); }
.chart { flex: 1; min-height: 0; }
.chart svg { display: block; width: 100%; height: 100%; }
.chart .axis { fill: var(--faint); font-size: 11px; }
.legend { display: flex; gap: 14px; list-style: none; margin: 0; padding: 0; font-size: 12.5px; color: var(--dim); }
.legend i { display: inline-block; width: 14px; height: 4px; border-radius: 2px; margin-right: 6px; vertical-align: middle; }
.lamps { flex: 1; display: grid; grid-template-columns: 30px repeat(24, minmax(0, 1fr)); gap: 3px; align-content: center; font-size: 10.5px; color: var(--faint); }
.lamps .cell { aspect-ratio: 1; border-radius: 50%; background: var(--chip); }
.lamps .cell.on { background: color-mix(in srgb, var(--acc) calc(var(--v) * 100%), rgba(255,255,255,.1)); }
.bars { list-style: none; margin: 0; padding: 0; display: grid; align-content: start; gap: 5px; font-size: 13.5px; position: relative; flex: 1; min-height: 0; overflow: hidden; }
.bars li { display: grid; grid-template-columns: minmax(0, 150px) 1fr 50px; gap: 10px; align-items: center; }
.bars .n { text-align: right; color: var(--dim); }
.note { grid-column: 1 / -1; margin: 0; font-size: 12px; color: var(--dim); text-shadow: 0 1px 2px rgba(0,0,0,.6); }

/* runs */
#view-kayitlar { display: grid; grid-template-columns: minmax(300px, 380px) minmax(0, 1fr); gap: var(--gap); }
#view-kayitlar > section { padding: 14px 12px; min-height: 0; display: flex; flex-direction: column; }
#runs { overflow: auto; }
.r-detail { overflow: auto; padding: 18px 20px !important; }
```

- [ ] **Step 6: Write `web/sky.js`**:

```js
// The sky behind pano. It follows the local clock with the keyframes of
// agent-office-3d's daylight.js, so the office windows and the page agree.
// Sun and moon move once a minute, clouds drift on the GPU, and it all
// pauses while the tab is hidden. ?saat=18.25 pins the clock (for testing).

// hour, sky top, horizon, clouds, stars (0-1), accent, far hills, near hills
export const KEYS = [
  [0, "#060a1a", "#141d3a", "#2a3150", 1, "#ffb36b", "#0d1226", "#151b33"],
  [4.5, "#0a1130", "#2a2c55", "#3a3d63", 0.9, "#ffb36b", "#141a35", "#1c2242"],
  [6, "#3d3a72", "#f2a088", "#f3b4b8", 0.3, "#ff8fa3", "#5a3f5a", "#6e4a5e"],
  [7.5, "#5b98d6", "#ffd7a6", "#fff1e6", 0, "#ff9b4a", "#7c6a63", "#8f7766"],
  [12, "#3b8ce0", "#c4e6ff", "#ffffff", 0, "#ff9b4a", "#6f8a6a", "#7f9b74"],
  [16, "#4a8bd2", "#ffe0ae", "#fff6e8", 0, "#ff9b4a", "#7f8a62", "#958f68"],
  [18, "#5a5aa0", "#ff9658", "#ffc29a", 0.08, "#ff7a45", "#6a4a4a", "#7d5446"],
  [19.5, "#25275a", "#cf5f78", "#a06f88", 0.55, "#ff8f6b", "#2d2440", "#3a2a44"],
  [21, "#0a1130", "#1e2950", "#2a3352", 0.95, "#ffb36b", "#121833", "#19203b"],
  [24, "#060a1a", "#141d3a", "#2a3150", 1, "#ffb36b", "#0d1226", "#151b33"],
];

const rgb = (c) => [1, 3, 5].map((i) => parseInt(c.slice(i, i + 2), 16));
function mix(a, b, t) {
  const x = rgb(a), y = rgb(b);
  return `rgb(${x.map((v, i) => Math.round(v + (y[i] - v) * t)).join(",")})`;
}

// Where the sun or moon stands, in percent of the screen; null below the horizon.
function arc(t) {
  return t > 0 && t < 1 ? { x: 6 + t * 88, y: 78 - Math.sin(Math.PI * t) * 66, t } : null;
}

export function skyAt(hour) {
  const h = ((hour % 24) + 24) % 24;
  let i = 0;
  while (i < KEYS.length - 2 && h > KEYS[i + 1][0]) i++;
  const a = KEYS[i], b = KEYS[i + 1], t = (h - a[0]) / (b[0] - a[0]);
  return {
    top: mix(a[1], b[1], t), horizon: mix(a[2], b[2], t), cloud: mix(a[3], b[3], t),
    stars: a[4] + (b[4] - a[4]) * t, accent: t < 0.5 ? a[5] : b[5],
    hillFar: mix(a[6], b[6], t), hillNear: mix(a[7], b[7], t),
    sun: arc((h - 5.8) / (18.7 - 5.8)), moon: arc(((h - 19.2 + 24) % 24) / 10.5),
  };
}

export function clockHour(now = new Date()) {
  const pinned = new URLSearchParams(location.search).get("saat");
  if (pinned !== null && pinned !== "" && !Number.isNaN(+pinned)) return ((+pinned % 24) + 24) % 24;
  return now.getHours() + now.getMinutes() / 60;
}

export function fmtHour(h) {
  const m = Math.floor(h * 60) % 1440;
  return String(Math.floor(m / 60)).padStart(2, "0") + ":" + String(m % 60).padStart(2, "0");
}

const HILLS = `<svg class="hills" viewBox="0 0 1280 190" preserveAspectRatio="none">
<path class="far" d="M0 120 C 160 70 300 90 430 105 S 700 60 860 95 S 1120 70 1280 100 V190 H0 Z"/>
<path class="near" d="M0 150 C 200 115 360 140 520 132 S 820 110 1000 140 S 1180 125 1280 135 V190 H0 Z"/></svg>`;

const CLOUDS = [[60, 0.85, 140, 0], [140, 1.1, 190, -60], [40, 0.7, 120, -95], [190, 0.9, 170, -130], [100, 1.25, 230, -20]];

// startSky draws the sky into root and repaints it once a minute; onPaint(hour)
// runs after every paint (the clock in the top bar).
export function startSky(root, onPaint) {
  root.innerHTML = `<div class="grad"></div><div class="stars"></div><div class="orb sun"></div><div class="orb moon"></div><div class="clouds"></div>${HILLS}`;
  const stars = root.querySelector(".stars");
  for (let i = 0; i < 90; i++) {
    const s = document.createElement("i");
    s.style.left = Math.random() * 100 + "%";
    s.style.top = Math.random() * 62 + "%";
    s.style.animationDelay = -Math.random() * 3.5 + "s";
    if (Math.random() < 0.15) s.className = "big";
    stars.append(s);
  }
  const clouds = root.querySelector(".clouds");
  for (const [top, scale, secs, delay] of CLOUDS) {
    const c = document.createElement("div");
    c.className = "cloud";
    c.innerHTML = "<span></span>";
    Object.assign(c.style, { top: top + "px", scale: String(scale), animationDuration: secs + "s", animationDelay: delay + "s" });
    clouds.append(c);
  }
  const grad = root.querySelector(".grad"), sun = root.querySelector(".sun"), moon = root.querySelector(".moon");
  const far = root.querySelector(".far"), near = root.querySelector(".near");
  const place = (el, p) => {
    el.hidden = !p;
    if (p) Object.assign(el.style, { left: p.x + "%", top: p.y + "%" });
  };
  function paint() {
    const hour = clockHour(), s = skyAt(hour);
    grad.style.background = `linear-gradient(180deg, ${s.top} 0%, ${s.horizon} 78%)`;
    root.style.setProperty("--cloud", s.cloud);
    stars.style.opacity = s.stars.toFixed(2);
    place(sun, s.sun);
    if (s.sun) sun.style.filter = `hue-rotate(${(-25 * (1 - Math.sin(Math.PI * s.sun.t))).toFixed(1)}deg)`;
    place(moon, s.moon);
    far.setAttribute("fill", s.hillFar);
    near.setAttribute("fill", s.hillNear);
    document.documentElement.style.setProperty("--acc", s.accent);
    onPaint?.(hour);
  }
  let timer = null;
  const run = () => {
    paint();
    timer = setInterval(paint, 60_000);
  };
  document.addEventListener("visibilitychange", () => {
    root.classList.toggle("paused", document.hidden);
    if (document.hidden) {
      clearInterval(timer);
      timer = null;
    } else if (!timer) run();
  });
  run();
}
```

- [ ] **Step 7: Write `web/app.js`** (the core; Tasks 7 and 8 insert their sections above the final `boot();`):

```js
// pano's page: a live sky, the office in the middle, glass panels around it.
// Plain ES modules, no build step; every API call carries the per-start token.
import { startSky, fmtHour } from "/assets/sky.js";

const TOKEN = document.querySelector('meta[name="pano-token"]').content;
const $ = (s, el = document) => el.querySelector(s);
const h = (tag, attrs = {}, ...kids) => {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v == null || v === false) continue;
    if (k === "class") el.className = v;
    else if (k === "style") el.style.cssText = v;
    else if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
    else el.setAttribute(k, v === true ? "" : v);
  }
  for (const kid of kids.flat()) if (kid != null && kid !== false) el.append(kid.nodeType ? kid : String(kid));
  return el;
};
const svg = (tag, attrs = {}) => {
  const el = document.createElementNS("http://www.w3.org/2000/svg", tag);
  for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
  return el;
};

const nf = new Intl.NumberFormat("tr-TR");
const cf = new Intl.NumberFormat("tr-TR", { notation: "compact", maximumFractionDigits: 1 });
const usd = (n) => "$" + (n >= 100 ? nf.format(Math.round(n)) : n.toLocaleString("tr-TR", { minimumFractionDigits: 2, maximumFractionDigits: 2 }));
const tok = (n) => cf.format(n);
const pct = (x) => "%" + (x * 100).toLocaleString("tr-TR", { maximumFractionDigits: 1 });
function ago(sec) {
  if (sec < 60) return "az önce";
  const m = Math.floor(sec / 60);
  if (m < 60) return m + " dk";
  const hr = Math.floor(m / 60);
  if (hr < 24) return hr + " sa";
  return Math.floor(hr / 24) + " gün";
}
const sinceISO = (iso) => (Date.now() - new Date(iso).getTime()) / 1000;
const base = (p) => (p || "").split("/").filter(Boolean).pop() || "~";

async function api(path, opts = {}) {
  const res = await fetch(path, { ...opts, headers: { "X-Pano-Token": TOKEN, ...(opts.headers || {}) } });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = new Error(body.error || res.statusText);
    err.status = res.status;
    throw err;
  }
  return body;
}
const post = (path, data) => api(path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(data ?? {}) });

// Per-viewer conveniences only (last folder and model); storage may be missing.
const store = {
  get(k) { try { return localStorage.getItem(k); } catch { return null; } },
  set(k, v) { try { localStorage.setItem(k, v); } catch { /* private window */ } },
};

const state = { tab: "ana", days: 30, meta: null, ov: null, sessions: [], skills: [], memory: [], runs: [], known: new Map(), runSel: null, office: null };
const VIEWS = { ana() {}, kullanim() {}, kayitlar() {} };
const ON_BOOT = [];
const RENDER = {};

function toast(text, { kind = "", onclick } = {}) {
  const el = h("button", { type: "button", class: "toast " + kind, onclick: () => { el.remove(); onclick?.(); } }, text);
  $("#toasts").append(el);
  setTimeout(() => el.remove(), 7000);
}
async function copy(text, msg) {
  try {
    await navigator.clipboard.writeText(text);
    toast(msg, { kind: "ok" });
  } catch {
    toast(text);
  }
}

// fitList hides the rows that would not fit, so panels never scroll the page;
// it returns how many rows it hid.
function fitList(ul) {
  const kids = [...ul.children];
  kids.forEach((li) => (li.hidden = false));
  let hidden = 0;
  for (const li of kids) {
    if (li.offsetTop + li.offsetHeight > ul.clientHeight) {
      li.hidden = true;
      hidden++;
    }
  }
  return hidden;
}

const facts = (rows) => h("dl", { class: "facts" }, rows.filter(Boolean).flatMap(([k, v]) => [h("dt", {}, k), h("dd", {}, v)]));

function openDrawer(kind, item) {
  $("#drawer-body").replaceChildren(...[RENDER[kind](item)].flat().filter(Boolean));
  $("#drawer").hidden = false;
  $("#drawer-x").focus();
}
function closeDrawer() {
  $("#drawer").hidden = true;
}

function renderStats() {
  const ov = state.ov;
  if (!ov) return;
  const w = ov.windows;
  $("#stats").replaceChildren(
    h("span", {}, "5 sa ", h("b", {}, tok(w.fiveHour)), " · " + usd(w.fiveCost)),
    h("span", {}, "7 gün ", h("b", {}, usd(w.sevenCost))),
    h("span", {}, "önbellek ", h("b", {}, pct(ov.totals.cacheHit))));
  $("#demo").hidden = !ov.demo;
}
async function loadUsage() {
  state.ov = await api(`/api/overview?days=${state.days}`);
  renderStats();
  if (state.tab === "kullanim") VIEWS.kullanim();
}

function switchTab(tab) {
  state.tab = tab;
  for (const b of document.querySelectorAll(".tabs button")) b.setAttribute("aria-selected", String(b.dataset.tab === tab));
  for (const v of Object.keys(VIEWS)) $("#view-" + v).hidden = v !== tab;
  closeDrawer();
  VIEWS[tab]();
}

async function boot() {
  startSky($("#sky"), (hour) => ($("#clock").textContent = fmtHour(hour)));
  document.querySelectorAll(".tabs button").forEach((b) => b.addEventListener("click", () => switchTab(b.dataset.tab)));
  $("#drawer-x").addEventListener("click", closeDrawer);
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && !$("#drawer").hidden) closeDrawer();
  });
  try { state.meta = await api("/api/meta"); } catch { /* shown as empty panels */ }
  try { await loadUsage(); } catch { /* stats stay empty */ }
  for (const init of ON_BOOT) await init();
  setInterval(() => !document.hidden && loadUsage().catch(() => {}), 60_000);
  let t;
  addEventListener("resize", () => {
    clearTimeout(t);
    t = setTimeout(() => VIEWS[state.tab](), 150);
  });
  switchTab("ana");
}

boot();
```

- [ ] **Step 8: Run the Go tests**

Run: `go vet ./... && go test ./...`
Expected: PASS (`TestAssets` included).

- [ ] **Step 9: Check it in a browser**

```bash
go build -o pano . && ./pano serve -addr 127.0.0.1:7979 &
```

Open `http://127.0.0.1:7979/?saat=12` and `?saat=23` (Claude in Chrome or any browser). Expected: sky with sun (noon) / moon and stars (night), drifting clouds, hills; the glass top bar with clock `12:00` / `23:00`, tabs and the three small numbers; empty side panels; empty prompt bar; tabs switch views; no console errors; no page scroll. Stop the server (`kill %1`).

- [ ] **Step 10: Commit**

```bash
gofmt -l .
git add main.go assets_test.go web/index.html web/app.css web/sky.js web/app.js
git commit -m "New page shell: live sky, glass top bar, three views

The electrical-panel page goes. index.html is a skeleton; app.css,
sky.js and app.js are served from /assets/ as ES modules.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: Ana tab — panels, drawer, office, prompt bar, runs

**Files:**
- Modify: `web/app.js` (insert the block below above the final `boot();`)

**Interfaces:**
- Consumes (Task 6): `h`, `$`, `api`, `post`, `store`, `toast`, `copy`, `fitList`, `facts`, `tok`, `usd`, `nf`, `ago`, `sinceISO`, `base`, `state`, `VIEWS`, `ON_BOOT`, `RENDER`, `openDrawer`, `closeDrawer`, `switchTab`.
- Consumes (backend): `GET /api/sessions?limit=30` → `Session[]` (`id, project, cwd, title, model, family, start, idleSec, cache, cacheLeftSec, context, cost, prompts, nextWarm, nextCold`); `GET /api/skills` → `Skill[]` (`name, kind, scope, owner, path, description, model, tokens, dirTokens, files, uses, lastUsed`); `GET /api/memory` → `MemFile[]` (`path, label, group, kind, tokens, lines, headings, refs[], missing`); `GET /api/runs` → `Run[]` (`id, skill, prompt, model, cwd, note, status, started, finished, cost, turns, result, report`); `POST /api/run`; `GET /api/office` → `{url, up}`; `POST /api/office/start`.
- Produces: `sortedSkills(q)`, `skillRow(s, max)`, `memRow(m)`, `runTitle(r)`, `runDetail(r)`, `setPrompt(text)`, `loadRuns()`, `renderJobs()`; `RENDER.session/skill/memory/skills/memories/run`; `VIEWS.ana`. Task 8 uses `runTitle`, `runDetail`, `state.runs`, `state.runSel`.

- [ ] **Step 1: Insert the Ana block** into `web/app.js` above `boot();`:

```js
/* ---------- Ana: sessions, skills, memory ---------- */
function sessionRow(s) {
  const warm = s.cache === "warm";
  return h("li", {}, h("button", { type: "button", class: "sess", onclick: () => openDrawer("session", s) },
    h("span", { class: "lamp" + (warm ? " warm" : "") }),
    h("span", { class: "t" }, s.title || "(başlıksız)"),
    h("span", { class: "c" }, usd(warm ? s.nextWarm : s.nextCold), h("small", {}, warm ? `sıcak ${Math.ceil(s.cacheLeftSec / 60)} dk` : ago(s.idleSec))),
    h("span", { class: "m" }, `${s.project} · ${s.family} · ${tok(s.context)} bağlam`)));
}
function renderSessions() {
  const list = state.sessions, warm = list.filter((s) => s.cache === "warm").length;
  $("#sessions-sub").textContent = `${warm} sıcak · ${list.length - warm} soğuk`;
  $("#sessions").replaceChildren(...(list.length ? list.map(sessionRow) : [h("li", { class: "empty" }, "Henüz oturum yok.")]));
  fitList($("#sessions"));
}

function skillLabel(name) {
  const i = name.lastIndexOf(":");
  return i < 0 ? [name] : [h("i", {}, name.slice(0, i + 1)), name.slice(i + 1)];
}
function sortedSkills(q = "") {
  const query = q.trim().toLowerCase();
  return state.skills
    .filter((s) => !query || s.name.toLowerCase().includes(query) || (s.description || "").toLowerCase().includes(query))
    .sort((a, b) => b.uses - a.uses || new Date(b.lastUsed) - new Date(a.lastUsed) || a.name.localeCompare(b.name));
}
function skillRow(s, max) {
  return h("li", {}, h("button", { type: "button", class: "skill", title: s.description || s.name, onclick: () => openDrawer("skill", s) },
    h("span", { class: "n" }, ...skillLabel(s.name)),
    h("span", { class: "meter" }, h("i", { style: `width:${max ? (s.uses / max) * 100 : 0}%` })),
    h("span", { class: "u" }, s.uses ? s.uses + "×" : "—")));
}
function renderSkills() {
  const list = sortedSkills($("#skill-search").value), max = Math.max(0, ...list.map((s) => s.uses));
  $("#skills").replaceChildren(...(list.length ? list.map((s) => skillRow(s, max)) : [h("li", { class: "empty" }, "Eşleşen skill yok.")]));
  fitList($("#skills"));
  $("#skills-all").textContent = `tümü (${state.skills.length}) →`;
}

const memName = (m) => `${m.group} / ${m.label.split("/").pop()}` + (m.kind === "memory" && m.refs?.length ? ` → ${m.refs.length} not` : "");
function memRow(m) {
  const broken = (m.refs || []).some((r) => r.missing);
  return h("li", {}, h("button", { type: "button", class: "mem" + (broken ? " bad" : ""), title: broken ? "Kırık bağlantı var" : m.label, onclick: () => openDrawer("memory", m) },
    h("span", { class: "n" }, memName(m)), h("span", { class: "k" }, tok(m.tokens))));
}
function renderMemory() {
  const list = [...state.memory].sort((a, b) => b.tokens - a.tokens);
  $("#memory-sub").textContent = `${list.length} dosya`;
  $("#memory").replaceChildren(...(list.length ? list.map(memRow) : [h("li", { class: "empty" }, "CLAUDE.md bulunamadı.")]));
  const hidden = fitList($("#memory"));
  $("#memory-all").textContent = hidden ? `+ ${hidden} dosya →` : "tümü →";
}

VIEWS.ana = () => {
  renderSessions();
  renderSkills();
  renderMemory();
};

/* ---------- drawer contents ---------- */
const SCOPE = { user: "kullanıcı skill'i", project: "proje skill'i", plugin: "plugin skill'i", builtin: "yerleşik" };
const shq = (s) => "'" + s.replaceAll("'", "'\\''") + "'";
const runTitle = (r) => (r.skill ? "/" + r.skill + (r.note ? " " + r.note : "") : (r.prompt || "").split("\n")[0]);

function listDrawer(title, filter, row) {
  const ul = h("ul", { class: "list" });
  const paint = (q) => {
    const items = filter(q), max = Math.max(0, ...items.map((x) => x.uses || 0));
    ul.replaceChildren(...items.map((x) => row(x, max)));
  };
  const input = h("input", { class: "search", type: "search", placeholder: "ara…", "aria-label": "Ara", oninput: (e) => paint(e.target.value) });
  paint("");
  setTimeout(() => input.focus());
  return [h("h3", { id: "drawer-title" }, title), input, ul];
}

function runDetail(r) {
  const done = r.finished && !r.finished.startsWith("0001");
  const secs = Math.max(0, ((done ? new Date(r.finished) : new Date()) - new Date(r.started)) / 1000);
  return [
    h("h3", { id: "drawer-title" }, runTitle(r)),
    facts([
      ["Durum", { running: "çalışıyor", done: "bitti", error: "hata" }[r.status] || r.status],
      ["Klasör", r.cwd], ["Model", r.model],
      ["Süre", secs < 60 ? Math.round(secs) + " sn" : Math.round(secs / 60) + " dk"],
      ["Maliyet", usd(r.cost || 0)], ["Tur", nf.format(r.turns || 0)],
      r.report ? ["Rapor", r.report] : null,
    ]),
    r.prompt && r.prompt.includes("\n") ? h("pre", {}, r.prompt) : null,
    h("pre", { class: "output" }, r.status === "running" ? "Çalışıyor…" : r.result || "(çıktı yok)"),
  ];
}

Object.assign(RENDER, {
  session: (s) => [
    h("h3", { id: "drawer-title" }, s.title || "(başlıksız)"),
    facts([
      ["Klasör", s.cwd], ["Model", s.model],
      ["Başladı", ago(sinceISO(s.start)) + " önce"], ["Son etkinlik", ago(s.idleSec) + " önce"],
      ["Prompt", nf.format(s.prompts)], ["Bağlam", tok(s.context) + " token"],
      ["Önbellek", s.cache === "warm" ? `sıcak, ${Math.ceil(s.cacheLeftSec / 60)} dk kaldı` : "soğuk"],
      ["Sonraki mesaj", `${usd(s.nextWarm)} sıcakken, ${usd(s.nextCold)} soğukken`],
      ["Toplam", usd(s.cost)],
    ]),
    h("div", { class: "actions" }, h("button", { type: "button", class: "btn", onclick: () => copy(`cd ${shq(s.cwd)} && claude --resume ${s.id}`, "Devam komutu kopyalandı") }, "Devam et")),
  ],
  skill: (s) => [
    h("h3", { id: "drawer-title" }, "/" + s.name),
    s.description ? h("p", { class: "lead" }, s.description) : null,
    facts([
      ["Tür", `${s.kind === "command" ? "komut" : "skill"}, ${SCOPE[s.scope] || s.scope}${s.owner ? " · " + s.owner : ""}`],
      ["Boyut", s.scope === "builtin" ? "bilinmiyor (diskte değil)" : `${nf.format(s.tokens)} token açılınca, klasörüyle ${nf.format(s.dirTokens)} (${s.files} dosya)`],
      s.model ? ["Sabit model", s.model] : null,
      ["Kullanım", s.uses ? `${s.uses} kez, son: ${ago(sinceISO(s.lastUsed))} önce` : "hiç"],
      s.path ? ["Dosya", s.path] : null,
    ]),
    h("div", { class: "actions" }, h("button", { type: "button", class: "btn", onclick: () => { closeDrawer(); setPrompt("/" + s.name + " "); } }, "Çalıştır")),
  ],
  memory: (m) => [
    h("h3", { id: "drawer-title" }, m.label),
    facts([["Grup", m.group], ["Boyut", `${nf.format(m.tokens)} token, ${nf.format(m.lines)} satır`]]),
    m.headings?.length ? h("ul", { class: "tags" }, m.headings.map((x) => h("li", {}, x))) : null,
    m.refs?.length ? h("ul", { class: "refs" }, m.refs.map((r) => h("li", { class: r.missing ? "missing" : "" }, h("span", {}, r.label), h("span", {}, r.missing ? "yok" : tok(r.tokens) + " tok")))) : null,
  ],
  skills: () => listDrawer("Skill'ler", (q) => sortedSkills(q), skillRow),
  memories: () => listDrawer("Hafıza", (q) => state.memory.filter((m) => !q || m.label.toLowerCase().includes(q.toLowerCase())), memRow),
  run: runDetail,
});

/* ---------- office ---------- */
const card = (title, text, ...extra) => h("div", { class: "glass card" }, h("h3", {}, title), h("p", {}, text), ...extra);
async function loadOffice() {
  let st;
  try { st = await api("/api/office"); } catch { return; }
  const key = st.url + "|" + st.up;
  if (key === state.office) return;
  state.office = key;
  const stage = $("#stage");
  if (!st.url) return stage.replaceChildren(card("Ofis gizli", "pano -office off ile başlatıldı."));
  if (st.up) return stage.replaceChildren(h("iframe", { src: st.url, title: "agent-office-3d canlı ofis", referrerpolicy: "no-referrer" }));
  const out = h("pre", { hidden: true });
  const btn = h("button", {
    type: "button", class: "btn", onclick: async () => {
      btn.disabled = true;
      btn.textContent = "Başlatılıyor…";
      try {
        await post("/api/office/start");
        state.office = null;
        setTimeout(loadOffice, 1500);
      } catch (e) {
        out.hidden = false;
        out.textContent = e.status === 404
          ? "agent-office-3d bulunamadı. github.com/NuriOkumus/agent-office-3d (linux-macos-support dalı) indir, ~/agent-office-3d ya da ~/Projects/agent-office-3d içine koy ve install.sh çalıştır."
          : e.message;
        btn.disabled = false;
        btn.textContent = "Tekrar dene";
      }
    },
  }, "Başlat");
  stage.replaceChildren(card("Ofis kapalı", "agent-office-3d çalışmıyor.", out, btn));
}

/* ---------- prompt bar ---------- */
const picker = { items: [], sel: 0 };
function autosize() {
  const ta = $("#prompt-text");
  ta.style.height = "auto";
  ta.style.height = Math.min(ta.scrollHeight, 120) + "px";
}
function updatePicker() {
  const ul = $("#picker"), m = /^\/(\S*)$/.exec($("#prompt-text").value);
  picker.items = m ? sortedSkills(m[1]).slice(0, 8) : [];
  picker.sel = 0;
  ul.hidden = !picker.items.length;
  ul.replaceChildren(...picker.items.map((s, i) => h("li", { role: "option", "aria-selected": String(i === 0), onmousedown: (e) => { e.preventDefault(); pick(i); } },
    h("span", {}, "/" + s.name), h("small", {}, s.description || ""))));
}
function movePicker(d) {
  picker.sel = (picker.sel + d + picker.items.length) % picker.items.length;
  [...$("#picker").children].forEach((li, i) => li.setAttribute("aria-selected", String(i === picker.sel)));
}
function pick(i) {
  const s = picker.items[i];
  if (s) setPrompt("/" + s.name + " ");
}
function setPrompt(text) {
  if (state.tab !== "ana") switchTab("ana");
  const ta = $("#prompt-text");
  ta.value = text;
  ta.focus();
  ta.setSelectionRange(text.length, text.length);
  autosize();
  updatePicker();
}
function fillSelects() {
  const meta = state.meta;
  if (!meta) return;
  const projects = meta.projects || [], models = meta.models || [];
  const saved = store.get("pano.cwd"), latest = state.sessions[0]?.cwd;
  $("#prompt-cwd").replaceChildren(h("option", { value: "~" }, "📁 ~"), ...projects.map((p) => h("option", { value: p, title: p }, "📁 " + base(p))));
  $("#prompt-cwd").value = saved === "~" || projects.includes(saved) ? saved : projects.includes(latest) ? latest : "~";
  const savedModel = store.get("pano.model");
  $("#prompt-model").replaceChildren(...models.map((m) => h("option", { value: m }, m)));
  $("#prompt-model").value = models.includes(savedModel) ? savedModel : models.includes("sonnet") ? "sonnet" : models[0];
}
async function sendPrompt(e) {
  e.preventDefault();
  const text = $("#prompt-text").value.trim();
  if (!text) return;
  const cwd = $("#prompt-cwd").value, model = $("#prompt-model").value;
  let body = { prompt: text, cwd, model };
  const m = /^\/(\S+)\s*([\s\S]*)$/.exec(text);
  if (m) {
    if (!state.skills.some((s) => s.name === m[1])) return toast(`Bilinmeyen skill: /${m[1]}`, { kind: "err" });
    body = { skill: m[1], note: m[2].trim(), cwd, model };
  }
  const go = $("#prompt .go");
  go.disabled = true;
  try {
    const run = await post("/api/run", body);
    state.known.set(run.id, run.status);
    $("#prompt-text").value = "";
    autosize();
    toast(`Gönderildi · ${base(run.cwd)} · ${run.model}`, { kind: "ok" });
    loadRuns();
  } catch (err) {
    toast(err.status === 429 ? "İki iş zaten çalışıyor, biri bitsin." : "Gönderilemedi: " + err.message, { kind: "err" });
  } finally {
    go.disabled = false;
  }
}

/* ---------- runs ---------- */
function renderJobs() {
  const n = state.runs.filter((r) => r.status === "running").length;
  $("#jobs").hidden = !n;
  $("#jobs").textContent = `${n} iş çalışıyor`;
}
async function loadRuns() {
  clearTimeout(loadRuns.t);
  if (!document.hidden) {
    try {
      const runs = await api("/api/runs");
      for (const r of runs) {
        if (state.known.get(r.id) === "running" && r.status !== "running") {
          toast(r.status === "done" ? `Bitti · ${usd(r.cost || 0)} · ${r.turns} tur` : "İş hata verdi: " + runTitle(r).slice(0, 40),
            { kind: r.status === "done" ? "ok" : "err", onclick: () => openDrawer("run", r) });
        }
        state.known.set(r.id, r.status);
      }
      state.runs = runs;
      renderJobs();
      if (state.tab === "kayitlar") VIEWS.kayitlar();
    } catch { /* next poll */ }
  }
  loadRuns.t = setTimeout(loadRuns, state.runs.some((r) => r.status === "running") ? 3000 : 15000);
}

/* ---------- loading ---------- */
async function loadMain() {
  if (document.hidden) return;
  try {
    const [sessions, skills] = await Promise.all([api("/api/sessions?limit=30"), api("/api/skills")]);
    state.sessions = sessions;
    state.skills = skills;
    if (state.tab === "ana") { renderSessions(); renderSkills(); }
  } catch { /* next poll */ }
}
async function loadMemory() {
  if (document.hidden) return;
  try {
    state.memory = await api("/api/memory");
    if (state.tab === "ana") renderMemory();
  } catch { /* next poll */ }
}

ON_BOOT.push(async () => {
  $("#skill-search").addEventListener("input", renderSkills);
  $("#skills-all").addEventListener("click", () => openDrawer("skills"));
  $("#memory-all").addEventListener("click", () => openDrawer("memories"));
  $("#jobs").addEventListener("click", () => switchTab("kayitlar"));
  $("#prompt").addEventListener("submit", sendPrompt);
  $("#prompt-cwd").addEventListener("change", (e) => store.set("pano.cwd", e.target.value));
  $("#prompt-model").addEventListener("change", (e) => store.set("pano.model", e.target.value));
  const ta = $("#prompt-text");
  ta.addEventListener("input", () => { autosize(); updatePicker(); });
  ta.addEventListener("blur", () => setTimeout(() => ($("#picker").hidden = true), 150));
  ta.addEventListener("keydown", (e) => {
    const open = !$("#picker").hidden;
    if (open && (e.key === "ArrowDown" || e.key === "ArrowUp")) { e.preventDefault(); movePicker(e.key === "ArrowDown" ? 1 : -1); return; }
    if (open && (e.key === "Enter" || e.key === "Tab")) { e.preventDefault(); pick(picker.sel); return; }
    if (e.key === "Escape") { $("#picker").hidden = true; return; }
    if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); $("#prompt").requestSubmit(); }
  });
  document.addEventListener("keydown", (e) => {
    const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement?.tagName || "");
    if (e.key === "/" && !typing && !e.ctrlKey && !e.metaKey && !e.altKey) { e.preventDefault(); setPrompt("/"); }
  });
  document.addEventListener("visibilitychange", () => { if (!document.hidden) { loadMain(); loadRuns(); } });
  await Promise.all([loadMain(), loadMemory()]);
  fillSelects();
  loadRuns();
  loadOffice();
  setInterval(loadMain, 10_000);
  setInterval(loadMemory, 60_000);
  setInterval(() => !document.hidden && loadOffice(), 20_000);
});
```

- [ ] **Step 2: Check it in a browser**

```bash
go build -o pano . && ./pano serve -addr 127.0.0.1:7979 &
```

Open `http://127.0.0.1:7979/`. Expected:
- Oturumlar lists sessions with lamps; the current session is warm with minutes left; no row is cut in half.
- Skill'ler shows `superpowers:brainstorming` first with its real count, builtin rows like `anthropic-skills:pdf`; the search box filters; "tümü (n) →" opens the full list in the drawer.
- Hafıza lists CLAUDE.md / MEMORY.md files; clicking one opens its headings and links.
- Clicking a session opens the drawer; "Devam et" copies `cd '<cwd>' && claude --resume <id>`; Esc closes it.
- The centre shows "Ofis kapalı" with Başlat; pressing it shows the start script's output (e.g. "Packages are missing…") when agent-office-3d is not installed.
- Typing `/brai` in the prompt bar opens the picker; Enter picks `/superpowers:brainstorming `.
- `/` anywhere focuses the bar.

- [ ] **Step 3: One real run end to end** (costs about $0.01): pick folder `~`, model `haiku`, type `Bu klasörde kaç tane .md dosyası var? Sadece sayıyı yaz.` and press Enter. Expected: toast "Gönderildi", the top bar chip "1 iş çalışıyor", then a toast "Bitti · $0,0x · n tur"; clicking it opens the run with its output. Stop the server.

- [ ] **Step 4: Commit**

```bash
git add web/app.js
git commit -m "Ana tab: sessions, skills, memory, drawer, office card, prompt bar

The prompt bar starts background runs (free prompt or /skill note)
in the chosen folder and model; finished runs pop a toast.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: Kullanım and Kayıtlar tabs

**Files:**
- Modify: `web/app.js` (insert above the final `boot();`)

**Interfaces:**
- Consumes: `state.ov` (`Overview`: `days, demo, totals{cost, sessions, calls, prompts, cacheHit, subagentShare}, windows{fiveHour, sevenDay, fiveCost, sevenCost}, daily[{date, tokens{family: n}, cost}], tools[{name, count}], heatmap[7][24], projects[{name, count}]`), `loadUsage()`, `state.runs`, `state.runSel`, `runTitle`, `runDetail`, `fitList`, `svg`.
- Produces: `VIEWS.kullanim`, `VIEWS.kayitlar`.

- [ ] **Step 1: Insert the block** above `boot();`:

```js
/* ---------- Kullanım ---------- */
const FAM = { fable: "#c5a3ff", opus: "#e08a4f", sonnet: "#9fb4d8", haiku: "#8fd6b4", other: "#c9c6cf" };
const famOrder = ["fable", "opus", "sonnet", "haiku", "other"];
const DAYS = ["Pzt", "Sal", "Çar", "Per", "Cum", "Cmt", "Paz"];
function niceStep(x) { const p = 10 ** Math.floor(Math.log10(x || 1)); const m = x / p; return (m <= 1 ? 1 : m <= 2 ? 2 : m <= 5 ? 5 : 10) * p; }
function fmtDate(iso) { return new Date(iso + "T12:00:00").toLocaleDateString("tr-TR", { day: "numeric", month: "short" }); }

function renderDaily(days, fams) {
  const box = $("#daily");
  const W = Math.max(300, Math.round(box.clientWidth)), H = Math.max(120, Math.round(box.clientHeight)), padL = 44, padB = 22, padT = 8;
  const max = Math.max(1, ...days.map((d) => fams.reduce((a, f) => a + (d.tokens[f] || 0), 0)));
  const step = niceStep(max / 3), top = Math.ceil(max / step) * step;
  const s = svg("svg", { viewBox: `0 0 ${W} ${H}`, role: "img", "aria-label": `Günlük token kullanımı, en yüksek gün ${tok(max)} token` });
  for (let v = 0; v <= top; v += step) {
    const y = H - padB - (v / top) * (H - padB - padT);
    s.append(svg("line", { x1: padL, x2: W, y1: y, y2: y, stroke: "rgba(255,255,255,.1)", "stroke-width": 1 }));
    const tx = svg("text", { x: padL - 6, y: y + 4, "text-anchor": "end", class: "axis" });
    tx.textContent = tok(v);
    s.append(tx);
  }
  const bw = (W - padL) / days.length, every = Math.max(1, Math.ceil(days.length / 5));
  days.forEach((d, i) => {
    let y = H - padB;
    const x = padL + i * bw + bw * 0.16, w = bw * 0.68;
    const g = svg("g"), title = svg("title");
    const total = fams.reduce((a, f) => a + (d.tokens[f] || 0), 0);
    title.textContent = `${fmtDate(d.date)}: ${tok(total)} token, ${usd(d.cost)}`;
    g.append(title);
    for (const f of fams) {
      const v = d.tokens[f] || 0;
      if (!v) continue;
      const hgt = (v / top) * (H - padB - padT);
      y -= hgt;
      g.append(svg("rect", { x, y, width: w, height: Math.max(hgt, 0.5), fill: FAM[f], rx: 1.5 }));
    }
    s.append(g);
    if ((days.length - 1 - i) % every === 0) {
      const cx = x + w / 2, anchor = cx > W - 24 ? "end" : "middle";
      const tx = svg("text", { x: anchor === "end" ? W - 1 : cx, y: H - 6, "text-anchor": anchor, class: "axis" });
      tx.textContent = fmtDate(d.date);
      s.append(tx);
    }
  });
  box.replaceChildren(s);
}
function renderLamps(hm) {
  const max = Math.max(1, ...hm.flat());
  const kids = [h("span")];
  for (let hr = 0; hr < 24; hr++) kids.push(h("span", {}, hr % 6 === 0 ? String(hr).padStart(2, "0") : ""));
  hm.forEach((row, d) => {
    kids.push(h("span", {}, DAYS[d]));
    row.forEach((v, hr) => kids.push(h("span", { class: "cell" + (v ? " on" : ""), style: `--v:${(v / max).toFixed(3)}`, title: `${DAYS[d]} ${String(hr).padStart(2, "0")}:00, ${v} istek` })));
  });
  $("#lamps").replaceChildren(...kids);
}
function bars(sel, list) {
  const max = Math.max(1, ...list.map((x) => x.count));
  $(sel).replaceChildren(...(list.length
    ? list.map((x) => h("li", {}, h("span", { class: "t", title: x.name }, x.name), h("span", { class: "meter" }, h("i", { style: `width:${(x.count / max) * 100}%` })), h("span", { class: "n" }, nf.format(x.count))))
    : [h("li", { class: "empty" }, "Bu dönemde kayıt yok.")]));
  fitList($(sel));
}
VIEWS.kullanim = () => {
  const ov = state.ov;
  if (!ov) return;
  const t = ov.totals, w = ov.windows;
  const cells = [
    ["Son 5 saat", tok(w.fiveHour) + " tok", usd(w.fiveCost) + " API fiyatıyla"],
    ["Son 7 gün", tok(w.sevenDay) + " tok", usd(w.sevenCost) + " API fiyatıyla"],
    [`${ov.days} günlük maliyet`, usd(t.cost), "tahmini, faturan değil"],
    ["Önbellek isabeti", pct(t.cacheHit), "girdinin önbellekten okunan payı"],
    ["Oturum", nf.format(t.sessions), nf.format(t.prompts) + " prompt"],
    ["Alt ajan payı", pct(t.subagentShare), nf.format(t.calls) + " istek"],
  ];
  $("#sum").replaceChildren(...cells.map(([k, v, s]) => h("div", {}, h("dt", {}, k), h("dd", {}, v, h("small", {}, s)))));
  const fams = famOrder.filter((f) => ov.daily.some((d) => d.tokens[f]));
  const totalBy = Object.fromEntries(fams.map((f) => [f, ov.daily.reduce((a, d) => a + (d.tokens[f] || 0), 0)]));
  const all = Object.values(totalBy).reduce((a, b) => a + b, 0) || 1;
  $("#legend").replaceChildren(...fams.map((f) => h("li", {}, h("i", { style: `background:${FAM[f]}` }), `${f} ${pct(totalBy[f] / all)}`)));
  renderDaily(ov.daily, fams);
  renderLamps(ov.heatmap);
  bars("#tools", ov.tools);
  bars("#projects", ov.projects);
  $("#usage-note").textContent = `Son ${ov.days} gün. Tutarlar API liste fiyatıyla tahmin; abonelikte gerçek maliyetin bu değil.`;
  document.querySelectorAll("#days button").forEach((b) => b.setAttribute("aria-pressed", String(+b.dataset.days === state.days)));
};
ON_BOOT.push(() => {
  document.querySelectorAll("#days button").forEach((b) => b.addEventListener("click", () => {
    state.days = +b.dataset.days;
    loadUsage().catch(() => toast("Kullanım yüklenemedi", { kind: "err" }));
  }));
});

/* ---------- Kayıtlar ---------- */
VIEWS.kayitlar = () => {
  const ul = $("#runs");
  $("#runs-sub").textContent = `${state.runs.length} iş`;
  if (!state.runs.length) {
    ul.replaceChildren(h("li", { class: "empty" }, "Henüz iş yok. Ana sayfadaki kutudan bir iş ver."));
    $("#run-detail").replaceChildren(h("p", { class: "empty" }, "Soldan bir iş seç."));
    return;
  }
  if (!state.runs.some((r) => r.id === state.runSel)) state.runSel = state.runs[0].id;
  ul.replaceChildren(...state.runs.map((r) => h("li", {}, h("button", {
    type: "button", class: "run", "aria-current": String(r.id === state.runSel),
    onclick: () => { state.runSel = r.id; VIEWS.kayitlar(); },
  },
    h("span", { class: "lamp" + (r.status === "running" ? " running" : r.status === "error" ? " error" : "") }),
    h("span", { class: "t" }, runTitle(r)),
    h("span", { class: "c" }, usd(r.cost || 0)),
    h("span", { class: "m" }, `${base(r.cwd)} · ${r.model} · ${ago(sinceISO(r.started))} önce`)))));
  $("#run-detail").replaceChildren(...runDetail(state.runs.find((r) => r.id === state.runSel)).filter(Boolean));
};
```

- [ ] **Step 2: Check it in a browser**

```bash
go build -o pano . && ./pano serve -addr 127.0.0.1:7979 &
```

Expected on `http://127.0.0.1:7979/`: Kullanım shows six summary numbers, the daily chart (bars coloured per model, legend), the hour map, tools and projects; 7 / 30 / 90 gün switches; nothing scrolls. Kayıtlar lists the haiku run from Task 7 with its output on the right. Stop the server.

- [ ] **Step 3: Commit**

```bash
git add web/app.js
git commit -m "Kullanım and Kayıtlar tabs on the new page

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: Cleanup, README, checks and install

**Files:**
- Delete: `web/fonts/barlow-condensed-latin-600-normal.woff2`, `web/fonts/barlow-condensed-latin-700-normal.woff2`, `web/fonts/barlow-condensed-latin-ext-600-normal.woff2`, `web/fonts/barlow-condensed-latin-ext-700-normal.woff2`
- Modify: `README.md` (intro list, Licenses)
- Modify: `launch.go` / `main.go` comments only if they still say "breaker" or "rating plate" (`grep -n -i 'breaker\|rating plate\|sigorta' *.go`)

- [ ] **Step 1: Drop the condensed font**

```bash
git rm web/fonts/barlow-condensed-*.woff2
grep -rn "Barlow Condensed" web/ && echo "still referenced" || echo "ok"
```

Expected: `ok`.

- [ ] **Step 2: README** — replace everything from the second line of the file up to (not including) `Nothing leaves your computer.` with:

```markdown
A local dashboard for Claude Code. It reads what Claude Code already keeps in `~/.claude` and shows
it on one screen at http://127.0.0.1:7878, without scrolling, over a sky that follows your clock:

- **Sessions (Oturumlar):** recent sessions with a warm / cold prompt-cache lamp, the context the
  next message re-sends and what it costs warm or cold; "Devam et" copies the resume command.
- **Skills (Skill'ler):** installed skills, slash commands and plugin skills by use, plus builtin
  skills Claude called; size, description and path in the drawer.
- **Memory (Hafıza):** every `CLAUDE.md` / `AGENTS.md` / `MEMORY.md` Claude loads and the files they
  point to; a red dot marks a broken link.
- **Live office:** [agent-office-3d](https://github.com/ozyusuf/agent-office-3d) in the middle when it
  runs on 127.0.0.1:7847, with a button that starts it when it does not.
- **Prompt bar:** give Claude a job in the background (`claude -p`) in a folder and model you pick;
  `/` picks a skill. Runs use permission mode `auto` by default.
- **Usage (Kullanım):** tokens and cost at API list prices for 7 / 30 / 90 days, daily tokens per
  model, an hour-of-week map, top tools and projects.
- **Runs (Kayıtlar):** background runs with status, cost and output. Reports are saved to
  `~/.local/share/pano/runs/`.

```

and in **Licenses** replace `Fonts: Barlow and Barlow Condensed,` with `Font: Barlow,`.

- [ ] **Step 3: Full test run and demo mode**

```bash
gofmt -l . ; go vet ./... && go test -count=1 ./...
go build -o pano . && ./pano serve -demo -addr 127.0.0.1:7980 &
```

Expected: tests PASS; `http://127.0.0.1:7980/` shows the DEMO badge and synthetic sessions, skills, memory and usage. Stop it.

- [ ] **Step 4: No-scroll and time-of-day checks** (`./pano serve -addr 127.0.0.1:7979 &`)

In the browser, at window sizes 1280×720 and 1920×1080, on each tab run in the console
`document.documentElement.scrollHeight <= innerHeight && document.body.scrollHeight <= innerHeight` → `true`.
Take screenshots of `?saat=6.5`, `?saat=12`, `?saat=18.25`, `?saat=23` on the Ana tab: dawn colours with the sun low on the left; noon blue with the sun high; sunset orange with the sun low on the right; night with moon and stars. Panels stay readable in all four. Stop the server.

- [ ] **Step 5: Commit**

```bash
git add -A README.md web/fonts
git commit -m "Drop the condensed font, describe the new page in the README

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

- [ ] **Step 6: Install and restart the user's pano**

```bash
~/.local/bin/pano stop
go build -o ~/.local/bin/pano .
pano
```

Expected: `pano started on http://127.0.0.1:7878`, an app window opens with the new page.
