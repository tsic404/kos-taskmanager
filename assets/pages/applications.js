// The applications surface: the installed set with its health, the installable
// catalogue entries and the sampled resource usage, each display item rendered
// from the projection the control plane serves with the source type, the
// declared source, the moment it was observed and the control plane's stale
// verdict (TASK-C-2, KOSD-C-53, KOSD-C-13). Every modification entry is offered
// per row, for exactly the applications that entry's own target list names, so
// the page never widens what the control plane trimmed by role and ownership
// (KOSD-C-59). The page submits the control-plane write the entry declares and
// the result is read back from the application object's phase and conditions,
// never from a task record (KOSD-C-12, KOSD-AC-61). No upgrade or update prompt
// lives here (APPSTORE-C-79, APPSTORE-C-80).
import { escapeHTML } from "../html.js";

export const id = "applications";
export const title = "应用";

// NO_SESSION_NOTE covers a window without a console session: it reads nothing.
export const NO_SESSION_NOTE = "等待控制台会话：应用经控制面读取，独立预览不显示数据。";

// LOADING_NOTE is shown while the projection is being read.
export const LOADING_NOTE = "正在读取应用投影…";

// EMPTY_NOTE is the one empty value every item falls back to.
export const EMPTY_NOTE = "暂无数据";

// STALE_NOTE is the freshness bound every source type shares (KOSD-C-13).
export const STALE_NOTE = "观测时间超过 2 分钟即标记陈旧";

// UNAVAILABLE_NOTE is what an unmeasurable usage item states before its reason
// (TASK-C-2): no number is shown beside it.
export const UNAVAILABLE_NOTE = "不可测量";

// MASKED_CONFIG_VALUE is the placeholder the control plane substitutes for a
// config value its definition marks sensitive (KOSD-C-31). The read path never
// returns the value, so the page must never write the placeholder back.
export const MASKED_CONFIG_VALUE = "***";

// DISPLAY_ITEMS are the three items the page reads as values, in the order
// TASK-C-2 declares them.
export const DISPLAY_ITEMS = [
  "installed-applications",
  "installable-applications",
  "application-usage",
];

// ENTRY_ITEMS are the six modification entries in declaration order. The page
// draws exactly these and nothing else: an entry it does not name — an upgrade
// entry, for instance — has no control here (TASK-C-2, APPSTORE-C-79).
export const ENTRY_ITEMS = [
  "stop-applications",
  "start-applications",
  "restart-applications",
  "scale-applications",
  "application-resources",
  "application-uninstall",
];

// The entry identities the form logic treats specially.
export const ITEM_SCALE = "scale-applications";
export const ITEM_RESOURCE_SPEC = "application-resources";
export const ITEM_UNINSTALL = "application-uninstall";

// DEFAULT_API_VERSION is the kind version the control plane serves; the object
// read before a resource write carries its own and wins.
const DEFAULT_API_VERSION = "kos.tsic.top/v1beta1";

// ACCEPTED_ACTIONS maps a page action identity to the value the lifecycle
// endpoint accepts (KOSD-C-22, KOSD-C-66). An identity the endpoint does not
// accept has no entry in this table, so the page cannot submit one.
export const ACCEPTED_ACTIONS = {
  "stop-application": "stop",
  "start-application": "start",
  "restart-application": "restart",
  "scale-application": "scale",
};

// PHASE_LABELS and HEALTH_LABELS render the object's own values; an unknown
// value stays readable as declared instead of being mapped to a guess.
export const PHASE_LABELS = {
  Pending: "等待",
  Installing: "安装中",
  Ready: "就绪",
  Upgrading: "升级中",
  Failed: "失败",
  Uninstalling: "卸载中",
};

export const HEALTH_LABELS = {
  Healthy: "健康",
  Degraded: "降级",
  Unavailable: "不可用",
  Unknown: "未知",
};

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

