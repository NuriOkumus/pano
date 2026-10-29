package main

// The launcher. `pano` starts the server in the background when it is not
// running yet, starts agent-office-3d when it is installed and down, and opens
// the dashboard in a window of its own. `pano stop` stops both.

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

//go:embed web/pano.svg
var appIcon []byte

var commands = map[string]bool{"open": true, "serve": true, "stop": true, "install-desktop": true}

// parseCommand splits the arguments into a subcommand and its flags. No
// subcommand (or flags only) means "open".
func parseCommand(args []string) (string, []string, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "open", args, nil
	}
	if !commands[args[0]] {
		return "", nil, fmt.Errorf("unknown command %q (pano, pano serve, pano stop, pano install-desktop)", args[0])
	}
	return args[0], args[1:], nil
}

func openPano(o serveOpts, flags []string) int {
	if _, up := panoHealth(o.addr); up {
		fmt.Printf("pano is already running on http://%s\n", o.addr)
	} else {
		if err := startDetached(flags); err != nil {
			fmt.Fprintln(os.Stderr, "pano:", err)
			return 1
		}
		if !waitFor(func() bool { _, up := panoHealth(o.addr); return up }, 10*time.Second) {
			fmt.Fprintf(os.Stderr, "pano: the server did not start within 10 s; see %s\n", logPath())
			return 1
		}
		fmt.Printf("pano started on http://%s (log: %s)\n", o.addr, logPath())
	}
	startOffice(o)

	u := "http://" + o.addr + "/"
	argv := browserCommand(u, runtime.GOOS, exec.LookPath)
	cmd := exec.Command(argv[0], argv[1:]...)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		fmt.Printf("Open %s in your browser.\n", u)
		return 0
	}
	_ = cmd.Process.Release()
	return 0
}

func stopPano(o serveOpts) int {
	code := 0
	if pid, up := panoHealth(o.addr); !up {
		fmt.Println("pano is not running")
	} else if err := terminate(pid); err != nil {
		fmt.Fprintln(os.Stderr, "pano:", err)
		code = 1
	} else if waitFor(func() bool { _, up := panoHealth(o.addr); return !up }, 5*time.Second) {
		fmt.Println("pano stopped")
	} else {
		fmt.Fprintf(os.Stderr, "pano: process %d did not stop\n", pid)
		code = 1
	}
	if dir := officeDir(o); dir != "" {
		if up, _ := officeHealth(o.office); up {
			runScript(dir, "stop.sh")
		}
	}
	return code
}

// startOffice runs agent-office-3d's own start script when the office is
// enabled, installed and down. A failure only means the panel stays empty.
func startOffice(o serveOpts) {
	dir := officeDir(o)
	if dir == "" {
		return
	}
	if up, _ := officeHealth(o.office); up {
		return
	}
	if !runScript(dir, "start.sh", "--no-browser") {
		fmt.Fprintln(os.Stderr, "pano: agent-office-3d did not start; the dashboard opens without it")
	}
}

func officeDir(o serveOpts) string {
	if o.office == "off" || o.office == "" {
		return ""
	}
	home, _ := os.UserHomeDir()
	return findOfficeDir(loadConfig(o.cfgPath).OfficeDir, home)
}

// findOfficeDir returns the agent-office-3d folder: the configured one, else
// the first default place that has scripts/start.sh; "" when there is none.
func findOfficeDir(configured, home string) string {
	places := []string{filepath.Join(home, "agent-office-3d"), filepath.Join(home, "Projects", "agent-office-3d")}
	if configured != "" {
		if strings.HasPrefix(configured, "~/") {
			configured = filepath.Join(home, configured[2:])
		}
		places = []string{configured}
	}
	for _, d := range places {
		if st, err := os.Stat(filepath.Join(d, "scripts", "start.sh")); err == nil && !st.IsDir() {
			return d
		}
	}
	return ""
}

// runScript runs one of agent-office-3d's scripts in a session of its own, so
// the server start.sh leaves behind outlives the terminal that ran `pano`.
func runScript(dir, name string, args ...string) bool {
	cmd := exec.Command("sh", append([]string{filepath.Join(dir, "scripts", name)}, args...)...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	detach(cmd)
	return cmd.Run() == nil
}

// appBrowsers open a URL as a standalone app window (--app), in this order.
var appBrowsers = []string{"google-chrome-stable", "google-chrome", "chromium", "chromium-browser", "brave-browser", "microsoft-edge-stable"}

// browserCommand picks how to open url: a Chromium-family browser as an app
// window when one is installed, else the system opener (a normal tab).
func browserCommand(url, goos string, look func(string) (string, error)) []string {
	for _, b := range appBrowsers {
		if p, err := look(b); err == nil {
			return []string{p, "--app=" + url}
		}
	}
	if goos == "darwin" {
		return []string{"open", url}
	}
	return []string{"xdg-open", url}
}

// startDetached starts `pano serve` with the same flags in a session of its
// own, so closing the terminal does not stop it. Its log goes to logPath().
func startDetached(flags []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir(), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(logPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	cmd := exec.Command(exe, append([]string{"serve"}, flags...)...)
	cmd.Stdout, cmd.Stderr = f, f
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func logPath() string { return filepath.Join(dataDir(), "pano.log") }

// panoHealth asks /health at addr; ok only when pano itself answers.
func panoHealth(addr string) (pid int, ok bool) {
	c := http.Client{Timeout: 700 * time.Millisecond}
	resp, err := c.Get("http://" + addr + "/health")
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	var h struct {
		OK  bool   `json:"ok"`
		App string `json:"app"`
		PID int    `json:"pid"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&h) != nil || !h.OK || h.App != "pano" {
		return 0, false
	}
	return h.PID, true
}

func waitFor(cond func() bool, limit time.Duration) bool {
	for end := time.Now().Add(limit); time.Now().Before(end); time.Sleep(200 * time.Millisecond) {
		if cond() {
			return true
		}
	}
	return false
}

// installDesktop adds pano to the app menu (freedesktop: GNOME, KDE, ...).
func installDesktop() int {
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "pano:", err)
		return 1
	}
	entry := filepath.Join(dataHome(), "applications", "pano.desktop")
	icon := filepath.Join(dataHome(), "icons", "hicolor", "scalable", "apps", "pano.svg")
	for _, f := range []struct {
		path string
		body []byte
	}{{entry, []byte(desktopEntry(exe))}, {icon, appIcon}} {
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "pano:", err)
			return 1
		}
		if err := os.WriteFile(f.path, f.body, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "pano:", err)
			return 1
		}
	}
	fmt.Printf("Added pano to the app menu:\n  %s\n  %s\n", entry, icon)
	return 0
}

func desktopEntry(exe string) string {
	if strings.ContainsAny(exe, " \t\"\\") {
		exe = strconv.Quote(exe)
	}
	return "[Desktop Entry]\nType=Application\nName=pano\nComment=Claude Code dashboard\nExec=" + exe +
		"\nIcon=pano\nTerminal=false\nCategories=Development;\n"
}
