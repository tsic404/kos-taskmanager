// The window's control-plane read path: the two page reads the overview uses,
// the credential it carries and the failure it reports when the control plane
// does not answer (KOSD-C-12). The module offers no write call at all.

import assert from "node:assert/strict";
import test from "node:test";

import { CODE_INVALID_RESPONSE, CODE_UNREACHABLE, createAPI, listPath, readPath } from "../../assets/api.js";

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

test("a failure keeps the control plane's code and message", async () => {
  const { fetchFn } = recorder(
    jsonResponse({ code: "not_found", message: "页面不存在，或提供它的组件未生效" }, 404),
  );
  const result = await createAPI({ token: "t", fetchFn }).readPage("absent");
  assert.equal(result.ok, false);
  assert.equal(result.error.code, "not_found");
  assert.equal(result.error.message, "页面不存在，或提供它的组件未生效");
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

test("the module exposes no write path", async () => {
  const { fetchFn, calls } = recorder(jsonResponse({}));
  const api = createAPI({ token: "t", fetchFn });
  assert.deepEqual(Object.keys(api).sort(), ["listPages", "readPage"]);
  for (const name of Object.keys(api)) {
    await api[name]("overview");
  }
  assert.equal(calls.every((call) => call.options.method === "GET"), true);
});
