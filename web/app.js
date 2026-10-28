// pano's page: a live sky, the office in the middle, glass panels around it.
// Plain ES modules, no build step; every API call carries the per-start token.
import { startSky, fmtHour } from "/assets/sky.js";

const TOKEN = document.querySelector('meta[name="pano-token"]').content;
const $ = (s, el = document) => el.querySelector(s);
const h = (tag, attrs = {}, ...kids) => {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v == null || v === false) continue;
    if (k === "class") el.className = v;
    else if (k === "style") el.style.cssText = v;
    else if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
    else el.setAttribute(k, v === true ? "" : v);
  }
  for (const kid of kids.flat()) if (kid != null && kid !== false) el.append(kid.nodeType ? kid : String(kid));
  return el;
};
const svg = (tag, attrs = {}) => {
  const el = document.createElementNS("http://www.w3.org/2000/svg", tag);
  for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
  return el;
};

const nf = new Intl.NumberFormat("tr-TR");
const cf = new Intl.NumberFormat("tr-TR", { notation: "compact", maximumFractionDigits: 1 });
const usd = (n) => "$" + (n >= 100 ? nf.format(Math.round(n)) : n.toLocaleString("tr-TR", { minimumFractionDigits: 2, maximumFractionDigits: 2 }));
const tok = (n) => cf.format(n);
const pct = (x) => "%" + (x * 100).toLocaleString("tr-TR", { maximumFractionDigits: 1 });
function ago(sec) {
  if (sec < 60) return "az önce";
  const m = Math.floor(sec / 60);
  if (m < 60) return m + " dk";
  const hr = Math.floor(m / 60);
  if (hr < 24) return hr + " sa";
  return Math.floor(hr / 24) + " gün";
}
const sinceISO = (iso) => (Date.now() - new Date(iso).getTime()) / 1000;
const base = (p) => (p || "").split("/").filter(Boolean).pop() || "~";

async function api(path, opts = {}) {
  const res = await fetch(path, { ...opts, headers: { "X-Pano-Token": TOKEN, ...(opts.headers || {}) } });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = new Error(body.error || res.statusText);
    err.status = res.status;
    throw err;
  }
  return body;
}
const post = (path, data) => api(path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(data ?? {}) });

// Per-viewer conveniences only (last folder and model); storage may be missing.
const store = {
  get(k) { try { return localStorage.getItem(k); } catch { return null; } },
  set(k, v) { try { localStorage.setItem(k, v); } catch { /* private window */ } },
};

const state = { tab: "ana", days: 30, meta: null, ov: null, sessions: [], skills: [], memory: [], runs: [], known: new Map(), runSel: null, office: null };
const VIEWS = { ana() {}, kullanim() {}, kayitlar() {} };
const ON_BOOT = [];
const RENDER = {};

function toast(text, { kind = "", onclick } = {}) {
  const el = h("button", { type: "button", class: "toast " + kind, onclick: () => { el.remove(); onclick?.(); } }, text);
  $("#toasts").append(el);
  setTimeout(() => el.remove(), 7000);
}
async function copy(text, msg) {
  try {
    await navigator.clipboard.writeText(text);
    toast(msg, { kind: "ok" });
  } catch {
    toast(text);
  }
}

// fitList hides the rows that would not fit, so panels never scroll the page;
// it returns how many rows it hid.
function fitList(ul) {
  const kids = [...ul.children];
  kids.forEach((li) => (li.hidden = false));
  let hidden = 0;
  for (const li of kids) {
    if (li.offsetTop + li.offsetHeight > ul.clientHeight) {
      li.hidden = true;
      hidden++;
    }
  }
  return hidden;
}

const facts = (rows) => h("dl", { class: "facts" }, rows.filter(Boolean).flatMap(([k, v]) => [h("dt", {}, k), h("dd", {}, v)]));

