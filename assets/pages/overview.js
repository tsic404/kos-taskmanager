// The overview surface: node readiness, abnormal objects, pending upgrades,
// recent events and maintenance. The page is read-only; this round ships the
// shell, and the control-plane projection that fills it arrives with the page
// itself.
export const id = "overview";
export const title = "概览";

export function render() {
  return `
    <section class="page" data-page="overview" aria-labelledby="page-title">
      <header class="page-header">
        <h1 id="page-title">概览</h1>
        <p class="page-note">只读</p>
      </header>
      <p class="empty-state" data-empty="no-projection">
        控制面投影尚未接入，本页暂不显示数据。
      </p>
    </section>`;
}
