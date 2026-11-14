// The applications projection the page cases render: one page as the control
// plane serves it (TASK-C-2, KOSD-C-53), shaped after the world TASK-TC-3..6
// reads — one installed application with its health, a catalogue entry, the
// sampled usage and the six entries with their per-application targets.
//
// The forms are read back out of the rendered page, so a case drives the form
// the page actually emits instead of one assembled beside it.

export const OBSERVED_AT = "2026-03-01T12:00:00Z";

// MASKED_VALUE is what the control plane substitutes for a config value its
// definition marks sensitive (kosd pkg/controlplane/application.go maskSensitive).
export const MASKED_VALUE = "***";

// CONTROL_PLANE_FORBIDDEN and CONTROL_PLANE_NOT_FOUND are the failure envelopes
// the control plane answers with (kosd pkg/controlplane/server.go writeError):
// the code and the user-facing reason sit under "error".
export const CONTROL_PLANE_FORBIDDEN = {
  error: {
    code: "permission_denied",
    message: "当前身份不能对 kos.tsic.top/Application/home/shop 执行 update",
    object: { group: "kos.tsic.top", kind: "Application", name: "shop", namespace: "home" },
    requiresUserAction: true,
    retryable: false,
    sideEffects: "none",
    stage: "authorize",
  },
};

export const CONTROL_PLANE_NOT_FOUND = {
  error: {
    code: "not_found",
    message: "页面不存在，或提供它的组件未生效",
    requiresUserAction: false,
    retryable: false,
    sideEffects: "none",
    stage: "read",
  },
};

export const RULE_INSTALLABLE = "定义存在且必需能力需求已被已验证的 Provider 满足（KOSD-C-53）";

export const REASONS = {
  installed: "应用事实由应用调和写入，修改经同一页的操作入口",
  installable: "可安装列表按定义与能力需求计算，安装经应用中心发起",
  usage: "资源占用按指标采样读取，页面不写入",
};

// ITEM_* are the three display item identities and the six entry identities the
// page declares (TASK-C-2).
export const ITEM_INSTALLED = "installed-applications";
export const ITEM_INSTALLABLE = "installable-applications";
export const ITEM_RESOURCE_USAGE = "application-usage";
export const ITEM_STOP = "stop-applications";
export const ITEM_START = "start-applications";
export const ITEM_RESTART = "restart-applications";
export const ITEM_SCALE = "scale-applications";
export const ITEM_RESOURCE_SPEC = "application-resources";
export const ITEM_UNINSTALL = "application-uninstall";

// ENTRY_DEFINITIONS are the six entries in declaration order with the control
// plane write each one names (KOSD-C-12, KOSD-C-22).
export const ENTRY_DEFINITIONS = [
  [ITEM_STOP, "停止应用", "stop-application", "POST", "/api/v1/action-requests"],
  [ITEM_START, "启动应用", "start-application", "POST", "/api/v1/action-requests"],
  [ITEM_RESTART, "重启应用", "restart-application", "POST", "/api/v1/action-requests"],
  [ITEM_SCALE, "扩缩容", "scale-application", "POST", "/api/v1/action-requests"],
  [ITEM_RESOURCE_SPEC, "资源规格", "set-application-resources", "PUT", "/api/v1/resources/Application/{name}?namespace={namespace}"],
  [ITEM_UNINSTALL, "卸载应用", "uninstall-application", "DELETE", "/api/v1/resources/Application/{name}?namespace={namespace}"],
];

// applicationRow is one row of the installed set (TASK-C-2, KOSD-C-33).
export function applicationRow(overrides = {}) {
  return {
    namespace: "home",
    name: "shop",
    owner: "lin",
    phase: "Ready",
    health: "Healthy",
    conditions: [
      { type: "Reconciled", status: "True", reason: "Reconciled", message: "下游对象已收敛" },
    ],
    paused: false,
    replicas: 2,
    ...overrides,
  };
}

// target is one entry's target application with the values the entry starts
// from (KOSD-AC-61): the replica count, the resource specification and the
// version precondition.
export function target(overrides = {}) {
  return {
    namespace: "home",
    name: "shop",
    phase: "Ready",
    paused: false,
    replicas: 2,
    resources: {
      requests: { cpu: "250m", memory: "256Mi" },
      limits: { cpu: "500m", memory: "512Mi" },
    },
    resourceVersion: "7",
    ...overrides,
  };
}

// defaultTargets is a running application's entries: stop, restart, scale, the
// resource specification and uninstall apply, start does not (KOSD-C-66).
export function defaultTargets() {
  return {
    [ITEM_STOP]: [target()],
    [ITEM_START]: [],
    [ITEM_RESTART]: [target()],
    [ITEM_SCALE]: [target()],
    [ITEM_RESOURCE_SPEC]: [target()],
    [ITEM_UNINSTALL]: [target()],
  };
}