function openDrawer(kind, item) {
  $("#drawer-body").replaceChildren(...[RENDER[kind](item)].flat().filter(Boolean));
  $("#drawer").hidden = false;
  $("#drawer-x").focus();
}
function closeDrawer() {
  $("#drawer").hidden = true;
}

function renderStats() {
  const ov = state.ov;
  if (!ov) return;
  const w = ov.windows;
  $("#stats").replaceChildren(
    h("span", {}, "5 sa ", h("b", {}, tok(w.fiveHour)), " · " + usd(w.fiveCost)),
    h("span", {}, "7 gün ", h("b", {}, usd(w.sevenCost))),
    h("span", {}, "önbellek ", h("b", {}, pct(ov.totals.cacheHit))));
  $("#demo").hidden = !ov.demo;
}
async function loadUsage() {
  state.ov = await api(`/api/overview?days=${state.days}`);
  renderStats();
  if (state.tab === "kullanim") VIEWS.kullanim();
}

function switchTab(tab) {
  state.tab = tab;
  for (const b of document.querySelectorAll(".tabs button")) b.setAttribute("aria-selected", String(b.dataset.tab === tab));
  for (const v of Object.keys(VIEWS)) $("#view-" + v).hidden = v !== tab;
  closeDrawer();
  VIEWS[tab]();
}

async function boot() {
  startSky($("#sky"), (hour) => ($("#clock").textContent = fmtHour(hour)));
  document.querySelectorAll(".tabs button").forEach((b) => b.addEventListener("click", () => switchTab(b.dataset.tab)));
  $("#drawer-x").addEventListener("click", closeDrawer);
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && !$("#drawer").hidden) closeDrawer();
  });
  try { state.meta = await api("/api/meta"); } catch { /* shown as empty panels */ }
  try { await loadUsage(); } catch { /* stats stay empty */ }
  for (const init of ON_BOOT) {
    try { await init(); } catch (err) { console.error("pano init", err); }
  }
  setInterval(() => !document.hidden && loadUsage().catch(() => {}), 60_000);
  let t;
  addEventListener("resize", () => {
    clearTimeout(t);
    t = setTimeout(() => VIEWS[state.tab](), 150);
  });
  switchTab("ana");
}

/* ---------- Ana: sessions, skills, memory ---------- */
function sessionRow(s) {
  const warm = s.cache === "warm";
  return h("li", {}, h("button", { type: "button", class: "sess", onclick: () => openDrawer("session", s) },
    h("span", { class: "lamp" + (warm ? " warm" : "") }),
    h("span", { class: "t" }, s.title || "(başlıksız)"),
    h("span", { class: "c" }, usd(warm ? s.nextWarm : s.nextCold), h("small", {}, warm ? `sıcak ${Math.ceil(s.cacheLeftSec / 60)} dk` : ago(s.idleSec))),
    h("span", { class: "m" }, `${s.project} · ${s.family} · ${tok(s.context)} bağlam`)));
}
function renderSessions() {
  const list = state.sessions, warm = list.filter((s) => s.cache === "warm").length;
  $("#sessions-sub").textContent = `${warm} sıcak · ${list.length - warm} soğuk`;
  $("#sessions").replaceChildren(...(list.length ? list.map(sessionRow) : [h("li", { class: "empty" }, "Henüz oturum yok.")]));
  fitList($("#sessions"));
}

