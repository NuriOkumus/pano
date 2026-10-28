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

	usage := map[string]stamp{"superpowers:brainstorming": {Count: 10, Last: time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC), Tool: true}}
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
