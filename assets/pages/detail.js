// The application detail surface: the seven items DASH-C-52 lists — health,
// volumes, ingress, backup, drift, events and logs — for the application the
// address names. Every item states the source type it is judged by, the moment
// it was observed and the control plane's stale verdict (TASK-C-1, KOSD-C-53,
// KOSD-C-13). A read that carries no row for the addressed application is an
// application this session may not see, so the page shows none of its facts
// (KOSD-C-59, KOSD-AC-16). The log item is the one read-source item: the reader
// asks for that application's lines and the page shows the bounded result —
// window, line bound, truncation position or the failure reason — without
// storing anything (KOSD-C-64). The page is read-only, so nothing here submits
// a modification and no entry is rendered.
import { escapeHTML } from "../html.js";
import { HEALTH_LABELS, PHASE_LABELS } from "./applications.js";

export const id = "detail";
export const title = "应用详情";

// READ_ONLY_NOTE is the page's own statement; the control plane's reason for it
// is rendered beside the note once the projection is read (KOSD-C-53).
export const READ_ONLY_NOTE = "只读";

// NO_SESSION_NOTE covers a window without a console session: it reads nothing.
export const NO_SESSION_NOTE = "等待控制台会话：应用详情经控制面读取，独立预览不显示数据。";

// NO_APPLICATION_NOTE covers an address that names no application.
export const NO_APPLICATION_NOTE = "未选择应用：请从应用页进入一个应用的详情。";

// LOADING_NOTE is shown while the projection is being read.
export const LOADING_NOTE = "正在读取应用详情投影…";

// EMPTY_NOTE is the one empty value every item falls back to.
export const EMPTY_NOTE = "暂无数据";

// STALE_NOTE is the freshness bound every source type shares (KOSD-C-13).
export const STALE_NOTE = "观测时间超过 2 分钟即标记陈旧";

// LOG_BUTTON_LABEL is the one control the page offers; it reads, never writes.
export const LOG_BUTTON_LABEL = "读取日志";

// LOG_READING_NOTE is shown while one application's log read runs.
export const LOG_READING_NOTE = "正在读取该应用的日志…";

// LOG_EMPTY_NOTE is what a read inside the window with no lines states: an
// empty answer from a reachable source, never a stand-in for a failure.
export const LOG_EMPTY_NOTE = "暂无数据：该应用在时间窗内没有日志行";

// LOG_TRUNCATED_NOTE names what the truncation mark means (KOSD-C-64).
export const LOG_TRUNCATED_NOTE = "已截断：只保留最近的行，截断位置";

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

// The item identities DASH-C-52 fixes for this page.
export const ITEM_HEALTH = "detail-health";
export const ITEM_VOLUMES = "detail-volumes";
export const ITEM_INGRESS = "detail-ingress";
export const ITEM_BACKUP = "detail-backup-policy";
export const ITEM_DRIFT = "detail-drift";
export const ITEM_EVENTS = "detail-events";
export const ITEM_LOGS = "detail-logs";

// present reports whether the page read carried the addressed application. The
// control plane trims every item to the applications the session may see, so an
// application with no row is one this identity cannot see (KOSD-C-59); the
// window then shows neither the page nor its entry.
export function present({ app = "", projection = null } = {}) {
  const page = projection && projection.state === "ready" ? projection.page : null;
  if (!page) return true;
  return subjectOf(page, app) !== null;
}

// logRequestOf reads the log read a click names. The control carries the
// application's own address, so the page never assembles one itself.
export function logRequestOf(target) {
  const dataset = target && target.dataset;
  if (!dataset || dataset.logRead !== "true") return null;
  const path = String(dataset.logPath || "");
  if (path === "") return null;
  return { path };
}

// render draws the page for one projection state and one log read.
export function render({ app = "", projection = null, logState = null } = {}) {
  const selected = String(app || "");
  const page = projection && projection.state === "ready" ? projection.page : null;
  return `
    <section class="page" data-page="detail" aria-labelledby="page-title">
      <header class="page-header">
        <h1 id="page-title">应用详情</h1>
        <p class="page-note" data-readonly="true">${escapeHTML(READ_ONLY_NOTE)}</p>
      </header>
      ${selected === "" ? noApplication() : `${contextOf(selected, page)}${bodyOf(selected, page, projection, logState)}`}
    </section>`;
}

