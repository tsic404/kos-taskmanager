// The Pod runtime surface of one application: the control plane's read-only Pod
// projection (KOSD-C-76), each row's own log read and the delete of exactly that
// one Pod (KOSD-C-77). The projection, the log and the delete are all
// control-plane addresses the page read names, so the window never reaches the
// runtime and holds no cluster credential (KOSD-C-12, KOSD-F-9). The page offers
// no Pod create, no Pod spec write, no terminal and no port forward
// (KOSD-F-20). Nothing here writes: the delete form carries the entry the page
// declared and the shell submits it.
import { escapeHTML } from "../html.js";
import { PHASE_LABELS } from "./applications.js";

export const id = "pods";
export const title = "Pod 运行态";

// NO_SESSION_NOTE covers a window without a console session: it reads nothing.
export const NO_SESSION_NOTE = "等待控制台会话：Pod 运行态经控制面读取，独立预览不显示数据。";

// NO_APPLICATION_NOTE covers an address that names no application.
export const NO_APPLICATION_NOTE = "未选择应用：Pod 属于一个应用，请从应用页进入。";

// LOADING_NOTE is shown while the page projection is being read.
export const LOADING_NOTE = "正在读取 Pod 运行态投影…";

// PODS_LOADING_NOTE is shown while the application's Pod projection is read.
export const PODS_LOADING_NOTE = "正在读取该应用的 Pod 投影…";

// EMPTY_NOTE is the one empty value the page falls back to.
export const EMPTY_NOTE = "暂无数据";

// NO_PODS_NOTE is an empty answer from a reachable source, never a stand-in for
// a failed read (KOSD-C-76).
export const NO_PODS_NOTE = "该应用当前没有 Pod";

// STALE_NOTE is the freshness bound every source type shares (KOSD-C-13).
export const STALE_NOTE = "观测时间超过 2 分钟即标记陈旧";

// LOG_BUTTON_LABEL is the per-row read control.
export const LOG_BUTTON_LABEL = "读取日志";

// LOG_RULE_NOTE names what a per-Pod read covers (KOSD-C-64).
export const LOG_RULE_NOTE = "按 Pod 读取：只含该 Pod 容器的行，同应用其他 Pod 的行不出现";

// LOG_READING_NOTE is shown while one Pod's log read runs.
export const LOG_READING_NOTE = "正在读取该 Pod 的日志…";

// LOG_EMPTY_NOTE is what a read inside the window with no lines states: an
// empty answer from a reachable source, never a stand-in for a failure.
export const LOG_EMPTY_NOTE = "暂无数据：该 Pod 在时间窗内没有日志行";

// LOG_TRUNCATED_NOTE names what the truncation mark means (KOSD-C-64).
export const LOG_TRUNCATED_NOTE = "已截断：只保留最近的行，截断位置";

// DELETE_LABEL is the one write control the page offers (KOSD-C-77).
export const DELETE_LABEL = "删除 Pod";

// DELETE_NOTE states what the delete does and does not touch.
export const DELETE_NOTE = "删除只作用于这一个 Pod：应用 spec、副本数与卷不变，替代实例由运行内核的工作负载控制器产生";

// SOURCE_LABELS are the source types of KOSD-C-53 as the page shows them.
export const SOURCE_LABELS = {
  object: "对象源",
  computed: "计算源",
  metric: "指标源",
  probe: "探测源",
  read: "读取源",
  local: "本地源",
  database: "数据库源",
};

// ITEM_* are the three item identities the page declares (TASK-C-3).
export const ITEM_PODS = "pods";
export const ITEM_POD_LOGS = "pod-logs";
export const ITEM_POD_DELETE = "pod-delete";

// noticeOf states the outcome of the last delete. The accepted case names the
// read the list comes from; the refused case keeps the control plane's code and
// reason verbatim (KOSD-C-12, KOSD-C-77).
function noticeOf(notice) {
  if (!notice) return "";
  const label = escapeHTML(notice.label || DELETE_LABEL);
  if (notice.kind === "accepted") {
    return `<p class="entry-notice" data-entry-notice="accepted">已提交${label}；Pod 列表已重新读取。</p>`;
  }
  if (notice.kind !== "error") return "";
  const code = String(notice.code || "");
  const message = String(notice.message || "控制面没有给出原因");
  return `<p class="entry-notice is-error" data-entry-notice="error" data-code="${escapeHTML(code)}">提交${label}失败：${escapeHTML(code)} ${escapeHTML(message)}</p>`;
}

