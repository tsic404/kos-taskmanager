// The Pod runtime projection the page cases render: one application's Pod
// facts, the per-Pod log read and the single-Pod delete entry, shaped after the
// control plane's answer (KOSD-C-76, KOSD-C-64, KOSD-C-77). The page reads one
// application's projection from the address its own row names.

export const OBSERVED_AT = "2026-03-01T12:00:00Z";

export const PODS_REASON =
  "Pod 投影只读：Pod 是运行内核的事实，本页不创建 Pod、不改 Pod spec，也不提供终端执行与端口转发";

export const POD_LOGS_REASON =
  "按 Pod 在读取时读取日志：窗口与行数由控制面限制，超限截断并标出截断位置；不保存日志，不提供全量下载";

export const POD_LOG_RULE =
  "按 Pod 在读取时读取：只含该 Pod 容器的行，同应用其他 Pod 的行不出现；时间窗与行数由控制面限制，超限截断并标出截断位置";

export const STOPPED_REASON = "应用已停止：停止的应用没有 Pod（KOSD-C-76）";

export const ITEM_PODS = "pods";
export const ITEM_POD_LOGS = "pod-logs";
export const ITEM_POD_DELETE = "pod-delete";

export const DELETE_ACTION = "delete-pod";
export const DELETE_PATH = "/api/v1/pods/{namespace}/{name}/{pod}";

// podRow is one application the page can open: the projection is read from the
// address the row names, never assembled by the page.
export function podRow({
  namespace = "home",
  name = "shop",
  paused = false,
  reason = "",
  path = `/api/v1/pods/${namespace}/${name}`,
} = {}) {
  const row = { namespace, name, paused, path };
  if (reason !== "") row.reason = reason;
  return row;
}

// logRow is one application's log read address.
export function logRow({ namespace = "home", name = "shop", path = `/api/v1/logs/${namespace}/${name}` } = {}) {
  return { namespace, name, path };
}

// deleteTarget is one application the control plane names as a delete target.
export function deleteTarget({ namespace = "home", name = "shop" } = {}) {
  return { namespace, name };
}

function item(id, label, source, value, extra = {}) {
  return { id, label, source, value, observedAt: OBSERVED_AT, stale: false, ...extra };
}

// podsPage is the page projection one read returns.
export function podsPage({
  applications = [podRow(), podRow({ name: "reports" })],
  logs = [logRow(), logRow({ name: "reports" })],
  deleteTargets = [deleteTarget()],
  observedAt = OBSERVED_AT,
} = {}) {
  return {
    id: "pods",
    title: "Pod 运行态",
    location: "panel",
    sources: ["object", "read"],
    items: [
      item(ITEM_PODS, "Pod 列表", "object", { applications }, { observedAt }),
      item(ITEM_POD_LOGS, "按 Pod 日志", "read", { rule: POD_LOG_RULE, applications: logs }, { observedAt }),
      item(
        ITEM_POD_DELETE,
        "删除 Pod",
        "object",
        { applications: deleteTargets },
        {
          action: DELETE_ACTION,
          reason: PODS_REASON,
          entry: { method: "DELETE", path: DELETE_PATH },
        },
      ),
    ],
  };
}

// podFacts is one projected Pod (KOSD-C-76).
export function podFacts({
  name = "shop-1",
  node = "node-a",
  phase = "Running",
  ready = true,
  restarts = 0,
  containers = [{ name: "web", image: "registry.test/shop:2" }],
  startedAt = "2026-03-01T11:30:00Z",
  sourceType = "object",
  observedAt = OBSERVED_AT,
  stale = false,
} = {}) {
  const facts = { name, node, phase, ready, restarts, containers, sourceType, observedAt, stale };
  if (startedAt !== "") facts.startedAt = startedAt;
  return facts;
}

// podRead is one application's Pod projection answer.
export function podRead({
  namespace = "home",
  name = "shop",
  items = [
    podFacts(),
    podFacts({ name: "shop-2", node: "node-b", phase: "Pending", ready: false, restarts: 2 }),
  ],
} = {}) {
  return {
    application: { group: "kos.tsic.top", kind: "Application", namespace, name },
    items,
  };
}

// logRead is one per-Pod log read (KOSD-C-64).
export function logRead({
  namespace = "home",
  name = "shop",
  window = "15m0s",
  limit = 200,
  lines = [
    { pod: "shop-1", container: "web", timestamp: "2026-03-01T11:59:00Z", message: "shop-1 的行" },
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

// failedRead is a control-plane failure envelope (KOSD-C-12).
export function failedRead(code, message) {
  return { error: { code, message } };
}
