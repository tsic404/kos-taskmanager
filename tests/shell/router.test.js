// The window's four page surfaces: the route table, the navigation they render
// and the shell each address resolves to.

import assert from "node:assert/strict";
import test from "node:test";

import { ROUTES, matchRoute, renderNav, routeAddress } from "../../assets/router.js";
import { renderShell } from "../../assets/app.js";

test("the window declares the four page surfaces", () => {
  assert.deepEqual(
    ROUTES.map((route) => route.id),
    ["overview", "applications", "detail", "pods"],
  );
});

test("every page surface has an address that resolves to it", () => {
  assert.deepEqual(matchRoute("#/overview"), { route: ROUTES[0] });
  assert.deepEqual(matchRoute("#/applications"), { route: ROUTES[1] });
  assert.deepEqual(matchRoute("#/applications/web"), { route: ROUTES[2], app: "web" });
  assert.deepEqual(matchRoute("#/applications/web/pods"), { route: ROUTES[3], app: "web" });
  assert.equal(matchRoute("#/elsewhere"), null);
  assert.equal(matchRoute(""), null);
});

test("an address outside the table is not a page", () => {
  assert.equal(matchRoute("#/applications/web/logs"), null);
  assert.equal(matchRoute("#/applications/web/pods/extra"), null);
});

test("the navigation lists all four pages and links the scoped ones once an application is chosen", () => {
  const withoutApp = renderNav("overview", "");
  for (const route of ROUTES) {
    assert.match(withoutApp, new RegExp(`data-route="${route.id}"`));
  }
  assert.equal((withoutApp.match(/<a /g) || []).length, 2);
  assert.equal((withoutApp.match(/is-contextual/g) || []).length, 2);

  const withApp = renderNav("pods", "web");
  assert.equal((withApp.match(/<a /g) || []).length, 4);
  assert.match(withApp, /href="#\/applications\/web\/pods" aria-current="page"/);
});

test("each page surface renders its own shell", () => {
  const cases = [
    ["#/overview", "overview"],
    ["#/applications", "applications"],
    ["#/applications/web", "detail"],
    ["#/applications/web/pods", "pods"],
  ];
  for (const [hash, page] of cases) {
    const shell = renderShell({ hash, status: "standalone" });
    assert.match(shell, new RegExp(`data-page="${page}"`));
    assert.match(shell, new RegExp(`href="#/${hash.replace(/^#\//, "")}"`));
  }
});

test("the scoped pages state why they are empty without an application", () => {
  assert.match(renderShell({ hash: "#/applications/web" }), /data-empty="no-projection"/);
  const detail = renderShell({ hash: "#/applications/web" });
  assert.match(detail, /应用 web/);
  const noApplication = renderShell({ hash: "#/overview" });
  assert.doesNotMatch(noApplication, /data-empty="no-application"/);
});

test("an application identity is rendered as text, never as markup", () => {
  const shell = renderShell({ hash: "#/applications/%3Cimg%20src%3Dx%3E" });
  assert.match(shell, /data-app="&lt;img src=x&gt;"/);
  assert.doesNotMatch(shell, /<img/);
});

test("routeAddress omits the scoped pages without an application", () => {
  assert.equal(routeAddress(ROUTES[0]), "#/overview");
  assert.equal(routeAddress(ROUTES[1]), "#/applications");
  assert.equal(routeAddress(ROUTES[2]), "");
  assert.equal(routeAddress(ROUTES[2], "web"), "#/applications/web");
  assert.equal(routeAddress(ROUTES[3], "web"), "#/applications/web/pods");
});
