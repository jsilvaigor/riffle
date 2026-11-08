const KEY = "riffle.adminToken";
const store = {
  get() { try { return localStorage.getItem(KEY) || ""; } catch (e) { return ""; } },
  set(v) { try { localStorage.setItem(KEY, v); } catch (e) {} },
  clear() { try { localStorage.removeItem(KEY); } catch (e) {} },
};
const $ = (id) => document.getElementById(id);
let authRequired = true;
let toastTimer;

function toast(msg, ok) {
  $("toast").innerHTML = "";
  const div = document.createElement("div");
  div.className = "toast" + (ok ? "" : " err");
  if (!ok) div.insertAdjacentHTML("beforeend",
    '<svg class="icon" aria-hidden="true"><use href="/admin/assets/icons.svg#alert"/></svg>');
  div.append(msg);
  $("toast").appendChild(div);
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { $("toast").innerHTML = ""; }, 8000);
}
function showLogin(msg) {
  $("login").hidden = false;
  $("panels").hidden = true;
  if (msg) $("login-msg").textContent = msg;
  $("token-input").focus();
}
function showPanels() {
  $("login").hidden = true;
  $("panels").hidden = false;
  htmx.trigger(document.body, "refresh");
}

document.addEventListener("htmx:configRequest", (e) => {
  const t = store.get();
  if (t) e.detail.headers["Authorization"] = "Bearer " + t;
});
// Don't poll with no token; the login form is showing.
document.addEventListener("htmx:beforeRequest", (e) => {
  if (authRequired && !store.get()) e.preventDefault();
});
document.addEventListener("htmx:responseError", (e) => {
  const status = e.detail.xhr.status;
  if (status === 401) {
    store.clear();
    showLogin("That token was rejected. Paste the server's RIFFLE_TOKEN.");
  } else {
    toast("The server answered " + status + ". Check its logs.", false);
  }
});
document.addEventListener("htmx:sendError", () => {
  toast("Can't reach Riffle Server. Check that it's running.", false);
});
document.addEventListener("htmx:afterSwap", (e) => {
  if (e.detail.target.id === "toast") {
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { $("toast").innerHTML = ""; }, 8000);
  }
});

$("login-form").addEventListener("submit", (e) => {
  e.preventDefault();
  store.set($("token-input").value.trim());
  $("token-input").value = "";
  showPanels();
});
$("token-btn").addEventListener("click", () => {
  store.clear();
  showLogin("Paste the server's RIFFLE_TOKEN. It's kept in this browser only.");
});

fetch("/api/health").then((r) => r.json()).then((h) => {
  authRequired = h.authRequired;
  $("token-btn").hidden = !authRequired;
  if (authRequired && !store.get()) showLogin();
  else showPanels();
}).catch(() => {
  $("panels").hidden = true;
  $("offline").hidden = false;
});
