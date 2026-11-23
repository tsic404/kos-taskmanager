// TASK-TC-13 from the page side: a crashed provider leaves a Ready application
// Degraded and the application page offers the reinstall entry beside the other
// actions. The window draws the entry from the projection the control plane
// serves, submits the method, path and action that entry declares instead of a
// value of its own, and offers nothing for a row the control plane trimmed out
// of the entry's target list (TASK-C-2, KOSD-C-56, KOSD-C-59).

import assert from "node:assert/strict";
import test from "node:test";

import { createShell } from "../../../assets/app.js";
import {
  ITEM_REINSTALL,
  applicationRow,
  applicationsPage,
  formIn,
  noTargets,
  target,
} from "../fixtures/applications.js";
import { bootstrapWindow, jsonResponse } from "../fixtures/window.js";

// serve answers the registry and the page reads, and hands every write to the
// case so it can assert what the window actually submitted.
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

function submit(fixture, form) {
  assert.ok(form, "the page offers the reinstall entry");
  fixture.dispatchRoot("submit", { target: form, preventDefault() {} });
}

test("the reinstall entry renders from the projection and submits the declared action", async () => {
  const degraded = applicationRow({
    phase: "Ready",
    health: "Degraded",
    conditions: [
      { type: "CapabilityUnsatisfied", status: "True", reason: "CapabilityUnsatisfied", message: "Provider storage.volume 不可用" },
    ],
  });
  let answer = applicationsPage({
    applications: [degraded],
    targets: { ...noTargets(), [ITEM_REINSTALL]: [target({ phase: "Ready" })] },
  });
  const { fixture, calls, shell } = await boot({
    page: () => answer,
    write: () => {
      answer = applicationsPage({
        applications: [degraded],
        targets: { ...noTargets(), [ITEM_REINSTALL]: [target({ phase: "Ready" })] },
      });
      return jsonResponse({ accepted: true }, { status: 202 });
    },
  });
  try {
    const html = fixture.root.innerHTML;
    assert.match(html, /data-phase="Ready" data-health="Degraded"/, "the crashed provider is readable on the row");
    assert.match(html, /Provider storage\.volume 不可用/);

    const form = formIn(html, ITEM_REINSTALL);
    assert.equal(form.dataset.entryAction, "reinstall-application", "the action identity comes from the declaration");
    assert.equal(form.dataset.entryMethod, "POST", "the method comes from the declaration");
    assert.equal(form.dataset.entryPath, "/api/v1/action-requests", "the path comes from the declaration");
    assert.equal(form.dataset.targetName, "shop");

    submit(fixture, form);
    await shell.settle();

    const post = calls.find((call) => call.options.method === "POST");
    assert.ok(post, "the window submits the entry");
    assert.equal(post.url, "/api/v1/action-requests");
    assert.deepEqual(
      JSON.parse(post.options.body),
      { kind: "Application", namespace: "home", name: "shop", action: "reinstall" },
      "the body names the action value the control plane accepts",
    );
    assert.match(fixture.root.innerHTML, /data-entry-notice="accepted"/);
  } finally {
    shell.channel.stop();
  }
});

test("a row the control plane leaves out of the target list gets no reinstall control", async () => {
  const { fixture, shell } = await boot({
    page: () => applicationsPage({ targets: noTargets() }),
    write: () => jsonResponse({ accepted: true }, { status: 202 }),
  });
  try {
    assert.doesNotMatch(fixture.root.innerHTML, /data-entry-item="application-reinstall"/);
  } finally {
    shell.channel.stop();
  }
});
