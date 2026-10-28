# pano

A local dashboard for Claude Code. It reads what Claude Code already keeps in `~/.claude` and shows
it on one screen at http://127.0.0.1:7878, without scrolling, over a sky that follows your clock:

- **Sessions (Oturumlar):** recent sessions with a warm / cold prompt-cache lamp, the context the
  next message re-sends and what it costs warm or cold; "Devam et" copies the resume command.
- **Skills (Skill'ler):** installed skills, slash commands and plugin skills by use, plus builtin
  skills Claude called; size, description and path in the drawer.
- **Memory (Hafıza):** every `CLAUDE.md` / `AGENTS.md` / `MEMORY.md` Claude loads and the files they
  point to; a red dot marks a broken link.
- **Live office:** [agent-office-3d](https://github.com/ozyusuf/agent-office-3d) in the middle when it
  runs on 127.0.0.1:7847, with a button that starts it when it does not.
- **Prompt bar:** give Claude a job in the background (`claude -p`) in a folder and model you pick;
  `/` picks a skill. Runs use permission mode `auto` by default.
- **Usage (Kullanım):** tokens and cost at API list prices for 7 / 30 / 90 days, daily tokens per
  model, an hour-of-week map, top tools and projects.
- **Runs (Kayıtlar):** background runs with status, cost and output. Reports are saved to
  `~/.local/share/pano/runs/`.

pano itself only reads local files and listens on 127.0.0.1. Background runs you start go
through your own `claude` CLI, and with the default `auto` permission mode they can edit files
and run commands in the folder you pick.

## Run it

Needs Go 1.24 or newer.

```sh
cd pano
go build -o ~/.local/bin/pano .
pano                    # starts in the background if needed, opens http://127.0.0.1:7878
pano stop               # stops pano (and agent-office-3d)
pano install-desktop    # adds pano to the app menu, for one click
pano -demo              # synthetic data, to see the look first
```

`pano` starts the server in the background (log: `~/.local/share/pano/pano.log`), starts
agent-office-3d with its own `scripts/start.sh` when it is installed and down, and opens the
dashboard as an app window in Chrome / Chromium / Brave / Edge (a normal tab otherwise). Run it
again and it only opens the window. `pano serve` runs the server in the foreground.

Flags: `-addr 127.0.0.1:7878`, `-claude-dir ~/.claude` (default; `$CLAUDE_CONFIG_DIR` if set),
`-office http://127.0.0.1:7847` (`off` hides the panel), `-config ~/.config/pano/config.json`.

### Start on login (systemd)

```sh
mkdir -p ~/.config/systemd/user
cp pano.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now pano
```

## The live office

agent-office-3d gets Claude Code's events through a hook. Its Linux / macOS support is in
[a pull request](https://github.com/NuriOkumus/agent-office-3d/tree/linux-macos-support); until it
is merged, install from that branch:

```sh
git clone -b linux-macos-support https://github.com/NuriOkumus/agent-office-3d.git ~/agent-office-3d
cd ~/agent-office-3d
sh install.sh         # shows the change to ~/.claude/settings.json and asks first
```

Its README has a systemd unit for starting it on login. pano finds it by itself; the office panel
switches on within 20 seconds of the server starting.

## Config

`~/.config/pano/config.json`, every field optional:

```json
{
  "prices": { "opus": { "input": 5, "output": 25 }, "sonnet": { "input": 3, "output": 15 } },
  "cache_ttl_minutes": 60,
  "models": ["haiku", "sonnet", "opus"],
  "extra_args": [],
  "max_run_minutes": 30,
  "permission_mode": "auto",
  "office_dir": "~/agent-office-3d"
}
```

- `prices`: USD per million tokens, per model family. Cache writes default to 1.25x input and
  cache reads to 0.1x; set `cache_write` / `cache_read` to override. These are estimates for the
  dashboard. On a Pro / Max plan your real cost is the subscription, not these numbers.
- `permission_mode`: `claude --permission-mode` for background runs, `auto` by default (Claude decides
  per action, as in an interactive session in auto mode). Also `acceptEdits`, `plan`, `dontAsk`,
  `manual`, `bypassPermissions`.
- `extra_args`: added to every background run, after the permission mode. A `--permission-mode` here
  wins over `permission_mode`.
- `office_dir`: the agent-office-3d folder `pano` starts and `pano stop` stops. Empty by default:
  `~/agent-office-3d` or `~/Projects/agent-office-3d`, whichever exists.

## Security

- Listens on loopback only; refuses any other `-addr`.
- Every request must carry our `Host` (stops DNS rebinding). Every API call needs a token that is
  generated at start and only served inside the page, and our `Origin` when the browser sends one,
  so other sites cannot read your data or start runs. `/health` (used by the launcher) needs no
  token and returns only the name, version and process id.
- Strict Content-Security-Policy; the only frame allowed is the agent-office-3d URL.
- Runs use `exec` with an argument list (no shell). The skill must exist on disk, the model must
  be in `models`, the folder must be one of your known project folders, at most two run at once.

## How it reads ~/.claude

- `projects/*/*.jsonl`: one transcript per session. Usage is counted once per request (Claude Code
  repeats it on every content block) and deduped across files (a resumed session can copy history).
- `skills/*/SKILL.md`, `commands/*.md`, the same under each project's `.claude/`, and plugin skills
  under `plugins/`. Use counts come from `Skill` tool calls and `/command` prompts in transcripts.
- `CLAUDE.md`, `AGENTS.md`, `CLAUDE.local.md` in `~/.claude` and in project folders, plus
  `projects/*/memory/MEMORY.md`. `@path` imports and links to `.md` files are followed one level.

The transcript format is Claude Code's internal format and can change between versions; pano reads
only a few fields and skips lines it does not understand. `go test ./...` covers the parser.

## Licenses

pano: MIT. Font: Barlow, SIL Open Font License 1.1 (`web/fonts/OFL.txt`).
