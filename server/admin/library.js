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

// Navigation state lives in the hash: #folder=<path>&item=<id>. Picking a folder pushes
// a history entry, so back and forward walk the folders; the item view replaces it.
const form = document.getElementById("filters");
const folderInput = document.getElementById("folder-input");
const dialog = document.getElementById("item");

function hashParams() { return new URLSearchParams(location.hash.slice(1)); }
function setHash(params, push) {
  const h = params.toString();
  const url = h ? "#" + h : location.pathname;
  if (push) history.pushState(null, "", url); else history.replaceState(null, "", url);
}
function browse(folder) {
  folderInput.value = folder;
  htmx.trigger(form, "browse");
}
function goFolder(folder) {
  const p = hashParams();
  p.delete("item");
  if (folder) p.set("folder", folder); else p.delete("folder");
  setHash(p, true);
  form.elements.q.value = ""; // a new folder starts unsearched
  browse(folder);
}

// Folder links: tree, breadcrumb and folder tiles all carry data-folder.
document.addEventListener("click", (e) => {
  const twisty = e.target.closest(".twisty");
  if (twisty && twisty.tagName === "BUTTON") {
    const li = twisty.closest("li");
    li.setAttribute("aria-expanded", li.getAttribute("aria-expanded") === "true" ? "false" : "true");
    return;
  }
  const a = e.target.closest("a[data-folder]");
  if (!a) return;
  e.preventDefault();
  goFolder(a.dataset.folder);
});
window.addEventListener("popstate", () => {
  if (dialog.open) dialog.close();
  browse(hashParams().get("folder") || "");
});

// Show the current folder in the tree: highlight it and open its ancestors.
function markTree() {
  const folder = folderInput.value;
  document.querySelectorAll("#tree a[aria-current]").forEach((a) => a.removeAttribute("aria-current"));
  const li = [...document.querySelectorAll("#tree li[data-path]")].find((el) => el.dataset.path === folder);
  if (!li) return;
  li.querySelector(":scope > .node > a").setAttribute("aria-current", "page");
  for (let up = li.parentElement.closest("li"); up; up = up.parentElement.closest("li")) {
    up.setAttribute("aria-expanded", "true");
  }
  li.querySelector(":scope > .node").scrollIntoView({ block: "nearest" });
}

document.addEventListener("htmx:afterSwap", (e) => {
  const t = e.detail.target;
  if (t.id === "tree") {
    // Top-level folders start open.
    t.querySelectorAll(".tree-root > li[aria-expanded]").forEach((li) => li.setAttribute("aria-expanded", "true"));
    markTree();
  } else if (t.id === "results") {
    // The server resolves the folder (e.g. a single media root is opened directly).
    const head = t.querySelector(".browse-head");
    if (head) {
      folderInput.value = head.dataset.folder;
      const name = head.dataset.folder.split("/").pop();
      form.elements.q.placeholder = name ? `Search in ${name}` : "Search all folders";
      markTree();
    }
  } else if (t === dialog) {
    if (!dialog.open) dialog.showModal();
    const p = hashParams();
    p.set("item", e.detail.pathInfo.requestPath.split("/").pop());
    setHash(p, false);
  }
});

// The item view opens in a dialog. Click on the backdrop closes it.
dialog.addEventListener("click", (e) => { if (e.target === dialog) dialog.close(); });
dialog.addEventListener("close", () => {
  dialog.innerHTML = "";
  const p = hashParams();
  p.delete("item");
  setHash(p, false);
});

// admin.js fires "refresh" once the token check passes, and again after each action.
// The first one opens the linked folder (and item); later ones reload the current folder.
let started = false;
document.body.addEventListener("refresh", () => {
  if (started) {
    browse(folderInput.value);
    // An action from the item view (e.g. Mark as watched): show its new state.
    const open = hashParams().get("item");
    if (dialog.open && open && /^[0-9a-f]+$/.test(open)) {
      htmx.ajax("GET", "/admin/f/item/" + open, { target: dialog });
    }
    return;
  }
  started = true;
  const p = hashParams();
  browse(p.get("folder") || "");
  const item = p.get("item");
  if (item && /^[0-9a-f]+$/.test(item)) {
    htmx.ajax("GET", "/admin/f/item/" + item, { target: dialog });
  }
});
