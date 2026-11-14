package app

import (
	"net/http"

	"github.com/tsic404/kosd/pkg/app"
	"github.com/tsic404/kosd/pkg/extension"
)

// PageApplications is the installed-application surface (TASK-C-2).
const PageApplications = "applications"

// Item identities of the application page. The first three are display items:
// the installed set, the installable entries and the sampled usage. Every other
// item carries one modification entry, so a role that may not write an
// application never receives a target for it (TASK-C-2, KOSD-C-59).
const (
	ItemInstalled     = "installed-applications"
	ItemInstallable   = "installable-applications"
	ItemResourceUsage = "application-usage"
	ItemStop          = "stop-applications"
	ItemStart         = "start-applications"
	ItemRestart       = "restart-applications"
	ItemScale         = "scale-applications"
	ItemResourceSpec  = "application-resources"
	ItemUninstall     = "application-uninstall"
)

// Action identities of the application page (KOSD-C-22). The browser supplies
// the target application in the request body or path.
const (
	ActionStop         = "stop-application"
	ActionStart        = "start-application"
	ActionRestart      = "restart-application"
	ActionScale        = "scale-application"
	ActionResourceSpec = "set-application-resources"
	ActionUninstall    = "uninstall-application"
)

// Control-plane paths the entries write through (KOSD-C-12, KOSD-C-22).
const (
	// PathActionRequests carries the lifecycle actions C-66 names as one write.
	PathActionRequests = "/api/v1/action-requests"
	// PathApplications is the application resource; the resource-spec entry
	// updates it and the uninstall entry deletes it behind a version
	// precondition (KOSD-C-33, KOSD-C-67).
	PathApplications = "/api/v1/resources/Application"
)

// Display reasons of the items that are not themselves modification entries
// (KOSD-C-53).
const (
	ReasonInstalled   = "应用事实由应用调和写入，修改经同一页的操作入口"
	ReasonInstallable = "可安装列表按定义与能力需求计算，安装经应用中心发起"
	ReasonUsage       = "资源占用按指标采样读取，页面不写入"
)

// ControlPlaneAction names the action value the control plane accepts for one
// lifecycle action identity (KOSD-C-22, KOSD-C-66). The identity belongs to the
// page, the value to the API, so an entry that names a value the endpoint does
// not accept is caught before the browser ever offers it.
func ControlPlaneAction(id string) (string, bool) {
	switch id {
	case ActionStop:
		return app.ActionStop, true
	case ActionStart:
		return app.ActionStart, true
	case ActionRestart:
		return app.ActionRestart, true
	case ActionScale:
		return app.ActionScale, true
	default:
		return "", false
	}
}

// ApplicationsPage declares the application surface TASK-C-2 names: the
// installed, installable and usage reads, and the stop, start, restart, scale,
// resource-spec and uninstall entries. It carries no upgrade or update entry:
// both live in the app center alone (APPSTORE-C-79, APPSTORE-C-80).
func ApplicationsPage() extension.Page {
	return extension.Page{
		ID:       PageApplications,
		Title:    "应用",
		Location: "panel",
		Items: []extension.PageItem{
			{ID: ItemInstalled, Label: "已安装应用与健康", Source: extension.PageSourceObject, Reference: reference(ItemInstalled), Reason: ReasonInstalled},
			{ID: ItemInstallable, Label: "可安装列表", Source: extension.PageSourceComputed, Reference: reference(ItemInstallable), Reason: ReasonInstallable},
			{ID: ItemResourceUsage, Label: "资源占用", Source: extension.PageSourceMetric, Reference: reference(ItemResourceUsage), Reason: ReasonUsage},
			{ID: ItemStop, Label: "停止应用", Source: extension.PageSourceObject, Reference: reference(ItemStop), Action: ActionStop},
			{ID: ItemStart, Label: "启动应用", Source: extension.PageSourceObject, Reference: reference(ItemStart), Action: ActionStart},
			{ID: ItemRestart, Label: "重启应用", Source: extension.PageSourceObject, Reference: reference(ItemRestart), Action: ActionRestart},
			{ID: ItemScale, Label: "扩缩容", Source: extension.PageSourceObject, Reference: reference(ItemScale), Action: ActionScale},
			{ID: ItemResourceSpec, Label: "资源规格", Source: extension.PageSourceObject, Reference: reference(ItemResourceSpec), Action: ActionResourceSpec},
			{ID: ItemUninstall, Label: "卸载应用", Source: extension.PageSourceObject, Reference: reference(ItemUninstall), Action: ActionUninstall},
		},
		Actions: []extension.Action{
			{ID: ActionStop, Method: http.MethodPost, Path: PathActionRequests},
			{ID: ActionStart, Method: http.MethodPost, Path: PathActionRequests},
			{ID: ActionRestart, Method: http.MethodPost, Path: PathActionRequests},
			{ID: ActionScale, Method: http.MethodPost, Path: PathActionRequests},
			{ID: ActionResourceSpec, Method: http.MethodPut, Path: PathApplications + "/{name}?namespace={namespace}"},
			{ID: ActionUninstall, Method: http.MethodDelete, Path: PathApplications + "/{name}?namespace={namespace}"},
		},
	}
}