function noApplication() {
  return `<p class="empty-state" data-empty="no-application">${escapeHTML(NO_APPLICATION_NOTE)}</p>`;
}

// contextOf names the application the address selected, in the projection's own
// namespace/name form once it carries the row.
function contextOf(selected, page) {
  const subject = subjectOf(page, selected);
  const identity = subject === null ? selected : identityOf(subject);
  return `<p class="page-context" data-app="${escapeHTML(selected)}">应用 ${escapeHTML(identity)}</p>`;
}

function bodyOf(selected, page, projection, logState) {
  if (!projection) {
    return `<p class="empty-state" data-empty="no-projection">${escapeHTML(NO_SESSION_NOTE)}</p>`;
  }
  switch (projection.state) {
    case "loading":
      return `<p class="page-state" data-page-state="loading">${escapeHTML(LOADING_NOTE)}</p>`;
    case "error":
      return errorState(projection.error);
    case "ready":
      if (!page) return `<p class="empty-state" data-empty="no-projection">${escapeHTML(NO_SESSION_NOTE)}</p>`;
      if (subjectOf(page, selected) === null) {
        return `<p class="empty-state" data-empty="application-hidden">当前身份看不到这个应用：应用详情不显示它的任何事实。</p>`;
      }
      return `${readOnlyReason(page)}${itemsOf(page, selected, logState)}`;
    default:
      return `<p class="page-state" data-page-state="hidden">当前身份看不到该页面。</p>`;
  }
}

// errorState names the failure instead of showing an empty page: an unreadable
// projection is not an empty one (KOSD-C-56).
function errorState(error) {
  const code = String((error && error.code) || "");
  const message = String((error && error.message) || "控制面没有给出原因");
  return `<p class="page-state is-error" data-page-state="error" data-code="${escapeHTML(code)}">应用详情投影不可读：${escapeHTML(message)}</p>`;
}

// readOnlyReason states why the page offers no modification entry (KOSD-C-53).
function readOnlyReason(page) {
  const reason = String((page && page.readOnlyReason) || "");
  if (reason === "") return "";
  return `<p class="page-reason" data-readonly-reason="true">${escapeHTML(reason)}</p>`;
}

function itemsOf(page, selected, logState) {
  const items = Array.isArray(page && page.items) ? page.items : [];
  if (items.length === 0) {
    return `<p class="empty-state" data-empty="no-items">${escapeHTML(EMPTY_NOTE)}</p>`;
  }
  const subject = subjectOf(page, selected);
  return `<ol class="items">${items.map((item) => itemOf(item, subject, logState)).join("")}</ol>`;
}

// itemOf is one display item: its label, the source type it is judged by, when
// it was observed, the control plane's freshness verdict and its value.
function itemOf(item, subject, logState) {
  const source = String((item && item.source) || "");
  const observedAt = String((item && item.observedAt) || "");
  return `
    <li class="item" data-item="${escapeHTML(item && item.id)}" data-source="${escapeHTML(source)}">
      <header class="item-header">
        <h2 class="item-label">${escapeHTML(item && item.label)}</h2>
        <span class="item-source" title="${escapeHTML(source)}">${escapeHTML(sourceLabel(source))}</span>
        ${observedAt === "" ? "" : `<span class="item-observed" data-observed-at="${escapeHTML(observedAt)}">观测于 ${escapeHTML(observedText(observedAt))}</span>`}
        ${item && item.stale ? staleMarker() : ""}
      </header>
      ${valueOf(item, subject, logState)}
    </li>`;
}

function staleMarker() {
  return `<span class="item-stale" data-stale="true" title="${escapeHTML(STALE_NOTE)}">数据陈旧</span>`;
}

// sourceLabel renders a source type; an unknown one stays readable as declared.
function sourceLabel(source) {
  return SOURCE_LABELS[source] || source;
}

// observedText renders the observation in a fixed form so two readers of the
// same projection see the same moment.
export function observedText(observedAt) {
  const moment = new Date(observedAt);
  if (Number.isNaN(moment.getTime())) return observedAt;
  return `${moment.toISOString().slice(0, 19).replace("T", " ")} UTC`;
}

