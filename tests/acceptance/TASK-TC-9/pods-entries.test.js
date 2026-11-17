// TASK-TC-9 from the page side: the delete entry appears only for rows the
// control plane's delete item names — the Owner and the Member owning the
// application — and a visible identity that may not write the application sees
// the page without a delete control. An identity that cannot see the
// application gets neither the page nor its navigation entry (KOSD-C-59).

import assert from "node:assert/strict";
import test from "node:test";

import { HIDDEN_NOTE } from "../../../assets/app.js";
import { render } from "../../../assets/pages/pods.js";
import { bootPodsWindow } from "../fixtures/pod-window.js";
import { deleteTarget, podFacts, podRead, podRow, podsPage } from "../fixtures/pods.js";

// pageHTML renders the Pod page for one projection, with one Pod projection
// answer so a row's controls are drawn.
function pageHTML(page) {
  return render({
    app: "home/shop",
    projection: { state: "ready", page },
    podState: { state: "ready", read: podRead() },
  });
}

test("a delete target renders the entry and a non-target renders none", () => {
  const allowedHTML = pageHTML(podsPage({ deleteTargets: [deleteTarget()] }));
  assert.match(allowedHTML, /data-entry-item="pod-delete"/);
  assert.match(allowedHTML, /data-entry-path="\/api\/v1\/pods\/home\/shop\/shop-1"/);

  const readOnlyHTML = pageHTML(podsPage({ deleteTargets: [] }));
  assert.match(readOnlyHTML, /data-page="pods"/, "the visible identity still sees the page");
  assert.match(readOnlyHTML, /data-pod="shop-1"/, "the Pod list is still readable");
  assert.doesNotMatch(readOnlyHTML, /data-entry-item="pod-delete"/, "no delete control is offered");
  assert.doesNotMatch(readOnlyHTML, /<form class="entry"/);
});

test("an identity that cannot see the application gets neither the page nor its entry", async () => {
  const invisible = podsPage({
    applications: [podRow({ name: "reports" })],
    logs: [],
    deleteTargets: [],
  });
  const { fixture, shell } = await bootPodsWindow({ page: invisible });
  try {
    const html = fixture.root.innerHTML;
    assert.doesNotMatch(html, /data-page="pods"/, "the page is absent");
    assert.doesNotMatch(html, /data-route="pods"/, "the navigation entry is absent");
    assert.match(html, /data-page="unavailable"/);
    assert.match(html, new RegExp(HIDDEN_NOTE));
    assert.doesNotMatch(html, /home\/shop/, "no fact of the invisible application stands in");
  } finally {
    shell.channel.stop();
  }
});
