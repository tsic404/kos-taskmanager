// The window document's control-plane read path: the pages a session may see
// and one page projection (KOSD-C-12, KOSD-C-53). Every call is a GET; the page
// is read-only and this module offers no write path at all (TASK-C-1).
//
// The credential is the application session token the handshake delivered
// (KOSD-C-75). It stays in memory; no console cookie is sent with a request.

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

  return {
    // listPages returns the pages this session role may see (DASH-C-22).
    listPages: () => getJSON(listPath()),
    // readPage returns one page projection, trimmed to the session (KOSD-C-59).
    readPage: (id) => getJSON(readPath(id)),
  };
}

// failureOf reads the stable error body KOSD-C-12 fixes; a body without a code
// still states the HTTP status rather than pretending the read succeeded.
function failureOf(body, status) {
  if (body && typeof body === "object" && typeof body.code === "string" && body.code !== "") {
    return { code: body.code, message: describeBody(body, status) };
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
