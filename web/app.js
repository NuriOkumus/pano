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
  for (const init of ON_BOOT) await init();
  setInterval(() => !document.hidden && loadUsage().catch(() => {}), 60_000);
  let t;
  addEventListener("resize", () => {
    clearTimeout(t);
    t = setTimeout(() => VIEWS[state.tab](), 150);
  });
  switchTab("ana");
}

boot();
