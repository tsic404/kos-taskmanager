// The detail projection the page cases render: one page as the control plane
// serves it (DASH-C-52, KOSD-C-53), shaped after the world TASK-TC-12 reads —
// one application with a volume, an endpoint, a covering backup policy, a drift
// record and an event inside the window, a second application with its own
// facts, and each application's log read address.

export const OBSERVED_AT = "2026-03-01T12:00:00Z";

export const READ_ONLY_REASON =
  "本页只读：健康、卷、入口、备份、漂移与事件读自应用与关联对象的投影，日志在读取时按应用读取；修改经应用页与对应产品发起";

export const LOG_RULE =
  "按应用在读取时读取：时间窗与行数由控制面限制，超限截断并标出截断位置；不保存日志，不提供全量下载";

// ITEM_* are the seven display item identities DASH-C-52 lists.
export const ITEM_HEALTH = "detail-health";
export const ITEM_VOLUMES = "detail-volumes";
export const ITEM_INGRESS = "detail-ingress";
export const ITEM_BACKUP = "detail-backup-policy";
export const ITEM_DRIFT = "detail-drift";
export const ITEM_EVENTS = "detail-events";
export const ITEM_LOGS = "detail-logs";

// DETAIL_ITEMS is the declaration order and source type of every item
// (DASH-C-52: six object sources and the read-source log item).
export const DETAIL_ITEMS = [
  [ITEM_HEALTH, "健康", "object"],
  [ITEM_VOLUMES, "卷", "object"],
  [ITEM_INGRESS, "入口", "object"],
  [ITEM_BACKUP, "备份", "object"],
  [ITEM_DRIFT, "漂移", "object"],
  [ITEM_EVENTS, "事件", "object"],
  [ITEM_LOGS, "日志", "read"],
];

// applicationFacts is one application's facts as every item carries them: each
// item row names the application by namespace and name (KOSD-C-53).
export function applicationFacts({
  namespace = "home",
  name = "shop",
  owner = "lin",
  phase = "Ready",
  health = "Healthy",
  conditions = [
    { type: "Reconciled", status: "True", reason: "Reconciled", message: "下游对象已收敛" },
  ],
  paused = false,
  replicas = 2,
  volumes = [
    {
      name: "data",
      resolved: true,
      phase: "Ready",
      health: "Healthy",
      mobility: "node-local",
      capacity: "10Gi",
      storageClass: "local-path",
      node: "node-a",
      boundClaim: "data-pvc",
    },
    { name: "gone", resolved: false },
  ],
  endpoints = [
    { name: "shop-web", kind: "http.host", host: "shop.test", phase: "Ready", health: "Healthy" },
  ],
  policies = [
    {
      name: "nightly",
      selector: "app=shop",
      schedule: "0 3 * * *",
      consistency: "crash",
      retention: 7,
      drill: true,
      phase: "Ready",
      health: "Healthy",
    },
  ],
  drift = {
    policy: "adopt",
    entries: [
      {
        object: { group: "apps", version: "v1", kind: "Deployment", namespace, name },
        path: "spec.replicas",
        external: "5",
        expected: "2",
      },
    ],
  },
  events = [
    {
      name: "shop-backoff",
      type: "Warning",
      reason: "BackOff",
      message: "容器反复退出",
      count: 1,
      lastSeen: "2026-03-01T11:50:00Z",
    },
  ],
  logsPath = `/api/v1/logs/${namespace}/${name}`,
} = {}) {
  return {
    health: { namespace, name, owner, phase, health, conditions, paused, replicas },
    volumes: { namespace, name, volumes },
    ingress: { namespace, name, endpoints },
    backup: { namespace, name, policies },
    drift: { namespace, name, policy: drift.policy, entries: drift.entries },
    events: { namespace, name, events },
    logs: { namespace, name, path: logsPath },
  };
}

function item(id, label, source, value, { observedAt = OBSERVED_AT, stale = false } = {}) {
  return { id, label, source, value, observedAt, stale };
}

// detailPage is the projection one detail read returns.
export function detailPage({
  applications = [applicationFacts()],
  stale = [],
  observedAt = OBSERVED_AT,
  eventWindow = "1h0m0s",
} = {}) {
  const state = (id) => ({ observedAt, stale: stale.includes(id) });
  const rows = (key) => applications.map((facts) => facts[key]);
  return {
    id: "detail",
    title: "应用详情",
    location: "panel",
    readOnly: true,
    readOnlyReason: READ_ONLY_REASON,
    sources: ["object", "read"],
    items: [
      item(ITEM_HEALTH, "健康", "object", { applications: rows("health") }, state(ITEM_HEALTH)),
      item(ITEM_VOLUMES, "卷", "object", { applications: rows("volumes") }, state(ITEM_VOLUMES)),
      item(ITEM_INGRESS, "入口", "object", { applications: rows("ingress") }, state(ITEM_INGRESS)),
      item(ITEM_BACKUP, "备份", "object", { applications: rows("backup") }, state(ITEM_BACKUP)),
      item(ITEM_DRIFT, "漂移", "object", { applications: rows("drift") }, state(ITEM_DRIFT)),
      item(
        ITEM_EVENTS,
        "事件",
        "object",
        { window: eventWindow, applications: rows("events") },
        state(ITEM_EVENTS),
      ),
      item(ITEM_LOGS, "日志", "read", { rule: LOG_RULE, applications: rows("logs") }, state(ITEM_LOGS)),
    ],
  };
}

// logRead is one application's log read (KOSD-C-64).
export function logRead({
  namespace = "home",
  name = "shop",
  window = "1h0m0s",
  limit = 1000,
  lines = [
    {
      pod: "shop-1",
      container: "web",
      timestamp: "2026-03-01T11:57:00Z",
      message: "shop 的行",
    },
    {
      pod: "shop-2",
      container: "web",
      timestamp: "2026-03-01T11:58:00Z",
      message: "shop 的另一个副本",
    },
  ],
  truncated = false,
  truncatedAt = null,
} = {}) {
  const body = {
    application: { group: "kos.tsic.top", kind: "Application", namespace, name },
    observedAt: OBSERVED_AT,
    window,
    limit,
    lines,
    truncated,
  };
  if (truncatedAt !== null) body.truncatedAt = truncatedAt;
  return body;
}