// noTargets is what an identity without write permission receives: every
// entry's target list is empty (KOSD-C-59).
export function noTargets() {
  const targets = {};
  for (const [id] of ENTRY_DEFINITIONS) targets[id] = [];
  return targets;
}

function item(id, label, source, value, { observedAt = OBSERVED_AT, stale = false, action = "", entry = null, reason = "" } = {}) {
  return { id, label, source, value, observedAt, stale, action, entry, reason };
}

// applicationsPage is the projection one read returns. targets joins each entry
// to the applications it may act on; an entry absent from targets offers none.
export function applicationsPage({
  applications = [applicationRow()],
  installable = {
    rule: RULE_INSTALLABLE,
    entries: [{ name: "shop", version: "1.0.0", installable: true }],
  },
  usage = { available: false, reason: "发行版没有接入资源指标来源，KOS 不采集工作负载用量" },
  targets = defaultTargets(),
  stale = [],
  observedAt = OBSERVED_AT,
} = {}) {
  const state = (id) => ({ observedAt, stale: stale.includes(id) });
  const items = [
    item(ITEM_INSTALLED, "已安装应用与健康", "object", { applications }, { ...state(ITEM_INSTALLED), reason: REASONS.installed }),
    item(ITEM_INSTALLABLE, "可安装列表", "computed", installable, { ...state(ITEM_INSTALLABLE), reason: REASONS.installable }),
    item(ITEM_RESOURCE_USAGE, "资源占用", "metric", usage, { ...state(ITEM_RESOURCE_USAGE), reason: REASONS.usage }),
  ];
  for (const [id, label, action, method, path] of ENTRY_DEFINITIONS) {
    items.push(item(id, label, "object", { applications: targets[id] || [] }, { ...state(id), action, entry: { method, path } }));
  }
  return {
    id: "applications",
    title: "应用",
    location: "panel",
    readOnly: false,
    sources: ["object", "computed", "metric"],
    items,
  };
}

// applicationProjection is what the control plane serves on the resource path:
// a projection DTO with the object inside (KOSD-C-12, KOSD-C-13).
export function applicationProjection(options = {}) {
  return {
    ref: { group: "kos.tsic.top", kind: "Application", namespace: "home", name: "shop" },
    observedAt: OBSERVED_AT,
    stale: false,
    object: applicationObject(options),
  };
}

// applicationObject is the application itself; the resource entry reads it
// before writing back the spec it changes. config carries the values the read
// path returns, including the mask it substituted for a sensitive one.
export function applicationObject({
  resources = target().resources,
  resourceVersion = "7",
  config = undefined,
  labels = undefined,
  annotations = undefined,
} = {}) {
  const metadata = { name: "shop", namespace: "home", resourceVersion };
  if (labels) metadata.labels = labels;
  if (annotations) metadata.annotations = annotations;
  const spec = {
    owner: "lin",
    definitionRef: "shop",
    replicas: 2,
    paused: false,
    volumes: ["data"],
    resources,
  };
  if (config !== undefined) spec.config = config;
  return {
    apiVersion: "kos.tsic.top/v1beta1",
    kind: "Application",
    metadata,
    spec,
    status: { phase: "Ready", health: { state: "Healthy" } },
  };
}

// formIn reads the form the page rendered for one entry, with its dataset and
// named fields, so a case submits what the reader sees.
export function formIn(html, itemId) {
  const opening = new RegExp(`<form[^>]*data-entry-item="${itemId}"[^>]*>`).exec(html);
  if (!opening) return null;
  const dataset = {};
  for (const [, key, value] of opening[0].matchAll(/data-([a-z-]+)="([^"]*)"/g)) {
    dataset[key.replace(/-([a-z])/g, (_, letter) => letter.toUpperCase())] = value;
  }
  const closing = html.indexOf("</form>", opening.index);
  const body = closing === -1 ? "" : html.slice(opening.index + opening[0].length, closing);
  const fields = new Map();
  for (const [, attributes] of body.matchAll(/<input([^>]*)>/g)) {
    const name = /name="([^"]*)"/.exec(attributes);
    if (!name) continue;
    const value = /value="([^"]*)"/.exec(attributes);
    fields.set(name[1], { value: value ? value[1] : "" });
  }
  return {
    dataset,
    querySelector(selector) {
      const name = /name="([^"]+)"/.exec(selector);
      return name ? fields.get(name[1]) || null : null;
    },
  };
}
