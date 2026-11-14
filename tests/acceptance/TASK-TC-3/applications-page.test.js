// TASK-TC-3 from the page side: the applications surface renders its three
// display items with the source type, the declared source, the moment each was
// observed and the control plane's stale verdict (KOSD-C-53, KOSD-C-13); the
// installed row carries the object's own owner, phase, health, pause and
// conditions (KOSD-AC-61); and the six entries appear only for the rows their
// `applications` target list names, so the two identities without write access
// see no control at all and a refused direct call states the control plane's
// answer instead of guessing one (KOSD-C-59, KOSD-AC-16).

import assert from "node:assert/strict";
import test from "node:test";

import { createShell } from "../../../assets/app.js";
import * as applications from "../../../assets/pages/applications.js";
import {
  ITEM_INSTALLED,
  ITEM_INSTALLABLE,
  ITEM_RESOURCE_SPEC,
  ITEM_RESOURCE_USAGE,
  ITEM_RESTART,
  ITEM_SCALE,
  ITEM_START,
  ITEM_STOP,
  CONTROL_PLANE_FORBIDDEN,
  ITEM_UNINSTALL,
  REASONS,
  RULE_INSTALLABLE,
  applicationRow,
  applicationsPage,
  formIn,
  noTargets,
  target,
} from "../fixtures/applications.js";
import { bootstrapWindow, jsonResponse } from "../fixtures/window.js";

// SOURCES is the source type and the text every display item must declare.
const SOURCES = [
  [ITEM_INSTALLED, "object", "对象源"],
  [ITEM_INSTALLABLE, "computed", "计算源"],
  [ITEM_RESOURCE_USAGE, "metric", "指标源"],
];

// blocksOf splits the rendered item list into one block per item identity.
function blocksOf(html) {
  const blocks = new Map();
  for (const part of html.split('<li class="item" ').slice(1)) {
    const identifier = /^data-item="([^"]+)"/.exec(part);
    if (identifier) blocks.set(identifier[1], part);
  }
  return blocks;
}

