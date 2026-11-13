// TASK-TC-1 from the page side: the overview renders every declared item with
// the source type it is judged by, the moment it was observed, the control
// plane's freshness verdict, and the value its source reports. Aggregate items
// carry the rule that judged them (KOSD-C-53), and the recent events are shown
// in the window and order the read returned them (KOSD-AC-59).

import assert from "node:assert/strict";
import test from "node:test";

import * as overview from "../../../assets/pages/overview.js";
import { OBSERVED_AT, RULE_ABNORMAL, RULE_UPGRADE, overviewPage } from "../fixtures/projection.js";

// SOURCES is the source type every item must declare (KOSD-C-53).
const SOURCES = [
  ["nodes-ready", "object"],
  ["abnormal-objects", "computed"],
  ["pending-upgrades", "computed"],
  ["recent-events", "object"],
  ["maintenance-nodes", "object"],
  ["maintenance-origin", "database"],
  ["maintenance-affected-applications", "computed"],
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

function renderPage(options = {}) {
  return overview.render({ projection: { state: "ready", page: overviewPage(options) } });
}

test("every item is rendered once, in declaration order, with its source type", () => {
  const html = renderPage();
  const blocks = blocksOf(html);
  assert.deepEqual([...blocks.keys()], SOURCES.map(([id]) => id));

  for (const [id, source] of SOURCES) {
    const block = blocks.get(id);
    assert.match(block, new RegExp(`^data-item="${id}" data-source="${source}"`), `${id} declares source ${source}`);
    assert.match(block, new RegExp(overview.SOURCE_LABELS[source]), `${id} shows its source type as text`);
  }
});

test("every item states when it was observed and whether it is stale", () => {
  const html = renderPage({ stale: ["recent-events"] });
  const blocks = blocksOf(html);

  for (const [id] of SOURCES) {
    const block = blocks.get(id);
    assert.match(block, /data-observed-at="2026-03-01T12:00:00Z"/, `${id} carries its observation time`);
    assert.match(block, /观测于 2026-03-01 12:00:00 UTC/, `${id} shows the observation moment`);
  }

  // One item past the two-minute bound is marked; the ones inside it are not
  // (KOSD-C-13).
  assert.match(blocks.get("recent-events"), /data-stale="true"/);
  assert.match(blocks.get("recent-events"), /数据陈旧/);
  for (const [id] of SOURCES) {
    if (id === "recent-events") continue;
    assert.doesNotMatch(blocks.get(id), /data-stale="true"/, `${id} is inside the freshness bound`);
  }
});

test("node readiness reports the count and each node's own condition", () => {
  const nodes = blocksOf(renderPage()).get("nodes-ready");
  assert.match(nodes, /data-figure="ready">3 \/ 4 就绪/);
  assert.match(nodes, /data-node="node-c" data-ready="false"/);
  assert.match(nodes, /data-node="node-a" data-ready="true"/);
  assert.match(nodes, /未就绪/);
});

test("abnormal objects carry the rule and the objects it finds", () => {
  const abnormal = blocksOf(renderPage()).get("abnormal-objects");
  assert.match(abnormal, new RegExp(`判定规则：${RULE_ABNORMAL}`));
  assert.match(abnormal, /data-object="home\/gateway"/);
  assert.match(abnormal, /data-object="home\/reports"/);
  assert.match(abnormal, /Failed · Degraded/);
  assert.match(abnormal, /data-condition="Reconciled" data-status="Unknown"/);
  assert.match(abnormal, /等待入口就绪/);
});

test("pending upgrades carry the rule, the target and the state", () => {
  const pending = blocksOf(renderPage()).get("pending-upgrades");
  assert.match(pending, new RegExp(`判定规则：${RULE_UPGRADE}`));
  assert.match(pending, /data-object="home\/reports" data-state="definition-changed"/);
  assert.match(pending, /定义已变更/);
  assert.match(pending, /sha256:reports-v1 → sha256:reports-v2（2.0.0）/);
  assert.match(pending, /data-object="home\/search" data-state="in-progress"/);
  assert.match(pending, /升级中/);
});

test("recent events keep the window and the newest-first order the read returned", () => {
  const events = blocksOf(renderPage()).get("recent-events");
  assert.match(events, /data-window="1h0m0s"/);
  assert.match(events, /窗口 1h0m0s，按时间倒序/);

  const order = [...events.matchAll(/data-event="([^"]+)"/g)].map((match) => match[1]);
  assert.deepEqual(order, ["shop-backoff", "node-b-drain", "reports-failed"]);
  assert.match(events, /2026-03-01 11:50:00 UTC/);
  assert.match(events, /Warning · BackOff/);
  assert.match(events, /Application home\/shop/);
  assert.match(events, /Node node-b/);
});

test("maintenance lists the nodes, their judged origin and the applications they affect", () => {
  const blocks = blocksOf(renderPage());
  const maintained = blocks.get("maintenance-nodes");
  for (const name of ["node-b", "node-c", "node-d"]) {
    assert.match(maintained, new RegExp(`data-node="${name}"`), `${name} is taken out of scheduling`);
  }

  const origins = blocks.get("maintenance-origin");
  assert.match(origins, /data-node="node-b" data-origin="planned"/);
  assert.match(origins, /计划维护/);
  assert.match(origins, /data-node="node-c" data-origin="unplanned"/);
  assert.match(origins, /非计划维护/);

  const affected = blocks.get("maintenance-affected-applications");
  assert.match(affected, /data-node="node-b"/);
  assert.match(affected, /home\/shop/);
});

test("an empty item says so instead of rendering nothing", () => {
  const page = overviewPage();
  for (const item of page.items) {
    if (item.id === "recent-events") item.value = { window: "1h0m0s", events: [] };
    if (item.id === "abnormal-objects") item.value = { rule: RULE_ABNORMAL, objects: [] };
  }
  const html = overview.render({ projection: { state: "ready", page } });
  assert.equal((html.match(/<p class="item-empty">暂无数据<\/p>/g) || []).length, 2, "an empty item keeps a visible empty state");
});

test("the page says it is read-only and why", () => {
  const html = renderPage();
  assert.match(html, /data-readonly="true">只读/);
  assert.match(html, /data-readonly-reason="true"/);
  assert.match(html, /本页只读/);
});

test("every value is escaped, so an object name cannot inject markup", () => {
  const page = overviewPage();
  page.items[0].value.nodes[0].name = "<img src=x>";
  const html = overview.render({ projection: { state: "ready", page } });
  assert.match(html, /&lt;img src=x&gt;/);
  assert.doesNotMatch(html, /<img/);
});

test("the observation moment is rendered in one fixed form", () => {
  assert.equal(overview.observedText(OBSERVED_AT), "2026-03-01 12:00:00 UTC");
  assert.equal(overview.observedText("not-a-time"), "not-a-time");
});