function skillLabel(name) {
  const i = name.lastIndexOf(":");
  return i < 0 ? [name] : [h("i", {}, name.slice(0, i + 1)), name.slice(i + 1)];
}
function sortedSkills(q = "") {
  const query = q.trim().toLowerCase();
  return state.skills
    .filter((s) => !query || s.name.toLowerCase().includes(query) || (s.description || "").toLowerCase().includes(query))
    .sort((a, b) => b.uses - a.uses || new Date(b.lastUsed) - new Date(a.lastUsed) || a.name.localeCompare(b.name));
}
function skillRow(s, max) {
  return h("li", {}, h("button", { type: "button", class: "skill", title: s.description || s.name, onclick: () => openDrawer("skill", s) },
    h("span", { class: "n" }, ...skillLabel(s.name)),
    h("span", { class: "meter" }, h("i", { style: `width:${max ? (s.uses / max) * 100 : 0}%` })),
    h("span", { class: "u" }, s.uses ? s.uses + "×" : "—")));
}
function renderSkills() {
  const list = sortedSkills($("#skill-search").value), max = Math.max(0, ...list.map((s) => s.uses));
  $("#skills").replaceChildren(...(list.length ? list.map((s) => skillRow(s, max)) : [h("li", { class: "empty" }, "Eşleşen skill yok.")]));
  fitList($("#skills"));
  $("#skills-all").textContent = `tümü (${state.skills.length}) →`;
}

const memName = (m) => `${m.group} / ${m.label.split("/").pop()}` + (m.kind === "memory" && m.refs?.length ? ` → ${m.refs.length} not` : "");
function memRow(m) {
  const broken = (m.refs || []).some((r) => r.missing);
  return h("li", {}, h("button", { type: "button", class: "mem" + (broken ? " bad" : ""), title: broken ? "Kırık bağlantı var" : m.label, onclick: () => openDrawer("memory", m) },
    h("span", { class: "n" }, memName(m)), h("span", { class: "k" }, tok(m.tokens))));
}
function renderMemory() {
  const list = [...state.memory].sort((a, b) => b.tokens - a.tokens);
  $("#memory-sub").textContent = `${list.length} dosya`;
  $("#memory").replaceChildren(...(list.length ? list.map(memRow) : [h("li", { class: "empty" }, "CLAUDE.md bulunamadı.")]));
  const hidden = fitList($("#memory"));
  $("#memory-all").textContent = hidden ? `+ ${hidden} dosya →` : "tümü →";
}

VIEWS.ana = () => {
  renderSessions();
  renderSkills();
  renderMemory();
};

/* ---------- drawer contents ---------- */
const SCOPE = { user: "kullanıcı skill'i", project: "proje skill'i", plugin: "plugin skill'i", builtin: "yerleşik" };
const shq = (s) => "'" + s.replaceAll("'", "'\\''") + "'";
const runTitle = (r) => (r.skill ? "/" + r.skill + (r.note ? " " + r.note : "") : (r.prompt || "").split("\n")[0]);

function listDrawer(title, filter, row) {
  const ul = h("ul", { class: "list" });
  const paint = (q) => {
    const items = filter(q), max = Math.max(0, ...items.map((x) => x.uses || 0));
    ul.replaceChildren(...items.map((x) => row(x, max)));
  };
  const input = h("input", { class: "search", type: "search", placeholder: "ara…", "aria-label": "Ara", oninput: (e) => paint(e.target.value) });
  paint("");
  setTimeout(() => input.focus());
  return [h("h3", { id: "drawer-title" }, title), input, ul];
}

function runDetail(r) {
  const done = r.finished && !r.finished.startsWith("0001");
  const secs = Math.max(0, ((done ? new Date(r.finished) : new Date()) - new Date(r.started)) / 1000);
  return [
    h("h3", { id: "drawer-title" }, runTitle(r)),
    facts([
      ["Durum", { running: "çalışıyor", done: "bitti", error: "hata" }[r.status] || r.status],
      ["Klasör", r.cwd], ["Model", r.model],
      ["Süre", secs < 60 ? Math.round(secs) + " sn" : Math.round(secs / 60) + " dk"],
      ["Maliyet", usd(r.cost || 0)], ["Tur", nf.format(r.turns || 0)],
      r.report ? ["Rapor", r.report] : null,
    ]),
    r.prompt && r.prompt.includes("\n") ? h("pre", {}, r.prompt) : null,
    h("pre", { class: "output" }, r.status === "running" ? "Çalışıyor…" : r.result || "(çıktı yok)"),
  ];
}

