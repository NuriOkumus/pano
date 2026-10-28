package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A transcript in the shape Claude Code writes: one JSON object per line,
// assistant usage repeated on every content block of the same request.
const transcript = `{"type":"summary","summary":"Fix ArgoCD sync","leafUuid":"x"}
{"type":"user","sessionId":"s1","cwd":"/work/se-cluster","timestamp":"2026-10-28T10:00:00.000Z","isSidechain":false,"message":{"role":"user","content":"why is overleaf out of sync?"}}
{"type":"assistant","sessionId":"s1","cwd":"/work/se-cluster","timestamp":"2026-10-28T10:00:05.000Z","isSidechain":false,"requestId":"req_1","message":{"id":"msg_1","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"Looking."}],"usage":{"input_tokens":10,"output_tokens":50,"cache_creation_input_tokens":20000,"cache_read_input_tokens":0}}}
{"type":"assistant","sessionId":"s1","cwd":"/work/se-cluster","timestamp":"2026-10-28T10:00:05.100Z","isSidechain":false,"requestId":"req_1","message":{"id":"msg_1","role":"assistant","model":"claude-opus-5-5","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"kubectl get app"}}],"usage":{"input_tokens":10,"output_tokens":50,"cache_creation_input_tokens":20000,"cache_read_input_tokens":0}}}
{"type":"user","sessionId":"s1","cwd":"/work/se-cluster","timestamp":"2026-10-28T10:00:09.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}
{"type":"assistant","sessionId":"s1","cwd":"/work/se-cluster","timestamp":"2026-10-28T10:00:12.000Z","isSidechain":false,"requestId":"req_2","message":{"id":"msg_2","role":"assistant","model":"claude-opus-5-5","content":[{"type":"tool_use","id":"t2","name":"Skill","input":{"skill":"k8s-health"}}],"usage":{"input_tokens":3,"output_tokens":120,"cache_creation_input_tokens":500,"cache_read_input_tokens":20060}}}
{"type":"user","sessionId":"s1","cwd":"/work/se-cluster","timestamp":"2026-10-28T10:01:00.000Z","isSidechain":false,"message":{"role":"user","content":"<command-message>calibrate</command-message>\n<command-name>/calibrate</command-name>"}}
{"type":"assistant","sessionId":"s1","cwd":"/work/se-cluster","timestamp":"2026-10-28T10:01:30.000Z","isSidechain":true,"requestId":"req_3","message":{"id":"msg_3","role":"assistant","model":"claude-haiku-5-5","content":[{"type":"text","text":"sub"}],"usage":{"input_tokens":5,"output_tokens":40,"cache_creation_input_tokens":3000,"cache_read_input_tokens":0}}}
{"type":"assistant","sessionId":"s1","timestamp":"2026-10-28T10:01:31.000Z","requestId":"req_x","message":{"id":"msg_x","role":"assistant","model":"<synthetic>","content":[],"usage":{"input_tokens":0,"output_tokens":0}}}
this line is not json
`

func writeClaudeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	proj := filepath.Join(dir, "projects", "-work-se-cluster")
	must(t, os.MkdirAll(proj, 0o755))
	must(t, os.WriteFile(filepath.Join(proj, "s1.jsonl"), []byte(transcript), 0o644))
	// A resumed session that copied the first request into a new file.
	copied := strings.Split(transcript, "\n")[2] + "\n"
	must(t, os.WriteFile(filepath.Join(proj, "s1-resumed.jsonl"), []byte(copied), 0o644))

	sk := filepath.Join(dir, "skills", "k8s-health")
	must(t, os.MkdirAll(sk, 0o755))
	must(t, os.WriteFile(filepath.Join(sk, "SKILL.md"), []byte("---\nname: k8s-health\ndescription: >\n  Checks the cluster\n  and reports only problems.\nmodel: sonnet\n---\n# body\n"+strings.Repeat("x", 4000)), 0o644))
	must(t, os.WriteFile(filepath.Join(sk, "notes.md"), []byte(strings.Repeat("y", 800)), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, "commands"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "commands", "calibrate.md"), []byte("---\ndescription: \"Tune up\"\n---\nbody"), 0o644))
	plug := filepath.Join(dir, "plugins", "cache", "m", "tools", "1.0.0")
	must(t, os.MkdirAll(filepath.Join(plug, "skills", "lint"), 0o755))
	must(t, os.WriteFile(filepath.Join(plug, "skills", "lint", "SKILL.md"), []byte("---\nname: lint\n---\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "plugins", "installed_plugins.json"),
		[]byte(`{"version":2,"plugins":{"tools@m":[{"scope":"user","installPath":`+strconv.Quote(plug)+`}]}}`), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, "notes"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "notes", "infra.md"), []byte("# Infra\nline\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Who I am\nSee @notes/infra.md and [school](notes/school.md).\n```\n@ignored/in/code.md\n```\n"), 0o644))
	return dir
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseAndAggregate(t *testing.T) {
	dir := writeClaudeDir(t)
	files := NewStore(dir).Refresh()
	if len(files) != 2 {
		t.Fatalf("want 2 transcripts, got %d", len(files))
	}
	fd := files[filepath.Join(dir, "projects", "-work-se-cluster", "s1.jsonl")]
	if fd.Title != "Fix ArgoCD sync" {
		t.Errorf("title from summary: %q", fd.Title)
	}
	if len(fd.Requests) != 3 {
		t.Errorf("duplicated content blocks and <synthetic> must not count: %d requests", len(fd.Requests))
	}
	if fd.UserMsgs != 2 {
		t.Errorf("tool results are not prompts: %d", fd.UserMsgs)
	}
	if fd.Skills["k8s-health"].Count != 1 || fd.Skills["calibrate"].Count != 1 {
		t.Errorf("skills: %+v", fd.Skills)
	}
	if fd.Tools["Bash"] != 1 || fd.Tools["Skill"] != 1 {
		t.Errorf("tools: %+v", fd.Tools)
	}
	if want := int64(3 + 500 + 20060 + 120); fd.LastCtx != want {
		t.Errorf("context of last main request: got %d want %d", fd.LastCtx, want)
	}

	cfg := defaultConfig()
	now := time.Date(2026, 10, 28, 10, 30, 0, 0, time.UTC)
	ov := buildOverview(files, cfg, 7, now)
	if ov.Totals.Calls != 3 {
		t.Errorf("the resumed copy must be deduped across files: %d calls", ov.Totals.Calls)
	}
	if ov.Totals.CacheRead != 20060 || ov.Totals.CacheWrite != 23500 {
		t.Errorf("cache totals: %+v", ov.Totals)
	}
	if ov.Totals.Sessions != 1 {
		t.Errorf("sessions: %d", ov.Totals.Sessions)
	}
	if ov.Totals.Subagent <= 0 || ov.Totals.Subagent >= 0.2 {
		t.Errorf("subagent share: %v", ov.Totals.Subagent)
	}
	if ov.Windows.FiveHour == 0 || ov.Windows.SevenDay != ov.Windows.FiveHour {
		t.Errorf("windows: %+v", ov.Windows)
	}
	wantCost := (10*5+50*25+20000*6.25)/1e6 + (3*5+120*25+500*6.25+20060*0.5)/1e6 + (5*1+40*5+3000*1.25)/1e6
	if d := ov.Totals.Cost - wantCost; d > 1e-9 || d < -1e-9 {
		t.Errorf("cost: got %v want %v", ov.Totals.Cost, wantCost)
	}

	ss := buildSessions(files, cfg, 10, now)
	if len(ss) != 1 || ss[0].Cache != "warm" || ss[0].Project != "se-cluster" || ss[0].Family != "opus" {
		t.Fatalf("sessions: %+v", ss)
	}
	later := buildSessions(files, cfg, 10, now.Add(2*time.Hour))
	if later[0].Cache != "cold" || later[0].NextCold <= later[0].NextWarm {
		t.Errorf("cold after the TTL, and a cold reload costs more: %+v", later[0])
	}
}