// present reports whether the page read carried the addressed application. The
// control plane trims every row to the applications the session may see, so an
// application without a row is one this identity cannot see (KOSD-C-59); the
// window then shows neither the page nor its entry.
export function present({ app = "", projection = null } = {}) {
  const page = projection && projection.state === "ready" ? projection.page : null;
  if (!page) return true;
  return subjectOf(page, app) !== null;
}

// subjectOf reads the row the addressed application carries. An address without
// a namespace names the application inside its namespace, which is the only
// reading a bare name allows.
export function subjectOf(page, app = "") {
  const selected = String(app || "");
  if (selected === "") return null;
  for (const row of rowsOf(itemOf(page, ITEM_PODS))) {
    if (matches(row, selected)) return row;
  }
  return null;
}

function matches(row, selected) {
  if (!row) return false;
  const identity = identityOf(row);
  if (identity === selected) return true;
  return !selected.includes("/") && String(row.name || "") === selected;
}

// projectionRequestOf reads the projection read the addressed application names.
// The page never assembles the address itself: it comes from the page read
// (KOSD-C-76).
export function projectionRequestOf(page, app = "") {
  const row = subjectOf(page, app);
  if (!row) return null;
  const path = String(row.path || "");
  if (path === "") return null;
  return { path };
}

// logRequestOf reads the per-Pod log read a click names. The control carries the
// application's own log address and the Pod it narrows to, so the page never
// widens a read past one Pod (KOSD-C-64).
export function logRequestOf(target) {
  const dataset = target && target.dataset;
  if (!dataset || dataset.logRead !== "true") return null;
  const path = String(dataset.logPath || "");
  if (path === "") return null;
  return { path };
}

// deleteRequestOf reads the delete entry a submitted form names. The form
// carries the method and path the page declared, already resolved to the one Pod
// (KOSD-C-77).
export function deleteRequestOf(form) {
  const dataset = form && form.dataset;
  if (!dataset || dataset.entryItem !== ITEM_POD_DELETE) return null;
  const path = String(dataset.entryPath || "");
  const method = String(dataset.entryMethod || "");
  if (path === "" || method === "") return null;
  return { item: ITEM_POD_DELETE, label: dataset.entryLabel || DELETE_LABEL, method, path };
}

// render draws the page for one projection state, one Pod projection read and
// one log read.
export function render({ app = "", projection = null, podState = null, logState = null, entryNotice = null } = {}) {
  const selected = String(app || "");
  const page = projection && projection.state === "ready" ? projection.page : null;
  return `
    <section class="page" data-page="pods" aria-labelledby="page-title">
      <header class="page-header">
        <h1 id="page-title">Pod 运行态</h1>
      </header>
      ${noticeOf(entryNotice)}
      ${selected === "" ? noApplication() : `${contextOf(selected, page)}${bodyOf(selected, page, projection, podState, logState)}`}
    </section>`;
}

function noApplication() {
  return `<p class="empty-state" data-empty="no-application">${escapeHTML(NO_APPLICATION_NOTE)}</p>`;
}

function contextOf(selected, page) {
  const subject = subjectOf(page, selected);
  const identity = subject === null ? selected : identityOf(subject);
  return `<p class="page-context" data-app="${escapeHTML(selected)}">应用 ${escapeHTML(identity)}</p>`;
}