Object.assign(RENDER, {
  session: (s) => [
    h("h3", { id: "drawer-title" }, s.title || "(başlıksız)"),
    facts([
      ["Klasör", s.cwd], ["Model", s.model],
      ["Başladı", ago(sinceISO(s.start)) + " önce"], ["Son etkinlik", ago(s.idleSec) + " önce"],
      ["Prompt", nf.format(s.prompts)], ["Bağlam", tok(s.context) + " token"],
      ["Önbellek", s.cache === "warm" ? `sıcak, ${Math.ceil(s.cacheLeftSec / 60)} dk kaldı` : "soğuk"],
      ["Sonraki mesaj", `${usd(s.nextWarm)} sıcakken, ${usd(s.nextCold)} soğukken`],
      ["Toplam", usd(s.cost)],
    ]),
    h("div", { class: "actions" }, h("button", { type: "button", class: "btn", onclick: () => copy(`cd ${shq(s.cwd)} && claude --resume ${s.id}`, "Devam komutu kopyalandı") }, "Devam et")),
  ],
  skill: (s) => [
    h("h3", { id: "drawer-title" }, "/" + s.name),
    s.description ? h("p", { class: "lead" }, s.description) : null,
    facts([
      ["Tür", `${s.kind === "command" ? "komut" : "skill"}, ${SCOPE[s.scope] || s.scope}${s.owner ? " · " + s.owner : ""}`],
      ["Boyut", s.scope === "builtin" ? "bilinmiyor (diskte değil)" : `${nf.format(s.tokens)} token açılınca, klasörüyle ${nf.format(s.dirTokens)} (${s.files} dosya)`],
      s.model ? ["Sabit model", s.model] : null,
      ["Kullanım", s.uses ? `${s.uses} kez, son: ${ago(sinceISO(s.lastUsed))} önce` : "hiç"],
      s.path ? ["Dosya", s.path] : null,
    ]),
    h("div", { class: "actions" }, h("button", { type: "button", class: "btn", onclick: () => { closeDrawer(); setPrompt("/" + s.name + " "); } }, "Çalıştır")),
  ],
  memory: (m) => [
    h("h3", { id: "drawer-title" }, m.label),
    facts([["Grup", m.group], ["Boyut", `${nf.format(m.tokens)} token, ${nf.format(m.lines)} satır`]]),
    m.headings?.length ? h("ul", { class: "tags" }, m.headings.map((x) => h("li", {}, x))) : null,
    m.refs?.length ? h("ul", { class: "refs" }, m.refs.map((r) => h("li", { class: r.missing ? "missing" : "" }, h("span", {}, r.label), h("span", {}, r.missing ? "yok" : tok(r.tokens) + " tok")))) : null,
  ],
  skills: () => listDrawer("Skill'ler", (q) => sortedSkills(q), skillRow),
  memories: () => listDrawer("Hafıza", (q) => state.memory.filter((m) => !q || m.label.toLowerCase().includes(q.toLowerCase())), memRow),
  run: runDetail,
});

/* ---------- office ---------- */
const card = (title, text, ...extra) => h("div", { class: "glass card" }, h("h3", {}, title), h("p", {}, text), ...extra);
async function loadOffice() {
  let st;
  try { st = await api("/api/office"); } catch { return; }
  const key = st.url + "|" + st.up;
  if (key === state.office) return;
  state.office = key;
  const stage = $("#stage");
  if (!st.url) return stage.replaceChildren(card("Ofis gizli", "pano -office off ile başlatıldı."));
  if (st.up) return stage.replaceChildren(h("iframe", { src: st.url, title: "agent-office-3d canlı ofis", referrerpolicy: "no-referrer" }));
  const out = h("pre", { hidden: true });
  const btn = h("button", {
    type: "button", class: "btn", onclick: async () => {
      btn.disabled = true;
      btn.textContent = "Başlatılıyor…";
      try {
        await post("/api/office/start");
        state.office = null;
        setTimeout(loadOffice, 1500);
      } catch (e) {
        out.hidden = false;
        out.textContent = e.status === 404
          ? "agent-office-3d bulunamadı. github.com/NuriOkumus/agent-office-3d (linux-macos-support dalı) indir, ~/agent-office-3d ya da ~/Projects/agent-office-3d içine koy ve install.sh çalıştır."
          : e.message;
        btn.disabled = false;
        btn.textContent = "Tekrar dene";
      }
    },
  }, "Başlat");
  stage.replaceChildren(card("Ofis kapalı", "agent-office-3d çalışmıyor.", out, btn));
}

