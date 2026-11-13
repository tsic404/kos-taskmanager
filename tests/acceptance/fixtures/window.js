// The document, window and console the window cases boot: the console answers
// the handshake with one init message the application accepts (KOSD-C-75), so
// the shell reaches the state a framed window runs in.

export const CONSOLE_ORIGIN = "https://console.kos.tsic.top";
export const CONSOLE_ADDRESS = "?kos-console=https%3A%2F%2Fconsole.kos.tsic.top&kos-window=win-1";

// initMessage is the console's answer: a token, a theme and no capability.
export function initMessage(overrides = {}) {
  return {
    type: "init",
    token: "app-token-1",
    theme: "system",
    capabilities: [],
    expiresAt: "2999-01-01T00:00:00Z",
    ...overrides,
  };
}

// jsonResponse is one control-plane answer as fetch hands it back.
export function jsonResponse(body, { status = 200 } = {}) {
  return { ok: status >= 200 && status < 300, status, json: async () => body };
}

// bootstrapWindow stands up the shell's document and window. With answer the
// parent answers `ready` at once; the listeners are wired before that, exactly
// as the packaged entry does.
export function bootstrapWindow({
  search = CONSOLE_ADDRESS,
  hash = "#/overview",
  answer = true,
  init = initMessage(),
} = {}) {
  const rootListeners = new Map();
  const root = {
    innerHTML: "",
    dataset: {},
    addEventListener(type, handler) {
      rootListeners.set(type, [...(rootListeners.get(type) || []), handler]);
    },
  };
  const document = { getElementById: (id) => (id === "shell" ? root : null) };
  const listeners = new Map();
  const replaced = [];
  const posted = [];
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
      listeners.set(type, [...(listeners.get(type) || []), handler]);
    },
  };
  window.parent = {
    postMessage(message, target) {
      posted.push({ message, target });
      if (!answer || message.type !== "ready") return;
      for (const handler of listeners.get("message") || []) {
        handler({ origin: CONSOLE_ORIGIN, source: window.parent, data: init });
      }
    },
  };
  return {
    document,
    window,
    root,
    replaced,
    posted,
    dispatch(type, event) {
      for (const handler of listeners.get(type) || []) handler(event);
    },
    dispatchRoot(type, event) {
      for (const handler of rootListeners.get(type) || []) handler(event);
    },
  };
}
