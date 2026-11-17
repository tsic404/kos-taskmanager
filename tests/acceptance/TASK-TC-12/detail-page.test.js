// TASK-TC-12 from the page side: the detail surface renders the seven items
// DASH-C-52 lists — health, volumes, ingress, backup, drift, events and logs —
// each with its source type, the moment it was observed and the control plane's
// stale verdict (KOSD-C-53, KOSD-C-13); it shows the facts of the application
// the address names and no other application's (KOSD-C-59); the log item reads
// the application's own address when the reader asks for it, marks truncation
// and states an unreachable source's reason instead of an empty list
// (KOSD-C-64). The window's entry for the page follows the component that
// declares it: a disabled component leaves neither the page nor its entry, while
// another surface stays served (KOSD-AC-54).

import assert from "node:assert/strict";
import test from "node:test";

import { HIDDEN_NOTE, createShell, renderShell } from "../../../assets/app.js";
import { applicationsPage } from "../fixtures/applications.js";
import {
  DETAIL_ITEMS,
  ITEM_BACKUP,
  ITEM_DRIFT,
  ITEM_EVENTS,
  ITEM_HEALTH,
  ITEM_INGRESS,
  ITEM_LOGS,
  ITEM_VOLUMES,
  LOG_RULE,
  OBSERVED_AT,
  READ_ONLY_REASON,
  applicationFacts,
  detailPage,
  logRead,
} from "../fixtures/detail.js";
import { overviewPage } from "../fixtures/projection.js";
import { bootstrapWindow, jsonResponse } from "../fixtures/window.js";

const DETAIL_HASH = "#/applications/home%2Fshop";

// SOURCE_LABELS is the text every item's source type renders as.
const SOURCE_LABELS = { object: "对象源", read: "读取源" };

// blocksOf splits the rendered item list into one block per item identity.
function blocksOf(html) {
  const blocks = new Map();
  for (const part of html.split('<li class="item" ').slice(1)) {
    const identifier = /^data-item="([^"]+)"/.exec(part);
    if (identifier) blocks.set(identifier[1], part);
  }
  return blocks;
}

function renderPage({ app = "home/shop", page = detailPage(), projection = null } = {}) {
  return renderShell({
    hash: `#/applications/${encodeURIComponent(app)}`,
    status: "connected",
    projection: projection || { state: "ready", page },
  });
}

// serve answers the reads a window makes for one detail page, the overview a
// withdrawn detail surface falls back to, and one log read.
function serve({
  registry = { items: [{ id: "detail", title: "应用详情" }, { id: "overview", title: "概览" }] },
  page,
  log,
  pages = { overview: overviewPage() },
} = {}) {
  const calls = [];
  const fetchFn = async (url, options = {}) => {
    calls.push({ url, options });
    if (url === "/api/v1/pages") return jsonResponse(typeof registry === "function" ? registry() : registry);
    if (url === "/api/v1/pages/detail") return jsonResponse(typeof page === "function" ? page() : page);
    const other = url.replace("/api/v1/pages/", "");
    if (url.startsWith("/api/v1/pages/") && Object.prototype.hasOwnProperty.call(pages, other)) {
      return jsonResponse(pages[other]);
    }
    if (url.startsWith("/api/v1/logs/")) return (typeof log === "function" ? log : () => jsonResponse(log || logRead()))(url);
    return jsonResponse({ error: { code: "not_found", message: "没有该资源" } }, { status: 404 });
  };
  return { fetchFn, calls };
}

async function boot({ hash = DETAIL_HASH, registry, page = detailPage(), log = logRead(), pages } = {}) {
  const fixture = bootstrapWindow({ hash });
  const server = serve({ registry, page, log, pages });
  const shell = createShell({ document: fixture.document, window: fixture.window, fetchFn: server.fetchFn });
  await shell.settle();
  return { fixture, calls: server.calls, shell };
}

// logButtonIn reads the log-read control the page actually emitted, so a case
// drives the button the reader sees.
function logButtonIn(html) {
  const opening = /<button[^>]*data-log-read="true"[^>]*>/.exec(html);
  if (!opening) return null;
  const path = /data-log-path="([^"]*)"/.exec(opening[0]);
  return { dataset: { logRead: "true", logPath: path ? path[1] : "" } };
}

test("the detail page renders the seven items DASH-C-52 lists with their source types", () => {
  const html = renderPage();
  assert.match(html, /data-page="detail"/);
  const blocks = blocksOf(html);
  assert.deepEqual([...blocks.keys()], DETAIL_ITEMS.map(([id]) => id), "the seven items render in declaration order");

  for (const [id, label, source] of DETAIL_ITEMS) {
    const block = blocks.get(id);
    assert.match(block, new RegExp(`^data-item="${id}" data-source="${source}"`), `${id} declares ${source}`);
    assert.match(block, new RegExp(SOURCE_LABELS[source]), `${id} shows its source type as text`);
    assert.match(block, /data-observed-at="2026-03-01T12:00:00Z"/, `${id} carries its observation time`);
    assert.match(block, /观测于 2026-03-01 12:00:00 UTC/, `${id} shows the observation moment`);
    assert.doesNotMatch(block, /data-stale="true"/, `${id} is inside the freshness bound`);
  }
});

