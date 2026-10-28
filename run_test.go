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