function bodyOf(selected, page, projection, podState, logState) {
  if (!projection) {
    return `<p class="empty-state" data-empty="no-projection">${escapeHTML(NO_SESSION_NOTE)}</p>`;
  }
  switch (projection.state) {
    case "loading":
      return `<p class="page-state" data-page-state="loading">${escapeHTML(LOADING_NOTE)}</p>`;
    case "error":
      return errorState("Pod 运行态投影不可读", projection.error);
    case "ready": {
      if (!page) return `<p class="empty-state" data-empty="no-projection">${escapeHTML(NO_SESSION_NOTE)}</p>`;
      const subject = subjectOf(page, selected);
      if (subject === null) {
        return `<p class="empty-state" data-empty="application-hidden">当前身份看不到这个应用：Pod 运行态不显示它的任何事实。</p>`;
      }
      return listOf(page, subject, podState, logState);
    }
    default:
      return `<p class="page-state" data-page-state="hidden">当前身份看不到该页面。</p>`;
  }
}

// errorState names the failure instead of showing an empty list: an unreadable
// projection is not an empty one (KOSD-C-76, KOSD-C-64).
function errorState(prefix, error) {
  const code = String((error && error.code) || "");
  const message = String((error && error.message) || "控制面没有给出原因");
  return `<p class="page-state is-error" data-pod-state="error" data-code="${escapeHTML(code)}">${escapeHTML(prefix)}：${escapeHTML(message)}</p>`;
}

// listOf renders the addressed application's Pods. The read that actually ran
// decides first: a failed projection stays a failure with the control plane's
// code and reason even when the cached row says the application is stopped, so
// an unreadable runtime is never presented as a known empty list (KOSD-C-76,
// KOSD-C-64). Only a successful read falls through to the stopped answer, which
// has no Pods and offers no delete entry (TASK-C-3, KOSD-C-66).
function listOf(page, subject, podState, logState) {
  if (!podState) {
    return `<p class="empty-state" data-empty="no-pods-projection">${escapeHTML(PODS_LOADING_NOTE)}</p>`;
  }
  if (podState.state === "loading") {
    return `<p class="page-state" data-pods-state="loading">${escapeHTML(PODS_LOADING_NOTE)}</p>`;
  }
  if (podState.state === "error") {
    return errorState("Pod 列表不可读", podState.error);
  }
  const read = podState.read || {};
  const pods = Array.isArray(read.items) ? read.items : [];
  if (pods.length === 0 && subject.paused === true) {
    const reason = String(subject.reason || "应用已停止：停止的应用没有 Pod（KOSD-C-76）");
    return `<p class="empty-state" data-empty="application-stopped" data-reason="true">${escapeHTML(reason)}</p>`;
  }
  if (pods.length === 0) {
    return `<p class="empty-state" data-empty="no-pods">${escapeHTML(NO_PODS_NOTE)}</p>`;
  }
  return `<ol class="items pods">${pods.map((pod) => podOf(page, subject, pod, logState)).join("")}</ol>`;
}

// podOf is one Pod row: its projected facts, the source type and moment it was
// observed (KOSD-C-13), its own log read and — only when the control plane names
// this application as a delete target — the delete entry (KOSD-C-77).
function podOf(page, subject, pod, logState) {
  const source = String((pod && pod.sourceType) || "object");
  const observedAt = String((pod && pod.observedAt) || "");
  const stale = pod && pod.stale === true;
  return `
    <li class="item pod" data-pod="${escapeHTML(pod && pod.name)}" data-source="${escapeHTML(source)}" data-observed-at="${escapeHTML(observedAt)}" data-stale="${stale}">
      <header class="item-header">
        <h2 class="item-label">${escapeHTML(pod && pod.name)}</h2>
        <span class="item-source" title="${escapeHTML(source)}">${escapeHTML(sourceLabel(source))}</span>
        ${observedAt === "" ? "" : `<span class="item-observed" data-observed-at="${escapeHTML(observedAt)}">观测于 ${escapeHTML(observedText(observedAt))}</span>`}
        ${stale ? `<span class="item-stale" data-stale="true" title="${escapeHTML(STALE_NOTE)}">数据陈旧</span>` : ""}
      </header>
      ${factsOf(pod)}
      ${actionsOf(page, subject, pod, logState)}
    </li>`;
}

