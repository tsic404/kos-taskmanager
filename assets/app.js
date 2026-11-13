// The window document's entry: it negotiates the console channel, renders the
// shell, and resolves the four page surfaces from the address. The page modules
// are pure renderers, so the shell and its routes run in a test without a
// browser.
//
// The declared pages read their projection from the control plane (KOSD-C-12):
// the page registry decides which entries the session may see (DASH-C-22) and
// each page read decides what its items show (KOSD-C-53, KOSD-C-59).

import { createAPI } from "./api.js";
import { createChannel, consoleOriginFrom, windowIDFrom } from "./handshake.js";
import { escapeHTML } from "./html.js";
import { ROUTES, defaultHash, matchRoute, renderNav, routeAddress, isHidden } from "./router.js";
import * as applications from "./pages/applications.js";
import * as detail from "./pages/detail.js";
import * as overview from "./pages/overview.js";
import * as pods from "./pages/pods.js";

// DECLARED_PERMISSIONS mirrors the packed `App` contribution: the browser
// cannot read the manifest, so the handshake refuses a capability the package
// never declared (KOSD-C-73, KOSD-C-75).
const DECLARED_PERMISSIONS = [];

const PAGES = new Map(
  [overview, applications, detail, pods].map((page) => [page.id, page]),
);

const STATUS_LABEL = {
  pending: "等待控制台握手",
  connected: "已连接控制台",
  standalone: "独立预览：未连接控制台",
};

// HIDDEN_NOTE is what the window shows for an address naming a page the
// identity cannot see; the page itself does not appear (DASH-C-22).
export const HIDDEN_NOTE = "本页不可见：提供它的组件未生效，或当前身份看不到它。";

// renderShell draws one state of the window: the navigation, the page the
// address names, and whether the console channel is established. hiddenPages
// names the page surfaces the control plane does not serve this session;
// projection is the state of the page being read.
export function renderShell({
  hash = "",
  session = null,
  status = "pending",
  hiddenPages = [],
  projection = null,
  entryNotice = null,
} = {}) {
  const matched = matchRoute(hash) || { route: ROUTES[0], app: "" };
  const app = matched.app || "";
  const hidden = isHidden(matched.route, hiddenPages);
  const page = PAGES.get(matched.route.id) || overview;
  const body = hidden ? renderHidden() : page.render({ app, session, projection, entryNotice });
  return `
    <div class="shell">
      <header class="shell-header">
        <span class="brand">运行视图</span>
        ${renderNav(matched.route.id, app, hiddenPages)}
        <span class="status" data-status="${escapeHTML(status)}">${escapeHTML(STATUS_LABEL[status] || STATUS_LABEL.pending)}</span>
      </header>
      <main class="shell-main">${body}</main>
    </div>`;
}

// renderHidden stands in for a page the identity cannot see. It carries no page
// identity, so a hidden page and a page no component declares look the same.
function renderHidden() {
  return `<section class="page" data-page="unavailable"><p class="empty-state" data-empty="page-hidden">${escapeHTML(HIDDEN_NOTE)}</p></section>`;
}

// defaultHashFor is the address a window without one opens: the first surface
// the identity may see. A scoped surface has no address without an application,
// so it is never the default; with no page the identity may see, the window
// keeps its address and states that the page is unavailable.
function defaultHashFor(hiddenPages) {
  const route = ROUTES.find((candidate) => !isHidden(candidate, hiddenPages) && routeAddress(candidate) !== "");
  return route ? routeAddress(route) : defaultHash();
}

