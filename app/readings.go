package app

import (
	"context"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/tsic404/kosd/api"
	"github.com/tsic404/kosd/api/v1beta1"
	kosapp "github.com/tsic404/kosd/pkg/app"
	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/pages"
	"github.com/tsic404/kosd/pkg/pages/cluster"
)

// eventLimit bounds the recent events of one read; the window already decides
// what is readable (KOSD-C-64).
const eventLimit = 50

// Maintenance origins the origin item reports (KOSD-C-82): only the pair of
// writes that moved the unschedulable field decides, and an unreadable trail is
// unknown rather than guessed.
const (
	MaintenanceNone      = "none"
	MaintenancePlanned   = "planned"
	MaintenanceUnplanned = "unplanned"
	MaintenanceUnknown   = "unknown"
)

// maintenanceVerbs is the cordon pair a node's origin is judged by (KOSD-C-82).
var maintenanceVerbs = []string{controlplane.VerbMaintain, controlplane.VerbRestore}

// maintenanceResults are the outcomes that took effect; a refused attempt keeps
// its record but never moved the field (KOSD-C-14, KOSD-C-82).
var maintenanceResults = []string{controlplane.ResultAccepted}

// AuditReader reads the control-plane audit records the maintenance origin
// comes from (KOSD-C-14, KOSD-C-82). The read is scoped to one node's own
// records and to the outcomes that took effect.
type AuditReader interface {
	RecentAuditEntriesForObject(ctx context.Context, object string, verbs, results []string, limit int) ([]identity.AuditEntry, error)
}

// Readings resolves the items this product's pages declare (KOSD-C-53): every
// read goes through the control plane's cluster client and is trimmed to the
// session's visibility (KOSD-C-59, KOSD-C-64).
type Readings struct {
	// Dynamic reads the cluster.
	Dynamic dynamic.Interface
	// Audit judges a node's maintenance origin; nil leaves it unknown.
	Audit AuditReader
	// Usage reports resource samples; nil leaves the usage item stating that no
	// metric source is configured (TASK-C-2).
	Usage UsageSource
	// Now supplies the read clock; defaults to time.Now.
	Now func() time.Time
}

// Read implements pages.Readings.
func (r *Readings) Read(ctx context.Context, session identity.Session, item extension.PageItem) (pages.Reading, *controlplane.Error) {
	observedAt := r.clock()
	reader := &cluster.Reader{Dynamic: r.Dynamic, Now: func() time.Time { return observedAt }}
	value, failure := r.value(ctx, session, reader, item)
	if failure != nil {
		return pages.Reading{}, failure
	}
	return pages.Reading{Value: value, ObservedAt: observedAt}, nil
}

// value resolves one item from the source type it declares.
func (r *Readings) value(ctx context.Context, session identity.Session, reader *cluster.Reader, item extension.PageItem) (any, *controlplane.Error) {
	switch item.ID {
	case ItemNodesReady:
		return nodesReady(ctx, reader)
	case ItemAbnormalObjects:
		return abnormalObjects(ctx, reader, session)
	case ItemPendingUpgrades:
		return pendingUpgrades(ctx, reader, session)
	case ItemRecentEvents:
		return recentEvents(ctx, reader, session)
	case ItemMaintenanceNodes:
		return maintenanceNodes(ctx, reader)
	case ItemMaintenanceOrigin:
		return r.maintenanceOrigins(ctx, reader)
	case ItemMaintenanceAffected:
		return maintenanceAffected(ctx, reader, session)
	case ItemInstalled:
		return installedApplications(ctx, reader, session)
	case ItemInstallable:
		return installableApplications(ctx, reader)
	case ItemResourceUsage:
		return r.applicationUsage(ctx, session)
	case ItemStop, ItemStart, ItemRestart, ItemScale, ItemResourceSpec, ItemUninstall:
		return entryTargets(ctx, reader, session, item.ID)
	case ItemDetailHealth:
		return detailHealthItem(ctx, reader, session)
	case ItemDetailVolumes:
		return detailVolumesItem(ctx, reader, session)
	case ItemDetailIngress:
		return detailIngressItem(ctx, reader, session)
	case ItemDetailBackup:
		return detailBackupItem(ctx, reader, session)
	case ItemDetailDrift:
		return detailDriftItem(ctx, reader, session)
	case ItemDetailEvents:
		return detailEventsItem(ctx, reader, session)
	case ItemDetailLogs:
		return detailLogsItem(ctx, reader, session)
	default:
		return nil, &controlplane.Error{
			Code:        controlplane.CodeInternal,
			Message:     "页面条目 " + item.ID + " 没有来源",
			Stage:       controlplane.StageRead,
			SideEffects: controlplane.SideEffectsNone,
		}
	}
}

