// The application side of the console channel: the `ready` handshake, the
// single accepted `init`, and the two reports of KOSD-C-81. Every dependency a
// browser would supply is injected, so the same checks run in a test.
//
// The channel holds no console session material: the application token lives in
// this module's memory and never reaches storage, a cookie or the document
// (KOSD-C-75).

export const MESSAGE_READY = "ready";
export const MESSAGE_INIT = "init";
export const MESSAGE_ALIVE = "alive";
export const MESSAGE_UNSAVED = "unsaved";

// HEARTBEAT_INTERVAL_MS is well inside the console's silence window, so a
// running application is never reported as stopped by a slow beat.
export const HEARTBEAT_INTERVAL_MS = 10000;

// INIT_FIELDS is the console's `init` message as the frame receives it: the
// token it issued for this window, the theme and the capabilities granted here.
// The token is opaque to the application — the console mints it and keeps only
// its digest — and the field set is closed, so a message the console did not
// send leaves the window uninitialised (KOSD-C-75).
const INIT_FIELDS = ["type", "token", "theme", "capabilities", "expiresAt"];

// consoleOriginFrom reads the console origin the window address carries. The
// address is issued by the console that frames the application, so the origin
// the application answers is the one it was loaded for, never a wildcard.
export function consoleOriginFrom(search) {
  const raw = new URLSearchParams(search || "").get("kos-console");
  return raw ? normalizeOrigin(raw) : "";
}

// windowIDFrom reads the window instance the console framed, which every
// message of KOSD-C-81 may name.
export function windowIDFrom(search) {
  return new URLSearchParams(search || "").get("kos-window") || "";
}

// normalizeOrigin accepts an origin and nothing else: plain HTTP is admitted
// only on a loopback host, which is how a local deployment serves the console.
export function normalizeOrigin(raw) {
  let parsed;
  try {
    parsed = new URL(raw);
  } catch {
    return "";
  }
  if (parsed.username || parsed.password) return "";
  if (parsed.pathname !== "/" || parsed.search || parsed.hash) return "";
  if (parsed.protocol === "https:") return parsed.origin;
  if (parsed.protocol === "http:" && isLoopback(parsed.hostname)) return parsed.origin;
  return "";
}

// createChannel binds one document to the console it was framed by.
export function createChannel({
  hostWindow,
  consoleOrigin,
  windowID = "",
  declaredPermissions = [],
  now = () => Date.now(),
  heartbeatMs = HEARTBEAT_INTERVAL_MS,
  setIntervalFn = setInterval,
  clearIntervalFn = clearInterval,
}) {
  let session = null;
  let heartbeat = null;

  function post(message) {
    if (!consoleOrigin || !hostWindow) return false;
    // The target origin is the console origin the address named; no message of
    // this channel is posted with a wildcard (KOSD-C-75).
    hostWindow.postMessage(message, consoleOrigin);
    return true;
  }

  // ready announces the loaded document once. Without a console origin the
  // application runs standalone, which is how the shell is inspected locally.
  function ready() {
    return post(windowID ? { type: MESSAGE_READY, window: windowID } : { type: MESSAGE_READY });
  }

  // accept judges one inbound event as the console's answer. Anything from
  // another window, another origin, of another shape or outside the declared
  // capability set is ignored and leaves the window uninitialised.
  function accept(event) {
    if (session || !consoleOrigin) return null;
    if (!event || event.source !== hostWindow) return null;
    if (event.origin !== consoleOrigin) return null;
    const accepted = readInit(event.data, { declaredPermissions, now: now() });
    if (!accepted) return null;
    session = accepted;
    postAlive();
    heartbeat = setIntervalFn(postAlive, heartbeatMs);
    return session;
  }

  // reportUnsaved states whether the window holds unsaved changes. The console
  // vetoes the first close while it is true (KOSD-C-81).
  function reportUnsaved(hasUnsaved) {
    if (!session) return false;
    return post(withWindow({ type: MESSAGE_UNSAVED, unsaved: Boolean(hasUnsaved) }));
  }

  function postAlive() {
    post(withWindow({ type: MESSAGE_ALIVE }));
  }

  function withWindow(message) {
    return windowID ? { ...message, window: windowID } : message;
  }

  function stop() {
    if (heartbeat !== null) {
      clearIntervalFn(heartbeat);
      heartbeat = null;
    }
    session = null;
  }

  return {
    ready,
    accept,
    reportUnsaved,
    stop,
    get session() {
      return session;
    },
  };
}

// readInit validates one `init` against this application. The field set is
// closed, the token must be the non-empty string the console issued, the theme
// must be present, and no capability may exceed what the manifest declares
// (KOSD-C-73, KOSD-C-75).
function readInit(data, { declaredPermissions = [], now = Date.now() } = {}) {
  if (!isPlainObject(data)) return null;
  if (!hasOnlyFields(data, INIT_FIELDS)) return null;
  if (data.type !== MESSAGE_INIT) return null;
  if (typeof data.token !== "string" || data.token === "") return null;
  if (!Array.isArray(data.capabilities)) return null;
  if (data.theme === undefined || data.theme === null) return null;

  const declared = new Set(declaredPermissions);
  const capabilities = [];
  for (const capability of data.capabilities) {
    if (typeof capability !== "string" || !declared.has(capability)) return null;
    capabilities.push(capability);
  }

  if (typeof data.expiresAt !== "string") return null;
  const expiresAt = Date.parse(data.expiresAt);
  if (Number.isNaN(expiresAt) || expiresAt <= now) return null;

  return {
    token: data.token,
    theme: data.theme,
    capabilities,
    expiresAt,
  };
}

function isLoopback(hostname) {
  const host = String(hostname || "").toLowerCase();
  return host === "localhost" || host === "127.0.0.1" || host === "[::1]" || host === "::1";
}

function isPlainObject(value) {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function hasOnlyFields(value, fields) {
  return Object.keys(value).every((key) => fields.includes(key));
}
