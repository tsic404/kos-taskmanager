// TASK-TC-11 from the page side: an unreachable runtime makes the Pod list and
// a Pod's log read state the control plane's reason instead of showing an empty
// list or an empty log (KOSD-C-76, KOSD-C-64), and the reads work again once
// the runtime answers.

import assert from "node:assert/strict";
import test from "node:test";

import { bootPodsWindow, logButtonOf } from "../fixtures/pod-window.js";
import { failedRead, logRead, podRead } from "../fixtures/pods.js";

const UNAVAILABLE = "集群 API 不可达，读取不可用";

test("an unreadable Pod projection states the reason, not an empty list", async () => {
  const { fixture, shell } = await bootPodsWindow({
    pod: failedRead("cluster_unavailable", UNAVAILABLE),
  });
  try {
    const html = fixture.root.innerHTML;
    assert.match(html, /data-pod-state="error"/);
    assert.match(html, /data-code="cluster_unavailable"/);
    assert.match(html, new RegExp(UNAVAILABLE));
    assert.doesNotMatch(html, /data-empty="no-pods"/, "a failed read is not an empty list");
    assert.doesNotMatch(html, /data-pod="shop-1"/);
  } finally {
    shell.channel.stop();
  }
});

test("an unreadable per-Pod log states the reason, not an empty log", async () => {
  const { fixture, shell } = await bootPodsWindow({
    log: failedRead("cluster_unavailable", UNAVAILABLE),
  });
  try {
    const button = logButtonOf(fixture.root.innerHTML, "shop-1");
    assert.ok(button, "the Pod row carries its log control");
    fixture.dispatchRoot("click", { target: button, preventDefault() {} });
    await shell.settle();

    const html = fixture.root.innerHTML;
    assert.match(html, /data-log-state="error"/);
    assert.match(html, /data-code="cluster_unavailable"/);
    assert.match(html, new RegExp(UNAVAILABLE));
    assert.doesNotMatch(html, /data-empty="no-log-lines"/, "a failed read is not an empty log");
  } finally {
    shell.channel.stop();
  }
});

test("the reads recover once the runtime answers again", async () => {
  let reachable = false;
  const server = {
    pod: () => (reachable ? podRead() : failedRead("cluster_unavailable", UNAVAILABLE)),
    log: () => (reachable ? logRead() : failedRead("cluster_unavailable", UNAVAILABLE)),
  };
  const first = await bootPodsWindow(server);
  try {
    assert.match(first.fixture.root.innerHTML, /data-pod-state="error"/);
  } finally {
    first.shell.channel.stop();
  }

  reachable = true;
  const second = await bootPodsWindow(server);
  try {
    const html = second.fixture.root.innerHTML;
    assert.match(html, /data-pod="shop-1"/, "the Pod list is readable again");
    assert.doesNotMatch(html, /data-pod-state="error"/);
    const button = logButtonOf(html, "shop-1");
    assert.ok(button, "the log control is back");
    second.fixture.dispatchRoot("click", { target: button, preventDefault() {} });
    await second.shell.settle();
    assert.match(second.fixture.root.innerHTML, /data-log-state="ready"/);
    assert.match(second.fixture.root.innerHTML, /shop-1 的行/);
  } finally {
    second.shell.channel.stop();
  }
});
