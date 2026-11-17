// The window the Pod page cases boot: the console handshake, the page registry,
// the page projection, the application's Pod projection and the per-Pod log
// read all answer from one fetch stand-in, so a case drives the page the reader
// actually sees instead of rendering an isolated string.

import { createShell } from "../../../assets/app.js";
import { overviewPage } from "./projection.js";
import { logRead, podRead, podsPage } from "./pods.js";
import { bootstrapWindow, jsonResponse } from "./window.js";

// servePods answers the reads one Pod page makes. Each answer may be a value or
// a function, so a case can change what a later read returns.
export function servePods({
  registry = { items: [{ id: "pods", title: "Pod 运行态" }, { id: "overview", title: "概览" }] },
  page = podsPage,
  pod = podRead,
  log = logRead,
  onDelete = null,
  pages = { overview: overviewPage() },
} = {}) {
  const calls = [];
  const fetchFn = async (url, options = {}) => {
    const method = String(options.method || "GET").toUpperCase();
    calls.push({ url, method, options });
    if (method === "DELETE") {
      if (!onDelete) return jsonResponse({ accepted: true, ref: { kind: "Pod" } });
      return onDelete(url, options);
    }
    if (url === "/api/v1/pages") {
      return jsonResponse(typeof registry === "function" ? registry() : registry);
    }
    if (url === "/api/v1/pages/pods") {
      return jsonResponse(typeof page === "function" ? page() : page);
    }
    const other = url.replace("/api/v1/pages/", "");
    if (url.startsWith("/api/v1/pages/") && Object.prototype.hasOwnProperty.call(pages, other)) {
      return jsonResponse(pages[other]);
    }
    if (url.startsWith("/api/v1/pods/")) {
      const answer = typeof pod === "function" ? pod(url) : pod;
      return answer && answer.error ? jsonResponse(answer, { status: 502 }) : jsonResponse(answer);
    }
    if (url.startsWith("/api/v1/logs/")) {
      const answer = typeof log === "function" ? log(url) : log;
      return answer && answer.error ? jsonResponse(answer, { status: 502 }) : jsonResponse(answer);
    }
    return jsonResponse({ error: { code: "not_found", message: "没有该资源" } }, { status: 404 });
  };
  return { fetchFn, calls };
}

// bootPodsWindow opens one Pod page at the address of the named application.
export async function bootPodsWindow({ app = "home/shop", ...serverOptions } = {}) {
  const hash = serverOptions.hash || `#/applications/${encodeURIComponent(app)}/pods`;
  const fixture = bootstrapWindow({ hash });
  const server = servePods(serverOptions);
  const shell = createShell({ document: fixture.document, window: fixture.window, fetchFn: server.fetchFn });
  await shell.settle();
  return { fixture, calls: server.calls, shell, serve: server };
}

// podBlockOf splits one Pod row out of the rendered page.
export function podBlockOf(html, name) {
  const marker = `<li class="item pod" data-pod="${name}"`;
  const start = html.indexOf(marker);
  if (start === -1) return "";
  const next = html.indexOf('<li class="item pod"', start + marker.length);
  return next === -1 ? html.slice(start) : html.slice(start, next);
}

// deleteFormOf reads the delete form the page emitted for one Pod, so a case
// submits the control the reader actually sees.
export function deleteFormOf(html, name) {
  const block = podBlockOf(html, name);
  const opening = /<form[^>]*data-entry-item="pod-delete"[^>]*>/.exec(block);
  if (!opening) return null;
  const dataset = {};
  for (const match of opening[0].matchAll(/data-([a-z-]+)="([^"]*)"/g)) {
    dataset[camel(match[1])] = match[2];
  }
  return { dataset };
}

// logButtonOf reads the per-Pod log control the page emitted for one Pod.
export function logButtonOf(html, name) {
  const block = podBlockOf(html, name);
  const opening = /<button[^>]*data-log-read="true"[^>]*>/.exec(block);
  if (!opening) return null;
  const dataset = { logRead: "true" };
  const path = /data-log-path="([^"]*)"/.exec(opening[0]);
  if (path) dataset.logPath = path[1];
  return { dataset };
}

function camel(value) {
  return value.replace(/-([a-z])/g, (_, letter) => letter.toUpperCase());
}
