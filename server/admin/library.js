// Library page: posters and trickplay sheets come from the API, which needs the token.
// <img> can't send headers, so images are rendered with data-src and get ?token= here.
// Loads after admin.js, which defines `store` (the token in localStorage) and `toast`.

function withToken(url) {
  const t = store.get();
  if (!t) return url;
  return url + (url.includes("?") ? "&" : "?") + "token=" + encodeURIComponent(t);
}

function loadImages(root) {
  const imgs = root.matches?.("img[data-src]") ? [root] : root.querySelectorAll("img[data-src]");
  for (const img of imgs) {
    img.onload = () => img.classList.remove("missing");
    img.onerror = () => img.classList.add("missing"); // 202 while thumbnails are pending
    img.src = withToken(img.dataset.src);
    img.removeAttribute("data-src");
  }
}

function timecode(sec) {
  const t = Math.floor(sec);
  const h = Math.floor(t / 3600), m = Math.floor(t / 60) % 60, s = String(t % 60).padStart(2, "0");
  return h ? `${h}:${String(m).padStart(2, "0")}:${s}` : `${m}:${s}`;
}

// The scrub preview shows one tile of a sprite sheet, picked from the pointer position
// or the slider, the same arithmetic the iOS scrubber uses (see the manifest docs).
function initScrub(el) {
  const d = el.dataset;
  const m = {
    id: d.id, interval: +d.interval, cols: +d.cols, rows: +d.rows,
    tw: +d.tw, th: +d.th, count: +d.count, sheets: +d.sheets,
  };
  const perSheet = m.cols * m.rows;
  const frame = el.querySelector(".frame");
  const range = el.querySelector("input[type=range]");
  const label = el.querySelector(".t");
  range.max = m.count - 1;

  const urls = [];
  for (let i = 0; i < m.sheets; i++) {
    urls.push(withToken(`/api/items/${m.id}/trickplay/${i}.jpg`));
    new Image().src = urls[i]; // prefetch every sheet, like the app does
  }

  let scale = 1, k = Math.min(m.count - 1, Math.round(m.count * 0.1));
  function layout() {
    // Fit the tile in the column, and in 60% of the window height for tall videos.
    const maxW = el.clientWidth || m.tw;
    const maxH = window.innerHeight * 0.6;
    scale = Math.min(maxW / m.tw, maxH / m.th, 3);
    frame.style.width = m.tw * scale + "px";
    frame.style.height = m.th * scale + "px";
    frame.style.backgroundSize = `${m.cols * m.tw * scale}px ${m.rows * m.th * scale}px`;
    show(k);
  }
  function show(i) {
    k = Math.max(0, Math.min(m.count - 1, i));
    const sheet = Math.floor(k / perSheet), cell = k % perSheet;
    const x = (cell % m.cols) * m.tw * scale, y = Math.floor(cell / m.cols) * m.th * scale;
    frame.style.backgroundImage = `url("${urls[sheet]}")`;
    frame.style.backgroundPosition = `-${x}px -${y}px`;
    label.textContent = timecode(k * m.interval);
    range.value = k;
  }
  function fromPointer(e) {
    const r = frame.getBoundingClientRect();
    show(Math.floor(((e.clientX - r.left) / r.width) * m.count));
  }
  frame.addEventListener("pointermove", fromPointer);
  frame.addEventListener("pointerdown", (e) => { frame.setPointerCapture(e.pointerId); fromPointer(e); });
  range.addEventListener("input", () => show(+range.value));
  new ResizeObserver(layout).observe(el);
  layout();
}

htmx.onLoad((root) => {
  loadImages(root);
  root.querySelectorAll?.(".scrub").forEach(initScrub);
});

// The item view opens in a dialog and is linkable: /admin/library#item=<id>.
const dialog = document.getElementById("item");
function openItem(id) {
  htmx.ajax("GET", "/admin/f/item/" + encodeURIComponent(id), { target: dialog });
}
document.addEventListener("htmx:afterSwap", (e) => {
  if (e.detail.target !== dialog) return;
  if (!dialog.open) dialog.showModal();
  const id = e.detail.pathInfo.requestPath.split("/").pop();
  history.replaceState(null, "", "#item=" + id);
});
// Click on the backdrop closes the item view.
dialog.addEventListener("click", (e) => { if (e.target === dialog) dialog.close(); });
dialog.addEventListener("close", () => {
  dialog.innerHTML = "";
  history.replaceState(null, "", location.pathname);
});
// Open a linked item once the token check has run (admin.js fires "refresh").
document.body.addEventListener("refresh", () => {
  const m = location.hash.match(/^#item=([0-9a-f]+)$/);
  if (m && !dialog.open) openItem(m[1]);
}, { once: true });
