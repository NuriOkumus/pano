package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Claude Code keeps one JSONL transcript per session under
// ~/.claude/projects/<encoded-cwd>/. Each line is one event. We read only the
// fields we need and ignore everything else, so format additions don't break us.

type Usage struct {
	In  int64 `json:"input_tokens"`
	Out int64 `json:"output_tokens"`
	CW  int64 `json:"cache_creation_input_tokens"`
	CR  int64 `json:"cache_read_input_tokens"`
}

func (u Usage) total() int64 { return u.In + u.Out + u.CW + u.CR }

// context is what the next request re-sends: everything that went in.
func (u Usage) context() int64 { return u.In + u.CW + u.CR }

type line struct {
	Type        string          `json:"type"`
	Timestamp   string          `json:"timestamp"`
	SessionID   string          `json:"sessionId"`
	Cwd         string          `json:"cwd"`
	IsSidechain bool            `json:"isSidechain"`
	RequestID   string          `json:"requestId"`
	Summary     string          `json:"summary"`
	Message     json.RawMessage `json:"message"`
}

type message struct {
	ID      string          `json:"id"`
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Usage   *Usage          `json:"usage"`
	Content json.RawMessage `json:"content"`
}

type block struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type request struct {
	Key       string
	TS        time.Time
	Model     string
	U         Usage
	Sidechain bool
}

type stamp struct {
	Count int
	Last  time.Time
}

type fileData struct {
	modTime time.Time
	size    int64

	SessionID string
	Cwd       string
	Title     string
	Summary   string
	Requests  []request
	First     time.Time
	Last      time.Time
	LastMain  time.Time
	LastCtx   int64
	LastModel string
	UserMsgs  int
	OnlySide  bool
	Skills    map[string]*stamp
	Tools     map[string]int
}

