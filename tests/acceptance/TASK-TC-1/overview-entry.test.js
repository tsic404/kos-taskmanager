// The page side: the window reads the page registry and one page projection from
// the control plane (KOSD-C-12). A page the registry does not serve the session,
// or that the read answers as absent, has neither a navigation entry nor a page:
// the window moves to the first page it may see, and when it may see none, it
// states that the page is unavailable instead of inventing one (KOSD-C-53,
// KOSD-C-59, DASH-C-22).

import assert from "node:assert/strict";
import test from "node:test";

import { HIDDEN_NOTE, createShell, renderShell } from "../../../assets/app.js";
import { CONTROL_PLANE_NOT_FOUND, applicationsPage } from "../fixtures/applications.js";
import { overviewPage } from "../fixtures/projection.js";
import { bootstrapWindow, jsonResponse } from "../fixtures/window.js";

// serve builds a fetch stand-in from one registry answer and one answer per
// page, recording every request the window makes.
function serve({ registry, pages = {} }) {
  const calls = [];
  const fetchFn = async (url) => {
    calls.push(url);
    const read = (answer) => (typeof answer === "function" ? answer() : jsonResponse(answer));
    if (url === "/api/v1/pages") return read(registry);
    const page = url.replace("/api/v1/pages/", "");
    if (Object.prototype.hasOwnProperty.call(pages, page)) return read(pages[page]);
    return jsonResponse({ code: "not_found", message: "没有该资源" }, { status: 404 });
  };
  return { fetchFn, calls };
}

async function boot({ registry, pages = {}, hash = "#/overview" }) {
  const fixture = bootstrapWindow({ hash });
  const server = serve({ registry, pages });
  const shell = createShell({ document: fixture.document, window: fixture.window, fetchFn: server.fetchFn });
  try {
    await shell.settle();
    return { fixture, calls: server.calls };
  } finally {
    shell.channel.stop();
  }
}

test("a page the registry serves is rendered with its entry", async () => {
  const { fixture, calls } = await boot({
    registry: { items: [{ id: "overview", title: "概览", readOnly: true }] },
    pages: { overview: overviewPage() },
  });
  assert.deepEqual(calls, ["/api/v1/pages", "/api/v1/pages/overview"]);
  assert.match(fixture.root.innerHTML, /<a class="nav-entry" data-route="overview" href="#\/overview"/);
  assert.match(fixture.root.innerHTML, /data-page="overview"/);
  assert.match(fixture.root.innerHTML, /data-item="nodes-ready"/);
});

test("a page the identity cannot see has neither an entry nor a page", async () => {
  const { fixture, calls } = await boot({
    registry: { items: [] },
    pages: { overview: overviewPage(), applications: applicationsPage() },
  });
  assert.deepEqual(calls, ["/api/v1/pages"], "an invisible page is not read");
  assert.doesNotMatch(fixture.root.innerHTML, /data-route="overview"/, "the entry is absent");
  assert.doesNotMatch(fixture.root.innerHTML, /data-route="applications"/, "the application entry is absent");
  assert.doesNotMatch(fixture.root.innerHTML, /data-page="overview"/, "the page is absent");
  assert.doesNotMatch(fixture.root.innerHTML, /data-page="applications"/);
  assert.match(fixture.root.innerHTML, /data-page="unavailable"/, "the window states it has no page to show");
  assert.match(fixture.root.innerHTML, new RegExp(HIDDEN_NOTE));
});

test("a read that answers not_found withdraws the entry and the page", async () => {
  const { fixture } = await boot({
    registry: { items: [{ id: "overview", title: "概览" }] },
    pages: { overview: () => jsonResponse(CONTROL_PLANE_NOT_FOUND, { status: 404 }) },
  });
  assert.doesNotMatch(fixture.root.innerHTML, /data-route="overview"/);
  assert.doesNotMatch(fixture.root.innerHTML, /data-page="overview"/);
  assert.match(fixture.root.innerHTML, /data-page="unavailable"/);
});

test("a rendered hidden page carries no page identity", () => {
  const html = renderShell({ hash: "#/overview", hiddenPages: ["overview"] });
  assert.match(html, /data-page="unavailable"/);
  assert.match(html, new RegExp(HIDDEN_NOTE));
  assert.doesNotMatch(html, /data-page="overview"/);
});

test("an unreadable projection keeps the entry and states the reason", async () => {
  const { fixture } = await boot({
    registry: { items: [{ id: "overview", title: "概览" }] },
    pages: {
      overview: () => jsonResponse({ code: "capability_missing", message: "页面来源 computed 尚未接入控制面" }, { status: 503 }),
    },
  });
  assert.match(fixture.root.innerHTML, /data-route="overview"/, "the entry stays");
  assert.match(fixture.root.innerHTML, /data-page="overview"/);
  assert.match(fixture.root.innerHTML, /data-page-state="error"/);
  assert.match(fixture.root.innerHTML, /capability_missing/);
  assert.match(fixture.root.innerHTML, /页面来源 computed 尚未接入控制面/);
  assert.doesNotMatch(fixture.root.innerHTML, /data-empty="no-items"/, "an unreadable page is not an empty page");
});

test("the window opens the first page it may see when the overview is hidden", async () => {
  const { fixture, calls } = await boot({
    registry: { items: [] },
    pages: { overview: overviewPage(), applications: applicationsPage() },
    hash: "#/elsewhere",
  });
  assert.equal(fixture.replaced[fixture.replaced.length - 1], "#/overview");
  assert.doesNotMatch(fixture.root.innerHTML, /data-route="overview"/, "no served page means no entry");

  const { fixture: served } = await boot({
    registry: { items: [{ id: "applications", title: "应用" }] },
    pages: { overview: overviewPage(), applications: applicationsPage() },
    hash: "#/elsewhere",
  });
  assert.equal(served.replaced[served.replaced.length - 1], "#/applications");
  assert.match(served.root.innerHTML, /data-page="applications"/);
  assert.match(served.root.innerHTML, /data-item="installed-applications"/);
  assert.ok(calls.includes("/api/v1/pages"));
});
