// The application side of the console channel: the ready handshake, the single
// accepted init, and the liveness and unsaved reports.

import assert from "node:assert/strict";
import test from "node:test";

import {
  createChannel,
  consoleOriginFrom,
  normalizeOrigin,
  MESSAGE_ALIVE,
  MESSAGE_INIT,
  MESSAGE_READY,
  MESSAGE_UNSAVED,
} from "../../assets/handshake.js";

const CONSOLE_ORIGIN = "https://console.kos.tsic.top";
const NOW = Date.parse("2026-11-12T12:00:00Z");

// initMessage is what the console posts back: the token it issued for this
// window, the theme, the granted capabilities and when the token expires.
function initMessage(overrides = {}) {
  return {
    type: MESSAGE_INIT,
    token: "app-token-1",
    theme: "system",
    capabilities: [],
    expiresAt: "2026-11-12T12:05:00Z",
    ...overrides,
  };
}

function harness(overrides = {}) {
  const posted = [];
  const hostWindow = { postMessage: (message, target) => posted.push({ message, target }) };
  const timers = [];
  const channel = createChannel({
    hostWindow,
    consoleOrigin: CONSOLE_ORIGIN,
    windowID: "win-1",
    declaredPermissions: [],
    now: () => NOW,
    setIntervalFn: (fn, ms) => {
      timers.push({ fn, ms });
      return timers.length;
    },
    clearIntervalFn: () => {},
    ...overrides,
  });
  return { channel, posted, timers, hostWindow };
}

test("the console origin is read from the window address and must be an origin", () => {
  assert.equal(consoleOriginFrom("?kos-console=https%3A%2F%2Fconsole.kos.tsic.top"), CONSOLE_ORIGIN);
  assert.equal(consoleOriginFrom("?kos-console=http%3A%2F%2F127.0.0.1%3A8080"), "http://127.0.0.1:8080");
  assert.equal(consoleOriginFrom(""), "");
  assert.equal(consoleOriginFrom("?kos-console=*"), "");
});

test("plain HTTP is admitted only on a loopback host", () => {
  assert.equal(normalizeOrigin("http://localhost:8080"), "http://localhost:8080");
  assert.equal(normalizeOrigin("http://127.0.0.1:8080"), "http://127.0.0.1:8080");
  assert.equal(normalizeOrigin("http://console.kos.tsic.top"), "");
  assert.equal(normalizeOrigin("https://user:secret@console.kos.tsic.top"), "");
  assert.equal(normalizeOrigin("https://console.kos.tsic.top/apps"), "");
  assert.equal(normalizeOrigin("https://console.kos.tsic.top/?x=1"), "");
});

test("ready names the window instance and targets the console origin", () => {
  const { channel, posted } = harness();
  assert.equal(channel.ready(), true);
  assert.deepEqual(posted, [
    { message: { type: MESSAGE_READY, window: "win-1" }, target: CONSOLE_ORIGIN },
  ]);
});

test("a standalone window sends nothing and accepts no init", () => {
  const { channel, posted } = harness({ consoleOrigin: "" });
  assert.equal(channel.ready(), false);
  assert.equal(posted.length, 0);
  assert.equal(channel.accept({ origin: CONSOLE_ORIGIN, source: null, data: initMessage() }), null);
});

test("an init the console did not send is ignored", () => {
  const { channel, hostWindow } = harness();
  const cases = [
    { origin: "https://evil.example", source: hostWindow, data: initMessage() },
    { origin: CONSOLE_ORIGIN, source: {}, data: initMessage() },
    { origin: CONSOLE_ORIGIN, source: hostWindow, data: { ...initMessage(), account: "owner" } },
    { origin: CONSOLE_ORIGIN, source: hostWindow, data: initMessage({ type: "alive" }) },
    { origin: CONSOLE_ORIGIN, source: hostWindow, data: initMessage({ token: { value: "app-token-1" } }) },
    { origin: CONSOLE_ORIGIN, source: hostWindow, data: initMessage({ token: "" }) },
    { origin: CONSOLE_ORIGIN, source: hostWindow, data: initMessage({ theme: undefined }) },
    { origin: CONSOLE_ORIGIN, source: hostWindow, data: initMessage({ capabilities: "none" }) },
    { origin: CONSOLE_ORIGIN, source: hostWindow, data: initMessage({ capabilities: ["notifications"] }) },
    { origin: CONSOLE_ORIGIN, source: hostWindow, data: initMessage({ expiresAt: "soon" }) },
    { origin: CONSOLE_ORIGIN, source: hostWindow, data: initMessage({ expiresAt: "2026-11-12T11:59:00Z" }) },
  ];
  for (const event of cases) {
    assert.equal(channel.accept(event), null, JSON.stringify(event));
    assert.equal(channel.session, null);
  }
});

test("the first valid init establishes the session and starts the liveness beat", () => {
  const { channel, posted, timers, hostWindow } = harness();
  const session = channel.accept({ origin: CONSOLE_ORIGIN, source: hostWindow, data: initMessage() });
  assert.equal(session.token, "app-token-1");
  assert.equal(session.theme, "system");
  assert.deepEqual(session.capabilities, []);
  assert.equal(session.expiresAt, Date.parse("2026-11-12T12:05:00Z"));
  assert.deepEqual(posted.map((entry) => entry.message.type), [MESSAGE_ALIVE]);
  assert.equal(timers.length, 1);
  assert.ok(timers[0].ms > 0 && timers[0].ms < 35000);

  timers[0].fn();
  assert.deepEqual(posted.at(-1), {
    message: { type: MESSAGE_ALIVE, window: "win-1" },
    target: CONSOLE_ORIGIN,
  });

  // A second init changes nothing: the token crosses the handshake once.
  const other = initMessage({ token: "another-token" });
  assert.equal(channel.accept({ origin: CONSOLE_ORIGIN, source: hostWindow, data: other }), null);
  assert.equal(channel.session.token, "app-token-1");
});

test("unsaved is reported only after the handshake and never carries the token", () => {
  const { channel, posted, hostWindow } = harness();
  assert.equal(channel.reportUnsaved(true), false);
  channel.accept({ origin: CONSOLE_ORIGIN, source: hostWindow, data: initMessage() });
  assert.equal(channel.reportUnsaved(true), true);
  assert.deepEqual(posted.at(-1), {
    message: { type: MESSAGE_UNSAVED, unsaved: true, window: "win-1" },
    target: CONSOLE_ORIGIN,
  });
  assert.equal(channel.reportUnsaved(false), true);
  for (const entry of posted) {
    assert.doesNotMatch(JSON.stringify(entry.message), /app-token-1/);
  }
});

test("stop clears the session and the beat", () => {
  let cleared = null;
  const { channel, hostWindow } = harness({ clearIntervalFn: (handle) => (cleared = handle) });
  channel.accept({ origin: CONSOLE_ORIGIN, source: hostWindow, data: initMessage() });
  channel.stop();
  assert.equal(channel.session, null);
  assert.equal(cleared, 1);
  assert.equal(channel.reportUnsaved(true), false);
});