function valueOf(item, subject, logState) {
  const value = item && item.value;
  switch (item && item.id) {
    case ITEM_HEALTH:
      return healthValue(value, subject);
    case ITEM_VOLUMES:
      return volumesValue(value, subject);
    case ITEM_INGRESS:
      return ingressValue(value, subject);
    case ITEM_BACKUP:
      return backupValue(value, subject);
    case ITEM_DRIFT:
      return driftValue(value, subject);
    case ITEM_EVENTS:
      return eventsValue(value, subject);
    case ITEM_LOGS:
      return logsValue(value, subject, logState);
    default:
      if (value === null || value === undefined) return emptyValue();
      return `<pre class="item-raw">${escapeHTML(JSON.stringify(value, null, 2))}</pre>`;
  }
}

// emptyValue states why an item has nothing to show.
function emptyValue(reason = "", kind = "no-value") {
  const text = reason === "" ? EMPTY_NOTE : `${EMPTY_NOTE}：${reason}`;
  return `<p class="item-empty" data-empty="${escapeHTML(kind)}">${escapeHTML(text)}</p>`;
}

// subjectOf reads the namespace/name the addressed application carries in the
// projection. An address without a namespace names the application inside its
// namespace, which is the only reading a bare name allows.
export function subjectOf(page, app = "") {
  const selected = String(app || "");
  if (selected === "") return null;
  const items = Array.isArray(page && page.items) ? page.items : [];
  for (const item of items) {
    for (const row of rowsOf(item && item.value)) {
      if (matches(row, selected)) {
        return { namespace: String((row && row.namespace) || ""), name: String((row && row.name) || "") };
      }
    }
  }
  return null;
}

function matches(row, selected) {
  if (!row) return false;
  const identity = identityOf(row);
  if (identity === selected) return true;
  return !selected.includes("/") && String(row.name || "") === selected;
}

// rowOf picks one item's row for the addressed application.
function rowOf(value, subject) {
  if (!subject) return null;
  return rowsOf(value).find((row) => row && row.namespace === subject.namespace && row.name === subject.name) || null;
}

function rowsOf(value) {
  const rows = value && value.applications;
  return Array.isArray(rows) ? rows : [];
}

// healthValue is the application object's own state (DASH-C-52).
function healthValue(value, subject) {
  const row = rowOf(value, subject);
  if (!row) return emptyValue("投影没有该应用的健康行", "no-application-row");
  const conditions = Array.isArray(row.conditions) ? row.conditions : [];
  const replicas = typeof row.replicas === "number" ? `<span class="fact-detail" data-replicas="${row.replicas}">副本 ${row.replicas}</span>` : "";
  return facts([
    `
    <li class="fact" data-health-row="true" data-object="${escapeHTML(identityOf(row))}" data-phase="${escapeHTML(row.phase)}" data-health="${escapeHTML(row.health)}">
      <span class="fact-name">${escapeHTML(identityOf(row))}</span>
      <span class="fact-state">${escapeHTML(labelOf(PHASE_LABELS, row.phase))} · ${escapeHTML(labelOf(HEALTH_LABELS, row.health))}</span>
      <span class="fact-detail" data-owner="${escapeHTML(row.owner)}">属主 ${escapeHTML(row.owner || "无")}</span>
      <span class="fact-detail" data-paused="${row.paused === true}">${row.paused === true ? "已停止" : "运行中"}</span>
      ${replicas}
      ${conditions.length === 0 ? "" : `<ul class="conditions">${conditions.map(conditionFact).join("")}</ul>`}
    </li>`,
  ]);
}

function conditionFact(condition) {
  const text = [condition && condition.type, condition && condition.status, condition && condition.reason, condition && condition.message]
    .filter(truthy)
    .join(" · ");
  return `<li class="condition" data-condition="${escapeHTML(condition && condition.type)}" data-status="${escapeHTML(condition && condition.status)}">${escapeHTML(text)}</li>`;
}