var (
	cmdNameRe = regexp.MustCompile(`<command-name>/?([^<\s]+)</command-name>`)
	tagRe     = regexp.MustCompile(`<[^>]+>`)
	// Wrappers Claude Code puts inside user turns that are not what the user typed.
	noiseRes = []*regexp.Regexp{
		regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`),
		regexp.MustCompile(`(?s)<local-command-[a-z]+>.*?</local-command-[a-z]+>`),
		regexp.MustCompile(`(?s)<command-message>.*?</command-message>`),
		regexp.MustCompile(`(?s)<command-args>.*?</command-args>`),
	}
	spaceRe = regexp.MustCompile(`\s+`)
)

func parseFile(path string) (*fileData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fd := &fileData{Skills: map[string]*stamp{}, Tools: map[string]int{}, OnlySide: true}
	seen := map[string]bool{}
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		raw, err := r.ReadBytes('\n')
		if len(bytes.TrimSpace(raw)) > 0 {
			fd.consume(raw, seen)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fd, err
		}
	}
	if fd.Title == "" {
		fd.Title = fd.Summary
	}
	if fd.Summary != "" {
		fd.Title = fd.Summary
	}
	return fd, nil
}

func (fd *fileData) consume(raw []byte, seen map[string]bool) {
	var l line
	if json.Unmarshal(raw, &l) != nil {
		return
	}
	if l.Type == "summary" && l.Summary != "" {
		fd.Summary = clip(l.Summary, 90)
		return
	}
	if l.Type != "user" && l.Type != "assistant" {
		return
	}
	ts, _ := time.Parse(time.RFC3339Nano, l.Timestamp)
	if fd.SessionID == "" && l.SessionID != "" {
		fd.SessionID = l.SessionID
	}
	if fd.Cwd == "" && l.Cwd != "" {
		fd.Cwd = l.Cwd
	}
	if !l.IsSidechain {
		fd.OnlySide = false
	}
	if !ts.IsZero() {
		if fd.First.IsZero() || ts.Before(fd.First) {
			fd.First = ts
		}
		if ts.After(fd.Last) {
			fd.Last = ts
		}
	}
	var m message
	if json.Unmarshal(l.Message, &m) != nil {
		return
	}
	blocks := contentBlocks(m.Content)

	if l.Type == "user" {
		text := ""
		for _, b := range blocks {
			if b.Type == "text" {
				text += b.Text
			}
		}
		if text == "" {
			return // tool results ride on user lines; not a real prompt
		}
		if !l.IsSidechain {
			fd.UserMsgs++
		}
		if mm := cmdNameRe.FindStringSubmatch(text); mm != nil {
			fd.skill(mm[1], ts)
		}
		if fd.Title == "" && !l.IsSidechain {
			if t := cleanTitle(text); t != "" {
				fd.Title = t
			}
		}
		return
	}

	// assistant
	for _, b := range blocks {
		if b.Type != "tool_use" || b.Name == "" {
			continue
		}
		fd.Tools[b.Name]++
		if b.Name == "Skill" {
			var in struct {
				Skill   string `json:"skill"`
				Command string `json:"command"`
			}
			_ = json.Unmarshal(b.Input, &in)
			name := in.Skill
			if name == "" {
				name = in.Command
			}
			if name != "" {
				fd.skill(name, ts)
			}
		}
	}
	if m.Usage == nil || m.Model == "" || m.Model == "<synthetic>" {
		return
	}
	// Claude Code writes one line per content block, each repeating the
	// request's usage. Count each request once.
	key := m.ID + ":" + l.RequestID
	if m.ID == "" {
		key = l.Timestamp + ":" + l.RequestID
	}
	if seen[key] {
		return
	}
	seen[key] = true
	fd.Requests = append(fd.Requests, request{Key: key, TS: ts, Model: m.Model, U: *m.Usage, Sidechain: l.IsSidechain})
	if !l.IsSidechain && !ts.Before(fd.LastMain) {
		fd.LastMain = ts
		fd.LastCtx = m.Usage.context() + m.Usage.Out
		fd.LastModel = m.Model
	}
}

func (fd *fileData) skill(name string, ts time.Time) {
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
	if ts.After(s.Last) {
		s.Last = ts
	}
}

func contentBlocks(raw json.RawMessage) []block {
	if len(raw) == 0 {
		return nil
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return []block{{Type: "text", Text: s}}
		}
		return nil
	}
	var bs []block
	_ = json.Unmarshal(raw, &bs)
	return bs
}

func cleanTitle(text string) string {
	if mm := cmdNameRe.FindStringSubmatch(text); mm != nil {
		return "/" + mm[1]
	}
	t := text
	for _, re := range noiseRes {
		t = re.ReplaceAllString(t, " ")
	}
	t = strings.TrimSpace(t)
	if strings.HasPrefix(t, "Caveat:") {
		return ""
	}
	if strings.HasPrefix(t, "<") {
		t = tagRe.ReplaceAllString(t, " ")
	}
	t = strings.TrimSpace(spaceRe.ReplaceAllString(t, " "))
	return clip(t, 90)
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// Store keeps parsed transcripts and re-reads only files that changed.
type Store struct {
	root  string
	mu    sync.Mutex
	files map[string]*fileData
	last  time.Time
}

func NewStore(claudeDir string) *Store {
	return &Store{root: filepath.Join(claudeDir, "projects"), files: map[string]*fileData{}}
}

// Refresh re-scans at most every few seconds.
func (s *Store) Refresh() map[string]*fileData {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.last) < 3*time.Second {
		return s.snapshot()
	}
	s.last = time.Now()
	alive := map[string]bool{}
	_ = filepath.WalkDir(s.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		alive[p] = true
		if old, ok := s.files[p]; ok && old.modTime.Equal(info.ModTime()) && old.size == info.Size() {
			return nil
		}
		fd, err := parseFile(p)
		if fd == nil {
			return nil
		}
		_ = err
		fd.modTime, fd.size = info.ModTime(), info.Size()
		if fd.Cwd == "" {
			fd.Cwd = decodeProjectDir(filepath.Base(filepath.Dir(p)))
		}
		s.files[p] = fd
		return nil
	})
	for p := range s.files {
		if !alive[p] {
			delete(s.files, p)
		}
	}
	return s.snapshot()
}

func (s *Store) snapshot() map[string]*fileData {
	out := make(map[string]*fileData, len(s.files))
	for k, v := range s.files {
		out[k] = v
	}
	return out
}

// decodeProjectDir turns "-home-nuri-code-pano" back into a readable path.
// The encoding is lossy (dashes in names), so this is only a label.
func decodeProjectDir(name string) string {
	return "/" + strings.ReplaceAll(strings.TrimPrefix(name, "-"), "-", "/")
}

// ---------- aggregation ----------

type ModelShare struct {
	Name   string  `json:"name"`
	Family string  `json:"family"`
	Tokens int64   `json:"tokens"`
	Cost   float64 `json:"cost"`
	Calls  int     `json:"calls"`
}

type Day struct {
	Date   string           `json:"date"`
	Tokens map[string]int64 `json:"tokens"` // by family
	Cost   float64          `json:"cost"`
	Calls  int              `json:"calls"`
}

type Count struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type Overview struct {
	GeneratedAt time.Time    `json:"generatedAt"`
	Days        int          `json:"days"`
	Demo        bool         `json:"demo"`
	Totals      Totals       `json:"totals"`
	Windows     Windows      `json:"windows"`
	Daily       []Day        `json:"daily"`
	Models      []ModelShare `json:"models"`
	Tools       []Count      `json:"tools"`
	Heatmap     [7][24]int   `json:"heatmap"` // [weekday Mon=0][hour]
	Projects    []Count      `json:"projects"`
}

type Totals struct {
	Input      int64   `json:"input"`
	Output     int64   `json:"output"`
	CacheWrite int64   `json:"cacheWrite"`
	CacheRead  int64   `json:"cacheRead"`
	Cost       float64 `json:"cost"`
	Sessions   int     `json:"sessions"`
	Calls      int     `json:"calls"`
	Prompts    int     `json:"prompts"`
	CacheHit   float64 `json:"cacheHit"` // share of input served from cache
	Subagent   float64 `json:"subagentShare"`
}

type Windows struct {
	FiveHour  int64   `json:"fiveHour"`
	SevenDay  int64   `json:"sevenDay"`
	FiveCost  float64 `json:"fiveCost"`
	SevenCost float64 `json:"sevenCost"`
}

func cost(cfg Config, model string, u Usage) float64 {
	p := cfg.price(family(model))
	return (float64(u.In)*p.Input + float64(u.Out)*p.Output + float64(u.CW)*p.write() + float64(u.CR)*p.read()) / 1e6
}

// allRequests flattens and dedupes across files: a resumed session can copy
// earlier history into a new transcript.
func allRequests(files map[string]*fileData) []request {
	seen := map[string]bool{}
	var out []request
	for _, fd := range files {
		for _, r := range fd.Requests {
			if seen[r.Key] {
				continue
			}
			seen[r.Key] = true
			out = append(out, r)
		}
	}
	return out
}

func buildOverview(files map[string]*fileData, cfg Config, days int, now time.Time) Overview {
	ov := Overview{GeneratedAt: now, Days: days}
	since := startOfDay(now).AddDate(0, 0, -(days - 1))
	dayIdx := map[string]*Day{}
	for i := 0; i < days; i++ {
		d := since.AddDate(0, 0, i).Format("2006-01-02")
		ov.Daily = append(ov.Daily, Day{Date: d, Tokens: map[string]int64{}})
	}
	for i := range ov.Daily {
		dayIdx[ov.Daily[i].Date] = &ov.Daily[i]
	}
	models := map[string]*ModelShare{}
	var sideTok, allTok int64
	for _, r := range allRequests(files) {
		c := cost(cfg, r.Model, r.U)
		age := now.Sub(r.TS)
		if age <= 5*time.Hour {
			ov.Windows.FiveHour += r.U.total()
			ov.Windows.FiveCost += c
		}
		if age <= 7*24*time.Hour {
			ov.Windows.SevenDay += r.U.total()
			ov.Windows.SevenCost += c
		}
		if r.TS.Before(since) {
			continue
		}
		lt := r.TS.Local()
		if d := dayIdx[lt.Format("2006-01-02")]; d != nil {
			d.Tokens[family(r.Model)] += r.U.total()
			d.Cost += c
			d.Calls++
		}
		wd := (int(lt.Weekday()) + 6) % 7
		ov.Heatmap[wd][lt.Hour()]++
		t := &ov.Totals
		t.Input += r.U.In
		t.Output += r.U.Out
		t.CacheWrite += r.U.CW
		t.CacheRead += r.U.CR
		t.Cost += c
		t.Calls++
		allTok += r.U.total()
		if r.Sidechain {
			sideTok += r.U.total()
		}
		ms := models[r.Model]
		if ms == nil {
			ms = &ModelShare{Name: r.Model, Family: family(r.Model)}
			models[r.Model] = ms
		}
		ms.Tokens += r.U.total()
		ms.Cost += c
		ms.Calls++
	}
	if in := ov.Totals.Input + ov.Totals.CacheWrite + ov.Totals.CacheRead; in > 0 {
		ov.Totals.CacheHit = float64(ov.Totals.CacheRead) / float64(in)
	}
	if allTok > 0 {
		ov.Totals.Subagent = float64(sideTok) / float64(allTok)
	}
	for _, m := range models {
		ov.Models = append(ov.Models, *m)
	}
	sort.Slice(ov.Models, func(i, j int) bool { return ov.Models[i].Tokens > ov.Models[j].Tokens })

	tools := map[string]int{}
	projects := map[string]int{}
	sessions := map[string]bool{}
	for _, fd := range files {
		if fd.Last.Before(since) || fd.OnlySide {
			continue
		}
		if fd.SessionID != "" {
			sessions[fd.SessionID] = true
		}
		ov.Totals.Prompts += fd.UserMsgs
		for k, v := range fd.Tools {
			tools[k] += v
		}
		projects[projectLabel(fd.Cwd)] += fd.UserMsgs
	}
	ov.Totals.Sessions = len(sessions)
	ov.Tools = topCounts(tools, 8)
	ov.Projects = topCounts(projects, 6)
	return ov
}

func topCounts(m map[string]int, n int) []Count {
	out := make([]Count, 0, len(m))
	for k, v := range m {
		if v > 0 {
			out = append(out, Count{k, v})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Name < out[j].Name
		}
		return out[i].Count > out[j].Count
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func projectLabel(cwd string) string {
	if cwd == "" {
		return "?"
	}
	h, _ := os.UserHomeDir()
	if cwd == h {
		return "~"
	}
	return filepath.Base(cwd)
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Local().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

// ---------- sessions ----------

type Session struct {
	ID        string    `json:"id"`
	Project   string    `json:"project"`
	Cwd       string    `json:"cwd"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`
	Family    string    `json:"family"`
	Start     time.Time `json:"start"`
	Last      time.Time `json:"last"`
	IdleSec   int64     `json:"idleSec"`
	Cache     string    `json:"cache"` // warm | cold
	CacheLeft int64     `json:"cacheLeftSec"`
	Context   int64     `json:"context"`
	Tokens    int64     `json:"tokens"`
	Cost      float64   `json:"cost"`
	Prompts   int       `json:"prompts"`
	NextWarm  float64   `json:"nextWarm"`
	NextCold  float64   `json:"nextCold"`
}

func buildSessions(files map[string]*fileData, cfg Config, limit int, now time.Time) []Session {
	ttl := time.Duration(cfg.CacheTTLMinutes) * time.Minute
	byID := map[string]*Session{}
	seen := map[string]bool{}
	for _, fd := range files {
		if fd.OnlySide && fd.SessionID == "" {
			continue
		}
		id := fd.SessionID
		if id == "" {
			continue
		}
		s := byID[id]
		if s == nil {
			s = &Session{ID: id}
			byID[id] = s
		}
		if !fd.OnlySide {
			if s.Cwd == "" {
				s.Cwd = fd.Cwd
			}
			if s.Title == "" || fd.Summary != "" {
				if fd.Title != "" {
					s.Title = fd.Title
				}
			}
			s.Prompts += fd.UserMsgs
			if fd.LastMain.After(s.Last) {
				s.Last = fd.LastMain
				s.Context = fd.LastCtx
				s.Model = fd.LastModel
			}
		}
		if s.Start.IsZero() || (!fd.First.IsZero() && fd.First.Before(s.Start)) {
			s.Start = fd.First
		}
		for _, r := range fd.Requests {
			if seen[r.Key] {
				continue
			}
			seen[r.Key] = true
			s.Tokens += r.U.total()
			s.Cost += cost(cfg, r.Model, r.U)
		}
	}
	out := make([]Session, 0, len(byID))
	for _, s := range byID {
		if s.Last.IsZero() {
			continue
		}
		s.Project = projectLabel(s.Cwd)
		s.Family = family(s.Model)
		idle := now.Sub(s.Last)
		s.IdleSec = int64(idle.Seconds())
		p := cfg.price(s.Family)
		s.NextWarm = float64(s.Context) * p.read() / 1e6
		s.NextCold = float64(s.Context) * p.write() / 1e6
		if idle < ttl {
			s.Cache = "warm"
			s.CacheLeft = int64((ttl - idle).Seconds())
		} else {
			s.Cache = "cold"
		}
		if s.Title == "" {
			s.Title = "(untitled)"
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Last.After(out[j].Last) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// skillUsage merges invocation counts from every transcript.
func skillUsage(files map[string]*fileData) map[string]stamp {
	out := map[string]stamp{}
	for _, fd := range files {
		for k, v := range fd.Skills {
			cur := out[k]
			cur.Count += v.Count
			if v.Last.After(cur.Last) {
				cur.Last = v.Last
			}
			out[k] = cur
		}
	}
	return out
}

// knownProjects lists working directories seen in transcripts that still exist.
func knownProjects(files map[string]*fileData) []string {
	set := map[string]time.Time{}
	for _, fd := range files {
		if fd.Cwd == "" || fd.OnlySide {
			continue
		}
		if fd.Last.After(set[fd.Cwd]) {
			set[fd.Cwd] = fd.Last
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return set[out[i]].After(set[out[j]]) })
	return out
}