// render draws the page for one projection state. Without a projection the page
// is an empty state, which is what a standalone preview shows.
export function render({ projection = null, entryNotice = null } = {}) {
  const page = projection && projection.state === "ready" ? projection.page : null;
  return `
    <section class="page" data-page="applications" aria-labelledby="page-title">
      <header class="page-header">
        <h1 id="page-title">应用</h1>
      </header>
      ${noticeOf(entryNotice)}
      ${bodyOf(projection, page)}
    </section>`;
}

// noticeOf states the outcome of the last submission. The accepted case names
// the object read the result comes from; the refused case keeps the control
// plane's code and reason verbatim (KOSD-C-12, KOSD-AC-16).
function noticeOf(notice) {
  if (!notice) return "";
  const label = escapeHTML(notice.label || "操作");
  if (notice.kind === "accepted") {
    return `<p class="entry-notice" data-entry-notice="accepted">已提交${label}；结果从应用对象的阶段与条件读出，已重新读取。</p>`;
  }
  if (notice.kind !== "error") return "";
  const code = String(notice.code || "");
  const message = String(notice.message || "控制面没有给出原因");
  return `<p class="entry-notice is-error" data-entry-notice="error" data-code="${escapeHTML(code)}">提交${label}失败：${escapeHTML(code)} ${escapeHTML(message)}</p>`;
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
      return itemsOf(page);
    default:
      return `<p class="page-state" data-page-state="hidden">当前身份看不到该页面。</p>`;
  }
}

// errorState names the failure instead of showing an empty page: an unreadable
// projection is not an empty one (KOSD-C-64).
function errorState(error) {
  const code = String((error && error.code) || "");
  const message = String((error && error.message) || "控制面没有给出原因");
  return `<p class="page-state is-error" data-page-state="error" data-code="${escapeHTML(code)}">应用投影不可读：${escapeHTML(message)}</p>`;
}

function itemsOf(page) {
  const items = Array.isArray(page && page.items) ? page.items : [];
  const shown = items.filter((item) => item && DISPLAY_ITEMS.includes(item.id));
  if (shown.length === 0) {
    return `<p class="empty-state" data-empty="no-items">${escapeHTML(EMPTY_NOTE)}</p>`;
  }
  return `<ol class="items">${shown.map((item) => itemOf(item, page)).join("")}</ol>`;
}