// volumesValue lists the volumes the application declares; one the cluster
// cannot resolve stays visible as unresolved (KOSD-C-40).
function volumesValue(value, subject) {
  const row = rowOf(value, subject);
  if (!row) return emptyValue("投影没有该应用的卷行", "no-application-row");
  const volumes = Array.isArray(row.volumes) ? row.volumes : [];
  if (volumes.length === 0) return emptyValue("该应用没有声明卷", "no-volumes");
  return facts(volumes.map(volumeFact));
}

function volumeFact(volume) {
  const resolved = volume && volume.resolved === true;
  const details = [
    volume && volume.mobility,
    volume && volume.capacity,
    volume && volume.storageClass,
    volume && volume.node ? `节点 ${volume.node}` : "",
    volume && volume.boundClaim ? `绑定 ${volume.boundClaim}` : "",
    volume && volume.phase,
    volume && volume.health,
  ].filter(truthy);
  return `
    <li class="fact" data-volume="${escapeHTML(volume && volume.name)}" data-resolved="${resolved}">
      <span class="fact-name">${escapeHTML(volume && volume.name)}</span>
      <span class="fact-state">${resolved ? "已解析" : "未解析"}</span>
      ${details.length === 0 ? "" : `<span class="fact-detail">${escapeHTML(details.join(" · "))}</span>`}
    </li>`;
}

// ingressValue lists the endpoints whose target is this application (KOSD-C-43).
function ingressValue(value, subject) {
  const row = rowOf(value, subject);
  if (!row) return emptyValue("投影没有该应用的入口行", "no-application-row");
  const endpoints = Array.isArray(row.endpoints) ? row.endpoints : [];
  if (endpoints.length === 0) return emptyValue("该应用没有入口", "no-endpoints");
  return facts(endpoints.map(endpointFact));
}

function endpointFact(endpoint) {
  const address = [endpoint && endpoint.host, endpoint && endpoint.path].filter(truthy).join("");
  const port = endpoint && endpoint.port ? `:${endpoint.port}` : "";
  const shape = [endpoint && endpoint.kind, address + port].filter(truthy).join(" · ");
  const state = [endpoint && endpoint.phase, endpoint && endpoint.health].filter(truthy).join(" · ");
  const certificate = endpoint && endpoint.certificate;
  return `
    <li class="fact" data-endpoint="${escapeHTML(endpoint && endpoint.name)}" data-kind="${escapeHTML(endpoint && endpoint.kind)}">
      <span class="fact-name">${escapeHTML(endpoint && endpoint.name)}</span>
      <span class="fact-state">${escapeHTML(shape)}</span>
      <span class="fact-detail">${escapeHTML(state)}</span>
      ${certificate ? `<span class="fact-detail" data-certificate="${certificate.ready === true}">证书 ${certificate.ready === true ? "就绪" : "未就绪"}${certificate.issuer ? `（${escapeHTML(certificate.issuer)}）` : ""}</span>` : ""}
    </li>`;
}

// backupValue lists the policies whose selector covers this application
// (KOSD-C-41); an unparseable selector keeps its reason.
function backupValue(value, subject) {
  const row = rowOf(value, subject);
  if (!row) return emptyValue("投影没有该应用的备份行", "no-application-row");
  const policies = Array.isArray(row.policies) ? row.policies : [];
  if (policies.length === 0) return emptyValue("没有备份策略覆盖该应用", "no-policies");
  return facts(policies.map(policyFact));
}

function policyFact(policy) {
  const details = [
    policy && policy.selector ? `选择器 ${policy.selector}` : "",
    policy && policy.consistency ? `一致性 ${policy.consistency}` : "",
    typeof (policy && policy.retention) === "number" ? `保留 ${policy.retention}` : "",
    policy && policy.drill ? "演练开启" : "",
    [policy && policy.phase, policy && policy.health].filter(truthy).join(" · "),
    policy && policy.invalid,
  ].filter(truthy);
  return `
    <li class="fact" data-policy="${escapeHTML(policy && policy.name)}">
      <span class="fact-name">${escapeHTML(policy && policy.name)}</span>
      <span class="fact-state">${escapeHTML(policy && policy.schedule)}</span>
      ${details.length === 0 ? "" : `<span class="fact-detail">${escapeHTML(details.join(" · "))}</span>`}
    </li>`;
}

