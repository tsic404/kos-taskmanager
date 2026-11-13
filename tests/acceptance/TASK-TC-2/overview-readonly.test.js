// TASK-TC-2 from the window side: the overview offers no modification entry at
// all — not on the page, not on an item, even when the projection carries the
// action and entry fields a writable page would — and the window issues no write
// request while it reads the page.

import assert from "node:assert/strict";
import test from "node:test";

import { createShell } from "../../../assets/app.js";
import * as overview from "../../../assets/pages/overview.js";
import { overviewPage } from "../fixtures/projection.js";
import { bootstrapWindow, jsonResponse } from "../fixtures/window.js";

// writableProjection is the overview as a page that offered writes would look:
// it carries an action, a per-item entry and an action name on every item.
function writableProjection() {
  const page = overviewPage();
  page.actions = [{ id: "cordon", entry: { method: "POST", path: "/api/v1/maintenance" } }];
  for (const item of page.items) {
    item.action = "cordon";
    item.entry = { method: "POST", path: "/api/v1/maintenance" };
  }
  return page;
}

test("the rendered overview carries no modification entry", () => {
  const html = overview.render({ projection: { state: "ready", page: writableProjection() } });
  for (const control of [/<button/i, /<form/i, /<input/i, /<select/i, /<textarea/i, /data-action=/i, /data-entry=/i]) {
    assert.doesNotMatch(html, control, `the page renders ${control}`);
  }
  assert.doesNotMatch(html, /\/api\/v1\/maintenance/);
});

test("the window issues only GET reads while it shows the overview", async () => {
  const calls = [];
  const fetchFn = async (url, options) => {
    calls.push({ url, options });
    if (url === "/api/v1/pages") {
      return jsonResponse({ items: [{ id: "overview", readOnly: true }] });
    }
    return jsonResponse(writableProjection());
  };
  const fixture = bootstrapWindow();
  const shell = createShell({ document: fixture.document, window: fixture.window, fetchFn });
  try {
    await shell.settle();
    assert.equal(calls.length, 2, "the window reads the registry and one page");
    for (const call of calls) {
      assert.equal(call.options.method, "GET", `${call.url} is a read`);
      assert.equal(call.options.body, undefined, `${call.url} carries no body`);
    }
    assert.deepEqual(calls.map((call) => call.url), ["/api/v1/pages", "/api/v1/pages/overview"]);
    assert.match(fixture.root.innerHTML, /data-page="overview"/);
    assert.doesNotMatch(fixture.root.innerHTML, /<button|<form|<input/i);
  } finally {
    shell.channel.stop();
  }
});
