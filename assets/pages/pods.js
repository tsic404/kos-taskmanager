import { escapeHTML } from "../html.js";

// The Pod runtime surface of one application: the read-only Pod projection, the
// per-Pod log and the single-Pod delete entry. The list is empty until the
// projection is read; this round ships the shell and the route.
export const id = "pods";
export const title = "Pod 运行态";

export function render({ app = "" } = {}) {
  const selected = String(app || "");
  const body = selected
    ? `<p class="page-context" data-app="${escapeHTML(selected)}">应用 ${escapeHTML(selected)}</p>
      <p class="empty-state" data-empty="no-projection">
        Pod 投影尚未接入，本页暂不显示数据。
      </p>`
    : `<p class="empty-state" data-empty="no-application">
        未选择应用：Pod 属于一个应用，请从应用页进入。
      </p>`;
  return `
    <section class="page" data-page="pods" aria-labelledby="page-title">
      <header class="page-header">
        <h1 id="page-title">Pod 运行态</h1>
      </header>
      ${body}
    </section>`;
}
