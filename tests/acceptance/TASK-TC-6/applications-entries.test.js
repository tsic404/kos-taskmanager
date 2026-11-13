// TASK-TC-6 from the page side: the scale entry starts from the target row's
// replica count and submits it on the lifecycle path; the resource entry starts
// from the target's requests and limits, reads the application it is about to
// write, submits only the field the user changed behind the version
// precondition, and the values it then shows are the ones the application
// carries (KOSD-C-67, KOSD-AC-61); the uninstall entry deletes behind If-Match
// and the application stays listed as Uninstalling until the object is gone
// (KOSD-C-33). A config value the read path masked as sensitive blocks the
// whole-spec write instead of storing the mask, and the metadata the read
// returned travels with the update. No task record is introduced anywhere.

import assert from "node:assert/strict";
import test from "node:test";

import { createShell } from "../../../assets/app.js";
import * as applications from "../../../assets/pages/applications.js";
import {
  ITEM_INSTALLED,
  ITEM_RESOURCE_SPEC,
  ITEM_SCALE,
  ITEM_UNINSTALL,
  MASKED_VALUE,
  applicationProjection,
  applicationRow,
  applicationsPage,
  formIn,
  noTargets,
  target,
} from "../fixtures/applications.js";
import { bootstrapWindow, jsonResponse } from "../fixtures/window.js";

const RESOURCE_PATH = "/api/v1/resources/Application/shop?namespace=home";

// serve answers the registry and the page, and hands a write to the case.
function serve({ page, write }) {
  const calls = [];
  const fetchFn = async (url, options = {}) => {
    calls.push({ url, options });
    if (url === "/api/v1/pages") {
      return jsonResponse({ items: [{ id: "applications", title: "应用" }] });
    }
    if (url === "/api/v1/pages/applications") {
      return jsonResponse(typeof page === "function" ? page() : page);
    }
    return write(url, options);
  };
  return { fetchFn, calls };
}

async function boot({ page, write }) {
  const fixture = bootstrapWindow({ hash: "#/applications" });
  const server = serve({ page, write });
  const shell = createShell({ document: fixture.document, window: fixture.window, fetchFn: server.fetchFn });
  await shell.settle();
  return { fixture, calls: server.calls, shell };
}

function submit(fixture, form) {
  assert.ok(form, "the page offers the entry");
  fixture.dispatchRoot("submit", { target: form, preventDefault() {} });
}

test("the scale entry starts from the target row's replica count and submits it", async () => {
  let answer = applicationsPage();
  const { fixture, calls, shell } = await boot({
    page: () => answer,
    write: () => {
      answer = applicationsPage({
        applications: [applicationRow({ replicas: 3 })],
        targets: { ...noTargets(), [ITEM_SCALE]: [target({ replicas: 3 })] },
      });
      return jsonResponse({ accepted: true }, { status: 202 });
    },
  });
  try {
    const form = formIn(fixture.root.innerHTML, ITEM_SCALE);
    assert.equal(form.dataset.targetName, "shop");
    assert.equal(form.querySelector('[name="replicas"]').value, "2", "the form starts from the target row");
    form.querySelector('[name="replicas"]').value = "3";
    submit(fixture, form);
    await shell.settle();

    const post = calls.find((call) => call.options.method === "POST");
    assert.equal(post.url, "/api/v1/action-requests");
    assert.deepEqual(JSON.parse(post.options.body), {
      kind: "Application",
      namespace: "home",
      name: "shop",
      action: "scale",
      replicas: 3,
    });

    assert.match(fixture.root.innerHTML, /data-entry-notice="accepted"/);
    assert.equal(
      formIn(fixture.root.innerHTML, ITEM_SCALE).querySelector('[name="replicas"]').value,
      "3",
      "the entry now gives the count the application carries",
    );
  } finally {
    shell.channel.stop();
  }
});

test("the resource entry submits only the changed field and keeps the rest of the spec", async () => {
  let answer = applicationsPage();
  const written = applicationProjection();
  const { fixture, calls, shell } = await boot({
    page: () => answer,
    write: (url, options) => {
      if (url === RESOURCE_PATH && options.method === "GET") {
        return jsonResponse(written);
      }
      if (url === RESOURCE_PATH && options.method === "PUT") {
        answer = applicationsPage({
          targets: {
            ...noTargets(),
            [ITEM_RESOURCE_SPEC]: [
              target({
                resources: {
                  requests: { cpu: "250m", memory: "256Mi" },
                  limits: { cpu: "1", memory: "512Mi" },
                },
              }),
            ],
          },
        });
        return jsonResponse({ accepted: true }, { status: 202 });
      }
      return jsonResponse({ code: "not_found", message: "没有该资源" }, { status: 404 });
    },
  });
  try {
    const form = formIn(fixture.root.innerHTML, ITEM_RESOURCE_SPEC);
    assert.equal(form.querySelector('[name="requestsCPU"]').value, "250m");
    assert.equal(form.querySelector('[name="requestsMemory"]').value, "256Mi");
    assert.equal(form.querySelector('[name="limitsCPU"]').value, "500m");
    assert.equal(form.querySelector('[name="limitsMemory"]').value, "512Mi");
    form.querySelector('[name="limitsCPU"]').value = "1";
    submit(fixture, form);
    await shell.settle();

    assert.deepEqual(
      calls.slice(2).map((call) => `${call.options.method} ${call.url}`),
      [`GET ${RESOURCE_PATH}`, `PUT ${RESOURCE_PATH}`, "GET /api/v1/pages/applications"],
      "the form reads the object, writes it back and re-reads the page",
    );

    const put = calls.find((call) => call.options.method === "PUT");
    assert.equal(put.options.headers["If-Match"], "7");
    const body = JSON.parse(put.options.body);
    assert.deepEqual(body.metadata, { name: "shop", namespace: "home" });
    assert.deepEqual(body.spec, {
      owner: "lin",
      definitionRef: "shop",
      replicas: 2,
      paused: false,
      volumes: ["data"],
      resources: {
        requests: { cpu: "250m", memory: "256Mi" },
        limits: { cpu: "1", memory: "512Mi" },
      },
    });

    const reread = formIn(fixture.root.innerHTML, ITEM_RESOURCE_SPEC);
    assert.equal(reread.querySelector('[name="limitsCPU"]').value, "1", "the entry gives the written limit");
    assert.equal(reread.querySelector('[name="limitsMemory"]').value, "512Mi", "the untouched limit stays");
    assert.equal(reread.querySelector('[name="requestsCPU"]').value, "250m", "the requests stay");
    assert.equal(reread.querySelector('[name="requestsMemory"]').value, "256Mi");
  } finally {
    shell.channel.stop();
  }
});

