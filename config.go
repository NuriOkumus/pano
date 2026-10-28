package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Price is USD per million tokens. Cache writes and reads are derived from
// Input unless set explicitly.
type Price struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheWrite float64 `json:"cache_write,omitempty"`
	CacheRead  float64 `json:"cache_read,omitempty"`
}

func (p Price) write() float64 {
	if p.CacheWrite > 0 {
		return p.CacheWrite
	}
	return p.Input * 1.25
}

func (p Price) read() float64 {
	if p.CacheRead > 0 {
		return p.CacheRead
	}
	return p.Input * 0.1
}

// Config lives at ~/.config/pano/config.json. Every field is optional.
type Config struct {
	// Prices per model family (opus, sonnet, haiku, fable, other). These are
	// estimates for the dashboard only; your bill or plan is the truth.
	Prices map[string]Price `json:"prices"`
	// Prompt cache lifetime in minutes, used for the warm/cold badge.
	CacheTTLMinutes int `json:"cache_ttl_minutes"`
	// Run panel: the claude binary and extra args appended to every run,
	// e.g. ["--permission-mode", "acceptEdits"]. They come after the permission
	// mode, so a --permission-mode given here wins over permission_mode.
	ClaudeBin  string   `json:"claude_bin"`
	ExtraArgs  []string `json:"extra_args"`
	Models     []string `json:"models"`
	MaxRunMins int      `json:"max_run_minutes"`
	// claude --permission-mode for background runs. auto lets Claude decide
	// per action, like an interactive session in auto mode.
	PermissionMode string `json:"permission_mode"`
	// agent-office-3d folder that `pano` starts and `pano stop` stops. Empty:
	// ~/agent-office-3d or ~/Projects/agent-office-3d, whichever exists.
	OfficeDir string `json:"office_dir"`
}

func defaultConfig() Config {
	return Config{
		Prices: map[string]Price{
			"fable":  {Input: 15, Output: 75},
			"opus":   {Input: 5, Output: 25},
			"sonnet": {Input: 3, Output: 15},
			"haiku":  {Input: 1, Output: 5},
			"other":  {Input: 3, Output: 15},
		},
		CacheTTLMinutes: 60,
		ClaudeBin:       "claude",
		Models:          []string{"haiku", "sonnet", "opus"},
		MaxRunMins:      30,
		PermissionMode:  "auto",
	}
}

func loadConfig(path string) Config {
	cfg := defaultConfig()
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	var user Config
	if json.Unmarshal(b, &user) != nil {
		return cfg
	}
	for k, v := range user.Prices {
		cfg.Prices[strings.ToLower(k)] = v
	}
	if user.CacheTTLMinutes > 0 {
		cfg.CacheTTLMinutes = user.CacheTTLMinutes
	}
	if user.ClaudeBin != "" {
		cfg.ClaudeBin = user.ClaudeBin
	}
	if user.ExtraArgs != nil {
		cfg.ExtraArgs = user.ExtraArgs
	}
	if len(user.Models) > 0 {
		cfg.Models = user.Models
	}
	if user.MaxRunMins > 0 {
		cfg.MaxRunMins = user.MaxRunMins
	}
	if permissionModes[user.PermissionMode] {
		cfg.PermissionMode = user.PermissionMode
	}
	cfg.OfficeDir = user.OfficeDir
	return cfg
}

// The modes claude accepts (claude --help, v2.1.296).
var permissionModes = map[string]bool{"acceptEdits": true, "auto": true, "bypassPermissions": true, "manual": true, "dontAsk": true, "plan": true}

func (c Config) price(family string) Price {
	if p, ok := c.Prices[family]; ok {
		return p
	}
	return c.Prices["other"]
}

// family maps a full model id like "claude-opus-5-5" to a price row.
func family(model string) string {
	m := strings.ToLower(model)
	for _, f := range []string{"fable", "mythos", "opus", "sonnet", "haiku"} {
		if strings.Contains(m, f) {
			if f == "mythos" {
				return "fable"
			}
			return f
		}
	}
	return "other"
}

func configDir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "pano")
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".config", "pano")
}

func dataDir() string { return filepath.Join(dataHome(), "pano") }

func dataHome() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return d
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".local", "share")
}
