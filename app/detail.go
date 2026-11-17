package app

import (
	"strings"

	"github.com/tsic404/kosd/pkg/extension"
)

// PageDetail is the application detail surface (TASK-C-1, DASH-C-52): one
// application's health, volumes, endpoints, backup policies, drift and events,
// read off the objects that carry them, plus its logs, read when the page reads
// them.
const PageDetail = "detail"

// Item identities of the detail page. The first six are the object-source items
// DASH-C-52 lists; the last is the read-source log item it hands to KOSD-C-64.
// Every item resolves for each application the session may see and names that
// application by namespace and name, because a page read is not scoped to one
// object (KOSD-C-53, KOSD-C-59).
const (
	ItemDetailHealth  = "detail-health"
	ItemDetailVolumes = "detail-volumes"
	ItemDetailIngress = "detail-ingress"
	ItemDetailBackup  = "detail-backup-policy"
	ItemDetailDrift   = "detail-drift"
	ItemDetailEvents  = "detail-events"
	ItemDetailLogs    = "detail-logs"
)

// DetailReason is why the detail page offers no modification entry: every item
// it displays is the projection of an object another surface owns, and a log
// read changes nothing (DASH-C-52, KOSD-C-64). The page that owns each write is
// the application page.
const DetailReason = "本页只读：健康、卷、入口、备份、漂移与事件读自应用与关联对象的投影，日志在读取时按应用读取；修改经应用页与对应产品发起"

// PathApplicationLogs is the control-plane read one application's logs come from
// (KOSD-C-64). The log item names it per application so the window reads the
// application's own lines with its own session; the page projection never
// carries lines.
const PathApplicationLogs = "/api/v1/logs/{namespace}/{name}"

// LogRule is what the log item states it reads by: the bounds, the truncation
// mark and the absence of a stored copy are the control plane's log read, so the
// page states them once instead of repeating numeric bounds (KOSD-C-64).
const LogRule = "按应用在读取时读取：时间窗与行数由控制面限制，超限截断并标出截断位置；不保存日志，不提供全量下载"

// DetailPage declares the application detail surface DASH-C-52 names. It is
// read-only: the six object items read the application and the objects bound to
// it, the log item reads at read time, and no item carries a modification entry.
// The page exists only while this component does, so disabling or removing the
// package withdraws it together with the window's entry (KOSD-AC-54).
func DetailPage() extension.Page {
	return extension.Page{
		ID:             PageDetail,
		Title:          "应用详情",
		Location:       "panel",
		ReadOnly:       true,
		ReadOnlyReason: DetailReason,
		Items: []extension.PageItem{
			{ID: ItemDetailHealth, Label: "健康", Source: extension.PageSourceObject, Reference: reference(ItemDetailHealth)},
			{ID: ItemDetailVolumes, Label: "卷", Source: extension.PageSourceObject, Reference: reference(ItemDetailVolumes)},
			{ID: ItemDetailIngress, Label: "入口", Source: extension.PageSourceObject, Reference: reference(ItemDetailIngress)},
			{ID: ItemDetailBackup, Label: "备份", Source: extension.PageSourceObject, Reference: reference(ItemDetailBackup)},
			{ID: ItemDetailDrift, Label: "漂移", Source: extension.PageSourceObject, Reference: reference(ItemDetailDrift)},
			{ID: ItemDetailEvents, Label: "事件", Source: extension.PageSourceObject, Reference: reference(ItemDetailEvents)},
			{ID: ItemDetailLogs, Label: "日志", Source: extension.PageSourceRead, Reference: reference(ItemDetailLogs)},
		},
	}
}

// applicationLogPath is the control-plane log read of one application, resolved
// from the one template the item names so the row and the declaration cannot
// drift (KOSD-C-64).
func applicationLogPath(namespace, name string) string {
	resolved := strings.ReplaceAll(PathApplicationLogs, "{namespace}", namespace)
	return strings.ReplaceAll(resolved, "{name}", name)
}
