// Web player: plays the original file from /api/items/{id}/stream in a <video> element
// (no transcoding, so only formats the browser decodes play), with a scrub bar whose
// preview comes from the trickplay sheets, like the app's (design/player.md, Scrubbing).
// Resume positions and watch state go through the same progress API as the app.
// Loads after admin.js, which defines `store` (the token), `toast` and `showLogin`.

(() => {
const id = location.pathname.split("/").pop();
const el = (x) => document.getElementById(x);
const video = el("video"), player = el("player"), chrome = el("chrome");
const timeline = el("timeline"), preview = el("preview"), pframe = el("preview-frame");

function withToken(url) {
  const t = store.get();
  return t ? url + (url.includes("?") ? "&" : "?") + "token=" + encodeURIComponent(t) : url;
}
function timecode(sec) {
  const t = Math.max(0, Math.floor(sec || 0));
  const h = Math.floor(t / 3600), m = Math.floor(t / 60) % 60, s = String(t % 60).padStart(2, "0");
  return h ? `${h}:${String(m).padStart(2, "0")}:${s}` : `${m}:${s}`;
}
async function api(path, opts = {}) {
  const t = store.get();
  const headers = Object.assign({}, opts.headers, t ? { Authorization: "Bearer " + t } : {});
  const res = await fetch(path, Object.assign({}, opts, { headers }));
  if (res.status === 401) {
    store.clear();
    showLogin("That token was rejected. Paste the server's RIFFLE_TOKEN.");
    throw new Error("unauthorized");
  }
  return res;
}
function useIcon(btn, name, label) {
  btn.querySelector("use").setAttribute("href", "/admin/assets/icons.svg#" + name);
  btn.setAttribute("aria-label", label);
}

// What this browser can decode, so a file it can't play says so up front rather than
// showing a black frame. Codec names are ffprobe's.
const videoTypes = {
  h264: 'video/mp4; codecs="avc1.640028"', hevc: 'video/mp4; codecs="hvc1.1.6.L120.90"',
  vp8: 'video/webm; codecs="vp8"', vp9: 'video/webm; codecs="vp9"',
  av1: 'video/mp4; codecs="av01.0.08M.08"', theora: 'video/ogg; codecs="theora"',
};
const audioTypes = {
  aac: 'audio/mp4; codecs="mp4a.40.2"', mp3: "audio/mpeg", opus: 'audio/webm; codecs="opus"',
  vorbis: 'audio/webm; codecs="vorbis"', flac: "audio/flac", alac: 'audio/mp4; codecs="alac"',
  ac3: 'audio/mp4; codecs="ac-3"', eac3: 'audio/mp4; codecs="ec-3"',
};
const canPlay = (types, codec) => !!types[codec] && video.canPlayType(types[codec]) !== "";
const containers = { mp4: 1, m4v: 1, mov: 1, webm: 1, mkv: 1, ogv: 1 };

let item, manifest, duration = 0;

function showMessage(html) {
  const msg = el("player-msg");
  msg.innerHTML = html;
  msg.hidden = false;
  el("spinner").hidden = true;
  return msg;
}
function cantPlay(why, tryAnyway) {
  const msg = showMessage(`<svg class="icon" aria-hidden="true"><use href="/admin/assets/icons.svg#alert"/></svg>
    <h3>This browser can't play this video</h3><p></p>`);
  msg.querySelector("p").textContent = why + " Riffle Server doesn't convert files; watch it in the Riffle app.";
  if (tryAnyway) {
    const b = document.createElement("button");
    b.type = "button";
    b.textContent = "Try anyway";
    b.onclick = () => { msg.hidden = true; video.play(); };
    msg.appendChild(b);
  }
}

async function start() {
  if (!/^[0-9a-f]+$/.test(id)) { el("not-found").hidden = false; return; }
  let res;
  try { res = await api("/api/items/" + id); } catch { return; }
  if (!res.ok) { el("not-found").hidden = false; return; }
  item = await res.json();

  const folder = item.path.includes("/") ? item.root + "/" + item.path.slice(0, item.path.lastIndexOf("/")) : item.root;
  const back = "/admin/library#" + new URLSearchParams({ folder });
  el("back").href = back;
  el("library-link").href = back;
  document.title = item.name + " · Riffle Server";
  el("title").textContent = item.name;
  el("where").textContent = item.root + "/" + item.path;

  const p = item.probe, v = p && p.video;
  const audio = p && (p.audio || []).find((a) => a.default) || (p && p.audio || [])[0];
  duration = (p && p.duration) || 0;
  const ext = item.name.split(".").pop().toLowerCase();
  el("specs").textContent = [
    v && `${v.displayWidth || v.width}×${v.displayHeight || v.height}`, v && v.codec,
    audio && `${audio.codec} ${audio.channels} ch`, ext.toUpperCase(), duration && timecode(duration),
  ].filter(Boolean).join(" · ");

  // Size the stage to the video so the page doesn't jump when it loads; portrait videos
  // play upright with bars at the sides (they're capped by height in the CSS).
  if (v) player.style.aspectRatio = `${v.displayWidth || v.width} / ${v.displayHeight || v.height}`;
  player.hidden = false;
  el("info").hidden = false;
  el("dur").textContent = timecode(duration);
  timeline.setAttribute("aria-valuemax", Math.round(duration));

  if (item.trickplay === "ready") video.poster = withToken(`/api/items/${id}/poster.jpg`);
  video.src = withToken(`/api/items/${id}/stream`);
  loadManifest();
  resume();

  if (v && !canPlay(videoTypes, v.codec)) {
    cantPlay(`Its video is ${v.codec}${containers[ext] ? "" : ", in " + ext.toUpperCase()}.`, true);
  } else if (audio && !canPlay(audioTypes, audio.codec)) {
    toast(`This browser may not play ${audio.codec} audio, so the video could be silent.`, false);
  }
  player.focus({ preventScroll: true });
}

// Resume where the app (or this page) left off; a watched video starts over.
async function resume() {
  let all;
  try { all = await (await api("/api/progress")).json(); } catch { return; }
  const pr = all[id];
  if (!pr || pr.state !== "partial" || !(pr.position > 5)) return;
  const at = pr.position;
  const apply = () => { if (video.currentTime < 1) video.currentTime = at; };
  if (video.readyState >= 1) apply(); else video.addEventListener("loadedmetadata", apply, { once: true });
  el("restart").hidden = false;
  toast(`Resuming at ${timecode(at)}.`, true);
}
el("restart").addEventListener("click", () => {
  video.currentTime = 0;
  video.play();
  el("restart").hidden = true;
});

// ---- Trickplay preview ----------------------------------------------------------
let sheets = [], loaded = [], scale = 1;
async function loadManifest() {
  for (;;) {
    let res;
    try { res = await api(`/api/items/${id}/trickplay/manifest.json`); } catch { return; }
    if (res.status === 202) { // being made, and moved up the queue: try again shortly
      await new Promise((r) => setTimeout(r, (+res.headers.get("Retry-After") || 5) * 1000));
      continue;
    }
    if (!res.ok) return;
    manifest = await res.json();
    break;
  }
  const m = manifest;
  scale = Math.min(200 / m.tileWidth, 150 / m.tileHeight, 1.5);
  pframe.style.width = m.tileWidth * scale + "px";
  pframe.style.height = m.tileHeight * scale + "px";
  pframe.style.backgroundSize = `${m.columns * m.tileWidth * scale}px ${m.rows * m.tileHeight * scale}px`;
  // Fetch every sheet now (a few MB for a long film), four at a time, so the preview
  // never waits on the network while scrubbing.
  sheets = Array.from({ length: m.sheets }, (_, i) => withToken(`/api/items/${id}/trickplay/${i}.jpg`));
  let next = 0;
  const worker = async () => {
    while (next < sheets.length) {
      const i = next++;
      await new Promise((done) => {
        const img = new Image();
        img.onload = () => { loaded[i] = true; done(); };
        img.onerror = done;
        img.src = sheets[i];
      });
    }
  };
  for (let i = 0; i < 4; i++) worker();
}

function showPreview(t) {
  el("preview-time").textContent = timecode(t);
  preview.hidden = false;
  const m = manifest;
  const note = pframe.querySelector(".note");
  if (!m) { pframe.style.backgroundImage = ""; note.hidden = false; pframe.classList.add("none"); return; }
  pframe.classList.remove("none");
  const k = Math.max(0, Math.min(m.count - 1, Math.floor(t / m.interval)));
  const per = m.columns * m.rows, sheet = Math.floor(k / per), cell = k % per;
  note.hidden = !!loaded[sheet];
  pframe.style.backgroundImage = loaded[sheet] ? `url("${sheets[sheet]}")` : "";
  pframe.style.backgroundPosition =
    `-${(cell % m.columns) * m.tileWidth * scale}px -${Math.floor(cell / m.columns) * m.tileHeight * scale}px`;
}

// ---- Scrubbing -------------------------------------------------------------------
const total = () => (isFinite(video.duration) && video.duration > 0 ? video.duration : duration);
let dragging = false, dragT = 0, lastSeek = 0;

function timeAt(e) {
  const r = timeline.getBoundingClientRect();
  return Math.max(0, Math.min(1, (e.clientX - r.left) / r.width)) * total();
}
function placePreview(t) {
  const w = timeline.clientWidth, pw = preview.offsetWidth;
  const x = total() ? (t / total()) * w : 0;
  preview.style.left = Math.max(0, Math.min(w - pw, x - pw / 2)) + "px";
}
function drawPosition(t) {
  const pct = total() ? (t / total()) * 100 : 0;
  el("played").style.width = pct + "%";
  el("thumb").style.left = pct + "%";
  el("cur").textContent = timecode(t);
  timeline.setAttribute("aria-valuenow", Math.round(t));
  timeline.setAttribute("aria-valuetext", timecode(t));
}
// While dragging, follow with keyframe seeks no more often than every 120 ms, and
// only once the previous one finished: never queue seeks, the preview leads.
function liveSeek(t) {
  const now = performance.now();
  if (video.seeking || now - lastSeek < 120) return;
  lastSeek = now;
  if (video.fastSeek) video.fastSeek(t); else video.currentTime = t;
}

timeline.addEventListener("pointerdown", (e) => {
  if (e.button !== 0) return;
  timeline.setPointerCapture(e.pointerId);
  dragging = true;
  player.classList.add("scrubbing");
  dragT = timeAt(e);
  drawPosition(dragT); showPreview(dragT); placePreview(dragT);
  liveSeek(dragT);
});
timeline.addEventListener("pointermove", (e) => {
  const t = timeAt(e);
  if (dragging) { dragT = t; drawPosition(t); liveSeek(t); }
  showPreview(t); placePreview(t);
});
function endDrag() {
  if (!dragging) return;
  dragging = false;
  player.classList.remove("scrubbing");
  video.currentTime = dragT; // precise seek on release; playing stays playing
  preview.hidden = true;
}
timeline.addEventListener("pointerup", endDrag);
timeline.addEventListener("pointercancel", endDrag);
timeline.addEventListener("pointerleave", () => { if (!dragging) preview.hidden = true; });

// ---- Transport -------------------------------------------------------------------
const toggle = () => (video.paused || video.ended ? video.play() : video.pause());
function skip(by) {
  video.currentTime = Math.max(0, Math.min(total(), video.currentTime + by));
  poke();
}
el("play-btn").addEventListener("click", toggle);
video.addEventListener("click", toggle);
video.addEventListener("dblclick", () => fullscreen());
video.addEventListener("play", () => { useIcon(el("play-btn"), "pause", "Pause"); poke(); });
video.addEventListener("pause", () => { useIcon(el("play-btn"), "play", "Play"); poke(); });
video.addEventListener("timeupdate", () => { if (!dragging) drawPosition(video.currentTime); });
video.addEventListener("durationchange", () => { el("dur").textContent = timecode(total()); });
video.addEventListener("progress", () => {
  const b = video.buffered;
  for (let i = 0; i < b.length; i++) {
    if (b.start(i) <= video.currentTime + 0.5 && b.end(i) >= video.currentTime) {
      el("buffered").style.left = (b.start(i) / total()) * 100 + "%";
      el("buffered").style.width = ((b.end(i) - b.start(i)) / total()) * 100 + "%";
    }
  }
});
video.addEventListener("waiting", () => { el("spinner").hidden = false; });
video.addEventListener("seeking", () => { if (!dragging) el("spinner").hidden = false; });
for (const ev of ["playing", "seeked", "canplay", "pause"]) {
  video.addEventListener(ev, () => { el("spinner").hidden = true; });
}
video.addEventListener("error", () => {
  const e = video.error;
  if (!e || e.code === e.MEDIA_ERR_ABORTED) return;
  if (e.code === e.MEDIA_ERR_NETWORK) {
    showMessage(`<h3>Lost the connection</h3><p>The server stopped sending the video. Reload to try again.</p>`);
    return;
  }
  const v = item && item.probe && item.probe.video;
  cantPlay(v ? `Its video is ${v.codec}, in ${item.name.split(".").pop().toUpperCase()}.` : "");
});
// Some browsers play the sound of a video they can't decode (H.265 without hardware
// support): say so instead of showing a black frame.
video.addEventListener("loadedmetadata", () => {
  const v = item && item.probe && item.probe.video;
  if (v && video.videoWidth === 0) cantPlay(`It plays sound only here: its video is ${v.codec}.`);
});

el("mute-btn").addEventListener("click", () => { video.muted = !video.muted; });
el("volume").addEventListener("input", (e) => { video.volume = +e.target.value; video.muted = video.volume === 0; });
video.addEventListener("volumechange", () => {
  const off = video.muted || video.volume === 0;
  useIcon(el("mute-btn"), off ? "muted" : "volume", off ? "Unmute" : "Mute");
  el("volume").value = video.muted ? 0 : video.volume;
  try { localStorage.setItem("riffle.volume", video.volume); } catch (e) {}
});
try { const vol = localStorage.getItem("riffle.volume"); if (vol !== null) video.volume = +vol; } catch (e) {}
el("speed").addEventListener("change", (e) => { video.playbackRate = +e.target.value; });

function fullscreen() {
  if (document.fullscreenElement) document.exitFullscreen();
  else if (player.requestFullscreen) player.requestFullscreen();
  else if (video.webkitEnterFullscreen) video.webkitEnterFullscreen(); // iPhone Safari
}
el("fs-btn").addEventListener("click", fullscreen);
document.addEventListener("fullscreenchange", () => {
  const on = !!document.fullscreenElement;
  useIcon(el("fs-btn"), on ? "exit-fullscreen" : "fullscreen", on ? "Exit full screen" : "Full screen");
});

document.addEventListener("keydown", (e) => {
  if (player.hidden || e.metaKey || e.ctrlKey || e.altKey) return;
  if (e.target.closest("input, select, textarea")) return;
  const step = e.shiftKey ? 60 : 10;
  switch (e.key) {
    case " ": case "k": if (e.target.closest("button")) return; toggle(); break;
    case "ArrowLeft": skip(-step); break;
    case "ArrowRight": skip(step); break;
    case "f": fullscreen(); break;
    case "m": video.muted = !video.muted; break;
    default: return;
  }
  e.preventDefault();
});

// Chrome hides 3 s after the last mouse move while playing; it stays while paused or
// scrubbing.
let idle;
function poke() {
  player.classList.remove("idle");
  clearTimeout(idle);
  idle = setTimeout(() => {
    if (!video.paused && !dragging && !chrome.matches(":hover")) player.classList.add("idle");
  }, 3000);
}
player.addEventListener("pointermove", poke);
player.addEventListener("pointerdown", poke);

// ---- Progress --------------------------------------------------------------------
// Saved every 10 s while playing, and on pause, end and leaving the page. Nothing is
// saved until playback starts, so opening a video doesn't mark it in progress.
let played = false, lastSaved = 0;
function save() {
  if (!played || !item) return;
  lastSaved = performance.now();
  api(`/api/items/${id}/progress`, {
    method: "PUT", keepalive: true,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ position: video.currentTime, duration: isFinite(video.duration) ? video.duration : 0 }),
  }).catch(() => {});
}
video.addEventListener("playing", () => { played = true; });
video.addEventListener("timeupdate", () => {
  if (!video.paused && performance.now() - lastSaved > 10000) save();
});
video.addEventListener("pause", save);
video.addEventListener("ended", save);
window.addEventListener("pagehide", save);
document.addEventListener("visibilitychange", () => { if (document.hidden) save(); });

// admin.js fires "refresh" once the token check passes.
let started = false;
document.body.addEventListener("refresh", () => {
  if (started) return;
  started = true;
  start();
});
})();