// clock returns the moment this read observes its values (KOSD-C-13).
func (r *Readings) clock() time.Time {
	if r.Now == nil {
		return time.Now()
	}
	return r.Now()
}

// nodeReadiness is one node's name and Ready condition.
type nodeReadiness struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}

// nodesReady reports how many nodes are ready together with each node's own
// condition (TASK-C-1).
func nodesReady(ctx context.Context, reader *cluster.Reader) (any, *controlplane.Error) {
	nodes, failure := reader.Nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	views := make([]nodeReadiness, 0, len(nodes))
	ready := 0
	for _, node := range nodes {
		isReady := nodeReady(node)
		if isReady {
			ready++
		}
		views = append(views, nodeReadiness{Name: node.Name, Ready: isReady})
	}
	sort.Slice(views, func(left, right int) bool { return views[left].Name < views[right].Name })
	return map[string]any{"ready": ready, "total": len(nodes), "nodes": views}, nil
}

// statused is the shared status shape every kernel kind carries.
type statused struct {
	Status v1beta1.ObjectStatus `json:"status"`
}

// abnormal is the rule KOSD-C-53 fixes for the computed item: an object is
// abnormal when its health is not Healthy or one of its conditions is not met.
func abnormal(status v1beta1.ObjectStatus) bool {
	if status.Health.State != "" && status.Health.State != v1beta1.HealthStateHealthy {
		return true
	}
	for _, condition := range status.Conditions {
		if condition.Status != metav1.ConditionTrue {
			return true
		}
	}
	return false
}

// abnormalObjects lists the objects the rule finds, trimmed to what the session
// may see (KOSD-C-53, KOSD-C-59).
func abnormalObjects(ctx context.Context, reader *cluster.Reader, session identity.Session) (any, *controlplane.Error) {
	objects := make([]map[string]any, 0)
	for _, kind := range cluster.StatusKinds() {
		stored, failure := reader.List(ctx, kind, "")
		if failure != nil {
			return nil, failure
		}
		for _, object := range stored {
			var shape statused
			if failure := cluster.Decode(object, &shape); failure != nil {
				return nil, failure
			}
			if !abnormal(shape.Status) {
				continue
			}
			visible, failure := reader.Visible(ctx, session, kind, object)
			if failure != nil {
				return nil, failure
			}
			if !visible {
				continue
			}
			objects = append(objects, map[string]any{
				"kind":       kind,
				"namespace":  cluster.Text(object, "metadata", "namespace"),
				"name":       cluster.Text(object, "metadata", "name"),
				"phase":      shape.Status.Phase,
				"health":     string(shape.Status.Health.State),
				"conditions": conditionsOf(shape.Status.Conditions),
			})
		}
	}
	sort.Slice(objects, func(left, right int) bool { return objectKey(objects[left]) < objectKey(objects[right]) })
	return map[string]any{
		"rule":    "健康状态不是 Healthy，或存在未满足的条件（KOSD-C-53）",
		"objects": objects,
	}, nil
}

// objectKey orders abnormal objects by kind, namespace and name.
func objectKey(object map[string]any) string {
	kind, _ := object["kind"].(string)
	namespace, _ := object["namespace"].(string)
	name, _ := object["name"].(string)
	return kind + "/" + namespace + "/" + name
}

// conditionsOf renders the conditions an object reports.
func conditionsOf(conditions []metav1.Condition) []map[string]any {
	views := make([]map[string]any, 0, len(conditions))
	for _, condition := range conditions {
		views = append(views, map[string]any{
			"type":    condition.Type,
			"status":  string(condition.Status),
			"reason":  condition.Reason,
			"message": condition.Message,
		})
	}
	return views
}

// pendingUpgrade is one application the version discovery finds behind its
// definition, or one the reconciler is already upgrading.
type pendingUpgrade struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	State     string `json:"state"`
	Installed string `json:"installed,omitempty"`
	Target    string `json:"target,omitempty"`
	Version   string `json:"version,omitempty"`
}

