// TASK-TC-8 from the page side: the delete form a Pod row renders submits the
// control-plane write the page declared, the accepted answer is stated, and the
// page re-reads the projection so the deleted Pod leaves the list and the
// replacement instance the runtime creates appears in its place (KOSD-C-77).

import assert from "node:assert/strict";
import test from "node:test";

import { bootPodsWindow, deleteFormOf } from "../fixtures/pod-window.js";
import { podFacts, podRead } from "../fixtures/pods.js";

test("deleting a Pod re-reads the page and lists the replacement instance", async () => {
  let deleted = false;
  const server = {
    pod: () =>
      podRead({
        items: deleted
          ? [podFacts({ name: "shop-2" }), podFacts({ name: "shop-3", phase: "Pending", ready: false })]
          : [podFacts({ name: "shop-1" }), podFacts({ name: "shop-2" })],
      }),
    onDelete: (url, options) => {
      assert.equal(options.method, "DELETE");
      assert.equal(url, "/api/v1/pods/home/shop/shop-1");
      deleted = true;
      return {
        ok: true,
        status: 202,
        json: async () => ({ accepted: true, ref: { kind: "Pod", namespace: "home", name: "shop-1" } }),
      };
    },
  };
  const { fixture, calls, shell } = await bootPodsWindow(server);
  try {
    assert.match(fixture.root.innerHTML, /data-pod="shop-1"/);
    const form = deleteFormOf(fixture.root.innerHTML, "shop-1");
    assert.ok(form, "the Pod row carries the delete entry");
    assert.equal(form.dataset.entryPath, "/api/v1/pods/home/shop/shop-1");
    assert.equal(form.dataset.entryMethod, "DELETE");
    assert.equal(form.dataset.entryItem, "pod-delete");

    fixture.dispatchRoot("submit", { target: form, preventDefault() {} });
    await shell.settle();

    const remove = calls.find((call) => call.method === "DELETE");
    assert.ok(remove, "the page submitted the declared delete");
    assert.equal(remove.url, "/api/v1/pods/home/shop/shop-1");

    // The projection is read again: once when the page opened and once after
    // the delete, so the list is the control plane's answer, not a local edit.
    const reads = calls.filter((call) => call.url === "/api/v1/pods/home/shop");
    assert.equal(reads.length, 2, "the Pod projection is re-read after the delete");

    const html = fixture.root.innerHTML;
    assert.doesNotMatch(html, /data-pod="shop-1"/, "the deleted Pod left the list");
    assert.match(html, /data-pod="shop-2"/, "the other replica stays listed");
    assert.match(html, /data-pod="shop-3"/, "the replacement instance appears");
    assert.match(html, /data-entry-notice="accepted"/);
  } finally {
    shell.channel.stop();
  }
});

test("a refused delete keeps the control plane's reason and the Pod row", async () => {
  const { fixture, shell } = await bootPodsWindow({
    onDelete: () => ({
      ok: false,
      status: 403,
      json: async () => ({ error: { code: "permission_denied", message: "当前身份不能删除这个 Pod" } }),
    }),
  });
  try {
    const form = deleteFormOf(fixture.root.innerHTML, "shop-1");
    assert.ok(form, "the Pod row carries the delete entry");
    fixture.dispatchRoot("submit", { target: form, preventDefault() {} });
    await shell.settle();

    const html = fixture.root.innerHTML;
    assert.match(html, /data-entry-notice="error" data-code="permission_denied"/);
    assert.match(html, /当前身份不能删除这个 Pod/);
    assert.match(html, /data-pod="shop-1"/, "a refused delete leaves the Pod listed");
  } finally {
    shell.channel.stop();
  }
});
