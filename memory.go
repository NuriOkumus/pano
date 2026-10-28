package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The memory map: every CLAUDE.md / AGENTS.md / MEMORY.md Claude loads at the
// start of a chat, and the files they point to. This is the "plain-text map"
// from the ARMS guide; big roots or dead links show up at a glance.

type MemFile struct {
	Path     string    `json:"path"`
	Label    string    `json:"label"`
	Group    string    `json:"group"` // user | project name
	Kind     string    `json:"kind"`  // claude | agents | memory | local | ref
	Tokens   int64     `json:"tokens"`
	Lines    int       `json:"lines"`
	Headings []string  `json:"headings"`
	Refs     []MemFile `json:"refs,omitempty"`
	Missing  bool      `json:"missing,omitempty"`
}

var (
	importRe = regexp.MustCompile(`(?:^|\s)@([~./A-Za-z0-9_-][^\s` + "`" + `)]*)`)
	mdLinkRe = regexp.MustCompile(`\]\(([^)\s#]+\.md)\)`)
)

func scanMemory(claudeDir string, projects []string) []MemFile {
	var out []MemFile
	home, _ := os.UserHomeDir()
	for _, c := range []struct{ p, kind string }{
		{filepath.Join(claudeDir, "CLAUDE.md"), "claude"},
		{filepath.Join(claudeDir, "AGENTS.md"), "agents"},
	} {
		if m, ok := memFile(c.p, "user", c.kind, home, true); ok {
			out = append(out, m)
		}
	}
	for i, p := range projects {
		if i >= 12 {
			break
		}
		group := filepath.Base(p)
		for _, c := range []struct{ rel, kind string }{
			{"CLAUDE.md", "claude"}, {".claude/CLAUDE.md", "claude"},
			{"CLAUDE.local.md", "local"}, {"AGENTS.md", "agents"},
		} {
			if m, ok := memFile(filepath.Join(p, c.rel), group, c.kind, home, true); ok {
				out = append(out, m)
			}
		}
	}
	// Auto memory that Claude Code keeps per project, when present.
	matches, _ := filepath.Glob(filepath.Join(claudeDir, "projects", "*", "memory", "MEMORY.md"))
	for _, m := range matches {
		group := projectLabel(decodeProjectDir(filepath.Base(filepath.Dir(filepath.Dir(m)))))
		if f, ok := memFile(m, group, "memory", home, true); ok {
			out = append(out, f)
		}
	}
	return out
}

func memFile(path, group, kind, home string, follow bool) (MemFile, bool) {
	f, err := os.Open(path)
	if err != nil {
		return MemFile{}, false
	}
	defer f.Close()
	m := MemFile{Path: path, Label: shortPath(path, home), Group: group, Kind: kind}
	var size int64
	refs := []string{}
	refSeen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 2<<20)
	inCode := false
	for sc.Scan() {
		ln := sc.Text()
		size += int64(len(ln)) + 1
		m.Lines++
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}
		if strings.HasPrefix(t, "#") && len(m.Headings) < 8 {
			h := strings.TrimSpace(strings.TrimLeft(t, "#"))
			if h != "" {
				m.Headings = append(m.Headings, clip(h, 60))
			}
		}
		if !follow {
			continue
		}
		for _, mm := range importRe.FindAllStringSubmatch(ln, -1) {
			r := strings.TrimRight(mm[1], ".,;:")
			if strings.Contains(r, "/") || strings.HasSuffix(r, ".md") {
				if !refSeen[r] {
					refSeen[r] = true
					refs = append(refs, r)
				}
			}
		}
		for _, mm := range mdLinkRe.FindAllStringSubmatch(ln, -1) {
			if strings.Contains(mm[1], "://") || refSeen[mm[1]] {
				continue
			}
			refSeen[mm[1]] = true
			refs = append(refs, mm[1])
		}
	}
	m.Tokens = size / 4
	base := filepath.Dir(path)
	for i, r := range refs {
		if i >= 24 {
			break
		}
		target := r
		if strings.HasPrefix(target, "~/") {
			target = filepath.Join(home, target[2:])
		} else if !filepath.IsAbs(target) {
			target = filepath.Join(base, target)
		}
		if child, ok := memFile(target, group, "ref", home, false); ok {
			m.Refs = append(m.Refs, child)
		} else {
			m.Refs = append(m.Refs, MemFile{Path: target, Label: shortPath(target, home), Group: group, Kind: "ref", Missing: true})
		}
	}
	return m, true
}

func shortPath(p, home string) string {
	if home != "" && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~/" + strings.TrimPrefix(p, home+string(filepath.Separator))
	}
	return p
}