// createShell wires the document to the console channel, the control plane and
// the address.
export function createShell({ document, window, fetchFn = null } = {}) {
  const root = document.getElementById("shell");
  if (!root) return null;

  const search = window.location.search;
  const consoleOrigin = consoleOriginFrom(search);
  const channel = createChannel({
    hostWindow: window.parent,
    consoleOrigin,
    windowID: windowIDFrom(search),
    declaredPermissions: DECLARED_PERMISSIONS,
  });
  let status = consoleOrigin ? "pending" : "standalone";
  let api = null;
  let hiddenPages = [];
  let projectionRoute = "";
  let projection = null;
  let registryState = "idle";
  let projectionSeq = 0;
  let entryNotice = null;
  let pending = Promise.resolve();
  let writing = Promise.resolve();

  function currentRoute() {
    const matched = matchRoute(window.location.hash);
    return matched ? matched.route : null;
  }

  function draw() {
    // An address outside the table names no page, and one naming a page the
    // identity cannot see names none either: the window moves to the first page
    // it may see, so a reload and a deep link agree (DASH-C-22).
    const address = matchRoute(window.location.hash);
    if (!address || isHidden(address.route, hiddenPages)) {
      const next = defaultHashFor(hiddenPages);
      if (next !== window.location.hash) window.location.replace(next);
    }
    const matched = matchRoute(window.location.hash);
    const route = matched ? matched.route : ROUTES[0];
    // A page with a session behind it is reading even before its projection
    // lands, so the window never shows an empty page where a read is running.
    const loaded = projectionRoute === route.id ? projection : null;
    const current = loaded || (channel.session && route.page ? { state: "loading" } : null);
    root.innerHTML = renderShell({
      hash: window.location.hash,
      session: channel.session,
      status,
      hiddenPages,
      projection: current,
      entryNotice,
    });
    root.dataset.state = "ready";
    root.dataset.session = channel.session ? "established" : "none";
  }

  // schedule queues one read and keeps the returned promise for the caller that
  // waits on the window reaching its state.
  function schedule(task) {
    pending = pending.then(task);
    return pending;
  }

  // loadRegistry reads the pages the session may see; an entry the registry
  // does not carry is absent from the navigation (DASH-C-22).
  async function loadRegistry() {
    if (!api) return;
    registryState = "loading";
    draw();
    const result = await api.listPages();
    if (!result.ok) return;
    const served = Array.isArray(result.body.items) ? result.body.items : [];
    const ids = served.map((entry) => String((entry && entry.id) || ""));
    hiddenPages = ROUTES.filter((route) => route.page && !ids.includes(route.id)).map((route) => route.id);
    registryState = "ready";
    draw();
  }

  // loadProjection reads the page the address names. A page the control plane
  // answers as absent is one the identity cannot see: its entry and its body
  // both go away. Any other failure keeps the entry and states the reason.
  async function loadProjection({ force = false } = {}) {
    const route = currentRoute();
    if (!api || !route || !route.page) {
      projectionRoute = "";
      projection = null;
      draw();
      return;
    }
    if (hiddenPages.includes(route.id)) {
      projectionRoute = route.id;
      projection = { state: "hidden" };
      draw();
      return;
    }
    if (!force && projectionRoute === route.id && projection && projection.state === "ready") return;
    const seq = ++projectionSeq;
    projectionRoute = route.id;
    projection = { state: "loading" };
    draw();
    const result = await api.readPage(route.id);
    if (seq !== projectionSeq || projectionRoute !== route.id) return;
    if (result.ok) {
      projection = { state: "ready", page: result.body };
    } else if (result.error.code === "not_found" || result.error.code === "permission_denied") {
      if (!hiddenPages.includes(route.id)) hiddenPages = hiddenPages.concat(route.id);
      projection = { state: "hidden" };
    } else {
      projection = { state: "error", error: result.error };
    }
    draw();
  }

  function refresh() {
    return schedule(async () => {
      await loadRegistry();
      await loadProjection();
    });
  }

  window.addEventListener("message", (event) => {
    if (!channel.accept(event)) return;
    status = "connected";
    api = createAPI({ token: channel.session.token, fetchFn: fetchFn || globalThis.fetch });
    refresh().catch(() => draw());
  });
  window.addEventListener("hashchange", () => {
    entryNotice = null;
    draw();
    if (channel.session) schedule(loadProjection).catch(() => draw());
  });

  // The entry forms are drawn, not bound: one delegated submit listener on the
  // window root reaches the entry of every page this window re-reads
  // (KOSD-C-12).
  if (typeof root.addEventListener === "function") {
    root.addEventListener("submit", (event) => {
      const entry = applications.formEntryOf(event && event.target);
      if (!entry) return;
      event.preventDefault();
      const form = event.target;
      writing = writing.then(() => submitEntry(entry, form)).catch(() => {});
    });
  }

  // submitEntry sends one entry with the values the reader holds. The control
  // plane's answer is kept as it came; the accepted state itself is read back
  // from the application object by the page read that follows (KOSD-AC-61).
  async function submitEntry(entry, form) {
    if (!api) return;
    entryNotice = null;
    draw();
    let request = null;
    if (entry.item === applications.ITEM_RESOURCE_SPEC) {
      const current = await api.readResource(applications.entryPath(entry));
      if (!current.ok) {
        entryNotice = { kind: "error", label: entry.label, code: current.error.code, message: current.error.message };
        draw();
        return;
      }
      request = applications.resourceSpecRequest(current.body, entry, applications.valuesOf(form));
      if (request && request.refused) {
        entryNotice = { kind: "error", label: entry.label, code: "masked_value", message: request.refused };
        draw();
        return;
      }
    } else {
      request = applications.entryRequest(entry, applications.valuesOf(form));
    }
    if (!request) {
      entryNotice = { kind: "error", label: entry.label, code: "invalid_field", message: invalidNote(entry) };
      draw();
      return;
    }
    const answer = await api.writeEntry(request);
    if (!answer.ok) {
      entryNotice = { kind: "error", label: entry.label, code: answer.error.code, message: answer.error.message };
      draw();
      return;
    }
    entryNotice = { kind: "accepted", label: entry.label };
    await loadProjection({ force: true });
  }

  // invalidNote states why a form cannot be submitted as it stands.
  function invalidNote(entry) {
    if (entry.item === applications.ITEM_SCALE) return "副本数必须是不小于 0 的整数";
    if (entry.item === applications.ITEM_UNINSTALL || entry.item === applications.ITEM_RESOURCE_SPEC) {
      return "缺少对象版本前置条件，请重新读取后再提交";
    }
    return "该入口没有可提交的写入";
  }

  // The ready message follows the listener: a console that answers immediately
  // must find the window listening.
  channel.ready();
  draw();
  return { channel, redraw: draw, settle: () => Promise.all([pending, writing]).then(() => {}) };
}

// connect starts the window document. A module loaded outside a browser (the
// acceptance cases) has no document and starts nothing.
export function connect(document, window) {
  return createShell({ document, window });
}

if (typeof window !== "undefined" && typeof document !== "undefined") {
  connect(document, window);
}
