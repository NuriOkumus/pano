package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
