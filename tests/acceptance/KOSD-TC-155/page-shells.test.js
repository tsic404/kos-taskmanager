// The four page surfaces travel inside the package: the document the window
// loads, its stylesheet and the four page shells. This case reads the files the
// packer ships, so a page shell that never loads is caught here rather than in a
// browser.

import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { renderShell } from "../../../assets/app.js";
import { ROUTES } from "../../../assets/router.js";

const ASSETS = fileURLToPath(new URL("../../../assets/", import.meta.url));
const PAGES = ["overview", "applications", "detail", "pods"];

async function asset(name) {
  return readFile(path.join(ASSETS, name), "utf8");
}

test("the document loads the packaged shell and icon", async () => {
  const document = await asset("index.html");
  assert.match(document, /src="\.\/app\.js"/);
  assert.match(document, /href="\.\/shell\.css"/);
  assert.match(document, /href="\.\/icon\.svg"/);
  assert.ok(existsSync(path.join(ASSETS, "icon.svg")));
});

test("every page surface ships as an asset", async () => {
  for (const page of PAGES) {
    assert.ok(existsSync(path.join(ASSETS, "pages", `${page}.js`)), `${page} shell is missing`);
  }
});

test("the packaged shell renders all four surfaces", async () => {
  for (const route of ROUTES) {
    assert.ok(PAGES.includes(route.id), `${route.id} has no page shell`);
  }
  const cases = [
    ["#/overview", "overview"],
    ["#/applications", "applications"],
    ["#/applications/web", "detail"],
    ["#/applications/web/pods", "pods"],
  ];
  for (const [hash, page] of cases) {
    assert.match(renderShell({ hash, status: "standalone" }), new RegExp(`data-page="${page}"`));
  }
});

test("the shell holds no console session material", async () => {
  // KOSD-F-9: the document reads neither the console cookie nor cluster
  // credentials; its only credential is the token the handshake delivers.
  for (const name of ["app.js", "handshake.js", "router.js", "html.js", ...PAGES.map((page) => `pages/${page}.js`)]) {
    const source = await asset(name);
    assert.doesNotMatch(source, /document\.cookie/, `${name} reads the console cookie`);
    assert.doesNotMatch(source, /kubeconfig|serviceaccount|service-account/i, `${name} names cluster credentials`);
    assert.doesNotMatch(source, /localStorage|sessionStorage/, `${name} persists state locally`);
  }
});
