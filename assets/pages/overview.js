// The overview surface: node readiness, abnormal objects, pending upgrades,
// recent events and maintenance, each item rendered from the projection the
// control plane serves (TASK-C-1, KOSD-C-53). Every item states the source type
// it is judged by and when it was observed; the control plane's freshness
// verdict is shown as the stale marker (KOSD-C-13). The page is read-only, so
// nothing here emits a modification entry.
import { escapeHTML } from "../html.js";

export const id = "overview";
export const title = "概览";

// READ_ONLY_NOTE is the page's own statement; the control plane's reason for it
// is rendered beside the note once the projection is read (TASK-C-1).
export const READ_ONLY_NOTE = "只读";

// NO_SESSION_NOTE covers a window without a console session: it reads nothing.
export const NO_SESSION_NOTE = "等待控制台会话：概览经控制面读取，独立预览不显示数据。";

// LOADING_NOTE is shown while the projection is being read.
export const LOADING_NOTE = "正在读取概览投影…";

// EMPTY_NOTE is the one empty value every item falls back to.
export const EMPTY_NOTE = "暂无数据";

// STALE_NOTE is the freshness bound every source type shares (KOSD-C-13).
export const STALE_NOTE = "观测时间超过 2 分钟即标记陈旧";

// SOURCE_LABELS are the seven source types of KOSD-C-53 as the page shows them.
export const SOURCE_LABELS = {
  object: "对象源",
  computed: "计算源",
  metric: "指标源",
  probe: "探测源",
  read: "读取源",
  local: "本地源",
  database: "数据库源",
};

// ORIGIN_LABELS are the maintenance origins KOSD-C-82 judges.
export const ORIGIN_LABELS = {
  planned: "计划维护",
  unplanned: "非计划维护",
  unknown: "未知",
  none: "无维护",
};

// UPGRADE_LABELS are the pending-upgrade states the version discovery reports.
export const UPGRADE_LABELS = {
  "definition-changed": "定义已变更",
  "in-progress": "升级中",
  requested: "已请求升级",
};

// VALUE_RENDERERS render one item's value by the item identity it declares.
const VALUE_RENDERERS = {
  "nodes-ready": nodesReadyValue,
  "abnormal-objects": abnormalObjectsValue,
  "pending-upgrades": pendingUpgradesValue,
  "recent-events": recentEventsValue,
  "maintenance-nodes": maintenanceNodesValue,
  "maintenance-origin": maintenanceOriginValue,
  "maintenance-affected-applications": maintenanceAffectedValue,
};

// render draws the page for one projection state. Without a projection the page
// is an empty state, which is what a standalone preview shows.
export function render({ projection = null } = {}) {
  const page = projection && projection.state === "ready" ? projection.page : null;
  return `
    <section class="page" data-page="overview" aria-labelledby="page-title">
      <header class="page-header">
        <h1 id="page-title">概览</h1>
        <p class="page-note" data-readonly="true">${escapeHTML(READ_ONLY_NOTE)}</p>
      </header>
      ${bodyOf(projection, page)}
    </section>`;
}

function bodyOf(projection, page) {
  if (!projection) {
    return `<p class="empty-state" data-empty="no-projection">${escapeHTML(NO_SESSION_NOTE)}</p>`;
  }
  switch (projection.state) {
    case "loading":
      return `<p class="page-state" data-page-state="loading">${escapeHTML(LOADING_NOTE)}</p>`;
    case "error":
      return errorState(projection.error);
    case "ready":
      return `${readOnlyReason(page)}${itemsOf(page)}`;
    default:
      return `<p class="page-state" data-page-state="hidden">当前身份看不到该页面。</p>`;
  }
}

// errorState names the failure instead of showing an empty page: an unreadable
// projection is not an empty one (KOSD-C-64).
function errorState(error) {
  const code = String((error && error.code) || "");
  const message = String((error && error.message) || "控制面没有给出原因");
  return `<p class="page-state is-error" data-page-state="error" data-code="${escapeHTML(code)}">概览投影不可读：${escapeHTML(message)}</p>`;
}

