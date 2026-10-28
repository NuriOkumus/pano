package main

import (
	"fmt"
	"math/rand"
	"time"
)

// Demo mode: synthetic transcripts that run through the real aggregation code,
// so the dashboard can be previewed (and screenshotted) without any real data.

type demoProject struct {
	name   string
	titles []string
}

var demoProjects = []demoProject{
	{"se-cluster", []string{"ArgoCD out of sync on overleaf", "Traefik cert renewal check", "Mongo replica set lag", "Zot registry GC", "node w3 NotReady triage"}},
	{"pano", []string{"skills panel token sizes", "cache warm/cold badge", "run button via claude -p"}},
	{"bittorrent-client", []string{"piece picker rarest-first", "topology observer websocket", "DHT bootstrap timeout"}},
	{"tubitak-1002a", []string{"ALTO peer selection section", "gossip learning literature table", "budget justification"}},
	{"flmanager", []string{"round scheduler refactor", "client heartbeat metrics"}},
	{"sayzek-tabya", []string{"ONOS intent API", "Mininet-WiFi topology script"}},
}

var demoTools = map[string]int{"Read": 9, "Bash": 7, "Edit": 6, "Grep": 4, "Glob": 2, "Write": 2, "WebSearch": 1, "Task": 1, "Skill": 1}

func demoFiles(now time.Time) map[string]*fileData {
	rng := rand.New(rand.NewSource(42))
	files := map[string]*fileData{}
	models := []string{"claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5"}
	pick := func() string {
		x := rng.Float64()
		switch {
		case x < 0.46:
			return models[0]
		case x < 0.82:
			return models[1]
		default:
			return models[2]
		}
	}
	n := 0
	mk := func(start time.Time, steps int, sideChance float64) {
		p := demoProjects[rng.Intn(len(demoProjects))]
		n++
		fd := &fileData{
			SessionID: fmt.Sprintf("demo-%04d", n),
			Cwd:       "/home/nuri/code/" + p.name,
			Title:     p.titles[rng.Intn(len(p.titles))],
			Skills:    map[string]*stamp{}, Tools: map[string]int{},
		}
		ctx := int64(14000 + rng.Intn(9000))
		ts := start
		model := pick()
		for i := 0; i < steps; i++ {
			gap := time.Duration(20+rng.Intn(140)) * time.Second
			if rng.Float64() < 0.08 {
				gap += time.Duration(5+rng.Intn(50)) * time.Minute
			}
			ts = ts.Add(gap)
			if ts.After(now) {
				break
			}
			side := rng.Float64() < sideChance
			m := model
			if side {
				m = models[2]
			}
			cw := int64(800 + rng.Intn(5000))
			if i == 0 {
				cw = ctx
			}
			u := Usage{In: int64(1 + rng.Intn(9)), Out: int64(150 + rng.Intn(2600)), CW: cw, CR: ctx}
			fd.Requests = append(fd.Requests, request{Key: fmt.Sprintf("%s-%d", fd.SessionID, i), TS: ts, Model: m, U: u, Sidechain: side})
			if !side {
				fd.LastMain, fd.LastCtx, fd.LastModel = ts, u.context()+u.Out, m
				ctx += cw + u.Out
				if ctx > 420000 {
					ctx = 60000 // compacted
				}
			}
			if i%3 == 0 {
				fd.UserMsgs++
			}
			for t, w := range demoTools {
				if rng.Intn(12) < w {
					fd.Tools[t]++
				}
			}
		}
		if len(fd.Requests) == 0 {
			return
		}
		fd.First, fd.Last = start, ts
		for name, w := range map[string]int{"k8s-health": 5, "calibrate": 3, "commit-msg": 6, "grammar": 2, "argocd-sync": 3, "gossip-sim": 1} {
			if rng.Intn(10) < w {
				s := &stamp{Count: 1 + rng.Intn(3), Last: ts}
				fd.Skills[name] = s
			}
		}
		files[fd.SessionID] = fd
	}

	for d := 29; d >= 0; d-- {
		day := startOfDay(now).AddDate(0, 0, -d)
		weekend := day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
		count := 2 + rng.Intn(5)
		if weekend {
			count = rng.Intn(3)
		}
		if d < 10 {
			count += 2 // ramp-up: recent weeks are busier
		}
		for i := 0; i < count; i++ {
			hour := []int{9, 10, 11, 13, 14, 15, 16, 20, 21, 22, 23, 0}[rng.Intn(12)]
			start := day.Add(time.Duration(hour)*time.Hour + time.Duration(rng.Intn(50))*time.Minute)
			if start.After(now.Add(-2 * time.Hour)) {
				continue
			}
			mk(start, 12+rng.Intn(70), 0.12)
		}
	}
	// Two live sessions with a warm cache and one that went cold this afternoon.
	mk(now.Add(-52*time.Minute), 40, 0.2)
	mk(now.Add(-35*time.Minute), 18, 0.05)
	mk(now.Add(-4*time.Hour), 30, 0.1)
	return files
}

