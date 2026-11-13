// The window document's bootstrap: the packaged document greets the console
// that framed it, accepts the init, and renders the page its address names.
// The document, the window and its messages are stubs, so the case runs without
// a browser and fails on a listener wired after `ready` or on an address that
// resolves to no page shell.

import assert from "node:assert/strict";
import test from "node:test";

import { createShell } from "../../assets/app.js";

const CONSOLE_ORIGIN = "https://console.kos.tsic.top";
const CONSOLE_ADDRESS = "?kos-console=https%3A%2F%2Fconsole.kos.tsic.top&kos-window=win-1";

function initMessage() {
  return {
    type: "init",
    token: "app-token-1",
    theme: "system",
    capabilities: [],
    expiresAt: "2999-01-01T00:00:00Z",
  };
}

// bootstrap stands up the document and the window a browser would hand the
// packaged entry. With consoleAnswers the parent answers `ready` in the same
// turn, which is how a stuck handshake shows up as a missed message.
function bootstrap({ search = "", hash = "", consoleAnswers = false } = {}) {
  const root = { innerHTML: "", dataset: {} };
  const document = { getElementById: (id) => (id === "shell" ? root : null) };
  const listeners = new Map();
  const posted = [];
  const replaced = [];
  const consoleOrigin = new URLSearchParams(search).get("kos-console") || "";

  const window = {
    parent: null,
    location: {
      search,
      hash,
      replace(next) {
        replaced.push(next);
        window.location.hash = next;
      },
    },
    addEventListener(type, handler) {
      const handlers = listeners.get(type) || [];
      handlers.push(handler);
      listeners.set(type, handlers);
    },
  };
  window.parent = {
    postMessage(message, target) {
      posted.push({ message, target });
      if (!consoleAnswers || message.type !== "ready") return;
      const handlers = listeners.get("message") || [];
      assert.ok(handlers.length > 0, "ready was posted before the window listened");
      for (const handler of handlers) {
        handler({ origin: consoleOrigin, source: window.parent, data: initMessage() });
      }
    },
  };

  function dispatch(type, event) {
    for (const handler of listeners.get(type) || []) handler(event);
  }

  return { document, window, root, posted, replaced, dispatch };
}

test("an address outside the table moves to the default page and renders it", () => {
  const fixture = bootstrap();
  const shell = createShell({ document: fixture.document, window: fixture.window });
  assert.ok(shell);
  assert.deepEqual(fixture.replaced, ["#/overview"]);
  assert.match(fixture.root.innerHTML, /data-page="overview"/);
  assert.equal(fixture.root.dataset.state, "ready");
  assert.equal(fixture.root.dataset.session, "none");
  assert.equal(fixture.posted.length, 0);
});

test("the document listens before it greets the console and accepts the answer", () => {
  const fixture = bootstrap({ search: CONSOLE_ADDRESS, hash: "#/overview", consoleAnswers: true });
  const shell = createShell({ document: fixture.document, window: fixture.window });
  try {
    assert.deepEqual(fixture.posted[0], {
      message: { type: "ready", window: "win-1" },
      target: CONSOLE_ORIGIN,
    });
    assert.equal(fixture.root.dataset.session, "established");
    assert.match(fixture.root.innerHTML, /已连接控制台/);
    assert.doesNotMatch(fixture.root.innerHTML, /app-token-1/);
  } finally {
    shell.channel.stop();
  }
});

test("a window the console has not answered stays pending and sends no liveness", () => {
  const fixture = bootstrap({ search: CONSOLE_ADDRESS, hash: "#/overview" });
  const shell = createShell({ document: fixture.document, window: fixture.window });
  try {
    assert.deepEqual(fixture.posted.map((entry) => entry.message.type), ["ready"]);
    assert.equal(fixture.root.dataset.session, "none");
    assert.match(fixture.root.innerHTML, /等待控制台握手/);
  } finally {
    shell.channel.stop();
  }
});

test("the address the window carries selects one of the four page shells", () => {
  const fixture = bootstrap({ hash: "#/overview" });
  const shell = createShell({ document: fixture.document, window: fixture.window });
  try {
    const cases = [
      ["#/overview", "overview"],
      ["#/applications", "applications"],
      ["#/applications/web", "detail"],
      ["#/applications/web/pods", "pods"],
    ];
    for (const [hash, page] of cases) {
      fixture.window.location.hash = hash;
      fixture.dispatch("hashchange");
      assert.match(fixture.root.innerHTML, new RegExp(`data-page="${page}"`), hash);
    }
    assert.deepEqual(fixture.replaced, []);
  } finally {
    shell.channel.stop();
  }
});