test("the uninstall entry deletes behind If-Match and keeps the row visible", async () => {
  let answer = applicationsPage();
  const { fixture, calls, shell } = await boot({
    page: () => answer,
    write: () => {
      answer = applicationsPage({
        applications: [
          applicationRow({
            phase: "Uninstalling",
            health: "Unknown",
            replicas: 0,
            uninstall: {
              blocked: [{ group: "kos.tsic.top", kind: "Volume", namespace: "home", name: "data" }],
            },
          }),
        ],
        targets: noTargets(),
      });
      return jsonResponse({ accepted: true }, { status: 202 });
    },
  });
  try {
    submit(fixture, formIn(fixture.root.innerHTML, ITEM_UNINSTALL));
    await shell.settle();

    const uninstall = calls.find((call) => call.options.method === "DELETE");
    assert.equal(uninstall.url, RESOURCE_PATH);
    assert.equal(uninstall.options.headers["If-Match"], "7", "the version precondition is the target's");
    assert.equal(uninstall.options.body, undefined, "a delete carries no body");
    assert.equal(calls.at(-1).url, "/api/v1/pages/applications", "the page is re-read after the write");

    const html = fixture.root.innerHTML;
    assert.match(html, /data-entry-notice="accepted"/);
    const row = html.slice(html.indexOf('data-object="home/shop"'));
    assert.match(row, /data-phase="Uninstalling"/);
    assert.match(row, /卸载中/);
    assert.match(row, /data-uninstall="true"/);
    assert.match(row, /Volume/);
    assert.match(row, /home\/data/);
    assert.doesNotMatch(row, /data-entry-item=/, "the application stays visible with no entry while it goes away");
    assert.doesNotMatch(html, /任务|data-task/);

    // Once the object is gone the next read no longer lists it (KOSD-C-33).
    const gone = applications.render({
      projection: { state: "ready", page: applicationsPage({ applications: [] }) },
    });
    assert.doesNotMatch(gone, /data-object="home\/shop"/, "the removed application is no longer listed");
    assert.match(gone, /暂无数据/);
  } finally {
    shell.channel.stop();
  }
});

test("a masked sensitive config refuses the resource write instead of storing the mask", async () => {
  const masked = applicationProjection({ config: { password: MASKED_VALUE, logLevel: "info" } });
  const { fixture, calls, shell } = await boot({
    page: () => applicationsPage(),
    write: (url, options) => {
      if (url === RESOURCE_PATH && options.method === "GET") return jsonResponse(masked);
      return jsonResponse({ accepted: true }, { status: 202 });
    },
  });
  try {
    const form = formIn(fixture.root.innerHTML, ITEM_RESOURCE_SPEC);
    const refusal = applications.resourceSpecRequest(
      masked,
      applications.formEntryOf(form),
      applications.valuesOf(form),
    );
    assert.deepEqual(refusal.masked, ["password"]);
    assert.equal(refusal.body, undefined, "no body is built from a masked read");
    assert.doesNotMatch(JSON.stringify(refusal), new RegExp(MASKED_VALUE.replace(/\*/g, "\\*")), "the mask never enters a submission");
    assert.match(refusal.refused, /password/);

    submit(fixture, form);
    await shell.settle();
    assert.deepEqual(calls.map((call) => call.options.method), ["GET", "GET", "GET"], "only the read happens");

    const html = fixture.root.innerHTML;
    assert.match(html, /data-entry-notice="error"/);
    assert.match(html, /data-code="masked_value"/);
    assert.match(html, /password/);
    assert.match(html, /敏感值已掩码/);
  } finally {
    shell.channel.stop();
  }
});

test("the resource update carries the labels and annotations it read", async () => {
  const written = applicationProjection({
    labels: { app: "shop" },
    annotations: { "kos.tsic.top/note": "kept" },
  });
  const { fixture, calls, shell } = await boot({
    page: () => applicationsPage(),
    write: (url, options) => {
      if (url === RESOURCE_PATH && options.method === "GET") return jsonResponse(written);
      return jsonResponse({ accepted: true }, { status: 202 });
    },
  });
  try {
    const form = formIn(fixture.root.innerHTML, ITEM_RESOURCE_SPEC);
    form.querySelector('[name="limitsCPU"]').value = "1";
    submit(fixture, form);
    await shell.settle();

    const put = calls.find((call) => call.options.method === "PUT");
    assert.deepEqual(JSON.parse(put.options.body).metadata, {
      name: "shop",
      namespace: "home",
      labels: { app: "shop" },
      annotations: { "kos.tsic.top/note": "kept" },
    });
  } finally {
    shell.channel.stop();
  }
});