func demoSkills(usage map[string]stamp) []Skill {
	list := []Skill{
		{Name: "k8s-health", Kind: "skill", Scope: "user", Description: "Checks node, pod and ArgoCD sync health on the department K3s cluster and only reports what is broken.", Model: "sonnet", Tokens: 1840, DirTokens: 5200, Files: 4},
		{Name: "argocd-sync", Kind: "skill", Scope: "project", Owner: "se-cluster", Description: "Diffs live vs desired state for an ArgoCD app and proposes the smallest fix.", Tokens: 1320, DirTokens: 2100, Files: 2},
		{Name: "calibrate", Kind: "skill", Scope: "user", Description: "In-session self-improvement. Reviews the conversation for corrections and suggests skill or memory updates.", Tokens: 903, DirTokens: 903, Files: 1},
		{Name: "commit-msg", Kind: "command", Scope: "user", Description: "Writes a conventional commit message from the staged diff.", Model: "haiku", Tokens: 210, DirTokens: 210, Files: 1},
		{Name: "grammar", Kind: "skill", Scope: "user", Description: "Spelling and grammar pass only. No rewrites.", Model: "haiku", Tokens: 380, DirTokens: 380, Files: 1},
		{Name: "gossip-sim", Kind: "skill", Scope: "project", Owner: "tubitak-1002a", Description: "Runs the gossip learning simulator with ALTO peer selection and plots convergence.", Tokens: 2960, DirTokens: 11800, Files: 7},
		{Name: "tubitak-1002a-writer", Kind: "skill", Scope: "plugin", Owner: "tubitak", Description: "TÜBİTAK 1002-A proposal writer: hypothesis, rules check, academic Turkish.", Tokens: 4120, DirTokens: 9300, Files: 5},
		{Name: "release-notes", Kind: "command", Scope: "project", Owner: "bittorrent-client", Description: "Drafts release notes from merged PRs since the last tag.", Tokens: 290, DirTokens: 290, Files: 1},
	}
	for i := range list {
		if u, ok := usage[list[i].Name]; ok {
			list[i].Uses, list[i].LastUsed = u.Count, u.Last
		}
		list[i].Path = "~/.claude/skills/" + list[i].Name + "/SKILL.md"
	}
	return list
}

func demoMemory() []MemFile {
	return []MemFile{
		{Label: "~/.claude/CLAUDE.md", Group: "user", Kind: "claude", Tokens: 640, Lines: 48,
			Headings: []string{"Who I am", "How I work", "Map of my work"},
			Refs: []MemFile{
				{Label: "~/notes/infra.md", Kind: "ref", Tokens: 1210, Lines: 96},
				{Label: "~/notes/research.md", Kind: "ref", Tokens: 880, Lines: 71},
				{Label: "~/notes/school.md", Kind: "ref", Missing: true},
			}},
		{Label: "~/code/se-cluster/CLAUDE.md", Group: "se-cluster", Kind: "claude", Tokens: 1980, Lines: 151,
			Headings: []string{"Cluster layout", "GitOps rules", "Never do"},
			Refs:     []MemFile{{Label: "docs/runbook.md", Kind: "ref", Tokens: 3400, Lines: 260}, {Label: "docs/network.md", Kind: "ref", Tokens: 900, Lines: 70}}},
		{Label: "~/code/bittorrent-client/AGENTS.md", Group: "bittorrent-client", Kind: "agents", Tokens: 720, Lines: 55, Headings: []string{"Build", "Tests"}},
		{Label: "~/.claude/projects/…/memory/MEMORY.md", Group: "pano", Kind: "memory", Tokens: 310, Lines: 22, Headings: []string{"Memory index"}},
	}
}

func demoProjectsList() []string {
	out := []string{}
	for _, p := range demoProjects {
		out = append(out, "/home/nuri/code/"+p.name)
	}
	return out
}