// readOnlyReason states why the page offers no modification entry (KOSD-C-53).
function readOnlyReason(page) {
  const reason = String((page && page.readOnlyReason) || "");
  if (reason === "") return "";
  return `<p class="page-reason" data-readonly-reason="true">${escapeHTML(reason)}</p>`;
}

function itemsOf(page) {
  const items = Array.isArray(page && page.items) ? page.items : [];
  if (items.length === 0) {
    return `<p class="empty-state" data-empty="no-items">${escapeHTML(EMPTY_NOTE)}</p>`;
  }
  return `<ol class="items">${items.map(itemOf).join("")}</ol>`;
}

// itemOf is one display item: its label, the source type it is judged by, when
// it was observed, the control plane's freshness verdict and its value.
function itemOf(item) {
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
      ${valueOf(item, source)}
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

function valueOf(item, source) {
  const value = item && item.value;
  const renderer = VALUE_RENDERERS[item && item.id];
  if (renderer) return renderer(value);
  if (value === null || value === undefined) return emptyValue();
  return `<pre class="item-raw">${escapeHTML(JSON.stringify(value, null, 2))}</pre>`;
}

function emptyValue() {
  return `<p class="item-empty">${escapeHTML(EMPTY_NOTE)}</p>`;
}

// ruleOf states the judging rule of an aggregate item (KOSD-C-53).
function ruleOf(value) {
  const rule = String((value && value.rule) || "");
  if (rule === "") return "";
  return `<p class="item-rule" data-rule="true">判定规则：${escapeHTML(rule)}</p>`;
}

function nodesReadyValue(value) {
  const nodes = entriesOf(value, "nodes");
  const ready = numberOr(value && value.ready, 0);
  const total = numberOr(value && value.total, nodes.length);
  return `
    <p class="item-figure" data-figure="ready">${escapeHTML(String(ready))} / ${escapeHTML(String(total))} 就绪</p>
    ${nodes.length === 0 ? emptyValue() : facts(nodes.map(nodeFact))}`;
}

function nodeFact(node) {
  const ready = node && node.ready === true;
  return `
    <li class="fact" data-node="${escapeHTML(node && node.name)}" data-ready="${ready}">
      <span class="fact-name">${escapeHTML(node && node.name)}</span>
      <span class="fact-state">${ready ? "就绪" : "未就绪"}</span>
    </li>`;
}

function abnormalObjectsValue(value) {
  const objects = entriesOf(value, "objects");
  return `${ruleOf(value)}${objects.length === 0 ? emptyValue() : facts(objects.map(objectFact))}`;
}

function objectFact(object) {
  const conditions = Array.isArray(object && object.conditions) ? object.conditions : [];
  const state = [object && object.phase, object && object.health].filter(truthy).join(" · ");
  return `
    <li class="fact" data-object="${escapeHTML(objectKey(object))}">
      <span class="fact-name">${escapeHTML(objectKey(object))}</span>
      <span class="fact-state">${escapeHTML(state)}</span>
      ${conditions.length === 0 ? "" : `<ul class="conditions">${conditions.map(conditionFact).join("")}</ul>`}
    </li>`;
}

function conditionFact(condition) {
  const text = [condition && condition.type, condition && condition.status, condition && condition.reason, condition && condition.message]
    .filter(truthy)
    .join(" · ");
  return `<li class="condition" data-condition="${escapeHTML(condition && condition.type)}" data-status="${escapeHTML(condition && condition.status)}">${escapeHTML(text)}</li>`;
}

function pendingUpgradesValue(value) {
  const pending = entriesOf(value, "pending");
  return `${ruleOf(value)}${pending.length === 0 ? emptyValue() : facts(pending.map(upgradeFact))}`;
}

function upgradeFact(entry) {
  const target = [entry && entry.target, entry && entry.version ? `（${entry.version}）` : ""].filter(truthy).join("");
  const move = entry && entry.installed ? `${entry.installed} → ${target}` : target;
  return `
    <li class="fact" data-object="${escapeHTML(joinIdentity(entry))}" data-state="${escapeHTML(entry && entry.state)}">
      <span class="fact-name">${escapeHTML(joinIdentity(entry))}</span>
      <span class="fact-state">${escapeHTML(UPGRADE_LABELS[entry && entry.state] || String((entry && entry.state) || ""))}</span>
      ${move === "" ? "" : `<span class="fact-detail">${escapeHTML(move)}</span>`}
    </li>`;
}

function recentEventsValue(value) {
  const events = entriesOf(value, "events");
  const window = String((value && value.window) || "");
  const note = window === "" ? "" : `<p class="item-note" data-window="${escapeHTML(window)}">窗口 ${escapeHTML(window)}，按时间倒序</p>`;
  return `${note}${events.length === 0 ? emptyValue() : facts(events.map(eventFact))}`;
}

function eventFact(event) {
  const moment = event && event.lastSeen ? observedText(event.lastSeen) : "";
  const detail = [moment, event && event.type, event && event.reason].filter(truthy).join(" · ");
  return `
    <li class="fact" data-event="${escapeHTML(event && event.name)}" data-type="${escapeHTML(event && event.type)}">
      <span class="fact-name">${escapeHTML(detail)}</span>
      <span class="fact-state">${escapeHTML(event && event.message)}</span>
      <span class="fact-detail">${escapeHTML(refOf(event && event.involvedObject))}</span>
    </li>`;
}

function maintenanceNodesValue(value) {
  const nodes = entriesOf(value, "nodes");
  return nodes.length === 0 ? emptyValue() : facts(nodes.map(nodeFact));
}

function maintenanceOriginValue(value) {
  const nodes = entriesOf(value, "nodes");
  return nodes.length === 0 ? emptyValue() : facts(nodes.map(originFact));
}

function originFact(entry) {
  const origin = String((entry && entry.origin) || "");
  return `
    <li class="fact" data-node="${escapeHTML(entry && entry.name)}" data-origin="${escapeHTML(origin)}">
      <span class="fact-name">${escapeHTML(entry && entry.name)}</span>
      <span class="fact-state">${escapeHTML(ORIGIN_LABELS[origin] || origin)}</span>
    </li>`;
}

function maintenanceAffectedValue(value) {
  const nodes = entriesOf(value, "nodes");
  return nodes.length === 0 ? emptyValue() : facts(nodes.map(affectedFact));
}

function affectedFact(entry) {
  const applications = Array.isArray(entry && entry.applications) ? entry.applications : [];
  const text = applications.length === 0 ? EMPTY_NOTE : applications.join("、");
  return `
    <li class="fact" data-node="${escapeHTML(entry && entry.node)}">
      <span class="fact-name">${escapeHTML(entry && entry.node)}</span>
      <span class="fact-state">${escapeHTML(text)}</span>
    </li>`;
}

// facts renders one item's entries as a list.
function facts(entries) {
  return `<ul class="facts">${entries.join("")}</ul>`;
}

// entriesOf reads an array the projection carries; anything else is empty.
function entriesOf(value, key) {
  const entries = value && value[key];
  return Array.isArray(entries) ? entries : [];
}

function numberOr(value, fallback) {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  return fallback;
}

function truthy(value) {
  return value !== undefined && value !== null && value !== "";
}

// objectKey is the kind-less identity the abnormal list orders by.
function objectKey(object) {
  return joinIdentity(object);
}

function joinIdentity(object) {
  if (!object) return "";
  const namespace = String(object.namespace || "");
  const name = String(object.name || "");
  return namespace === "" ? name : `${namespace}/${name}`;
}

// refOf renders an object reference as kind and identity.
function refOf(reference) {
  if (!reference) return "";
  const kind = String(reference.kind || "");
  return [kind, joinIdentity(reference)].filter(truthy).join(" ");
}
