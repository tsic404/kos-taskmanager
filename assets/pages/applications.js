// The applications surface: installed applications, health, resource usage and
// the lifecycle entries. Each row links to the application detail and Pod pages
// once the projection is read.
export const id = "applications";
export const title = "应用";

export function render() {
  return `
    <section class="page" data-page="applications" aria-labelledby="page-title">
      <header class="page-header">
        <h1 id="page-title">应用</h1>
      </header>
      <p class="empty-state" data-empty="no-projection">
        应用列表尚未接入控制面投影，本页暂不显示数据。
      </p>
    </section>`;
}