// factsOf lists the projected fields TASK-C-3 names, in that order.
function factsOf(pod) {
  const containers = Array.isArray(pod && pod.containers) ? pod.containers : [];
  const startedAt = String((pod && pod.startedAt) || "");
  const restarts = typeof (pod && pod.restarts) === "number" ? pod.restarts : 0;
  const facts = [
    fact("节点", String((pod && pod.node) || "未调度"), `data-pod-node="${escapeHTML(pod && pod.node)}"`),
    fact("阶段", labelOf(PHASE_LABELS, pod && pod.phase), `data-pod-phase="${escapeHTML(pod && pod.phase)}"`),
    fact("就绪", pod && pod.ready === true ? "已就绪" : "未就绪", `data-pod-ready="${pod && pod.ready === true}"`),
    fact("重启次数", String(restarts), `data-pod-restarts="${escapeHTML(restarts)}"`),
  ];
  if (containers.length === 0) {
    facts.push(fact("容器与镜像", "投影没有给出容器"));
  }
  for (const container of containers) {
    facts.push(
      fact(
        "容器与镜像",
        `${container && container.name} · ${container && container.image}`,
        `data-pod-container="${escapeHTML(container && container.name)}" data-pod-image="${escapeHTML(container && container.image)}"`,
      ),
    );
  }
  facts.push(
    fact(
      "启动时间",
      startedAt === "" ? "运行内核没有报告" : observedText(startedAt),
      startedAt === "" ? "" : `data-pod-started-at="${escapeHTML(startedAt)}"`,
    ),
  );
  return `<ul class="facts">${facts.join("")}</ul>`;
}

function fact(label, value, attributes = "") {
  return `<li class="fact"${attributes === "" ? "" : ` ${attributes}`}><span class="fact-name">${escapeHTML(label)}</span><span class="fact-state">${escapeHTML(value)}</span></li>`;
}

// actionsOf lays out the row's log read and, when the control plane offers it,
// the delete entry. Both addresses come from the page read.
function actionsOf(page, subject, pod, logState) {
  const logs = logRowFor(page, subject);
  const logPath = logs === null ? "" : podLogPath(logs, pod);
  const read = logPath === "" ? null : logState && logState.path === logPath ? logState : null;
  const form = deleteFormOf(page, subject, pod);
  const rule = logs === null ? "" : `<p class="item-rule" data-rule="true">${escapeHTML(LOG_RULE_NOTE)}</p>`;
  const button =
    logPath === ""
      ? `<p class="item-empty" data-empty="no-log-address">控制面没有给出该 Pod 的日志地址</p>`
      : `<button type="button" class="entry-submit" data-log-read="true" data-log-path="${escapeHTML(logPath)}">${escapeHTML(LOG_BUTTON_LABEL)}</button>`;
  return `
    <div class="pod-actions">
      ${rule}
      ${button}
      ${form}
    </div>
    ${read === null ? "" : logReadOf(read)}`;
}

function logRowFor(page, subject) {
  for (const row of rowsOf(itemOf(page, ITEM_POD_LOGS))) {
    if (row && row.namespace === subject.namespace && row.name === subject.name) return row;
  }
  return null;
}

// podLogPath narrows the application's log read to one Pod (KOSD-C-64). The
// address is the page read's; only the Pod it is narrowed to comes from here.
function podLogPath(logs, pod) {
  const path = String((logs && logs.path) || "");
  const name = String((pod && pod.name) || "");
  if (path === "" || name === "") return "";
  return `${path}?pod=${encodeURIComponent(name)}`;
}

// deleteFormOf renders the one write control, for the Pod rows the control plane
// named as a delete target. The path is the page entry's, resolved to this
// application and this Pod (KOSD-C-77).
function deleteFormOf(page, subject, pod) {
  const item = itemOf(page, ITEM_POD_DELETE);
  const entry = item && item.entry;
  if (!entry || !entry.method || !entry.path) return "";
  const allowed = rowsOf(item).some(
    (row) => row && row.namespace === subject.namespace && row.name === subject.name,
  );
  if (!allowed) return "";
  const path = String(entry.path)
    .split("{namespace}").join(encodeURIComponent(subject.namespace))
    .split("{name}").join(encodeURIComponent(subject.name))
    .split("{pod}").join(encodeURIComponent(String((pod && pod.name) || "")));
  if (path.includes("{")) return "";
  return `
    <form class="entry" data-entry-item="${ITEM_POD_DELETE}" data-entry-label="${escapeHTML(DELETE_LABEL)}" data-entry-method="${escapeHTML(entry.method)}" data-entry-path="${escapeHTML(path)}">
      <button type="submit" class="entry-submit">${escapeHTML(DELETE_LABEL)}</button>
      <span class="entry-note">${escapeHTML(DELETE_NOTE)}</span>
    </form>`;
}