test("the detail page states that it is read-only and why", () => {
  const html = renderPage();
  assert.match(html, /data-readonly="true">只读/);
  assert.match(html, new RegExp(`data-readonly-reason="true">${READ_ONLY_REASON}`));
  assert.doesNotMatch(html, /data-entry-item=/, "a read-only page offers no modification entry");
  assert.doesNotMatch(html, /<form class="entry"/);
});

test("each item marks the observation stale beyond the freshness bound", () => {
  const html = renderPage({
    page: detailPage({ stale: [ITEM_HEALTH, ITEM_LOGS] }),
  });
  const blocks = blocksOf(html);
  for (const [id] of DETAIL_ITEMS) {
    const stale = id === ITEM_HEALTH || id === ITEM_LOGS;
    if (stale) {
      assert.match(blocks.get(id), /data-stale="true"/, `${id} is stale beyond the bound`);
      assert.match(blocks.get(id), /数据陈旧/);
    } else {
      assert.doesNotMatch(blocks.get(id), /data-stale="true"/, `${id} is fresh`);
    }
  }
});

test("the page shows the facts of the application the address names and no other application's", () => {
  const html = renderPage({
    page: detailPage({
      applications: [
        applicationFacts(),
        applicationFacts({
          name: "reports",
          volumes: [{ name: "media", resolved: true, mobility: "shared-rwx", capacity: "50Gi" }],
          endpoints: [{ name: "reports-web", kind: "http.host", host: "reports.test", phase: "Ready", health: "Healthy" }],
          policies: [{ name: "reports-only", selector: "app=reports", schedule: "0 4 * * *", phase: "Ready", health: "Healthy" }],
          drift: { policy: "reconcile", entries: [] },
          events: [{ name: "reports-backoff", type: "Warning", reason: "BackOff", message: "另一个应用的事件", count: 1, lastSeen: "2026-03-01T11:55:00Z" }],
        }),
      ],
    }),
  });
  assert.match(html, /data-app="home\/shop"/);
  assert.match(html, /应用 home\/shop/);
  const blocks = blocksOf(html);

  assert.match(blocks.get(ITEM_HEALTH), /data-phase="Ready" data-health="Healthy"/);
  assert.match(blocks.get(ITEM_HEALTH), /data-owner="lin"/);
  assert.match(blocks.get(ITEM_HEALTH), /data-replicas="2"/);
  assert.match(blocks.get(ITEM_HEALTH), /下游对象已收敛/);

  assert.match(blocks.get(ITEM_VOLUMES), /data-volume="data"/);
  assert.match(blocks.get(ITEM_VOLUMES), /data-resolved="true"/);
  assert.match(blocks.get(ITEM_VOLUMES), /node-local/);
  assert.match(blocks.get(ITEM_VOLUMES), /node-a/);
  assert.match(blocks.get(ITEM_VOLUMES), /data-volume="gone"/);
  assert.match(blocks.get(ITEM_VOLUMES), /data-resolved="false"/);
  assert.match(blocks.get(ITEM_VOLUMES), /未解析/);

  assert.match(blocks.get(ITEM_INGRESS), /data-endpoint="shop-web"/);
  assert.match(blocks.get(ITEM_INGRESS), /shop\.test/);

  assert.match(blocks.get(ITEM_BACKUP), /data-policy="nightly"/);
  assert.match(blocks.get(ITEM_BACKUP), /0 3 \* \* \*/);

  assert.match(blocks.get(ITEM_DRIFT), /data-drift-policy="adopt"/);
  assert.match(blocks.get(ITEM_DRIFT), /spec\.replicas/);
  assert.match(blocks.get(ITEM_DRIFT), /5/);
  assert.match(blocks.get(ITEM_DRIFT), /2/);

  assert.match(blocks.get(ITEM_EVENTS), /data-window="1h0m0s"/);
  assert.match(blocks.get(ITEM_EVENTS), /shop-backoff/);
  assert.match(blocks.get(ITEM_EVENTS), /容器反复退出/);

  assert.match(blocks.get(ITEM_LOGS), new RegExp(LOG_RULE));
  assert.match(blocks.get(ITEM_LOGS), /data-log-path="\/api\/v1\/logs\/home\/shop"/);

  for (const other of ["reports", "reports-web", "reports-only", "reports-backoff", "media", "50Gi", "另一个应用的事件"]) {
    assert.ok(!html.includes(other), `another application's fact leaked into the page: ${other}`);
  }
});

