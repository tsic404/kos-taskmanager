import { escapeHTML } from "./html.js";

// The four page surfaces of the run view and the addresses they live at. The
// route table is the single statement of what the window can show, so the
// navigation, a reload and a deep link resolve to the same page shell.

// page marks a surface the extension declares as a control-plane page: its
// entry follows the page registry the control plane serves for the session, so
// an identity that cannot see the page gets neither the entry nor the page
// (KOSD-C-53, DASH-C-22). The other surfaces are the window's own views.
export const ROUTES = [
  { id: "overview", title: "概览", scoped: false, page: true },
  { id: "applications", title: "应用", scoped: false, page: false },
  { id: "detail", title: "应用详情", scoped: true, page: false },
  { id: "pods", title: "Pod 运行态", scoped: true, page: false },
];

const DEFAULT_ROUTE = ROUTES[0];

// matchRoute resolves one location hash to a page and, for the application-scoped
// pages, the application the address names. An address outside the table is not
// a page: the caller falls back to the overview rather than inventing one.
export function matchRoute(hash) {
  const segments = segmentsOf(hash);
  if (segments.length === 1 && segments[0] === "overview") return { route: routeByID("overview") };
  if (segments.length === 1 && segments[0] === "applications") return { route: routeByID("applications") };
  if (segments.length === 2 && segments[0] === "applications") {
    return { route: routeByID("detail"), app: segments[1] };
  }
  if (segments.length === 3 && segments[0] === "applications" && segments[2] === "pods") {
    return { route: routeByID("pods"), app: segments[1] };
  }
  return null;
}

// routeAddress is the address one page lives at. The scoped pages have no
// address without an application, which is why the navigation only links them
// once one is selected.
export function routeAddress(route, app = "") {
  switch (route.id) {
    case "overview":
      return "#/overview";
    case "applications":
      return "#/applications";
    case "detail":
      return app ? `#/applications/${encodeURIComponent(app)}` : "";
    case "pods":
      return app ? `#/applications/${encodeURIComponent(app)}/pods` : "";
    default:
      return "";
  }
}

// renderNav lists every page surface. The application-scoped pages become links
// once the current address names an application; before that they stay visible
// but unlinked, so the window never offers a page it cannot fill. A page the
// identity cannot see is absent from the navigation entirely (DASH-C-22).
export function renderNav(currentID, app = "", hiddenPages = []) {
  const items = ROUTES.filter((route) => !isHidden(route, hiddenPages)).map((route) => {
    const label = escapeHTML(route.title);
    const address = routeAddress(route, app);
    if (!address) {
      return `<span class="nav-entry is-contextual" data-route="${route.id}" title="先选择一个应用">${label}</span>`;
    }
    const current = route.id === currentID ? ' aria-current="page"' : "";
    return `<a class="nav-entry" data-route="${route.id}" href="${address}"${current}>${label}</a>`;
  });
  return `<nav class="nav" aria-label="页面">${items.join("")}</nav>`;
}

// isHidden reports a page surface the control plane says this identity cannot
// see; only surfaces that are declared pages can be hidden.
export function isHidden(route, hiddenPages = []) {
  return Boolean(route && route.page) && hiddenPages.includes(route.id);
}

export function defaultHash() {
  return routeAddress(DEFAULT_ROUTE);
}

function segmentsOf(hash) {
  const value = String(hash || "").replace(/^#/, "");
  return value
    .split("/")
    .filter((segment) => segment !== "")
    .map((segment) => {
      try {
        return decodeURIComponent(segment);
      } catch {
        return segment;
      }
    });
}

function routeByID(id) {
  return ROUTES.find((route) => route.id === id);
}
