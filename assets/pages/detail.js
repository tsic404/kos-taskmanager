import { escapeHTML } from "../html.js";

// The application detail surface: health, volumes, endpoints, backups, drift and
// events for one application. The address names the application; without one the
// page states why it is empty instead of showing another application's facts.
export const id = "detail";
export const title = "应用详情";

export function render({ app = "" } = {}) {
  const selected = String(app || "");
  const body = selected
    ? `<p class="page-context" data-app="${escapeHTML(selected)}">应用 ${escapeHTML(selected)}</p>
      <p class="empty-state" data-empty="no-projection">
        应用详情尚未接入控制面投影，本页暂不显示数据。
      </p>`
    : `<p class="empty-state" data-empty="no-application">
        未选择应用：请从应用页进入一个应用的详情。
      </p>`;
  return `
    <section class="page" data-page="detail" aria-labelledby="page-title">
      <header class="page-header">
        <h1 id="page-title">应用详情</h1>
      </header>
      ${body}
    </section>`;
}
