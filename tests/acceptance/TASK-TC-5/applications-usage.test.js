// TASK-TC-5 from the page side: with the metric source disconnected the resource
// usage item states that usage is unmeasurable and why, carries no number at any
// level and never presents zero as a measurement; the installed list, its health
// and the entry controls stay readable (TASK-C-2, KOSD-C-53). The connected
// source is read beside it, so the absence cases are not vacuously true.

import assert from "node:assert/strict";
import test from "node:test";

import * as applications from "../../../assets/pages/applications.js";
import {
  ITEM_INSTALLED,
  ITEM_RESOURCE_USAGE,
  ITEM_STOP,
  applicationRow,
  applicationsPage,
  target,
} from "../fixtures/applications.js";

const UNAVAILABLE_REASON = "发行版没有接入资源指标来源，KOS 不采集工作负载用量";
const UNREADABLE_REASON = "资源指标读取失败，应用用量暂不可读";

function renderPage(options = {}) {
  return applications.render({ projection: { state: "ready", page: applicationsPage(options) } });
}

// usageBlock is the rendered usage item, from its identity to the next item.
function usageBlock(html) {
  const start = html.indexOf(`data-item="${ITEM_RESOURCE_USAGE}"`);
  assert.notEqual(start, -1, "the usage item is rendered");
  const next = html.indexOf('data-item="', start + 1);
  return next === -1 ? html.slice(start) : html.slice(start, next);
}

// valueOf is the block's value area: the header (label, source, observation)
// stays out of the measurement assertions.
function valueOf(block) {
  const start = block.indexOf('data-reason="true"');
  return start === -1 ? block : block.slice(start);
}

test("an absent metric source renders the reason and no usage number", () => {
  const html = renderPage({ usage: { available: false, reason: UNAVAILABLE_REASON } });
  const usage = usageBlock(html);
  assert.match(usage, /data-usage="unavailable"/);
  assert.match(usage, new RegExp(UNAVAILABLE_REASON));
  assert.match(usage, /不可测量/);
  assert.doesNotMatch(valueOf(usage), /\d/, "no usage number or zero is presented");
  assert.doesNotMatch(usage, /data-usage-sample/);
  assert.doesNotMatch(usage, /cpu|memory|CPU|内存/i);
});

test("a failing metric source is unmeasurable too, and the rest of the page reads", () => {
  const html = renderPage({ usage: { available: false, reason: UNREADABLE_REASON } });
  const usage = usageBlock(html);
  assert.match(usage, /data-usage="unavailable"/);
  assert.match(usage, new RegExp(UNREADABLE_REASON));
  assert.doesNotMatch(valueOf(usage), /\d/);

  // The list, its health and the entry controls are unaffected by the metric
  // source: an unreachable metric source is not an unreachable cluster.
  const installed = html.slice(html.indexOf(`data-item="${ITEM_INSTALLED}"`), html.indexOf(`data-item="${ITEM_RESOURCE_USAGE}"`));
  assert.match(installed, /data-object="home\/shop"/);
  assert.match(installed, /data-health="Healthy"/);
  assert.match(installed, new RegExp(`data-entry-item="${ITEM_STOP}"`));
});

test("a connected source reports the samples it answered with", () => {
  const html = renderPage({
    usage: {
      available: true,
      applications: [{ namespace: "home", name: "shop", cpu: "25m", memory: "64Mi" }],
    },
  });
  const usage = usageBlock(html);
  assert.match(usage, /data-usage="available"/);
  assert.match(usage, /data-usage-sample="home\/shop"/);
  assert.match(usage, /data-cpu="25m"/);
  assert.match(usage, /data-memory="64Mi"/);
  assert.doesNotMatch(usage, /data-usage="unavailable"/);
});

test("a connected source without samples shows an empty state, never a zero", () => {
  const html = renderPage({ usage: { available: true, applications: [] } });
  const usage = usageBlock(html);
  assert.match(usage, /data-usage="available"/);
  assert.match(usage, /暂无数据：指标源本次没有返回样本/);
  assert.doesNotMatch(usage, /data-usage-sample/);
  assert.doesNotMatch(valueOf(usage), /\d/, "an empty sample list is not zero usage");
});

test("an installed row is still rendered only from what the projection carries", () => {
  const html = renderPage({
    applications: [applicationRow({ name: "reports", phase: "Failed", health: "Degraded" })],
    targets: { [ITEM_STOP]: [target({ name: "reports", phase: "Failed" })] },
    usage: { available: false, reason: UNAVAILABLE_REASON },
  });
  assert.match(html, /data-object="home\/reports"/);
  assert.match(html, /data-phase="Failed" data-health="Degraded"/);
  assert.match(html, /data-entry-item="stop-applications"/);
});
