// TASK-TC-7 from the page side: the Pod runtime page renders the projection of
// the application the address names — each row's identity, node, phase,
// readiness, restart count, containers and images, start time, source type and
// observation moment (KOSD-C-76, KOSD-C-13) — and reads that application's own
// projection address, never another application's.

import assert from "node:assert/strict";
import test from "node:test";

import * as pods from "../../../assets/pages/pods.js";
import { bootPodsWindow, logButtonOf } from "../fixtures/pod-window.js";
import { logRead, podFacts, podRead, podsPage } from "../fixtures/pods.js";

function renderPods({ app = "home/shop", page = podsPage(), read = podRead(), logState = null } = {}) {
  return pods.render({
    app,
    projection: { state: "ready", page },
    podState: { state: "ready", read },
    logState,
  });
}

test("the page renders the projected Pod facts with their source type and observation", () => {
  const html = renderPods();
  assert.match(html, /data-page="pods"/);
  assert.match(html, /应用 home\/shop/);

  const first = html.indexOf('data-pod="shop-1"');
  assert.notEqual(first, -1, "shop-1 is rendered");
  const block = html.slice(first);
  assert.match(block, /data-source="object"/);
  assert.match(block, /对象源/);
  assert.match(block, /data-observed-at="2026-03-01T12:00:00Z"/);
  assert.match(block, /观测于 2026-03-01 12:00:00 UTC/);
  assert.match(block, /data-pod-node="node-a"/);
  assert.match(block, /data-pod-phase="Running"/);
  assert.match(block, /data-pod-ready="true"/);
  assert.match(block, /已就绪/);
  assert.match(block, /data-pod-restarts="0"/);
  assert.match(block, /data-pod-container="web"/);
  assert.match(block, /registry\.test\/shop:2/);
  assert.match(block, /data-pod-started-at="2026-03-01T11:30:00Z"/);
  assert.match(block, /2026-03-01 11:30:00 UTC/);
  assert.doesNotMatch(block, /data-stale="true"/);

  assert.match(html, /data-pod="shop-2"/);
  assert.match(html, /data-pod-phase="Pending"/);
  assert.match(html, /未就绪/);
  assert.match(html, /data-pod-restarts="2"/);
});

test("a stale observation is marked as such", () => {
  const html = renderPods({
    read: podRead({ items: [podFacts({ stale: true })] }),
  });
  assert.match(html, /data-stale="true"/);
  assert.match(html, /数据陈旧/);
});

test("each Pod row offers its own narrow log read and the single delete entry", () => {
  const html = renderPods();
  assert.match(html, /data-log-read="true" data-log-path="\/api\/v1\/logs\/home\/shop\?pod=shop-1"/);
  assert.match(html, /data-log-read="true" data-log-path="\/api\/v1\/logs\/home\/shop\?pod=shop-2"/);
  assert.match(html, /data-entry-item="pod-delete"/);
  assert.match(html, /data-entry-method="DELETE"/);
  assert.match(html, /data-entry-path="\/api\/v1\/pods\/home\/shop\/shop-1"/);
  assert.match(html, new RegExp(pods.DELETE_LABEL));
  assert.match(html, new RegExp(pods.LOG_RULE_NOTE));
});

test("the page offers no Pod create, spec write, terminal or port forward control", () => {
  const html = renderPods();
  for (const forbidden of ["创建 Pod", "修改 Pod spec", "终端", "端口转发", "data-terminal", "data-port-forward"]) {
    assert.ok(!html.includes(forbidden), `the page offers ${forbidden}`);
  }
  const forms = [...html.matchAll(/<form[^>]*data-entry-item="([^"]*)"/g)].map((match) => match[1]);
  assert.deepEqual([...new Set(forms)], ["pod-delete"], "the only entry is the single-Pod delete");
});

test("the window reads only the addressed application's Pod projection", async () => {
  const { calls, shell } = await bootPodsWindow();
  try {
    const urls = calls.map((call) => call.url);
    assert.ok(urls.includes("/api/v1/pages/pods"), "the page projection is read");
    assert.ok(urls.includes("/api/v1/pods/home/shop"), "the addressed application's Pod projection is read");
    assert.ok(!urls.includes("/api/v1/pods/home/reports"), "another application's Pod projection is never read");
    for (const call of calls) assert.equal(call.options?.credentials, "omit");
  } finally {
    shell.channel.stop();
  }
});

test("an over-limit per-Pod log read states its window, bound and truncation position", async () => {
  const truncated = logRead({
    window: "15m0s",
    limit: 2,
    truncated: true,
    truncatedAt: "2026-03-01T11:58:00Z",
    lines: [
      { pod: "shop-1", container: "web", timestamp: "2026-03-01T11:58:00Z", message: "较早的保留行" },
      { pod: "shop-1", container: "web", timestamp: "2026-03-01T11:59:00Z", message: "最新的保留行" },
    ],
  });
  const { calls, fixture, shell } = await bootPodsWindow({ log: truncated });
  try {
    const button = logButtonOf(fixture.root.innerHTML, "shop-1");
    assert.ok(button, "the Pod row carries its log control");
    assert.match(button.dataset.logPath, /\?pod=shop-1$/, "the read is narrowed to this Pod");
    fixture.dispatchRoot("click", { target: button, preventDefault() {} });
    await shell.settle();

    const read = calls.find((call) => call.url.startsWith("/api/v1/logs/"));
    assert.ok(read, "the window read the application's log address");
    assert.equal(read.url, "/api/v1/logs/home/shop?pod=shop-1", "the request keeps the Pod narrowing");

    const html = fixture.root.innerHTML;
    assert.match(html, /data-log-state="ready"/);
    assert.match(html, /data-log-window="15m0s"/, "the window the read used is shown");
    assert.match(html, /data-log-limit="2"/, "the line bound is shown");
    assert.match(html, /data-log-bounds="true"/);
    assert.match(html, /窗口 15m0s，最多 2 行/);
    assert.match(html, /data-log-truncated="true"/, "the truncation mark is rendered");
    assert.match(html, /data-log-truncated-at="2026-03-01T11:58:00Z"/, "the cut position is the oldest kept line");
    assert.match(html, /2026-03-01 11:58:00 UTC/, "the cut position is readable");
    assert.equal(
      [...html.matchAll(/data-log-line="true"/g)].length,
      2,
      "only the lines the bound kept are rendered",
    );
    assert.doesNotMatch(html, /较早的行/, "the line the bound cut is not shown");
    const logStart = html.indexOf('data-log-state="ready"');
    const logBlock = html.slice(logStart, html.indexOf("</div>", logStart));
    assert.doesNotMatch(logBlock, /data-pod="shop-2"/, "another replica's line never appears in this Pod's read");
  } finally {
    shell.channel.stop();
  }
});
