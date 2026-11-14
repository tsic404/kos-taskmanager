// TASK-TC-4 from the page side: after the catalogue republishes a higher version
// of the definition an application was installed from, the application page
// shows no upgrade entry, no target version and no update notice; the higher
// version stays readable in the installable list only (APPSTORE-C-79,
// APPSTORE-C-80). A projection that carries an upgrade entry anyway is not
// rendered as one: the page draws the three display items and six entries
// TASK-C-2 names and nothing else.

import assert from "node:assert/strict";
import test from "node:test";

import * as applications from "../../../assets/pages/applications.js";
import {
  ITEM_INSTALLED,
  ITEM_INSTALLABLE,
  ITEM_RESOURCE_USAGE,
  applicationRow,
  applicationsPage,
  target,
} from "../fixtures/applications.js";

// FORBIDDEN is the vocabulary an upgrade entry or an update notice would carry
// (APPSTORE-C-79, APPSTORE-C-80).
const FORBIDDEN = [
  /data-entry-item="application-upgrade"/,
  /data-entry-item="upgrade-applications"/,
  /upgrade-application/,
  /升级应用/,
  /立即升级/,
  /可用更新/,
  /更新可用/,
  /更新提示/,
  /targetVersion/,
  /updateAvailable/,
  /availableVersion/,
];

function renderPage(page) {
  return applications.render({ projection: { state: "ready", page } });
}

// itemBlock is the rendered block of one display item.
function itemBlock(html, id) {
  const start = html.indexOf(`data-item="${id}"`);
  assert.notEqual(start, -1, `${id} is rendered`);
  const next = html.indexOf(`data-item="`, start + 1);
  return next === -1 ? html.slice(start) : html.slice(start, next);
}

test("a higher published version yields no upgrade entry and no update notice", () => {
  const html = renderPage(
    applicationsPage({
      applications: [applicationRow()],
      // The catalogue republished the entry at 2.0.0: the input an upgrade
      // entry in the application center would be computed from.
      installable: {
        rule: "定义存在且必需能力需求已被已验证的 Provider 满足（KOSD-C-53）",
        entries: [{ name: "shop", version: "2.0.0", installable: true }],
      },
    }),
  );

  for (const pattern of FORBIDDEN) {
    assert.doesNotMatch(html, pattern, `the page renders ${pattern}`);
  }

  // The application center's half keeps the catalogue entry readable with the
  // higher version; this page shows it as an installable definition only and
  // never as an update of the installed application.
  assert.match(itemBlock(html, ITEM_INSTALLABLE), /2\.0\.0/, "the catalogue version stays readable");
  assert.doesNotMatch(itemBlock(html, ITEM_INSTALLED), /2\.0\.0/, "the installed row carries no target version");
});

test("an upgrade item the projection happens to carry is not rendered as a control", () => {
  const page = applicationsPage();
  page.items.push({
    id: "application-upgrade",
    label: "升级应用",
    source: "object",
    value: { applications: [target()] },
    observedAt: "2026-03-01T12:00:00Z",
    stale: false,
    action: "upgrade-application",
    entry: { method: "POST", path: "/api/v1/action-requests" },
    reason: "",
  });
  page.items.push({
    id: "application-update",
    label: "有可用更新",
    source: "computed",
    value: { version: "2.0.0" },
    observedAt: "2026-03-01T12:00:00Z",
    stale: false,
    action: "",
    entry: null,
    reason: "应用中心发现更高版本",
  });

  const html = renderPage(page);
  for (const pattern of FORBIDDEN) {
    assert.doesNotMatch(html, pattern, `the page renders ${pattern}`);
  }
  assert.doesNotMatch(html, /有可用更新/);
  assert.deepEqual(
    [...html.matchAll(/data-item="([^"]+)"/g)].map((match) => match[1]),
    [ITEM_INSTALLED, ITEM_INSTALLABLE, ITEM_RESOURCE_USAGE],
  );
});
