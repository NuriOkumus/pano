// The sky behind pano. It follows the local clock with the keyframes of
// agent-office-3d's daylight.js, so the office windows and the page agree.
// Sun and moon move once a minute, clouds drift on the GPU, and it all
// pauses while the tab is hidden. ?saat=18.25 pins the clock (for testing).

// hour, sky top, horizon, clouds, stars (0-1), accent, far hills, near hills
export const KEYS = [
  [0, "#060a1a", "#141d3a", "#2a3150", 1, "#ffb36b", "#0d1226", "#151b33"],
  [4.5, "#0a1130", "#2a2c55", "#3a3d63", 0.9, "#ffb36b", "#141a35", "#1c2242"],
  [6, "#3d3a72", "#f2a088", "#f3b4b8", 0.3, "#ff8fa3", "#5a3f5a", "#6e4a5e"],
  [7.5, "#5b98d6", "#ffd7a6", "#fff1e6", 0, "#ff9b4a", "#7c6a63", "#8f7766"],
  [12, "#3b8ce0", "#c4e6ff", "#ffffff", 0, "#ff9b4a", "#6f8a6a", "#7f9b74"],
  [16, "#4a8bd2", "#ffe0ae", "#fff6e8", 0, "#ff9b4a", "#7f8a62", "#958f68"],
  [18, "#5a5aa0", "#ff9658", "#ffc29a", 0.08, "#ff7a45", "#6a4a4a", "#7d5446"],
  [19.5, "#25275a", "#cf5f78", "#a06f88", 0.55, "#ff8f6b", "#2d2440", "#3a2a44"],
  [21, "#0a1130", "#1e2950", "#2a3352", 0.95, "#ffb36b", "#121833", "#19203b"],
  [24, "#060a1a", "#141d3a", "#2a3150", 1, "#ffb36b", "#0d1226", "#151b33"],
];

const rgb = (c) => [1, 3, 5].map((i) => parseInt(c.slice(i, i + 2), 16));
function mix(a, b, t) {
  const x = rgb(a), y = rgb(b);
  return `rgb(${x.map((v, i) => Math.round(v + (y[i] - v) * t)).join(",")})`;
}

// Where the sun or moon stands, in percent of the screen; null below the horizon.
function arc(t) {
  return t > 0 && t < 1 ? { x: 6 + t * 88, y: 78 - Math.sin(Math.PI * t) * 66, t } : null;
}

export function skyAt(hour) {
  const h = ((hour % 24) + 24) % 24;
  let i = 0;
  while (i < KEYS.length - 2 && h > KEYS[i + 1][0]) i++;
  const a = KEYS[i], b = KEYS[i + 1], t = (h - a[0]) / (b[0] - a[0]);
  return {
    top: mix(a[1], b[1], t), horizon: mix(a[2], b[2], t), cloud: mix(a[3], b[3], t),
    stars: a[4] + (b[4] - a[4]) * t, accent: t < 0.5 ? a[5] : b[5],
    hillFar: mix(a[6], b[6], t), hillNear: mix(a[7], b[7], t),
    sun: arc((h - 5.8) / (18.7 - 5.8)), moon: arc(((h - 19.2 + 24) % 24) / 10.5),
  };
}

export function clockHour(now = new Date()) {
  const pinned = new URLSearchParams(location.search).get("saat");
  if (pinned !== null && pinned !== "" && !Number.isNaN(+pinned)) return ((+pinned % 24) + 24) % 24;
  return now.getHours() + now.getMinutes() / 60;
}

export function fmtHour(h) {
  const m = Math.floor(h * 60) % 1440;
  return String(Math.floor(m / 60)).padStart(2, "0") + ":" + String(m % 60).padStart(2, "0");
}

const HILLS = `<svg class="hills" viewBox="0 0 1280 190" preserveAspectRatio="none">
<path class="far" d="M0 120 C 160 70 300 90 430 105 S 700 60 860 95 S 1120 70 1280 100 V190 H0 Z"/>
<path class="near" d="M0 150 C 200 115 360 140 520 132 S 820 110 1000 140 S 1180 125 1280 135 V190 H0 Z"/></svg>`;

const CLOUDS = [[60, 0.85, 140, 0], [140, 1.1, 190, -60], [40, 0.7, 120, -95], [190, 0.9, 170, -130], [100, 1.25, 230, -20]];

// startSky draws the sky into root and repaints it once a minute; onPaint(hour)
// runs after every paint (the clock in the top bar).
export function startSky(root, onPaint) {
  root.innerHTML = `<div class="grad"></div><div class="stars"></div><div class="orb sun"></div><div class="orb moon"></div><div class="clouds"></div>${HILLS}`;
  const stars = root.querySelector(".stars");
  for (let i = 0; i < 90; i++) {
    const s = document.createElement("i");
    s.style.left = Math.random() * 100 + "%";
    s.style.top = Math.random() * 62 + "%";
    s.style.animationDelay = -Math.random() * 3.5 + "s";
    if (Math.random() < 0.15) s.className = "big";
    stars.append(s);
  }
  const clouds = root.querySelector(".clouds");
  for (const [top, scale, secs, delay] of CLOUDS) {
    const c = document.createElement("div");
    c.className = "cloud";
    c.innerHTML = "<span></span>";
    Object.assign(c.style, { top: top + "px", scale: String(scale), animationDuration: secs + "s", animationDelay: delay + "s" });
    clouds.append(c);
  }
  const grad = root.querySelector(".grad"), sun = root.querySelector(".sun"), moon = root.querySelector(".moon");
  const far = root.querySelector(".far"), near = root.querySelector(".near");
  const place = (el, p) => {
    el.hidden = !p;
    if (p) Object.assign(el.style, { left: p.x + "%", top: p.y + "%" });
  };
  function paint() {
    const hour = clockHour(), s = skyAt(hour);
    grad.style.background = `linear-gradient(180deg, ${s.top} 0%, ${s.horizon} 78%)`;
    root.style.setProperty("--cloud", s.cloud);
    stars.style.opacity = s.stars.toFixed(2);
    place(sun, s.sun);
    if (s.sun) sun.style.filter = `hue-rotate(${(-25 * (1 - Math.sin(Math.PI * s.sun.t))).toFixed(1)}deg)`;
    place(moon, s.moon);
    far.setAttribute("fill", s.hillFar);
    near.setAttribute("fill", s.hillNear);
    document.documentElement.style.setProperty("--acc", s.accent);
    onPaint?.(hour);
  }
  let timer = null;
  const run = () => {
    paint();
    timer = setInterval(paint, 60_000);
  };
  document.addEventListener("visibilitychange", () => {
    root.classList.toggle("paused", document.hidden);
    if (document.hidden) {
      clearInterval(timer);
      timer = null;
    } else if (!timer) run();
  });
  run();
}