test("an identity that cannot see the addressed application gets neither the page nor its entry", async () => {
  const { fixture, shell } = await boot({
    page: detailPage({
      applications: [
        applicationFacts({
          name: "reports",
          volumes: [],
          endpoints: [],
          policies: [],
          drift: { policy: "", entries: [] },
          events: [],
        }),
      ],
    }),
  });
  try {
    const html = fixture.root.innerHTML;
    assert.doesNotMatch(html, /data-page="detail"/, "the page is absent");
    assert.doesNotMatch(html, /data-route="detail"/, "the entry is absent");
    assert.match(html, /data-page="unavailable"/);
    assert.match(html, new RegExp(HIDDEN_NOTE));
    assert.doesNotMatch(html, /reports/, "no other application's facts stand in for the addressed one");
  } finally {
    shell.channel.stop();
  }
});

test("the log item reads the application's own address and shows the bounded result", async () => {
  const { fixture, calls, shell } = await boot({
    log: logRead({
      truncated: true,
      truncatedAt: "2026-03-01T11:57:00Z",
      limit: 1,
      lines: [
        { pod: "shop-2", container: "web", timestamp: "2026-03-01T11:58:00Z", message: "shop 的另一个副本" },
      ],
    }),
  });
  try {
    const button = logButtonIn(fixture.root.innerHTML);
    assert.ok(button, "the log item offers the application's read");
    assert.equal(button.dataset.logPath, "/api/v1/logs/home/shop");
    fixture.dispatchRoot("click", { target: button });
    await shell.settle();

    const read = calls.find((call) => call.url === "/api/v1/logs/home/shop");
    assert.ok(read, "the window read the application's own log address");
    assert.equal(read.options.method, "GET");
    assert.equal(read.options.headers.Authorization, "Bearer app-token-1");

    const html = fixture.root.innerHTML;
    const logs = blocksOf(html).get(ITEM_LOGS);
    assert.match(logs, /data-log-state="ready"/);
    assert.match(logs, /data-log-window="1h0m0s"/);
    assert.match(logs, /data-log-limit="1"/);
    assert.match(logs, /data-log-line="true"/);
    assert.match(logs, /data-pod="shop-2"/);
    assert.match(logs, /data-container="web"/);
    assert.match(logs, /shop 的另一个副本/);
    assert.match(logs, /data-log-truncated="true"/);
    assert.match(logs, /2026-03-01 11:57:00 UTC/, "the truncation position is shown");
    assert.doesNotMatch(html, /<a[^>]*download/, "no full download is offered");
    assert.doesNotMatch(html, /data-download/);
  } finally {
    shell.channel.stop();
  }
});

test("an unreachable log source states the reason instead of an empty line list", async () => {
  const { fixture, shell } = await boot({
    log: () => jsonResponse({ error: { code: "unreachable", message: "集群 API 不可达" } }, { status: 503 }),
  });
  try {
    fixture.dispatchRoot("click", { target: logButtonIn(fixture.root.innerHTML) });
    await shell.settle();

    const logs = blocksOf(fixture.root.innerHTML).get(ITEM_LOGS);
    assert.match(logs, /data-log-state="error"/);
    assert.match(logs, /data-code="unreachable"/);
    assert.match(logs, /集群 API 不可达/);
    assert.doesNotMatch(logs, /data-empty="no-log-lines"/, "an unreadable source is not an empty log");
    assert.doesNotMatch(logs, /data-log-line="true"/);
  } finally {
    shell.channel.stop();
  }
});

test("disabling the component withdraws the detail page and its entry while another surface stays", async () => {
  const withdrawn = await boot({ registry: { items: [{ id: "overview", title: "概览" }] } });
  try {
    const html = withdrawn.fixture.root.innerHTML;
    assert.doesNotMatch(html, /data-route="detail"/, "the entry is gone with the component");
    assert.doesNotMatch(html, /data-page="detail"/, "the page is gone with the component");
    assert.equal(withdrawn.fixture.replaced[withdrawn.fixture.replaced.length - 1], "#/overview");
    assert.match(html, /data-page="overview"/, "another surface is unaffected");
    assert.match(html, /data-route="overview"/);
  } finally {
    withdrawn.shell.channel.stop();
  }

  const enabled = await boot();
  try {
    const html = enabled.fixture.root.innerHTML;
    assert.match(html, /data-page="detail"/);
    assert.match(html, /<a class="nav-entry" data-route="detail" href="#\/applications\/home%2Fshop"/);
  } finally {
    enabled.shell.channel.stop();
  }
});

test("the applications surface links a visible application to its detail address", () => {
  const page = applicationsPage();
  const html = renderShell({
    hash: "#/applications",
    status: "connected",
    projection: { state: "ready", page },
  });
  assert.match(html, /data-detail-link="home\/shop"/);
  assert.match(html, /href="#\/applications\/home%2Fshop"/);
});