/* ---------- prompt bar ---------- */
const picker = { items: [], sel: 0 };
function autosize() {
  const ta = $("#prompt-text");
  ta.style.height = "auto";
  ta.style.height = Math.min(ta.scrollHeight, 120) + "px";
}
function updatePicker() {
  const ul = $("#picker"), m = /^\/(\S*)$/.exec($("#prompt-text").value);
  picker.items = m ? sortedSkills(m[1]).slice(0, 8) : [];
  picker.sel = 0;
  ul.hidden = !picker.items.length;
  ul.replaceChildren(...picker.items.map((s, i) => h("li", { role: "option", "aria-selected": String(i === 0), onmousedown: (e) => { e.preventDefault(); pick(i); } },
    h("span", {}, "/" + s.name), h("small", {}, s.description || ""))));
}
function movePicker(d) {
  picker.sel = (picker.sel + d + picker.items.length) % picker.items.length;
  [...$("#picker").children].forEach((li, i) => li.setAttribute("aria-selected", String(i === picker.sel)));
}
function pick(i) {
  const s = picker.items[i];
  if (s) setPrompt("/" + s.name + " ");
}
function setPrompt(text) {
  if (state.tab !== "ana") switchTab("ana");
  const ta = $("#prompt-text");
  ta.value = text;
  ta.focus();
  ta.setSelectionRange(text.length, text.length);
  autosize();
  updatePicker();
}
function fillSelects() {
  const meta = state.meta;
  if (!meta) return;
  const projects = meta.projects || [], models = meta.models || [];
  const saved = store.get("pano.cwd"), latest = state.sessions[0]?.cwd;
  $("#prompt-cwd").replaceChildren(h("option", { value: "~" }, "📁 ~"), ...projects.map((p) => h("option", { value: p, title: p }, "📁 " + base(p))));
  $("#prompt-cwd").value = saved === "~" || projects.includes(saved) ? saved : projects.includes(latest) ? latest : "~";
  const savedModel = store.get("pano.model");
  $("#prompt-model").replaceChildren(...models.map((m) => h("option", { value: m }, m)));
  $("#prompt-model").value = models.includes(savedModel) ? savedModel : models.includes("sonnet") ? "sonnet" : models[0];
}
async function sendPrompt(e) {
  e.preventDefault();
  const text = $("#prompt-text").value.trim();
  if (!text) return;
  const cwd = $("#prompt-cwd").value, model = $("#prompt-model").value;
  let body = { prompt: text, cwd, model };
  const m = /^\/(\S+)\s*([\s\S]*)$/.exec(text);
  if (m) {
    if (!state.skills.some((s) => s.name === m[1])) return toast(`Bilinmeyen skill: /${m[1]}`, { kind: "err" });
    body = { skill: m[1], note: m[2].trim(), cwd, model };
  }
  const go = $("#prompt .go");
  go.disabled = true;
  try {
    const run = await post("/api/run", body);
    state.known.set(run.id, run.status);
    $("#prompt-text").value = "";
    autosize();
    toast(`Gönderildi · ${base(run.cwd)} · ${run.model}`, { kind: "ok" });
    loadRuns();
  } catch (err) {
    toast(err.status === 429 ? "İki iş zaten çalışıyor, biri bitsin." : "Gönderilemedi: " + err.message, { kind: "err" });
  } finally {
    go.disabled = false;
  }
}

