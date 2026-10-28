# pano: the new face

Status: approved in brainstorming on 2026-10-28. Next step: implementation plan.

The electrical-panel look goes. pano becomes one screen with no page scroll: a live sky in the back,
agent-office-3d in the middle, dark glass panels around it and a prompt bar at the bottom.

Visual references (from the brainstorming companion, `.superpowers/brainstorm/1638854-1793205118/content/`):
`pano-gokyuzu.html` (approved: layout + live sky + glass panels, with a time slider),
`yonler.html` (direction C "Ofis sahnesi" chosen), `studyo-havasi.html` (studio look A, for the separate
studio-theme project).

## Decisions (made with the user)

| Topic | Decision |
|---|---|
| Theme | No electrical-panel metaphor, no panel/breaker names. |
| Scroll | The page never scrolls. Designed for 1280×720 and up. |
| Main screen | Sessions (left), skills + memory (right), office (centre), prompt bar (bottom). Usage and runs move to their own tabs. |
| Background | A live sky that follows the local clock: gradient, sun / moon on an arc, stars, drifting clouds, two rows of hills. |
| Panels | Dark frosted glass all day; only the accent colour follows the time of day. |
| Prompt bar | Free prompt or `/skill note`, run in the background with `claude -p`; result goes to Runs. |
| Permissions | Background runs use `--permission-mode auto` by default (verified: works headless with `-p`). |
| Office | agent-office-3d embedded in the centre as today; the studio theme is a separate project that comes after this one. |

Not in scope: the studio theme (separate spec, its design is paused at section 1), sending a prompt into
an existing session (`--resume`), a per-run permission selector, birds / weather effects in pano's sky.

## 1. Screen

Three layers, back to front:

1. **Sky** (`web/sky.js`): full-bleed. Keyframes by local hour, the same points as agent-office-3d's
   `daylight.js`, linear interpolation between them:

   | hour | sky top | horizon | clouds | stars | accent | hills (far, near) |
   |---|---|---|---|---|---|---|
   | 0 | `#060a1a` | `#141d3a` | `#2a3150` | 1 | `#ffb36b` | `#0d1226` `#151b33` |
   | 4.5 | `#0a1130` | `#2a2c55` | `#3a3d63` | .9 | `#ffb36b` | `#141a35` `#1c2242` |
   | 6 | `#3d3a72` | `#f2a088` | `#f3b4b8` | .3 | `#ff8fa3` | `#5a3f5a` `#6e4a5e` |
   | 7.5 | `#5b98d6` | `#ffd7a6` | `#fff1e6` | 0 | `#ff9b4a` | `#7c6a63` `#8f7766` |
   | 12 | `#3b8ce0` | `#c4e6ff` | `#ffffff` | 0 | `#ff9b4a` | `#6f8a6a` `#7f9b74` |
   | 16 | `#4a8bd2` | `#ffe0ae` | `#fff6e8` | 0 | `#ff9b4a` | `#7f8a62` `#958f68` |
   | 18 | `#5a5aa0` | `#ff9658` | `#ffc29a` | .08 | `#ff7a45` | `#6a4a4a` `#7d5446` |
   | 19.5 | `#25275a` | `#cf5f78` | `#a06f88` | .55 | `#ff8f6b` | `#2d2440` `#3a2a44` |
   | 21 | `#0a1130` | `#1e2950` | `#2a3352` | .95 | `#ffb36b` | `#121833` `#19203b` |
   | 24 | = 0 | | | | | |

   Sun visible 05:48–18:42 on an arc from left to right, moon 19:12–05:42. Position updates once a
   minute; clouds drift with a CSS transform (GPU, ~120–230 s per crossing); stars twinkle with opacity
   only. Everything stops while the tab is hidden. `?saat=HH.hh` overrides the clock (testing).
2. **Office** (centre): agent-office-3d in an iframe. When it is down: the sky shows through and a
   small glass card says "Ofis kapalı" with a **Başlat** button (`POST /api/office/start`), the
   script's error text if it fails, and install instructions when no agent-office-3d folder exists.
3. **Glass panels**: `--panel: rgba(16,14,22,.6)`, `backdrop-filter: blur(14px) saturate(1.2)`,
   hairline `rgba(255,255,255,.12)`, text `#f3f1ed` / `#c9c6cf` / `#9a97a3`. Semantic colours do not
   follow the clock: warm cache `#ffb347`, running `#56d999`, waiting `#f4c752`, error `#ff5a5f`.

Main tab ("Ana"):

- **Top bar**: `pano.`, clock, tabs Ana / Kullanım / Kayıtlar, small numbers (5 h tokens + cost,
  7 d cost, cache hit), "n iş çalışıyor" chip (opens Runs).
- **Left – Oturumlar**: recent sessions. Row: warm/cold lamp (minutes left when warm), title,
  project · model · context, cost of the next message.