func TestSkillsAndMemory(t *testing.T) {
	dir := writeClaudeDir(t)
	files := NewStore(dir).Refresh()
	skills := scanSkills(dir, nil, skillUsage(files))
	byName := map[string]Skill{}
	for _, s := range skills {
		byName[s.Name] = s
	}
	k := byName["k8s-health"]
	if k.Description != "Checks the cluster and reports only problems." || k.Model != "sonnet" || k.Uses != 1 || k.Files != 2 {
		t.Errorf("k8s-health: %+v", k)
	}
	if k.DirTokens <= k.Tokens {
		t.Errorf("folder tokens include notes.md: %+v", k)
	}
	if c := byName["calibrate"]; c.Kind != "command" || c.Description != "Tune up" || c.Uses != 1 {
		t.Errorf("command: %+v", c)
	}
	if _, ok := byName["tools:lint"]; !ok {
		t.Errorf("plugin skill missing: %v", skills)
	}
	if skills[0].Name != "k8s-health" {
		t.Errorf("sorted by size: %s first", skills[0].Name)
	}

	mem := scanMemory(dir, nil)
	if len(mem) != 1 || len(mem[0].Refs) != 2 {
		t.Fatalf("memory: %+v", mem)
	}
	if mem[0].Refs[0].Missing || !mem[0].Refs[1].Missing {
		t.Errorf("infra.md exists, school.md does not: %+v", mem[0].Refs)
	}
	if mem[0].Headings[0] != "Who I am" {
		t.Errorf("headings: %v", mem[0].Headings)
	}
}

func TestRunValidation(t *testing.T) {
	r := &Runner{runs: map[string]*Run{}, cfg: defaultConfig(), demo: true, slot: make(chan struct{}, 2)}
	skills := []Skill{{Name: "k8s-health"}}
	cases := []RunRequest{
		{Skill: "rm -rf", Model: "haiku"},
		{Skill: "unknown", Model: "haiku"},
		{Skill: "k8s-health", Model: "gpt"},
		{Skill: "k8s-health", Model: "haiku", Cwd: "/etc"},
		{Skill: "k8s-health", Model: "haiku", Note: strings.Repeat("a", 2001)},
	}
	for _, c := range cases {
		if _, err := r.Start(c, skills, []string{"/work/se-cluster"}); err == nil {
			t.Errorf("should refuse %+v", c)
		}
	}
	if _, err := r.Start(RunRequest{Skill: "k8s-health", Model: "haiku", Cwd: "/work/se-cluster"}, skills, []string{"/work/se-cluster"}); err != nil {
		t.Errorf("valid run refused: %v", err)
	}
}

func TestFamily(t *testing.T) {
	for in, want := range map[string]string{"claude-opus-5-5": "opus", "claude-sonnet-5-5": "sonnet", "claude-haiku-5-5": "haiku", "claude-mythos-5-1": "fable", "claude-fable-5-1": "fable", "x": "other"} {
		if got := family(in); got != want {
			t.Errorf("family(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanTitle(t *testing.T) {
	cases := map[string]string{
		"<system-reminder>The user's timezone is X.</system-reminder>\nfix the cluster":         "fix the cluster",
		"<command-message>calibrate</command-message>\n<command-name>/calibrate</command-name>": "/calibrate",
		"Caveat: The messages below were generated by the user while running local commands.":   "",
		"plain   prompt\n\nwith  spaces": "plain prompt with spaces",
	}
	for in, want := range cases {
		if got := cleanTitle(in); got != want {
			t.Errorf("cleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
}