/* ---------- runs ---------- */
function renderJobs() {
  const n = state.runs.filter((r) => r.status === "running").length;
  $("#jobs").hidden = !n;
  $("#jobs").textContent = `${n} iş çalışıyor`;
}
async function loadRuns() {
  clearTimeout(loadRuns.t);
  if (!document.hidden) {
    try {
      const runs = await api("/api/runs");
      for (const r of runs) {
        if (state.known.get(r.id) === "running" && r.status !== "running") {
          toast(r.status === "done" ? `Bitti · ${usd(r.cost || 0)} · ${r.turns} tur` : "İş hata verdi: " + runTitle(r).slice(0, 40),
            { kind: r.status === "done" ? "ok" : "err", onclick: () => openDrawer("run", r) });
        }
        state.known.set(r.id, r.status);
      }
      state.runs = runs;
      renderJobs();
      if (state.tab === "kayitlar") VIEWS.kayitlar();
    } catch { /* next poll */ }
  }
  loadRuns.t = setTimeout(loadRuns, state.runs.some((r) => r.status === "running") ? 3000 : 15000);
}

/* ---------- loading ---------- */
async function loadMain(force) {
  if (document.hidden && !force) return;
  try {
    const [sessions, skills] = await Promise.all([api("/api/sessions?limit=30"), api("/api/skills")]);
    state.sessions = sessions;
    state.skills = skills;
    if (state.tab === "ana") { renderSessions(); renderSkills(); }
  } catch { /* next poll */ }
}
async function loadMemory(force) {
  if (document.hidden && !force) return;
  try {
    state.memory = await api("/api/memory");
    if (state.tab === "ana") renderMemory();
  } catch { /* next poll */ }
}

ON_BOOT.push(async () => {
  $("#skill-search").addEventListener("input", renderSkills);
  $("#skills-all").addEventListener("click", () => openDrawer("skills"));
  $("#memory-all").addEventListener("click", () => openDrawer("memories"));
  $("#jobs").addEventListener("click", () => switchTab("kayitlar"));
  $("#prompt").addEventListener("submit", sendPrompt);
  $("#prompt-cwd").addEventListener("change", (e) => store.set("pano.cwd", e.target.value));
  $("#prompt-model").addEventListener("change", (e) => store.set("pano.model", e.target.value));
  const ta = $("#prompt-text");
  ta.addEventListener("input", () => { autosize(); updatePicker(); });
  ta.addEventListener("blur", () => setTimeout(() => ($("#picker").hidden = true), 150));
  ta.addEventListener("keydown", (e) => {
    const open = !$("#picker").hidden;
    if (open && (e.key === "ArrowDown" || e.key === "ArrowUp")) { e.preventDefault(); movePicker(e.key === "ArrowDown" ? 1 : -1); return; }
    if (open && (e.key === "Enter" || e.key === "Tab")) { e.preventDefault(); pick(picker.sel); return; }
    if (e.key === "Escape") { $("#picker").hidden = true; return; }
    if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); $("#prompt").requestSubmit(); }
  });
  document.addEventListener("keydown", (e) => {
    const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement?.tagName || "");
    if (e.key === "/" && !typing && !e.ctrlKey && !e.metaKey && !e.altKey) { e.preventDefault(); setPrompt("/"); }
  });
  document.addEventListener("visibilitychange", () => { if (!document.hidden) { loadMain(); loadRuns(); } });
  await Promise.all([loadMain(true), loadMemory(true)]);
  fillSelects();
  loadRuns();
  loadOffice();
  setInterval(() => loadMain(), 10_000);
  setInterval(() => loadMemory(), 60_000);
  setInterval(() => !document.hidden && loadOffice(), 20_000);
});

