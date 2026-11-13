// The window document's control-plane path: the pages a session may see, one
// page projection and the object read a resource form needs (KOSD-C-12,
// KOSD-C-53), plus the writes a page's declared entries name — the lifecycle
// request, the application resource and the version-preconditioned delete
// (KOSD-C-12, KOSD-C-33, KOSD-C-67). Every call carries the application session
// token the handshake delivered and nothing else (KOSD-C-75); no console cookie
// is sent.
export const API_PREFIX = "/api/v1";

// UNREACHABLE marks a request that never produced a control-plane answer, so
// the page states the reason instead of showing an empty projection (KOSD-C-64).
export const CODE_UNREACHABLE = "unreachable";

// CODE_INVALID_RESPONSE marks a body the page cannot read.
export const CODE_INVALID_RESPONSE = "invalid_response";

// listPath is the page registry of the caller's role.
export function listPath() {
  return `${API_PREFIX}/pages`;
}

// readPath is one declared page.
export function readPath(id) {
  return `${API_PREFIX}/pages/${encodeURIComponent(id)}`;
}

// createAPI binds the document to one control-plane origin and session token.
// base is relative by default: the packaged entry origin also serves the
// control-plane path, so the window never guesses another origin.
export function createAPI({ base = "", token = "", fetchFn = globalThis.fetch } = {}) {
  async function getJSON(path) {
    let response;
    try {
      response = await fetchFn(`${base}${path}`, {
        method: "GET",
        headers: { Accept: "application/json", Authorization: `Bearer ${token}` },
        // The console session never reaches the application origin (KOSD-C-75).
        credentials: "omit",
      });
    } catch (error) {
      return { ok: false, error: { code: CODE_UNREACHABLE, message: describe(error) } };
    }
    const body = await jsonOf(response);
    if (!response.ok) {
      return { ok: false, error: failureOf(body, response.status) };
    }
    if (body === null || typeof body !== "object") {
      return { ok: false, error: { code: CODE_INVALID_RESPONSE, message: "控制面返回了无法读取的响应" } };
    }
    return { ok: true, body };
  }

  // sendJSON submits one write the page declared and states its answer as it
  // came: a refused write keeps the control plane's code and reason instead of
  // being turned into a page-local verdict (KOSD-C-12).
  async function sendJSON({ method, path, body, headers = {} }) {
    const requestHeaders = { Accept: "application/json", Authorization: `Bearer ${token}`, ...headers };
    if (body !== undefined) requestHeaders["Content-Type"] = "application/json";
    let response;
    try {
      response = await fetchFn(`${base}${path}`, {
        method,
        headers: requestHeaders,
        credentials: "omit",
        body: body === undefined ? undefined : JSON.stringify(body),
      });
    } catch (error) {
      return { ok: false, error: { code: CODE_UNREACHABLE, message: describe(error) } };
    }
    const answer = await jsonOf(response);
    if (!response.ok) {
      return { ok: false, error: failureOf(answer, response.status) };
    }
    return { ok: true, body: answer === null ? {} : answer };
  }

  return {
    // listPages returns the pages this session role may see (DASH-C-22).
    listPages: () => getJSON(listPath()),
    // readPage returns one page projection, trimmed to the session (KOSD-C-59).
    readPage: (id) => getJSON(readPath(id)),
    // readResource reads one object the declared entries act on; the resource
    // form starts its write from what this read carries (KOSD-C-67).
    readResource: (path) => getJSON(path),
    // writeEntry submits one declared entry and returns its answer unchanged.
    writeEntry: (request) => sendJSON(request),
  };
}

// failureOf reads the stable failure envelope KOSD-C-12 fixes: the control
// plane answers a failure as {"error": {code, message, ...}} (its WriteError),
// so the code and the user-facing reason are one level down. A flat body is
// still accepted, and a body without a code states the HTTP status rather than
// pretending the call succeeded.
function failureOf(body, status) {
  const failure = body && typeof body === "object" && body.error && typeof body.error === "object" ? body.error : body;
  if (failure && typeof failure === "object" && typeof failure.code === "string" && failure.code !== "") {
    return { code: failure.code, message: describeBody(failure, status) };
  }
  return { code: `http_${status}`, message: `控制面返回 HTTP ${status}` };
}

function describeBody(body, status) {
  if (typeof body.message === "string" && body.message !== "") return body.message;
  return `控制面返回 HTTP ${status}`;
}

async function jsonOf(response) {
  if (typeof response.json !== "function") return null;
  try {
    return await response.json();
  } catch {
    return null;
  }
}

function describe(error) {
  if (error instanceof Error && error.message) return error.message;
  return String(error ?? "控制面不可达");
}