// rowsOf splits the installed row list into one block per application.
function rowsOf(html) {
  const rows = new Map();
  for (const part of html.split('<li class="fact" data-object="').slice(1)) {
    const identifier = /^([^"]+)"/.exec(part);
    if (identifier) rows.set(identifier[1], part);
  }
  return rows;
}

function renderPage(options = {}) {
  return applications.render({ projection: { state: "ready", page: applicationsPage(options) } });
}

// serve answers the two reads a window makes with one page, and one write with
// the answer the control plane gives.
function serve({ page, write }) {
  const calls = [];
  const fetchFn = async (url, options = {}) => {
    calls.push({ url, options });
    if (url === "/api/v1/pages") {
      return jsonResponse({ items: [{ id: "applications", title: "应用" }] });
    }
    if (url === "/api/v1/pages/applications") {
      return jsonResponse(typeof page === "function" ? page() : page);
    }
    return write(url, options);
  };
  return { fetchFn, calls };
}

async function boot({ page, write }) {
  const fixture = bootstrapWindow({ hash: "#/applications" });
  const server = serve({ page, write });
  const shell = createShell({ document: fixture.document, window: fixture.window, fetchFn: server.fetchFn });
  await shell.settle();
  return { fixture, calls: server.calls, shell };
}

test("the three display items render in declaration order each with a source type and a source", () => {
  const html = renderPage({ stale: [ITEM_RESOURCE_USAGE] });
  const blocks = blocksOf(html);
  assert.deepEqual([...blocks.keys()], SOURCES.map(([id]) => id));

  for (const [id, source, label] of SOURCES) {
    const block = blocks.get(id);
    assert.match(block, new RegExp(`^data-item="${id}" data-source="${source}"`), `${id} declares ${source}`);
    assert.match(block, new RegExp(label), `${id} shows its source type as text`);
  }
  assert.match(blocks.get(ITEM_INSTALLED), new RegExp(`data-reason="true">来源：${REASONS.installed}`));
  assert.match(blocks.get(ITEM_INSTALLABLE), new RegExp(`data-reason="true">来源：${REASONS.installable}`));
  assert.match(blocks.get(ITEM_RESOURCE_USAGE), new RegExp(`data-reason="true">来源：${REASONS.usage}`));
});

test("every item states when it was observed and whether it is stale", () => {
  const blocks = blocksOf(renderPage({ stale: [ITEM_RESOURCE_USAGE] }));
  for (const [id] of SOURCES) {
    const block = blocks.get(id);
    assert.match(block, /data-observed-at="2026-03-01T12:00:00Z"/, `${id} carries its observation time`);
    assert.match(block, /观测于 2026-03-01 12:00:00 UTC/, `${id} shows the observation moment`);
  }
  assert.match(blocks.get(ITEM_RESOURCE_USAGE), /data-stale="true"/);
  assert.match(blocks.get(ITEM_RESOURCE_USAGE), /数据陈旧/);
  for (const [id] of SOURCES) {
    if (id === ITEM_RESOURCE_USAGE) continue;
    assert.doesNotMatch(blocks.get(id), /data-stale="true"/, `${id} is inside the freshness bound`);
  }
});

test("the installed set shows the object's own facts and keeps an uninstalling row", () => {
  const html = renderPage({
    applications: [
      applicationRow(),
      applicationRow({
        name: "reports",
        owner: "",
        phase: "Uninstalling",
        health: "Unknown",
        paused: true,
        replicas: 0,
        uninstall: {
          blocked: [{ group: "kos.tsic.top", kind: "Volume", namespace: "home", name: "data" }],
          stalled: true,
        },
      }),
    ],
    targets: { ...noTargets(), [ITEM_STOP]: [target()] },
  });
  const rows = rowsOf(html);
  const shop = rows.get("home/shop");
  assert.match(shop, /data-phase="Ready" data-health="Healthy"/);
  assert.match(shop, /data-owner="lin"/);
  assert.match(shop, /data-paused="false"/);
  assert.match(shop, /data-replicas="2"/);
  assert.match(shop, /data-condition="Reconciled" data-status="True"/);
  assert.match(shop, /下游对象已收敛/);

  const uninstalling = rows.get("home/reports");
  assert.match(uninstalling, /data-phase="Uninstalling" data-health="Unknown"/);
  assert.match(uninstalling, /data-uninstall="true"/);
  assert.match(uninstalling, /data-uninstall-stalled="true"/);
  assert.match(uninstalling, /Volume/);
  assert.match(uninstalling, /home\/data/);
  assert.doesNotMatch(uninstalling, /data-entry-item=/, "an uninstalling row offers no entry");
});

test("each row offers exactly the entries its target list names", () => {
  const html = renderPage({
    applications: [applicationRow(), applicationRow({ name: "reports" })],
    targets: {
      ...noTargets(),
      [ITEM_STOP]: [target()],
      [ITEM_RESTART]: [target(), target({ name: "reports" })],
      [ITEM_SCALE]: [target({ name: "reports", replicas: 4 })],
      [ITEM_RESOURCE_SPEC]: [target()],
      [ITEM_UNINSTALL]: [target()],
    },
  });
  const rows = rowsOf(html);
  const entriesOf = (row) => [...row.matchAll(/data-entry-item="([^"]+)"/g)].map((match) => match[1]);

  assert.deepEqual(entriesOf(rows.get("home/shop")), [
    ITEM_STOP,
    ITEM_RESTART,
    ITEM_RESOURCE_SPEC,
    ITEM_UNINSTALL,
  ]);
  assert.deepEqual(entriesOf(rows.get("home/reports")), [ITEM_RESTART, ITEM_SCALE]);
});

test("an empty display item states why it is empty", () => {
  const html = renderPage({
    applications: [],
    installable: { rule: RULE_INSTALLABLE, entries: [] },
    targets: noTargets(),
  });
  assert.match(html, /data-empty="no-applications"/);
  assert.match(html, /暂无数据：当前身份看不到任何应用，或集群尚未安装应用/);
  assert.match(html, /data-empty="no-installable"/);
  assert.match(html, /暂无数据：目录没有可安装的应用定义/);
});

test("an identity without a single target receives no control at all", () => {
  const html = renderPage({ targets: noTargets() });
  assert.match(rowsOf(html).get("home/shop"), /home\/shop/, "the row is still readable");
  assert.doesNotMatch(html, /<form/, "no entry form is rendered");
  assert.doesNotMatch(html, /<button/, "no entry button is rendered");
  assert.doesNotMatch(html, /data-entry-item=/);
});

test("a refused direct call states the control plane's code and reason", async () => {
  const { fixture, calls, shell } = await boot({
    page: applicationsPage(),
    write: () => jsonResponse(CONTROL_PLANE_FORBIDDEN, { status: 403 }),
  });
  try {
    const form = formIn(fixture.root.innerHTML, ITEM_STOP);
    assert.ok(form, "the page offers the stop entry to an owning identity");
    fixture.dispatchRoot("submit", { target: form, preventDefault() {} });
    await shell.settle();

    const write = calls.find((call) => call.options.method === "POST");
    assert.ok(write, "the accepted control was submitted to the control plane");
    assert.deepEqual(JSON.parse(write.options.body), { kind: "Application", namespace: "home", name: "shop", action: "stop" });

    const html = fixture.root.innerHTML;
    assert.match(html, /data-entry-notice="error"/);
    assert.match(html, /data-code="permission_denied"/);
    assert.match(html, /当前身份不能对/);
    assert.doesNotMatch(html, /data-entry-notice="accepted"/);
    assert.equal(calls.filter((call) => call.options.method === "POST").length, 1, "the refusal is not retried");
  } finally {
    shell.channel.stop();
  }
});

test("an accepted call is read back from the application object's phase and conditions", async () => {
  let answer = applicationsPage();
  const { fixture, calls, shell } = await boot({
    page: () => answer,
    write: () => {
      answer = applicationsPage({
        applications: [
          applicationRow({
            paused: true,
            conditions: [
              { type: "Reconciled", status: "True", reason: "Paused", message: "应用已暂停" },
            ],
          }),
        ],
        targets: { ...noTargets(), [ITEM_START]: [target({ paused: true })] },
      });
      return jsonResponse({ accepted: true, ref: { kind: "Application", namespace: "home", name: "shop" } }, { status: 202 });
    },
  });
  try {
    fixture.dispatchRoot("submit", { target: formIn(fixture.root.innerHTML, ITEM_STOP), preventDefault() {} });
    await shell.settle();

    const reads = calls.filter((call) => call.url === "/api/v1/pages/applications");
    assert.equal(reads.length, 2, "the window re-reads the page after the write");
    assert.deepEqual(calls.map((call) => call.options.method), ["GET", "GET", "POST", "GET"]);

    const html = fixture.root.innerHTML;
    assert.match(html, /data-entry-notice="accepted"/);
    assert.match(html, /已重新读取/);
    const row = rowsOf(html).get("home/shop");
    assert.match(row, /data-paused="true"/);
    assert.match(row, /应用已暂停/, "the condition the object now carries is rendered");
    assert.match(row, /data-entry-item="start-applications"/, "the start entry follows the object's new state");
    assert.doesNotMatch(row, /data-entry-item="stop-applications"/);
    assert.doesNotMatch(html, /任务|data-task/, "no task record is introduced");
  } finally {
    shell.channel.stop();
  }
});