// pendingUpgrades compares the package a definition offers with the baseline the
// application applied, which is the version discovery of TASK-C-1.
func pendingUpgrades(ctx context.Context, reader *cluster.Reader, session identity.Session) (any, *controlplane.Error) {
	applications, failure := visibleApplications(ctx, reader, session)
	if failure != nil {
		return nil, failure
	}
	definitions, failure := definitions(ctx, reader)
	if failure != nil {
		return nil, failure
	}
	pending := make([]pendingUpgrade, 0)
	for _, application := range applications {
		entry := pendingUpgrade{Namespace: application.Namespace, Name: application.Name}
		definition, known := definitions[application.Spec.DefinitionRef]
		switch {
		case application.Status.Phase == kosapp.PhaseUpgrading:
			entry.State = "in-progress"
			if application.Status.Upgrade != nil {
				entry.Target = application.Status.Upgrade.Target
			}
		case known && behindDefinition(application, definition):
			entry.State = "definition-changed"
			if application.Status.Applied != nil {
				entry.Installed = application.Status.Applied.DefinitionDigest
			}
			entry.Target = definition.Spec.Package.Digest
			entry.Version = definition.Spec.Package.Version
		case application.Status.Upgrade != nil && application.Status.Upgrade.Target != "" &&
			!application.Status.Upgrade.RolledBack && behind(application):
			entry.State = "requested"
			entry.Target = application.Status.Upgrade.Target
		default:
			continue
		}
		pending = append(pending, entry)
	}
	return map[string]any{
		"rule":    "定义的包摘要与已应用基线不同，或应用正在升级（KOSD-C-53）",
		"pending": pending,
	}, nil
}

// behindDefinition reports a definition whose package digest differs from the
// baseline the application applied.
func behindDefinition(application v1beta1.Application, definition v1beta1.ApplicationDefinition) bool {
	if definition.Spec.Package.Digest == "" || application.Status.Applied == nil {
		return false
	}
	applied := application.Status.Applied.DefinitionDigest
	return applied != "" && applied != definition.Spec.Package.Digest
}

// behind reports a requested target the applied baseline has not reached, so a
// settled upgrade is not reported as pending.
func behind(application v1beta1.Application) bool {
	if application.Status.Applied == nil {
		return true
	}
	applied := application.Status.Applied.DefinitionDigest
	return applied == "" || applied != application.Status.Upgrade.Target
}

// definitions indexes the application definitions by name.
func definitions(ctx context.Context, reader *cluster.Reader) (map[string]v1beta1.ApplicationDefinition, *controlplane.Error) {
	objects, failure := reader.List(ctx, "ApplicationDefinition", "")
	if failure != nil {
		return nil, failure
	}
	indexed := make(map[string]v1beta1.ApplicationDefinition, len(objects))
	for _, object := range objects {
		var definition v1beta1.ApplicationDefinition
		if failure := cluster.Decode(object, &definition); failure != nil {
			return nil, failure
		}
		indexed[definition.Name] = definition
	}
	return indexed, nil
}

// recentEvents reads KOSD-C-64's window, newest first, trimmed to what the
// session may see (KOSD-AC-59).
func recentEvents(ctx context.Context, reader *cluster.Reader, session identity.Session) (any, *controlplane.Error) {
	events, failure := reader.Events(ctx, session, "", eventLimit)
	if failure != nil {
		return nil, failure
	}
	return map[string]any{"window": controlplane.EventWindow.String(), "events": events}, nil
}

// maintenanceNodes lists the nodes taken out of scheduling (KOSD-C-65).
func maintenanceNodes(ctx context.Context, reader *cluster.Reader) (any, *controlplane.Error) {
	nodes, failure := reader.Nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	views := make([]nodeReadiness, 0)
	for _, node := range nodes {
		if !node.Spec.Unschedulable {
			continue
		}
		views = append(views, nodeReadiness{Name: node.Name, Ready: nodeReady(node)})
	}
	sort.Slice(views, func(left, right int) bool { return views[left].Name < views[right].Name })
	return map[string]any{"nodes": views}, nil
}

// maintenanceOrigin is one unschedulable node's judged origin (KOSD-C-82).
type maintenanceOrigin struct {
	Name   string `json:"name"`
	Origin string `json:"origin"`
}

// maintenanceOrigins judges the origin of every node taken out of scheduling.
// A node back in scheduling has no origin at all, so its unpaired record stops
// counting (KOSD-C-82).
func (r *Readings) maintenanceOrigins(ctx context.Context, reader *cluster.Reader) (any, *controlplane.Error) {
	nodes, failure := reader.Nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	views := make([]maintenanceOrigin, 0)
	for _, node := range nodes {
		if !node.Spec.Unschedulable {
			continue
		}
		views = append(views, maintenanceOrigin{Name: node.Name, Origin: r.originOf(ctx, node)})
	}
	sort.Slice(views, func(left, right int) bool { return views[left].Name < views[right].Name })
	return map[string]any{"nodes": views}, nil
}

