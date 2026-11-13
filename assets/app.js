// The window document's entry: it negotiates the console channel, renders the
// shell, and resolves the four page surfaces from the address. The page modules
// are pure renderers, so the shell and its routes run in a test without a
// browser.

import { createChannel, consoleOriginFrom, windowIDFrom } from "./handshake.js";
import { escapeHTML } from "./html.js";
import { ROUTES, defaultHash, matchRoute, renderNav } from "./router.js";
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

// renderShell draws one state of the window: the navigation, the page the
// address names, and whether the console channel is established.
export function renderShell({ hash = "", session = null, status = "pending" } = {}) {
  const matched = matchRoute(hash) || { route: ROUTES[0], app: "" };
  const page = PAGES.get(matched.route.id) || overview;
  const app = matched.app || "";
  return `
    <div class="shell">
      <header class="shell-header">
        <span class="brand">运行视图</span>
        ${renderNav(matched.route.id, app)}
        <span class="status" data-status="${escapeHTML(status)}">${escapeHTML(STATUS_LABEL[status] || STATUS_LABEL.pending)}</span>
      </header>
      <main class="shell-main">${page.render({ app, session })}</main>
    </div>`;
}

// createShell wires the document to the console channel and the address.
export function createShell({ document, window }) {
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

  function draw() {
    // An address outside the table is not a page: the window shows the default
    // one and moves the address there, so a reload and a deep link agree.
    if (!matchRoute(window.location.hash)) {
      window.location.replace(defaultHash());
    }
    root.innerHTML = renderShell({ hash: window.location.hash, session: channel.session, status });
    root.dataset.state = "ready";
    root.dataset.session = channel.session ? "established" : "none";
  }

  window.addEventListener("message", (event) => {
    if (!channel.accept(event)) return;
    status = "connected";
    draw();
  });
  window.addEventListener("hashchange", draw);

  // The ready message follows the listener: a console that answers immediately
  // must find the window listening.
  channel.ready();
  draw();
  return { channel, redraw: draw };
}

// connect starts the window document. A module loaded outside a browser (the
// acceptance cases) has no document and starts nothing.
export function connect(document, window) {
  return createShell({ document, window });
}

if (typeof window !== "undefined" && typeof document !== "undefined") {
  connect(document, window);
}
