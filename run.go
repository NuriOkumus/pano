package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Run panel: start a skill in the background with `claude -p` (headless mode,
// one prompt, then exit), the self-triggered skill from the ARMS guide.
// Arguments go straight to exec (no shell), the skill must be one we found on
// disk, the model must be on the allow list and the folder a known project.

type Run struct {
	ID       string    `json:"id"`
	Skill    string    `json:"skill"`
	Model    string    `json:"model"`
	Cwd      string    `json:"cwd"`
	Note     string    `json:"note,omitempty"`
	Status   string    `json:"status"` // running | done | error
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished,omitempty"`
	Cost     float64   `json:"cost"`
	Turns    int       `json:"turns"`
	Result   string    `json:"result"`
	Report   string    `json:"report"` // path of the saved markdown report
}

type RunRequest struct {
	Skill string `json:"skill"`
	Model string `json:"model"`
	Cwd   string `json:"cwd"`
	Note  string `json:"note"`
}

type Runner struct {
	mu   sync.Mutex
	runs map[string]*Run
	dir  string
	cfg  Config
	demo bool
	slot chan struct{}
}

func NewRunner(cfg Config, demo bool) *Runner {
	r := &Runner{runs: map[string]*Run{}, dir: filepath.Join(dataDir(), "runs"), cfg: cfg, demo: demo, slot: make(chan struct{}, 2)}
	r.load()
	return r
}

// load restores recent runs from disk so the history survives restarts.
func (r *Runner) load() {
	if r.demo {
		return
	}
	files, _ := filepath.Glob(filepath.Join(r.dir, "*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var run Run
		if json.Unmarshal(b, &run) == nil && run.ID != "" {
			if run.Status == "running" {
				run.Status = "error"
				run.Result = "pano restarted while this run was going"
			}
			r.runs[run.ID] = &run
		}
	}
}

func (r *Runner) List() []Run {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Run, 0, len(r.runs))
	for _, v := range r.runs {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Started.After(out[j].Started) })
	if len(out) > 30 {
		out = out[:30]
	}
	return out
}

var errBusy = errors.New("two runs are already going; wait for one to finish")

func (r *Runner) Start(req RunRequest, skills []Skill, projects []string) (*Run, error) {
	found := false
	for _, s := range skills {
		if s.Name == req.Skill {
			found = true
			break
		}
	}
	if !found || !skillNameRe.MatchString(req.Skill) {
		return nil, errors.New("unknown skill")
	}
	okModel := false
	for _, m := range r.cfg.Models {
		if m == req.Model {
			okModel = true
		}
	}
	if !okModel {
		return nil, errors.New("model not allowed; add it to models in config.json")
	}
	home, _ := os.UserHomeDir()
	cwd := home
	if req.Cwd != "" && req.Cwd != "~" {
		ok := false
		for _, p := range projects {
			if p == req.Cwd {
				ok = true
			}
		}
		if !ok {
			return nil, errors.New("unknown project folder")
		}
		cwd = req.Cwd
	}
	note := strings.TrimSpace(req.Note)
	if len([]rune(note)) > 2000 {
		return nil, errors.New("note is too long (2000 characters max)")
	}
	select {
	case r.slot <- struct{}{}:
	default:
		return nil, errBusy
	}

	b := make([]byte, 4)
	_, _ = rand.Read(b)
	run := &Run{
		ID:    time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b),
		Skill: req.Skill, Model: req.Model, Cwd: cwd, Note: note,
		Status: "running", Started: time.Now(),
	}
	r.mu.Lock()
	r.runs[run.ID] = run
	r.mu.Unlock()
	go r.exec(run)
	cp := *run
	return &cp, nil
}

func (r *Runner) exec(run *Run) {
	defer func() { <-r.slot }()
	prompt := "/" + run.Skill
	if run.Note != "" {
		prompt += " " + run.Note
	}
	var result string
	var cost float64
	var turns int
	status := "done"

	if r.demo {
		time.Sleep(4 * time.Second)
		result = "Demo run: " + prompt + " finished. 3 findings, 1 suggested change, nothing applied."
		cost, turns = 0.031, 3
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.cfg.MaxRunMins)*time.Minute)
		defer cancel()
		args := append([]string{"-p", prompt, "--model", run.Model, "--output-format", "json"}, r.cfg.ExtraArgs...)
		cmd := exec.CommandContext(ctx, r.cfg.ClaudeBin, args...)
		cmd.Dir = run.Cwd
		cmd.Stdin = nil
		out, err := cmd.Output()
		var parsed struct {
			Result  string  `json:"result"`
			Cost    float64 `json:"total_cost_usd"`
			Turns   int     `json:"num_turns"`
			IsError bool    `json:"is_error"`
		}
		if jerr := json.Unmarshal(out, &parsed); jerr == nil && (parsed.Result != "" || parsed.Turns > 0) {
			result, cost, turns = parsed.Result, parsed.Cost, parsed.Turns
			if parsed.IsError {
				status = "error"
			}
		} else {
			result = strings.TrimSpace(string(out))
		}
		if err != nil {
			status = "error"
			var ee *exec.ExitError
			if errors.As(err, &ee) && len(ee.Stderr) > 0 {
				result = strings.TrimSpace(result + "\n" + string(ee.Stderr))
			} else if result == "" {
				result = err.Error()
			}
			if ctx.Err() == context.DeadlineExceeded {
				result = "stopped after " + time.Duration(r.cfg.MaxRunMins*int(time.Minute)).String() + "\n" + result
			}
		}
	}

	r.mu.Lock()
	run.Status, run.Result, run.Cost, run.Turns, run.Finished = status, clip(result, 20000), cost, turns, time.Now()
	cp := *run
	r.mu.Unlock()
	r.save(&cp)
}

func (r *Runner) save(run *Run) {
	if r.demo {
		return
	}
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return
	}
	report := filepath.Join(r.dir, run.ID+".md")
	md := "# /" + run.Skill + "\n\n" +
		"- model: " + run.Model + "\n- folder: " + run.Cwd + "\n- started: " + run.Started.Format(time.RFC3339) +
		"\n- status: " + run.Status + "\n\n" + run.Result + "\n"
	if os.WriteFile(report, []byte(md), 0o600) == nil {
		run.Report = report
		r.mu.Lock()
		if live := r.runs[run.ID]; live != nil {
			live.Report = report
		}
		r.mu.Unlock()
	}
	b, _ := json.MarshalIndent(run, "", "  ")
	_ = os.WriteFile(filepath.Join(r.dir, run.ID+".json"), b, 0o600)
}
