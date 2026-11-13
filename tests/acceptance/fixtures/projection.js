// The overview projection the page cases render: one page as the control plane
// serves it (KOSD-C-53), shaped after the world TASK-TC-1 reads — four nodes of
// which one is not ready, two abnormal objects, two pending upgrades, the events
// of the hour newest first, three unschedulable nodes with their judged origin
// and the one application a node-local volume binds.

export const OBSERVED_AT = "2026-03-01T12:00:00Z";

export const RULE_ABNORMAL = "健康状态不是 Healthy，或存在未满足的条件（KOSD-C-53）";
export const RULE_UPGRADE = "定义的包摘要与已应用基线不同，或应用正在升级（KOSD-C-53）";
export const READ_ONLY_REASON =
  "本页只读：节点、事件与升级状态由运行内核与版本发现持有，修改经应用页与系统设置发起";

// item is one projection item with the fields the page renders.
export function item(id, label, source, value, { observedAt = OBSERVED_AT, stale = false } = {}) {
  return { id, label, source, value, observedAt, stale };
}

// overviewItems returns the seven contracted items in declaration order.
export function overviewItems({ stale = [], observedAt = OBSERVED_AT } = {}) {
  const state = (id) => ({ observedAt, stale: stale.includes(id) });
  return [
    item(
      "nodes-ready",
      "节点就绪",
      "object",
      {
        ready: 3,
        total: 4,
        nodes: [
          { name: "node-a", ready: true },
          { name: "node-b", ready: true },
          { name: "node-c", ready: false },
          { name: "node-d", ready: true },
        ],
      },
      state("nodes-ready"),
    ),
    item(
      "abnormal-objects",
      "异常对象",
      "computed",
      {
        rule: RULE_ABNORMAL,
        objects: [
          {
            kind: "Application",
            namespace: "home",
            name: "gateway",
            phase: "Ready",
            health: "Healthy",
            conditions: [
              { type: "Reconciled", status: "Unknown", reason: "Waiting", message: "等待入口就绪" },
            ],
          },
          {
            kind: "Application",
            namespace: "home",
            name: "reports",
            phase: "Failed",
            health: "Degraded",
            conditions: [
              { type: "Reconciled", status: "False", reason: "RenderFailed", message: "渲染失败" },
            ],
          },
        ],
      },
      state("abnormal-objects"),
    ),
    item(
      "pending-upgrades",
      "待处理升级",
      "computed",
      {
        rule: RULE_UPGRADE,
        pending: [
          {
            namespace: "home",
            name: "reports",
            state: "definition-changed",
            installed: "sha256:reports-v1",
            target: "sha256:reports-v2",
            version: "2.0.0",
          },
          { namespace: "home", name: "search", state: "in-progress", target: "sha256:search-v2" },
        ],
      },
      state("pending-upgrades"),
    ),
    item(
      "recent-events",
      "最近事件",
      "object",
      {
        window: "1h0m0s",
        events: [
          {
            name: "shop-backoff",
            namespace: "home",
            type: "Warning",
            reason: "BackOff",
            message: "容器反复退出",
            count: 1,
            lastSeen: "2026-03-01T11:50:00Z",
            involvedObject: { group: "kos.tsic.io", kind: "Application", namespace: "home", name: "shop" },
          },
          {
            name: "node-b-drain",
            namespace: "home",
            type: "Warning",
            reason: "Drain",
            message: "节点进入维护",
            count: 1,
            lastSeen: "2026-03-01T11:40:00Z",
            involvedObject: { group: "", kind: "Node", name: "node-b" },
          },
          {
            name: "reports-failed",
            namespace: "home",
            type: "Warning",
            reason: "RenderFailed",
            message: "渲染失败",
            count: 1,
            lastSeen: "2026-03-01T11:10:00Z",
            involvedObject: { group: "kos.tsic.io", kind: "Application", namespace: "home", name: "reports" },
          },
        ],
      },
      state("recent-events"),
    ),
    item(
      "maintenance-nodes",
      "维护中的节点",
      "object",
      {
        nodes: [
          { name: "node-b", ready: true },
          { name: "node-c", ready: false },
          { name: "node-d", ready: true },
        ],
      },
      state("maintenance-nodes"),
    ),
    item(
      "maintenance-origin",
      "维护归属",
      "database",
      {
        nodes: [
          { name: "node-b", origin: "planned" },
          { name: "node-c", origin: "unplanned" },
          { name: "node-d", origin: "unplanned" },
        ],
      },
      state("maintenance-origin"),
    ),
    item(
      "maintenance-affected-applications",
      "维护受影响的 node-local 应用",
      "computed",
      { nodes: [{ node: "node-b", applications: ["home/shop"] }] },
      state("maintenance-affected-applications"),
    ),
  ];
}

// overviewPage is the projection one read returns.
export function overviewPage(options = {}) {
  const items = overviewItems(options);
  return {
    id: "overview",
    title: "概览",
    location: "panel",
    readOnly: true,
    readOnlyReason: READ_ONLY_REASON,
    sources: ["object", "computed", "database"],
    items,
  };
}
