// The window's control-plane path: the page reads the shell uses, the
// credential every call carries, the failure it reports when the control plane
// does not answer, and the declared entry writes the applications page submits
// (KOSD-C-12). Reads stay GET; a write carries only the method, path and
// body its page entry declared.

import assert from "node:assert/strict";
import test from "node:test";

import { CODE_INVALID_RESPONSE, CODE_UNREACHABLE, createAPI, listPath, readPath } from "../../assets/api.js";
import { CONTROL_PLANE_FORBIDDEN, CONTROL_PLANE_NOT_FOUND } from "../acceptance/fixtures/applications.js";

const RESOURCE_PATH = "/api/v1/resources/Application/shop?namespace=home";

function recorder(answer) {
  const calls = [];
  const fetchFn = async (url, options) => {
    calls.push({ url, options });
    if (answer instanceof Error) throw answer;
    return answer;
  };
  return { fetchFn, calls };
}

function jsonResponse(body, status = 200) {
  return { ok: status >= 200 && status < 300, status, json: async () => body };
}

test("the page reads are GET requests on the control-plane paths", () => {
  assert.equal(listPath(), "/api/v1/pages");
  assert.equal(readPath("overview"), "/api/v1/pages/overview");
  assert.equal(readPath("a/b"), "/api/v1/pages/a%2Fb");
});

test("every request carries the application token and no console credential", async () => {
  const { fetchFn, calls } = recorder(jsonResponse({ items: [] }));
  const api = createAPI({ token: "app-token-1", fetchFn });

  await api.listPages();
  await api.readPage("overview");

  assert.deepEqual(calls.map((call) => call.url), ["/api/v1/pages", "/api/v1/pages/overview"]);
  for (const call of calls) {
    assert.equal(call.options.method, "GET");
    assert.equal(call.options.headers.Authorization, "Bearer app-token-1");
    assert.equal(call.options.credentials, "omit");
    assert.equal(call.options.body, undefined);
  }
});

test("a base origin is prefixed when the document is served elsewhere", async () => {
  const { fetchFn, calls } = recorder(jsonResponse({ items: [] }));
  await createAPI({ base: "https://kos.example", token: "t", fetchFn }).listPages();
  assert.equal(calls[0].url, "https://kos.example/api/v1/pages");
});

test("a failure keeps the control plane's code and message from its envelope", async () => {
  const { fetchFn } = recorder(jsonResponse(CONTROL_PLANE_NOT_FOUND, 404));
  const result = await createAPI({ token: "t", fetchFn }).readPage("absent");
  assert.equal(result.ok, false);
  assert.equal(result.error.code, "not_found");
  assert.equal(result.error.message, CONTROL_PLANE_NOT_FOUND.error.message);
});

test("a flat failure body is still readable", async () => {
  const { fetchFn } = recorder(jsonResponse({ code: "conflict", message: "对象已被其他写入修改" }, 409));
  const result = await createAPI({ token: "t", fetchFn }).readPage("applications");
  assert.equal(result.ok, false);
  assert.equal(result.error.code, "conflict");
  assert.equal(result.error.message, "对象已被其他写入修改");
});

test("a failure without a control-plane body still names the status", async () => {
  const { fetchFn } = recorder({ ok: false, status: 405, json: async () => null });
  const result = await createAPI({ token: "t", fetchFn }).readPage("overview");
  assert.equal(result.ok, false);
  assert.equal(result.error.code, "http_405");
});

test("a transport that never answers is unreadable, not empty", async () => {
  const { fetchFn } = recorder(new Error("network down"));
  const result = await createAPI({ token: "t", fetchFn }).listPages();
  assert.equal(result.ok, false);
  assert.equal(result.error.code, CODE_UNREACHABLE);
  assert.match(result.error.message, /network down/);
});

test("a body the page cannot read is reported as such", async () => {
  const { fetchFn } = recorder({ ok: true, status: 200, json: async () => null });
  const result = await createAPI({ token: "t", fetchFn }).listPages();
  assert.equal(result.ok, false);
  assert.equal(result.error.code, CODE_INVALID_RESPONSE);
});

test("the reads are GET and the entry write submits the method it is given", async () => {
  const { fetchFn, calls } = recorder(jsonResponse({ accepted: true }));
  const api = createAPI({ token: "t", fetchFn });

  await api.listPages();
  await api.readPage("applications");
  await api.readResource(RESOURCE_PATH);
  await api.writeEntry({ method: "PUT", path: RESOURCE_PATH, headers: { "If-Match": "7" }, body: { spec: {} } });

  assert.deepEqual(calls.map((call) => call.options.method), ["GET", "GET", "GET", "PUT"]);
  assert.deepEqual(calls.map((call) => call.url), [
    "/api/v1/pages",
    "/api/v1/pages/applications",
    RESOURCE_PATH,
    RESOURCE_PATH,
  ]);
  for (const call of calls) {
    assert.equal(call.options.credentials, "omit");
    assert.equal(call.options.headers.Authorization, "Bearer t");
  }
  for (const call of calls.slice(0, 3)) assert.equal(call.options.body, undefined);
  assert.equal(calls[3].options.headers["If-Match"], "7");
  assert.equal(calls[3].options.headers["Content-Type"], "application/json");
  assert.equal(calls[3].options.body, JSON.stringify({ spec: {} }));
  assert.deepEqual(Object.keys(api).sort(), ["listPages", "readPage", "readResource", "writeEntry"]);
});

test("a refused write keeps the envelope's code and reason", async () => {
  const { fetchFn } = recorder(jsonResponse(CONTROL_PLANE_FORBIDDEN, 403));
  const result = await createAPI({ token: "t", fetchFn }).writeEntry({
    method: "POST",
    path: "/api/v1/action-requests",
    body: { kind: "Application", namespace: "home", name: "shop", action: "stop" },
  });
  assert.equal(result.ok, false);
  assert.equal(result.error.code, "permission_denied");
  assert.equal(result.error.message, CONTROL_PLANE_FORBIDDEN.error.message);
});

test("a write that never reaches the control plane is unreachable", async () => {
  const { fetchFn } = recorder(new Error("network down"));
  const result = await createAPI({ token: "t", fetchFn }).writeEntry({
    method: "POST",
    path: "/api/v1/action-requests",
    body: {},
  });
  assert.equal(result.ok, false);
  assert.equal(result.error.code, CODE_UNREACHABLE);
});