// itemOf is one display item: its label, the source type it is judged by, the
// source it states, when it was observed and the freshness verdict.
function itemOf(item, page) {
  const source = String((item && item.source) || "");
  const observedAt = String((item && item.observedAt) || "");
  const reason = String((item && item.reason) || "");
  return `
    <li class="item" data-item="${escapeHTML(item && item.id)}" data-source="${escapeHTML(source)}">
      <header class="item-header">
        <h2 class="item-label">${escapeHTML(item && item.label)}</h2>
        <span class="item-source" title="${escapeHTML(source)}">${escapeHTML(sourceLabel(source))}</span>
        ${observedAt === "" ? "" : `<span class="item-observed" data-observed-at="${escapeHTML(observedAt)}">观测于 ${escapeHTML(observedText(observedAt))}</span>`}
        ${item && item.stale ? staleMarker() : ""}
      </header>
      ${reason === "" ? "" : `<p class="item-reason" data-reason="true">来源：${escapeHTML(reason)}</p>`}
      ${valueOf(item, page)}
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
function observedText(observedAt) {
  const moment = new Date(observedAt);
  if (Number.isNaN(moment.getTime())) return observedAt;
  return `${moment.toISOString().slice(0, 19).replace("T", " ")} UTC`;
}

function valueOf(item, page) {
  switch (item && item.id) {
    case "installed-applications":
      return installedValue(item.value, page);
    case "installable-applications":
      return installableValue(item.value);
    case "application-usage":
      return usageValue(item.value);
    default:
      return emptyValue();
  }
}

// emptyValue states why an item has nothing to show: an empty state without a
// reason reads like a broken page (UAT 首次使用).
function emptyValue(reason = "", kind = "no-value") {
  const text = reason === "" ? EMPTY_NOTE : `${EMPTY_NOTE}：${reason}`;
  return `<p class="item-empty" data-empty="${escapeHTML(kind)}">${escapeHTML(text)}</p>`;
}

// installedValue lists the visible applications with the entries each one's
// target list offers it (KOSD-C-59, KOSD-AC-61).
function installedValue(value, page) {
  const rows = entriesOf(value, "applications");
  if (rows.length === 0) return emptyValue("当前身份看不到任何应用，或集群尚未安装应用", "no-applications");
  const entries = entryItemsOf(page);
  return facts(rows.map((row) => applicationFact(row, entries)));
}

// entryItemsOf reads the six entry items the page served, with their declared
// write and the applications each one may act on.
function entryItemsOf(page) {
  const items = Array.isArray(page && page.items) ? page.items : [];
  const served = new Map(items.filter((item) => item && ENTRY_ITEMS.includes(item.id)).map((item) => [item.id, item]));
  return ENTRY_ITEMS.map((id) => served.get(id)).filter(Boolean);
}

// targetOf reads one entry's target for a row; an absent target means the entry
// does not apply to that application and no control is rendered for it.
function targetOf(entry, row) {
  const targets = entriesOf(entry && entry.value, "applications");
  return targets.find((candidate) => candidate && candidate.namespace === row.namespace && candidate.name === row.name) || null;
}

function applicationFact(row, entries) {
  const identity = joinIdentity(row);
  const paused = row && row.paused === true;
  const replicas = row && typeof row.replicas === "number" ? row.replicas : null;
  const conditions = Array.isArray(row && row.conditions) ? row.conditions : [];
  const forms = entries
    .map((entry) => {
      const target = targetOf(entry, row);
      return target ? entryForm(entry, target) : "";
    })
    .join("");
  return `
    <li class="fact" data-object="${escapeHTML(identity)}" data-phase="${escapeHTML(row && row.phase)}" data-health="${escapeHTML(row && row.health)}">
      <span class="fact-name">${escapeHTML(identity)}</span>
      <span class="fact-state">${escapeHTML(labelOf(PHASE_LABELS, row && row.phase))} · ${escapeHTML(labelOf(HEALTH_LABELS, row && row.health))}</span>
      <span class="fact-detail" data-owner="${escapeHTML(row && row.owner)}">属主 ${escapeHTML((row && row.owner) || "无")}</span>
      <span class="fact-detail" data-paused="${paused}">${paused ? "已停止" : "运行中"}</span>
      ${replicas === null ? "" : `<span class="fact-detail" data-replicas="${replicas}">副本 ${replicas}</span>`}
      ${conditions.length === 0 ? "" : `<ul class="conditions">${conditions.map(conditionFact).join("")}</ul>`}
      ${uninstallFact(row)}
      ${forms === "" ? "" : `<div class="entries">${forms}</div>`}
    </li>`;
}

// uninstallFact states the uninstall gate: the application stays listed with the
// objects holding it and whether the block outlasted the stall window
// (KOSD-C-33).
function uninstallFact(row) {
  const uninstall = row && row.uninstall;
  if (!uninstall) return "";
  const blocked = Array.isArray(uninstall.blocked) ? uninstall.blocked : [];
  const held = blocked.length === 0 ? "" : "：" + blocked.map(refOf).join("、");
  return `<span class="fact-detail is-uninstall" data-uninstall="true" data-uninstall-stalled="${uninstall.stalled === true}">卸载门保持中${held}${uninstall.stalled === true ? "（已卡住）" : ""}</span>`;
}

function conditionFact(condition) {
  const text = [condition && condition.type, condition && condition.status, condition && condition.reason, condition && condition.message]
    .filter(truthy)
    .join(" · ");
  return `<li class="condition" data-condition="${escapeHTML(condition && condition.type)}" data-status="${escapeHTML(condition && condition.status)}">${escapeHTML(text)}</li>`;
}

// entryForm renders one modification entry for one application. The controls
// carry the entry's own write and the target's values, which is what the
// submission starts from (KOSD-AC-61).
function entryForm(entry, target) {
  const write = entry && entry.entry;
  const path = String((write && write.path) || "");
  if (path === "") return "";
  const label = String((entry && entry.label) || "");
  return `
    <form class="entry" data-entry-item="${escapeHTML(entry && entry.id)}" data-entry-label="${escapeHTML(label)}" data-entry-action="${escapeHTML(entry && entry.action)}" data-entry-method="${escapeHTML(write && write.method)}" data-entry-path="${escapeHTML(path)}" data-target-namespace="${escapeHTML(target.namespace)}" data-target-name="${escapeHTML(target.name)}" data-target-resource-version="${escapeHTML(target.resourceVersion)}">
      ${entryFields(entry && entry.id, target)}
      <button type="submit" class="entry-submit">${escapeHTML(label)}</button>
    </form>`;
}

// entryFields lays out the values one entry starts from. The two entries with
// values have a field per value; an entry without a value offers its button
// alone.
function entryFields(item, target) {
  if (item === ITEM_SCALE) {
    const replicas = typeof (target && target.replicas) === "number" ? String(target.replicas) : "";
    return `<label class="entry-field">副本数 <input type="number" name="replicas" min="0" step="1" value="${escapeHTML(replicas)}"></label>`;
  }
  if (item !== ITEM_RESOURCE_SPEC) return "";
  const resources = (target && target.resources) || {};
  const requests = resources.requests || {};
  const limits = resources.limits || {};
  return [
    resourceField("requestsCPU", "CPU 请求", requests.cpu),
    resourceField("requestsMemory", "内存请求", requests.memory),
    resourceField("limitsCPU", "CPU 上限", limits.cpu),
    resourceField("limitsMemory", "内存上限", limits.memory),
    target && target.unlimited === true ? `<span class="entry-note" data-unlimited="true">未声明请求与上限</span>` : "",
    target && target.limitRange ? `<span class="entry-note" data-limit-range="${escapeHTML(target.limitRange)}">命名空间 LimitRange：${escapeHTML(target.limitRange)}</span>` : "",
  ].join("");
}

function resourceField(name, label, value) {
  return `<label class="entry-field">${escapeHTML(label)} <input type="text" name="${name}" value="${escapeHTML(value)}" placeholder="留空保持当前值"></label>`;
}

// installableValue lists the catalogue entries with their verdict and gaps
// (APPSTORE-C-31); the list is computed by the control plane, not here.
function installableValue(value) {
  const entries = entriesOf(value, "entries");
  return `${ruleOf(value)}${entries.length === 0 ? emptyValue("目录没有可安装的应用定义", "no-installable") : facts(entries.map(catalogueFact))}`;
}

function catalogueFact(entry) {
  const gaps = Array.isArray(entry && entry.gaps) ? entry.gaps : [];
  const invalid = String((entry && entry.invalid) || "");
  const verdict = entry && entry.installable === true ? "可安装" : "不可安装";
  return `
    <li class="fact" data-catalogue="${escapeHTML(entry && entry.name)}" data-installable="${entry && entry.installable === true}">
      <span class="fact-name">${escapeHTML(entry && entry.name)}</span>
      <span class="fact-state">${escapeHTML(entry && entry.version)}</span>
      <span class="fact-detail">${escapeHTML(verdict)}</span>
      ${invalid === "" ? "" : `<span class="fact-detail">${escapeHTML(invalid)}</span>`}
      ${gaps.length === 0 ? "" : `<ul class="conditions">${gaps.map(gapFact).join("")}</ul>`}
    </li>`;
}

function gapFact(gap) {
  const requirement = String((gap && gap.requirement) || "");
  return `<li class="condition" data-gap="${escapeHTML(requirement)}" data-gap-kind="${escapeHTML(gap && gap.kind)}">缺少 ${escapeHTML(requirement)}</li>`;
}

// usageValue presents the sampled consumption of the metric source. An item
// that is not available states why and carries no number at all: zero is never
// presented as a measurement (TASK-C-2).
function usageValue(value) {
  if (!value || value.available !== true) {
    const reason = String((value && value.reason) || "指标源未接入");
    return `<div class="usage" data-usage="unavailable"><p class="item-reason" data-usage-reason="true">${escapeHTML(UNAVAILABLE_NOTE)}：${escapeHTML(reason)}</p></div>`;
  }
  const samples = entriesOf(value, "applications");
  const body = samples.length === 0 ? emptyValue("指标源本次没有返回样本", "no-samples") : facts(samples.map(usageFact));
  return `<div class="usage" data-usage="available">${body}</div>`;
}

function usageFact(sample) {
  const identity = joinIdentity(sample);
  return `
    <li class="fact" data-usage-sample="${escapeHTML(identity)}">
      <span class="fact-name">${escapeHTML(identity)}</span>
      <span class="fact-state" data-cpu="${escapeHTML(sample && sample.cpu)}">CPU ${escapeHTML(sample && sample.cpu)}</span>
      <span class="fact-detail" data-memory="${escapeHTML(sample && sample.memory)}">内存 ${escapeHTML(sample && sample.memory)}</span>
    </li>`;
}

// ruleOf states the judging rule of a computed item (KOSD-C-53).
function ruleOf(value) {
  const rule = String((value && value.rule) || "");
  if (rule === "") return "";
  return `<p class="item-rule" data-rule="true">判定规则：${escapeHTML(rule)}</p>`;
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

function labelOf(labels, value) {
  return labels[value] || String(value || "");
}

function truthy(value) {
  return value !== undefined && value !== null && value !== "";
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

// formEntryOf reads the entry a submitted form names, with the target it acts
// on and the values that target starts from.
export function formEntryOf(form) {
  const dataset = form && form.dataset;
  if (!dataset || !dataset.entryItem || !dataset.entryPath) return null;
  return {
    item: dataset.entryItem,
    label: dataset.entryLabel || dataset.entryItem,
    action: dataset.entryAction || "",
    method: dataset.entryMethod || "",
    path: dataset.entryPath,
    target: {
      namespace: dataset.targetNamespace || "",
      name: dataset.targetName || "",
      resourceVersion: dataset.targetResourceVersion || "",
    },
  };
}

// valuesOf reads the values a submitted form holds.
export function valuesOf(form) {
  return {
    replicas: fieldValue(form, "replicas"),
    requestsCPU: fieldValue(form, "requestsCPU"),
    requestsMemory: fieldValue(form, "requestsMemory"),
    limitsCPU: fieldValue(form, "limitsCPU"),
    limitsMemory: fieldValue(form, "limitsMemory"),
  };
}

function fieldValue(form, name) {
  if (!form || typeof form.querySelector !== "function") return "";
  const field = form.querySelector(`[name="${name}"]`);
  if (!field) return "";
  return String(field.value === undefined || field.value === null ? "" : field.value);
}

// entryPath expands the control-plane path an entry declared with its target
// (KOSD-C-12, KOSD-C-22).
export function entryPath(entry) {
  const target = (entry && entry.target) || {};
  return String((entry && entry.path) || "")
    .split("{name}").join(encodeURIComponent(target.name || ""))
    .split("{namespace}").join(encodeURIComponent(target.namespace || ""));
}

// entryRequest builds the request one lifecycle or uninstall entry submits. It
// returns null when the entry names no accepted action or no version
// precondition, so the page never sends a write the control plane cannot judge.
export function entryRequest(entry, values = {}) {
  if (!entry || !entry.method || !entry.path) return null;
  const path = entryPath(entry);
  if (entry.item === ITEM_UNINSTALL) {
    if (!entry.target.resourceVersion) return null;
    return { method: entry.method, path, headers: { "If-Match": entry.target.resourceVersion } };
  }
  const action = ACCEPTED_ACTIONS[entry.action];
  if (!action) return null;
  const body = {
    kind: "Application",
    namespace: entry.target.namespace,
    name: entry.target.name,
    action,
  };
  if (entry.item === ITEM_SCALE) {
    const replicas = Number.parseInt(values.replicas, 10);
    if (!Number.isInteger(replicas) || replicas < 0) return null;
    body.replicas = replicas;
  }
  return { method: entry.method, path, body };
}

// resourceSpecRequest builds the update the resource entry submits. The object
// is read first because the control plane replaces the spec it is given, so the
// submission carries the current spec and only the fields the user changed
// against the entry's target (KOSD-C-67); the read's version is the
// precondition (KOSD-C-12).
export function resourceSpecRequest(projection, entry, values = {}) {
  // The object read answers a projection DTO: the application is its object
  // (KOSD-C-12, KOSD-C-13).
  const application = projection && projection.object ? projection.object : projection;
  if (!application || typeof application !== "object" || !entry) return null;
  const metadata = application.metadata || {};
  const version = String(metadata.resourceVersion || entry.target.resourceVersion || "");
  if (version === "") return null;
  const spec = application.spec && typeof application.spec === "object" ? { ...application.spec } : {};
  // The read path masks the config values the definition marks sensitive. The
  // control plane replaces the whole spec with the submitted one, so writing
  // the mask back would store the placeholder as the credential. The page
  // refuses that submission instead of sending a body it cannot make safe.
  const config = withoutMaskedConfig(spec.config);
  if (config.masked.length > 0) {
    return {
      masked: config.masked,
      refused: "应用配置中的敏感值已掩码（" + config.masked.join("、") + "），整段回写会把掩码写成真实值；请先在应用配置中重新输入后再改资源规格。",
    };
  }
  if (config.value === undefined) delete spec.config;
  else spec.config = config.value;
  const resources = changedResources(spec.resources, (entry.target && entry.target.resources) || {}, values);
  if (resources === null) delete spec.resources;
  else spec.resources = resources;
  return {
    method: entry.method,
    path: entryPath(entry),
    headers: { "If-Match": version },
    body: {
      apiVersion: String(application.apiVersion || DEFAULT_API_VERSION),
      kind: "Application",
      metadata: metadataWith(application.metadata, entry.target),
      spec,
    },
  };
}

// metadataWith carries the identity and the labels/annotations the read
// returned: the control plane applies the submitted metadata over the stored
// one, so leaving them out would drop them (KOSD-C-12).
function metadataWith(metadata, target) {
  const carried = {
    name: String((metadata && metadata.name) || target.name || ""),
    namespace: String((metadata && metadata.namespace) || target.namespace || ""),
  };
  for (const key of ["labels", "annotations"]) {
    const value = metadata && metadata[key];
    if (value && typeof value === "object" && Object.keys(value).length > 0) carried[key] = value;
  }
  return carried;
}

// withoutMaskedConfig separates the config values the read path masked from
// the ones it returned verbatim.
function withoutMaskedConfig(config) {
  if (!config || typeof config !== "object" || Array.isArray(config)) {
    return { value: config, masked: [] };
  }
  const kept = {};
  const masked = [];
  for (const [key, value] of Object.entries(config)) {
    if (value === MASKED_CONFIG_VALUE) masked.push(key);
    else kept[key] = value;
  }
  return { value: kept, masked };
}

// RESOURCE_FIELDS joins one form field to the resource list it writes.
const RESOURCE_FIELDS = [
  ["requests", "cpu", "requestsCPU"],
  ["requests", "memory", "requestsMemory"],
  ["limits", "cpu", "limitsCPU"],
  ["limits", "memory", "limitsMemory"],
];

// changedResources applies the fields whose submitted value differs from the
// entry's target onto the specification just read; a field left as it was, or
// left blank, keeps the value the application carries (KOSD-C-67).
function changedResources(current, declared, values) {
  const merged = {
    requests: { ...((current && current.requests) || {}) },
    limits: { ...((current && current.limits) || {}) },
  };
  for (const [section, field, key] of RESOURCE_FIELDS) {
    const submitted = String(values[key] === undefined || values[key] === null ? "" : values[key]).trim();
    if (submitted === "") continue;
    const original = String(((declared && declared[section]) || {})[field] || "").trim();
    if (submitted === original) continue;
    merged[section][field] = submitted;
  }
  const declaredValues = [merged.requests.cpu, merged.requests.memory, merged.limits.cpu, merged.limits.memory];
  if (declaredValues.every((value) => !truthy(value))) return null;
  return merged;
}