// driftValue is the external-change record the application carries (KOSD-C-35).
function driftValue(value, subject) {
  const row = rowOf(value, subject);
  if (!row) return emptyValue("投影没有该应用的漂移行", "no-application-row");
  const policy = String((row && row.policy) || "");
  const note = policy === "" ? "" : `<p class="item-note" data-drift-policy="${escapeHTML(policy)}">漂移处理：${escapeHTML(policy)}</p>`;
  const entries = Array.isArray(row.entries) ? row.entries : [];
  if (entries.length === 0) return `${note}${emptyValue("没有记录到漂移", "no-drift")}`;
  return `${note}${facts(entries.map(driftEntryFact))}`;
}

function driftEntryFact(entry) {
  const object = (entry && entry.object) || {};
  const external = String((entry && entry.external) || "");
  const expected = String((entry && entry.expected) || "");
  const values = [external === "" ? "" : `外部值 ${external}`, expected === "" ? "" : `期望值 ${expected}`].filter(truthy);
  return `
    <li class="fact" data-drift-entry="true" data-drift-path="${escapeHTML(entry && entry.path)}">
      <span class="fact-name">${escapeHTML([object.kind, identityOf(object)].filter(truthy).join(" "))}</span>
      <span class="fact-state">${escapeHTML(entry && entry.path)}</span>
      ${values.length === 0 ? "" : `<span class="fact-detail">${escapeHTML(values.join(" · "))}</span>`}
    </li>`;
}

// eventsValue lists the events about the application, newest first, with the
// window the control plane read them in (KOSD-C-64, KOSD-AC-59).
function eventsValue(value, subject) {
  const row = rowOf(value, subject);
  if (!row) return emptyValue("投影没有该应用的事件行", "no-application-row");
  const window = String((value && value.window) || "");
  const note = window === "" ? "" : `<p class="item-note" data-window="${escapeHTML(window)}">窗口 ${escapeHTML(window)}，按时间倒序</p>`;
  const events = Array.isArray(row.events) ? row.events : [];
  if (events.length === 0) return `${note}${emptyValue("窗口内没有该应用的事件", "no-events")}`;
  return `${note}${facts(events.map(eventFact))}`;
}

function eventFact(event) {
  const moment = event && event.lastSeen ? observedText(event.lastSeen) : "";
  const shape = [event && event.type, event && event.reason].filter(truthy).join(" · ");
  const count = typeof (event && event.count) === "number" ? `（${event.count} 次）` : "";
  return `
    <li class="fact" data-event="${escapeHTML(event && event.name)}" data-type="${escapeHTML(event && event.type)}">
      <span class="fact-name">${escapeHTML(moment)}</span>
      <span class="fact-state">${escapeHTML(shape)}</span>
      <span class="fact-detail">${escapeHTML(event && event.message)}${escapeHTML(count)}</span>
    </li>`;
}

// logsValue states the read rule, the application's own address and the result
// of the read the reader asked for. Lines never come from the page projection
// (KOSD-C-64).
function logsValue(value, subject, logState) {
  const row = rowOf(value, subject);
  if (!row) return emptyValue("投影没有该应用的日志行", "no-application-row");
  const rule = String((value && value.rule) || "");
  const path = String((row && row.path) || "");
  const note = rule === "" ? "" : `<p class="item-rule" data-rule="true">读取规则：${escapeHTML(rule)}</p>`;
  if (path === "") return `${note}${emptyValue("控制面没有给出该应用的日志地址", "no-log-address")}`;
  const read = logState && logState.path === path ? logState : null;
  return `${note}
    <div class="log" data-log-address="${escapeHTML(path)}">
      <p class="item-note" data-log-path="${escapeHTML(path)}">按应用读取：${escapeHTML(identityOf(row))} <code>${escapeHTML(path)}</code></p>
      <button type="button" class="entry-submit" data-log-read="true" data-log-path="${escapeHTML(path)}">${escapeHTML(LOG_BUTTON_LABEL)}</button>
      ${logReadOf(read)}
    </div>`;
}

function logReadOf(read) {
  if (!read) return "";
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

// facts renders one item's entries as a list.
function facts(entries) {
  return `<ul class="facts">${entries.join("")}</ul>`;
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