// originOf applies KOSD-C-82 to one node: the newest record of the node's own
// writes that took effect decides, and a trail that cannot be read is unknown.
func (r *Readings) originOf(ctx context.Context, node corev1.Node) string {
	if !node.Spec.Unschedulable {
		return MaintenanceNone
	}
	if r.Audit == nil {
		return MaintenanceUnknown
	}
	entries, err := r.Audit.RecentAuditEntriesForObject(ctx, api.Group+"/Node/"+node.Name, maintenanceVerbs, maintenanceResults, 1)
	if err != nil {
		return MaintenanceUnknown
	}
	if len(entries) == 0 {
		return MaintenanceUnplanned
	}
	if entries[0].Verb == controlplane.VerbMaintain {
		return MaintenancePlanned
	}
	return MaintenanceUnplanned
}

// nodeApplications is one node and the node-local applications maintenance
// would stop (KOSD-C-65).
type nodeApplications struct {
	Node         string   `json:"node"`
	Applications []string `json:"applications"`
}

// maintenanceAffected lists, per unschedulable node, the applications whose
// node-local volumes are bound to it, trimmed to what the session may see
// (KOSD-C-65, KOSD-C-59).
func maintenanceAffected(ctx context.Context, reader *cluster.Reader, session identity.Session) (any, *controlplane.Error) {
	nodes, failure := reader.Nodes(ctx)
	if failure != nil {
		return nil, failure
	}
	unschedulable := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		if node.Spec.Unschedulable {
			unschedulable[node.Name] = true
		}
	}
	if len(unschedulable) == 0 {
		return map[string]any{"nodes": []any{}}, nil
	}
	bound, failure := nodeLocalVolumes(ctx, reader)
	if failure != nil {
		return nil, failure
	}
	applications, failure := visibleApplications(ctx, reader, session)
	if failure != nil {
		return nil, failure
	}
	affected := make(map[string][]string, len(unschedulable))
	for _, application := range applications {
		namespace := application.Namespace
		if application.Spec.Namespace != "" {
			namespace = application.Spec.Namespace
		}
		for _, volume := range application.Spec.Volumes {
			node, known := bound[namespace+"/"+volume]
			if !known || !unschedulable[node] {
				continue
			}
			affected[node] = append(affected[node], application.Namespace+"/"+application.Name)
		}
	}
	names := make([]string, 0, len(affected))
	for name := range affected {
		names = append(names, name)
	}
	sort.Strings(names)
	views := make([]nodeApplications, 0, len(names))
	for _, name := range names {
		sort.Strings(affected[name])
		views = append(views, nodeApplications{Node: name, Applications: affected[name]})
	}
	return map[string]any{"nodes": views}, nil
}

// nodeLocalVolumes maps the namespace-qualified name of a node-local volume to
// the node it is bound to (KOSD-C-38).
func nodeLocalVolumes(ctx context.Context, reader *cluster.Reader) (map[string]string, *controlplane.Error) {
	objects, failure := reader.List(ctx, "Volume", "")
	if failure != nil {
		return nil, failure
	}
	bound := make(map[string]string, len(objects))
	for _, object := range objects {
		var volume v1beta1.Volume
		if failure := cluster.Decode(object, &volume); failure != nil {
			return nil, failure
		}
		if volume.Spec.Mobility != v1beta1.VolumeMobilityNodeLocal || volume.Status.Node == "" {
			continue
		}
		bound[volume.Namespace+"/"+volume.Name] = volume.Status.Node
	}
	return bound, nil
}

// visibleApplications lists the applications the session may see, ordered by
// namespace and name (KOSD-C-59).
func visibleApplications(ctx context.Context, reader *cluster.Reader, session identity.Session) ([]v1beta1.Application, *controlplane.Error) {
	objects, failure := reader.List(ctx, "Application", "")
	if failure != nil {
		return nil, failure
	}
	applications := make([]v1beta1.Application, 0, len(objects))
	for _, object := range objects {
		var application v1beta1.Application
		if failure := cluster.Decode(object, &application); failure != nil {
			return nil, failure
		}
		visible, failure := reader.Visible(ctx, session, "Application", object)
		if failure != nil {
			return nil, failure
		}
		if !visible {
			continue
		}
		applications = append(applications, application)
	}
	sort.Slice(applications, func(left, right int) bool {
		if applications[left].Namespace != applications[right].Namespace {
			return applications[left].Namespace < applications[right].Namespace
		}
		return applications[left].Name < applications[right].Name
	})
	return applications, nil
}

// nodeReady reports whether a node carries the Ready condition (KOSD-C-65).
func nodeReady(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
