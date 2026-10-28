package main

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Skill struct {
	Name        string    `json:"name"`
	Kind        string    `json:"kind"`  // skill | command
	Scope       string    `json:"scope"` // user | project | plugin
	Owner       string    `json:"owner"` // project or plugin name
	Path        string    `json:"path"`
	Description string    `json:"description"`
	Model       string    `json:"model"`
	Tokens      int64     `json:"tokens"`    // the SKILL.md itself, loaded on use
	DirTokens   int64     `json:"dirTokens"` // text files next to it, read on demand
	Files       int       `json:"files"`
	Uses        int       `json:"uses"`
	LastUsed    time.Time `json:"lastUsed"`
}

var skillNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,80}$`)

func scanSkills(claudeDir string, projects []string, usage map[string]stamp) []Skill {
	var out []Skill
	seen := map[string]bool{}
	names := map[string]bool{}
	add := func(s Skill) {
		key := s.Scope + "|" + s.Owner + "|" + s.Name
		real := s.Path
		if r, err := filepath.EvalSymlinks(s.Path); err == nil {
			real = r
		}
		// A project opened in ~ sees ~/.claude/skills again; count each file once.
		if seen[key] || seen["path|"+real] || !skillNameRe.MatchString(s.Name) {
			return
		}
		seen[key], seen["path|"+real] = true, true
		if u, ok := usage[s.Name]; ok {
			s.Uses, s.LastUsed = u.Count, u.Last
		}
		names[s.Name] = true
		out = append(out, s)
	}

	for _, s := range skillsIn(filepath.Join(claudeDir, "skills"), "user", "") {
		add(s)
	}
	for _, s := range commandsIn(filepath.Join(claudeDir, "commands"), "user", "") {
		add(s)
	}
	for i, p := range projects {
		if i >= 25 {
			break
		}
		owner := filepath.Base(p)
		for _, s := range skillsIn(filepath.Join(p, ".claude", "skills"), "project", owner) {
			add(s)
		}
		for _, s := range commandsIn(filepath.Join(p, ".claude", "commands"), "project", owner) {
			add(s)
		}
	}
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

	sort.Slice(out, func(i, j int) bool {
		if out[i].Tokens == out[j].Tokens {
			return out[i].Name < out[j].Name
		}
		return out[i].Tokens > out[j].Tokens
	})
	return out
}

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

func skillsIn(dir, scope, owner string) []Skill {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		md := filepath.Join(dir, e.Name(), "SKILL.md")
		info, err := os.Stat(md)
		if err != nil {
			continue
		}
		fm := frontmatter(md)
		name := fm["name"]
		if name == "" {
			name = e.Name()
		}
		files, text := dirText(filepath.Join(dir, e.Name()))
		out = append(out, Skill{
			Name: name, Kind: "skill", Scope: scope, Owner: owner, Path: md,
			Description: clip(fm["description"], 220), Model: fm["model"],
			Tokens: info.Size() / 4, DirTokens: text / 4, Files: files,
		})
	}
	return out
}

func commandsIn(dir, scope, owner string) []Skill {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Skill
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		fm := frontmatter(p)
		out = append(out, Skill{
			Name: strings.TrimSuffix(e.Name(), ".md"), Kind: "command", Scope: scope, Owner: owner, Path: p,
			Description: clip(fm["description"], 220), Model: fm["model"],
			Tokens: info.Size() / 4, DirTokens: info.Size() / 4, Files: 1,
		})
	}
	return out
}

// dirText counts files and the bytes of text-like files in a skill folder.
func dirText(dir string) (int, int64) {
	files, total := 0, int64(0)
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		files++
		switch strings.ToLower(filepath.Ext(p)) {
		case ".md", ".txt", ".py", ".js", ".ts", ".sh", ".json", ".yaml", ".yml", ".html", ".css", ".go", ".csv", ".xml", "":
			if info, err := d.Info(); err == nil && info.Size() < 4<<20 {
				total += info.Size()
			}
		}
		return nil
	})
	return files, total
}

// frontmatter reads simple YAML key: value pairs between leading --- lines,
// including folded (>) and literal (|) multi-line values.
func frontmatter(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return out
	}
	key := ""
	for sc.Scan() {
		ln := sc.Text()
		if strings.TrimSpace(ln) == "---" {
			break
		}
		if key != "" && (strings.HasPrefix(ln, " ") || strings.HasPrefix(ln, "\t")) {
			out[key] = strings.TrimSpace(out[key] + " " + strings.TrimSpace(ln))
			continue
		}
		key = ""
		k, v, ok := strings.Cut(ln, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if v == ">" || v == "|" || v == ">-" || v == "|-" {
			v = ""
		}
		v = strings.Trim(v, `"'`)
		out[k] = v
		key = k
	}
	return out
}