- **Right – Skill'ler**: search box, usage bars, the most used first, "tümü (n) →".
  **Hafıza** below: CLAUDE.md / MEMORY.md files with token size, a red dot on files with a broken link,
  "+ n dosya →".
- **Bottom – prompt bar** (section 2).

Lists that do not fit end in "tümü →", which opens the drawer; panels never scroll the page.

Kullanım tab: the same sky and glass; summary numbers, daily tokens per model, hour-of-week map, top
tools, projects; 7 / 30 / 90 days. Kayıtlar tab: run list left, the selected run right.

Renames: Sayaçlar → Kullanım, Devreler → Skill'ler, Hatlar → Oturumlar, Tek hat şeması → Hafıza,
Kayıt defteri → Kayıtlar.

Type: Barlow (OFL, embedded) for everything, tabular numbers. Barlow Condensed is dropped.

## 2. Interactions

Prompt bar:

- Enter sends, Shift+Enter is a new line. `/` at the start opens a skill picker that filters as you
  type; picking inserts `/name ` and the rest is the note. `/` anywhere on the page (outside inputs)
  focuses the bar.
- Folder chip: known project folders + home; default = last used (localStorage), else the folder of
  the latest session. Model chip: `models` from config; default = last used, else sonnet.
- On send: the bar clears, a toast "Gönderildi · folder · model", the run chip counts up. On finish: a
  toast "Bitti · $cost · n tur" (red on error) that opens the run. Two runs at most at a time (as
  today); a third gets "biri bitsin".

Drawer: a glass card over the centre, closed by Esc or ×; the page underneath does not change.

- Session: title, folder, model, prompts, context, cache lamp with countdown, next message warm/cold
  cost, total cost; **Devam et** copies `cd <cwd> && claude --resume <id>`.
- Skill: description, size, uses, last use, path; **Çalıştır** puts `/name ` in the prompt bar and
  focuses it (the old run form goes away).
- Memory file: path, tokens, lines, headings, linked files; broken links in red.
- "tümü" lists: the full list with search.
- Run: prompt, folder, model, status, duration, cost, turns, output (pre-wrapped text, code blocks in
  mono), report path.

Refresh: sessions and skills every 10 s, runs every 3 s while one runs, sky every minute; nothing
while the tab is hidden.

## 3. Backend

- `skills.go`: plugin skills come from `plugins/installed_plugins.json` (version 2: key
  `name@marketplace` → entries with `installPath`). Name = the part before `@`; only
  `<installPath>/skills/*/SKILL.md` and `<installPath>/commands/*.md` are read. The marketplace
  catalogue is no longer walked. No file → no plugin skills.
- Used-but-not-found skills: names from the usage map with no file become rows with scope
  `builtin`, their use count and unknown size.
- `run.go`: `RunRequest` takes `skill` + `note` or `prompt` (≤ 8000 characters). A prompt that starts
  with `/` must name a known skill. `Run` gets `prompt`. Argument building moves to a pure function:
  `-p <prompt> --model <m> --output-format json --permission-mode <mode>` + `extra_args`; the mode is
  left out when `extra_args` already has `--permission-mode`. Report title: `/skill` or the prompt's
  first line.
- `config.go`: `permission_mode`, default `auto`, accepted values `acceptEdits auto bypassPermissions
  manual dontAsk plan` (from `claude --help`, v2.1.296); anything else falls back to `auto`.
- `main.go`: `POST /api/office/start` (token + Origin like every API call) runs
  `sh <office_dir>/scripts/start.sh --no-browser` and returns its output; static files from `web/`
  (`app.js`, `app.css`, `sky.js`) served from the embedded FS with fixed content types.
- Unchanged: the other APIs, loopback-only listening, Host/token/Origin checks, CSP, demo mode,
  the `pano` launcher and `/health`.

## 4. Testing and delivery

Go, test first: installed-plugin discovery (names, catalogue ignored, usage matched), builtin rows,
run request validation, argument builder and permission mode, office start needs the token.

Front end, in a real browser: screenshots at 06:30, 12:00, 18:15 and 23:00 via `?saat=`;
`document.documentElement.scrollHeight <= innerHeight` at 1280×720 and 1920×1080; one cheap haiku
run through the prompt bar end to end; `pano -demo`.

Stages: (1) backend, (2) shell + sky, (3) panels + drawer, (4) prompt bar + runs, (5) Kullanım and
Kayıtlar tabs, (6) cleanup (old CSS, condensed fonts), README, install with `go build -o ~/.local/bin/pano .`.

## Known issue elsewhere

agent-office-3d fork, `scripts/common.sh:10`: `[ -n "$AGENT_OFFICE_PORT" ]` under `set -u` leaves
`PORT` empty, so `start.sh` reports a failed start and `stop.sh` never finds the server. Fix:
`${AGENT_OFFICE_PORT:-}`. Belongs to the open Linux PR; not changed here.
