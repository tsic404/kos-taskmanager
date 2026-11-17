package app

import (
	"net/http"
	"strings"

	"github.com/tsic404/kosd/pkg/extension"
)

// PagePods is the Pod runtime surface of one application (TASK-C-3): the Pods
// the control plane projects for it, the log read of one Pod, and the single
// Pod delete KOSD-C-77 is the only write of this page.
const PagePods = "pods"

// Item identities of the Pod page. The list and the log item are display items;
// the delete item carries the page's one modification entry.
const (
	ItemPods      = "pods"
	ItemPodLogs   = "pod-logs"
	ItemPodDelete = "pod-delete"
)

// ActionDeletePod is the page action the delete entry submits (KOSD-C-77).
const ActionDeletePod = "delete-pod"

// Control-plane paths the page consumes (KOSD-C-12): the application's Pod
// projection, its per-Pod log read and the delete of one Pod (KOSD-C-76,
// KOSD-C-77, KOSD-C-64). Every address is the control plane's own, so the
// browser never reaches the runtime and holds no cluster credential.
const (
	// PathApplicationPods is the projection read of one application's Pods.
	PathApplicationPods = "/api/v1/pods/{namespace}/{name}"
	// PathPodDelete is the delete of exactly the named Pod of one application.
	PathPodDelete = PathApplicationPods + "/{pod}"
)

// PodsReason is why the projection item offers no modification entry: the Pod
// is a runtime fact and every Pod-level write beyond the single delete stays
// out of the browser (KOSD-F-20).
const PodsReason = "Pod 投影只读：Pod 是运行内核的事实，本页不创建 Pod、不改 Pod spec，也不提供终端执行与端口转发"

// PodLogsReason is why the log item offers no modification entry: the lines are
// read when the reader asks for them and are stored nowhere (KOSD-C-64).
const PodLogsReason = "按 Pod 在读取时读取日志：窗口与行数由控制面限制，超限截断并标出截断位置；不保存日志，不提供全量下载"

// PodLogRule is what the log item states it reads by (KOSD-C-64): one Pod's
// containers, bounded by the control plane, never the application's other
// replicas and never a stored copy.
const PodLogRule = "按 Pod 在读取时读取：只含该 Pod 容器的行，同应用其他 Pod 的行不出现；时间窗与行数由控制面限制，超限截断并标出截断位置"

// PodsPage declares the Pod runtime surface TASK-C-3 names: the projection list,
// the per-Pod log read and the single-Pod delete. The delete is the page's one
// write; no item declares a Pod create, a Pod spec write, a terminal or a port
// forward (KOSD-F-20). The page exists only while this component does, so
// disabling or removing the package withdraws it with the window (KOSD-AC-54).
func PodsPage() extension.Page {
	return extension.Page{
		ID:       PagePods,
		Title:    "Pod 运行态",
		Location: "panel",
		Items: []extension.PageItem{
			{ID: ItemPods, Label: "Pod 列表", Source: extension.PageSourceObject, Reference: reference(ItemPods), Reason: PodsReason},
			{ID: ItemPodLogs, Label: "按 Pod 日志", Source: extension.PageSourceRead, Reference: reference(ItemPodLogs), Reason: PodLogsReason},
			{ID: ItemPodDelete, Label: "删除 Pod", Source: extension.PageSourceObject, Reference: reference(ItemPodDelete), Action: ActionDeletePod},
		},
		Actions: []extension.Action{
			{ID: ActionDeletePod, Method: http.MethodDelete, Path: PathPodDelete},
		},
	}
}

// applicationPodsPath is the projection read of one application: the address the
// page row names, resolved from the one template so the row and the declaration
// cannot drift (KOSD-C-76).
func applicationPodsPath(namespace, name string) string {
	resolved := strings.ReplaceAll(PathApplicationPods, "{namespace}", namespace)
	return strings.ReplaceAll(resolved, "{name}", name)
}
