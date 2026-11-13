package app

import "github.com/tsic404/kosd/pkg/extension"

// PageOverview is the overview surface the run view opens on (TASK-C-1).
const PageOverview = "overview"

// Item identities of the overview page, one per source type it displays
// (KOSD-C-53). The maintenance group carries three: the node fact, the origin
// the audit trail decides, and the applications a node-local volume binds.
const (
	ItemNodesReady          = "nodes-ready"
	ItemAbnormalObjects     = "abnormal-objects"
	ItemPendingUpgrades     = "pending-upgrades"
	ItemRecentEvents        = "recent-events"
	ItemMaintenanceNodes    = "maintenance-nodes"
	ItemMaintenanceOrigin   = "maintenance-origin"
	ItemMaintenanceAffected = "maintenance-affected-applications"
)

// OverviewReason is why the overview offers no modification entry (TASK-C-1):
// every write it could show belongs to the application page or to settings.
const OverviewReason = "本页只读：节点、事件与升级状态由运行内核与版本发现持有，修改经应用页与系统设置发起"

// OverviewPage declares the overview items TASK-C-1 names. Each item is read
// from the one source type it declares, so a page read cannot present an
// aggregate as object state or an object as a computed value.
func OverviewPage() extension.Page {
	return extension.Page{
		ID:             PageOverview,
		Title:          "概览",
		Location:       "panel",
		ReadOnly:       true,
		ReadOnlyReason: OverviewReason,
		Items: []extension.PageItem{
			{ID: ItemNodesReady, Label: "节点就绪", Source: extension.PageSourceObject, Reference: reference(ItemNodesReady)},
			{ID: ItemAbnormalObjects, Label: "异常对象", Source: extension.PageSourceComputed, Reference: reference(ItemAbnormalObjects)},
			{ID: ItemPendingUpgrades, Label: "待处理升级", Source: extension.PageSourceComputed, Reference: reference(ItemPendingUpgrades)},
			{ID: ItemRecentEvents, Label: "最近事件", Source: extension.PageSourceObject, Reference: reference(ItemRecentEvents)},
			{ID: ItemMaintenanceNodes, Label: "维护中的节点", Source: extension.PageSourceObject, Reference: reference(ItemMaintenanceNodes)},
			{ID: ItemMaintenanceOrigin, Label: "维护归属", Source: extension.PageSourceDatabase, Reference: reference(ItemMaintenanceOrigin)},
			{ID: ItemMaintenanceAffected, Label: "维护受影响的 node-local 应用", Source: extension.PageSourceComputed, Reference: reference(ItemMaintenanceAffected)},
		},
	}
}

// reference is the item reference this product owns; the shell dispatches an
// item to the reader of the product in front of it (KOSD-C-18).
func reference(item string) string { return Identifier + "/" + item }