/* ---------- Kullanım ---------- */
const FAM = { fable: "#c5a3ff", opus: "#e08a4f", sonnet: "#9fb4d8", haiku: "#8fd6b4", other: "#c9c6cf" };
const famOrder = ["fable", "opus", "sonnet", "haiku", "other"];
const DAYS = ["Pzt", "Sal", "Çar", "Per", "Cum", "Cmt", "Paz"];
function niceStep(x) { const p = 10 ** Math.floor(Math.log10(x || 1)); const m = x / p; return (m <= 1 ? 1 : m <= 2 ? 2 : m <= 5 ? 5 : 10) * p; }
function fmtDate(iso) { return new Date(iso + "T12:00:00").toLocaleDateString("tr-TR", { day: "numeric", month: "short" }); }

function renderDaily(days, fams) {
  const box = $("#daily");
  const W = Math.max(300, Math.round(box.clientWidth)), H = Math.max(120, Math.round(box.clientHeight)), padL = 44, padB = 22, padT = 8;
  const max = Math.max(1, ...days.map((d) => fams.reduce((a, f) => a + (d.tokens[f] || 0), 0)));
  const step = niceStep(max / 3), top = Math.ceil(max / step) * step;
  const s = svg("svg", { viewBox: `0 0 ${W} ${H}`, role: "img", "aria-label": `Günlük token kullanımı, en yüksek gün ${tok(max)} token` });
  for (let v = 0; v <= top; v += step) {
    const y = H - padB - (v / top) * (H - padB - padT);
    s.append(svg("line", { x1: padL, x2: W, y1: y, y2: y, stroke: "rgba(255,255,255,.1)", "stroke-width": 1 }));
    const tx = svg("text", { x: padL - 6, y: y + 4, "text-anchor": "end", class: "axis" });
    tx.textContent = tok(v);
    s.append(tx);
  }
  const bw = (W - padL) / days.length, every = Math.max(1, Math.ceil(days.length / 5));
  days.forEach((d, i) => {
    let y = H - padB;
    const x = padL + i * bw + bw * 0.16, w = bw * 0.68;
    const g = svg("g"), title = svg("title");
    const total = fams.reduce((a, f) => a + (d.tokens[f] || 0), 0);
    title.textContent = `${fmtDate(d.date)}: ${tok(total)} token, ${usd(d.cost)}`;
    g.append(title);
    for (const f of fams) {
      const v = d.tokens[f] || 0;
      if (!v) continue;
      const hgt = (v / top) * (H - padB - padT);
      y -= hgt;
      g.append(svg("rect", { x, y, width: w, height: Math.max(hgt, 0.5), fill: FAM[f], rx: 1.5 }));
    }
    s.append(g);
    if ((days.length - 1 - i) % every === 0) {
      const cx = x + w / 2, anchor = cx > W - 24 ? "end" : "middle";
      const tx = svg("text", { x: anchor === "end" ? W - 1 : cx, y: H - 6, "text-anchor": anchor, class: "axis" });
      tx.textContent = fmtDate(d.date);
      s.append(tx);
    }
  });
  box.replaceChildren(s);
}
function renderLamps(hm) {
  const max = Math.max(1, ...hm.flat());
  const kids = [h("span")];
  for (let hr = 0; hr < 24; hr++) kids.push(h("span", {}, hr % 6 === 0 ? String(hr).padStart(2, "0") : ""));
  hm.forEach((row, d) => {
    kids.push(h("span", {}, DAYS[d]));
    row.forEach((v, hr) => kids.push(h("span", { class: "cell" + (v ? " on" : ""), style: `--v:${(v / max).toFixed(3)}`, title: `${DAYS[d]} ${String(hr).padStart(2, "0")}:00, ${v} istek` })));
  });
  $("#lamps").replaceChildren(...kids);
}
function bars(sel, list) {
  const max = Math.max(1, ...list.map((x) => x.count));
  $(sel).replaceChildren(...(list.length
    ? list.map((x) => h("li", {}, h("span", { class: "t", title: x.name }, x.name), h("span", { class: "meter" }, h("i", { style: `width:${(x.count / max) * 100}%` })), h("span", { class: "n" }, nf.format(x.count))))
    : [h("li", { class: "empty" }, "Bu dönemde kayıt yok.")]));
  fitList($(sel));
}
VIEWS.kullanim = () => {
  const ov = state.ov;
  if (!ov) return;
  const t = ov.totals, w = ov.windows;
  const cells = [
    ["Son 5 saat", tok(w.fiveHour) + " tok", usd(w.fiveCost) + " API fiyatıyla"],
    ["Son 7 gün", tok(w.sevenDay) + " tok", usd(w.sevenCost) + " API fiyatıyla"],
    [`${ov.days} günlük maliyet`, usd(t.cost), "tahmini, faturan değil"],
    ["Önbellek isabeti", pct(t.cacheHit), "girdinin önbellekten okunan payı"],
    ["Oturum", nf.format(t.sessions), nf.format(t.prompts) + " prompt"],
    ["Alt ajan payı", pct(t.subagentShare), nf.format(t.calls) + " istek"],
  ];
  $("#sum").replaceChildren(...cells.map(([k, v, s]) => h("div", {}, h("dt", {}, k), h("dd", {}, v, h("small", {}, s)))));
  const fams = famOrder.filter((f) => ov.daily.some((d) => d.tokens[f]));
  const totalBy = Object.fromEntries(fams.map((f) => [f, ov.daily.reduce((a, d) => a + (d.tokens[f] || 0), 0)]));
  const all = Object.values(totalBy).reduce((a, b) => a + b, 0) || 1;
  $("#legend").replaceChildren(...fams.map((f) => h("li", {}, h("i", { style: `background:${FAM[f]}` }), `${f} ${pct(totalBy[f] / all)}`)));
  renderDaily(ov.daily, fams);
  renderLamps(ov.heatmap);
  bars("#tools", ov.tools);
  bars("#projects", ov.projects);
  $("#usage-note").textContent = `Son ${ov.days} gün. Tutarlar API liste fiyatıyla tahmin; abonelikte gerçek maliyetin bu değil.`;
  document.querySelectorAll("#days button").forEach((b) => b.setAttribute("aria-pressed", String(+b.dataset.days === state.days)));
};
ON_BOOT.push(() => {
  document.querySelectorAll("#days button").forEach((b) => b.addEventListener("click", () => {
    state.days = +b.dataset.days;
    loadUsage().catch(() => toast("Kullanım yüklenemedi", { kind: "err" }));
  }));
});

