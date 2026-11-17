// TASK-TC-10 from the page side: a stopped application's Pod page states an
// empty list together with the reason the control plane gives, and renders no
// delete entry (KOSD-C-76). The stopped answer is only the answer of a read that
// succeeded: with the runtime unreachable the failure and its reason win, so a
// stopped application is never used to disguise an unreadable projection
// (KOSD-C-64).

import assert from "node:assert/strict";
import test from "node:test";

import { render } from "../../../assets/pages/pods.js";
import { bootPodsWindow } from "../fixtures/pod-window.js";
import { STOPPED_REASON, failedRead, podRow, podsPage } from "../fixtures/pods.js";

const STOPPED_HASH = "#/applications/home%2Fshop/pods";
const UNAVAILABLE = "集群 API 不可达，读取不可用";

test("a stopped application's page states an empty list and its reason", () => {
  const html = render({
    app: "home/shop",
    projection: {
      state: "ready",
      page: podsPage({
        applications: [podRow({ paused: true, reason: STOPPED_REASON })],
        logs: [],
        deleteTargets: [],
      }),
    },
    // The control plane answers a stopped application with an empty projection,
    // so the stopped answer follows a read that actually succeeded (KOSD-C-76).
    podState: { state: "ready", read: { items: [] } },
  });
  assert.match(html, /data-page="pods"/);
  assert.match(html, /data-empty="application-stopped"/);
  assert.match(html, new RegExp(STOPPED_REASON));
  assert.doesNotMatch(html, /data-empty="no-pods"/, "the empty answer is not the generic one");
  assert.doesNotMatch(html, /data-entry-item="pod-delete"/, "a stopped application offers no delete entry");
});

test("the window states the stopped empty list and offers no delete control", async () => {
  const { calls, fixture, shell } = await bootPodsWindow({
    hash: STOPPED_HASH,
    page: podsPage({
      applications: [podRow({ paused: true, reason: STOPPED_REASON })],
      logs: [],
      deleteTargets: [],
    }),
    // The control plane projects no Pods for a stopped application (KOSD-C-76).
    pod: { items: [] },
  });
  try {
    const html = fixture.root.innerHTML;
    assert.match(html, /data-empty="application-stopped"/, "the empty list is the stopped answer");
    assert.match(html, new RegExp(STOPPED_REASON), "the reason is stated");
    assert.doesNotMatch(html, /data-entry-item="pod-delete"/, "no delete control is offered");
    assert.doesNotMatch(html, /data-log-read="true"/, "no per-Pod log control is offered without a Pod");
    // The read that did run went to this application's own projection.
    for (const call of calls.filter((entry) => entry.url.startsWith("/api/v1/pods/"))) {
      assert.equal(call.url, "/api/v1/pods/home/shop");
    }
  } finally {
    shell.channel.stop();
  }
});

test("a stopped application whose projection fails states the failure, not the stopped empty list", async () => {
  const { fixture, shell } = await bootPodsWindow({
    hash: STOPPED_HASH,
    page: podsPage({
      applications: [podRow({ paused: true, reason: STOPPED_REASON })],
      logs: [],
      deleteTargets: [],
    }),
    pod: failedRead("cluster_unavailable", UNAVAILABLE),
  });
  try {
    const html = fixture.root.innerHTML;
    assert.match(html, /data-pod-state="error"/, "the unreadable projection is stated as a failure");
    assert.match(html, /data-code="cluster_unavailable"/, "the control plane's code survives");
    assert.match(html, new RegExp(UNAVAILABLE), "the reason survives");
    assert.doesNotMatch(
      html,
      /data-empty="application-stopped"/,
      "the cached stopped row never disguises an unreadable projection",
    );
    assert.doesNotMatch(html, /data-empty="no-pods"/, "a failed read is not an empty list");
  } finally {
    shell.channel.stop();
  }
});

test("switching to a stopped application's address after an outage keeps the failure", async () => {
  // The page row for the stopped application is already cached; the runtime is
  // then unreachable when the address switches to it (KOSD-C-76, KOSD-C-64).
  let reachable = true;
  const { fixture, shell } = await bootPodsWindow({
    app: "home/reports",
    page: podsPage({
      applications: [podRow({ name: "reports" }), podRow({ paused: true, reason: STOPPED_REASON })],
      logs: [],
      deleteTargets: [],
    }),
    pod: () => (reachable ? { items: [] } : failedRead("cluster_unavailable", UNAVAILABLE)),
  });
  try {
    assert.doesNotMatch(fixture.root.innerHTML, /data-pod-state="error"/);
    reachable = false;
    fixture.window.location.hash = "#/applications/home%2Fshop/pods";
    fixture.dispatch("hashchange", {});
    await shell.settle();

    const html = fixture.root.innerHTML;
    assert.match(html, /data-pod-state="error"/, "the unreadable projection is stated as a failure");
    assert.match(html, /data-code="cluster_unavailable"/, "the control plane's code survives the switch");
    assert.match(html, new RegExp(UNAVAILABLE), "the reason survives the switch");
    assert.doesNotMatch(
      html,
      /data-empty="application-stopped"/,
      "the cached stopped row does not withdraw the failure",
    );
  } finally {
    shell.channel.stop();
  }
});