// logReadOf renders one Pod's bounded log read: its window and bound, the
// truncation position, its lines, or the reason it could not be read.
function logReadOf(read) {
  if (read.state === "loading") {
    return `<p class="page-state" data-log-state="loading">${escapeHTML(LOG_READING_NOTE)}</p>`;
  }
  if (read.state === "error") {
    const code = String((read.error && read.error.code) || "");
    const message = String((read.error && read.error.message) || "控制面没有给出原因");
    return `<p class="page-state is-error" data-log-state="error" data-code="${escapeHTML(code)}">日志不可读：${escapeHTML(message)}</p>`;
  }
  const body = read.read || {};
  const window = String(body.window || "");
  const limit = typeof body.limit === "number" ? String(body.limit) : "";
  const truncatedAt = body.truncated === true ? String(body.truncatedAt || "") : "";
  const truncation =
    truncatedAt === ""
      ? ""
      : `<p class="item-note log-truncated" data-log-truncated="true" data-log-truncated-at="${escapeHTML(truncatedAt)}">${escapeHTML(LOG_TRUNCATED_NOTE)} ${escapeHTML(observedText(truncatedAt))}</p>`;
  const lines = Array.isArray(body.lines) ? body.lines : [];
  const content =
    lines.length === 0
      ? `<p class="item-empty" data-empty="no-log-lines">${escapeHTML(LOG_EMPTY_NOTE)}</p>`
      : `<ul class="facts log-lines" data-log-lines="true">${lines.map(logLineFact).join("")}</ul>`;
  return `
    <div class="log-result" data-log-state="ready" data-log-window="${escapeHTML(window)}" data-log-limit="${escapeHTML(limit)}">
      <p class="item-note" data-log-bounds="true">窗口 ${escapeHTML(window)}，最多 ${escapeHTML(limit)} 行</p>
      ${truncation}
      ${content}
    </div>`;
}

function logLineFact(line) {
  const moment = line && line.timestamp ? observedText(line.timestamp) : "";
  const source = [line && line.pod, line && line.container].filter(truthy).join("/");
  return `
    <li class="fact log-line" data-log-line="true" data-pod="${escapeHTML(line && line.pod)}" data-container="${escapeHTML(line && line.container)}">
      <span class="fact-name">${escapeHTML(moment)}</span>
      <span class="fact-detail">${escapeHTML(source)}</span>
      <span class="fact-state">${escapeHTML(line && line.message)}</span>
    </li>`;
}

// itemOf reads one declared item of the page.
function itemOf(page, id) {
  const items = Array.isArray(page && page.items) ? page.items : [];
  return items.find((item) => item && item.id === id) || null;
}

// rowsOf reads one item's application rows.
function rowsOf(item) {
  const value = item && item.value;
  const rows = value && value.applications;
  return Array.isArray(rows) ? rows : [];
}

// sourceLabel renders a source type; an unknown one stays readable as declared.
function sourceLabel(source) {
  return SOURCE_LABELS[source] || source;
}

// observedText renders the observation in a fixed form so two readers of the
// same projection see the same moment.
export function observedText(observedAt) {
  const moment = new Date(observedAt);
  if (Number.isNaN(moment.getTime())) return String(observedAt);
  return `${moment.toISOString().slice(0, 19).replace("T", " ")} UTC`;
}

function identityOf(object) {
  if (!object) return "";
  const namespace = String(object.namespace || "");
  const name = String(object.name || "");
  return namespace === "" ? name : `${namespace}/${name}`;
}

function labelOf(labels, value) {
  return labels[value] || String(value || "");
}

function truthy(value) {
  return value !== undefined && value !== null && value !== "";
}