/* ---------- Kayıtlar ---------- */
VIEWS.kayitlar = () => {
  const ul = $("#runs");
  $("#runs-sub").textContent = `${state.runs.length} iş`;
  if (!state.runs.length) {
    ul.replaceChildren(h("li", { class: "empty" }, "Henüz iş yok. Ana sayfadaki kutudan bir iş ver."));
    $("#run-detail").replaceChildren(h("p", { class: "empty" }, "Soldan bir iş seç."));
    return;
  }
  if (!state.runs.some((r) => r.id === state.runSel)) state.runSel = state.runs[0].id;
  ul.replaceChildren(...state.runs.map((r) => h("li", {}, h("button", {
    type: "button", class: "run", "aria-current": String(r.id === state.runSel),
    onclick: () => { state.runSel = r.id; VIEWS.kayitlar(); },
  },
    h("span", { class: "lamp" + (r.status === "running" ? " running" : r.status === "error" ? " error" : "") }),
    h("span", { class: "t" }, runTitle(r)),
    h("span", { class: "c" }, usd(r.cost || 0)),
    h("span", { class: "m" }, `${base(r.cwd)} · ${r.model} · ${ago(sinceISO(r.started))} önce`)))));
  $("#run-detail").replaceChildren(...runDetail(state.runs.find((r) => r.id === state.runSel)).filter(Boolean));
};

boot();
