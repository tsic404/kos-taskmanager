package app

import (
	"context"
	"sort"

	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/app"
	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/pages/cluster"
	"github.com/tsic404/kosd/pkg/write"
)

// UsageUnavailableReason is what the resource-usage item states when the
// distribution wired no metric source (TASK-C-2). Usage is then unmeasurable:
// the item carries no number at any level, and zero is never presented as a
// measurement.
const UsageUnavailableReason = "发行版没有接入资源指标来源，KOS 不采集工作负载用量"

// UsageUnreadableReason is what the item states when the source is configured
// but does not answer. The value is unmeasurable for that read, not zero.
const UsageUnreadableReason = "资源指标读取失败，应用用量暂不可读"

// UsageSource reports one sampled consumption row per application the session
// may see (TASK-C-2, KOSD-C-53). A distribution without a metric source leaves
// it nil and the item states that instead of inventing numbers.
type UsageSource interface {
	// Usage returns one sample per application the session may see.
	Usage(ctx context.Context, session identity.Session) ([]ApplicationUsage, error)
}

// ApplicationUsage is one application's sampled consumption.
type ApplicationUsage struct {
	// Namespace is the application's namespace.
	Namespace string `json:"namespace"`
	// Name is the application name.
	Name string `json:"name"`
	// CPU is the sampled CPU usage as a quantity.
	CPU string `json:"cpu"`
	// Memory is the sampled memory usage as a quantity.
	Memory string `json:"memory"`
}

// installedApplication is one row of the installed set: the facts the page
// reads off the application object (KOSD-C-33, KOSD-C-66).
type installedApplication struct {
	Namespace  string                   `json:"namespace"`
	Name       string                   `json:"name"`
	Owner      string                   `json:"owner,omitempty"`
	Phase      string                   `json:"phase"`
	Health     string                   `json:"health"`
	Conditions []map[string]any         `json:"conditions"`
	Paused     bool                     `json:"paused"`
	Replicas   *int32                   `json:"replicas,omitempty"`
	Uninstall  *v1beta1.UninstallStatus `json:"uninstall,omitempty"`
}

// installedApplications reads the application objects the session may see. The
// page shows the object's own phase, health and conditions, so the result of an
// accepted action is read from the object instead of a task record
// (KOSD-AC-61, KOSD-C-59).
func installedApplications(ctx context.Context, reader *cluster.Reader, session identity.Session) (any, *controlplane.Error) {
	applications, failure := visibleApplications(ctx, reader, session)
	if failure != nil {
		return nil, failure
	}
	rows := make([]installedApplication, 0, len(applications))
	for _, application := range applications {
		rows = append(rows, installedApplication{
			Namespace:  application.Namespace,
			Name:       application.Name,
			Owner:      application.Spec.Owner,
			Phase:      application.Status.Phase,
			Health:     string(application.Status.Health.State),
			Conditions: conditionsOf(application.Status.Conditions),
			Paused:     application.Spec.Paused,
			Replicas:   application.Spec.Replicas,
			Uninstall:  application.Status.Uninstall,
		})
	}
	return map[string]any{"applications": rows}, nil
}

// installableEntry is one catalogue entry of the installable list (APPSTORE-C-31).
type installableEntry struct {
	Name        string        `json:"name"`
	Version     string        `json:"version,omitempty"`
	Installable bool          `json:"installable"`
	Gaps        []cluster.Gap `json:"gaps,omitempty"`
	Invalid     string        `json:"invalid,omitempty"`
}

// installableApplications lists the definitions the catalogue offers. The list
// is computed, not an application list: entries carry no visibility, and an
// entry whose required capabilities the providers do not serve is not
// installable and names the gaps (APPSTORE-C-31, KOSD-C-53).
func installableApplications(ctx context.Context, reader *cluster.Reader) (any, *controlplane.Error) {
	objects, failure := reader.List(ctx, "ApplicationDefinition", "")
	if failure != nil {
		return nil, failure
	}
	providers, failure := providers(ctx, reader)
	if failure != nil {
		return nil, failure
	}
	entries := make([]installableEntry, 0, len(objects))
	for _, object := range objects {
		var definition v1beta1.ApplicationDefinition
		if failure := cluster.Decode(object, &definition); failure != nil {
			return nil, failure
		}
		entry := installableEntry{
			Name:    definition.Name,
			Version: definition.Spec.Package.Version,
		}
		result, err := cluster.Compare(definition.Spec.Requires, providers)
		if err != nil {
			entry.Invalid = "能力需求无法判定：" + err.Error()
		} else {
			entry.Installable = len(result.Missing) == 0
			entry.Gaps = cluster.Gaps(result)
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name < entries[right].Name })
	return map[string]any{
		"rule":    "定义存在且必需能力需求已被已验证的 Provider 满足（KOSD-C-53）",
		"entries": entries,
	}, nil
}

// providers lists the installed providers the capability comparison reads.
func providers(ctx context.Context, reader *cluster.Reader) ([]v1beta1.Provider, *controlplane.Error) {
	objects, failure := reader.List(ctx, "Provider", "")
	if failure != nil {
		return nil, failure
	}
	installed := make([]v1beta1.Provider, 0, len(objects))
	for _, object := range objects {
		var provider v1beta1.Provider
		if failure := cluster.Decode(object, &provider); failure != nil {
			return nil, failure
		}
		installed = append(installed, provider)
	}
	return installed, nil
}

// applicationUsage reports the sampled consumption of the metric source
// (TASK-C-2). Without a source, or when the source does not answer, the item is
// unmeasurable and states why; it never falls back to zero usage.
func (r *Readings) applicationUsage(ctx context.Context, session identity.Session) (any, *controlplane.Error) {
	if r.Usage == nil {
		return map[string]any{"available": false, "reason": UsageUnavailableReason}, nil
	}
	samples, err := r.Usage.Usage(ctx, session)
	if err != nil {
		return map[string]any{"available": false, "reason": UsageUnreadableReason}, nil
	}
	if samples == nil {
		samples = []ApplicationUsage{}
	}
	return map[string]any{"available": true, "applications": samples}, nil
}

// entryTarget is one application a modification entry applies to, together with
// the entry's own values the form starts from (KOSD-AC-61). A session that may
// not write the application never receives a target for it (KOSD-C-59).
type entryTarget struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Phase     string `json:"phase"`
	Paused    bool   `json:"paused"`
	// Replicas is the count the scale entry starts from (KOSD-C-66).
	Replicas *int32 `json:"replicas,omitempty"`
	// Resources is the specification the resource entry overwrites (KOSD-C-67).
	Resources *v1beta1.ResourceRequirements `json:"resources,omitempty"`
	// Unlimited marks an application KOS has not constrained (KOSD-C-67).
	Unlimited bool `json:"unlimited,omitempty"`
	// LimitRange names the namespace LimitRange that still applies (KOSD-C-67).
	LimitRange string `json:"limitRange,omitempty"`
	// ResourceVersion is the precondition the uninstall entry submits (KOSD-C-12).
	ResourceVersion string `json:"resourceVersion,omitempty"`
}

// entryTargets names the applications one modification entry applies to, using
// the role and ownership rule of KOSD-C-59 and the acceptance rule of the
// control plane (KOSD-C-66). The control plane remains the judge of the write;
// this only decides which rows the entry is offered for.
func entryTargets(ctx context.Context, reader *cluster.Reader, session identity.Session, item string) (any, *controlplane.Error) {
	applications, failure := visibleApplications(ctx, reader, session)
	if failure != nil {
		return nil, failure
	}
	targets := make([]entryTarget, 0, len(applications))
	for _, application := range applications {
		if !mayWriteApplication(session, application) || !actionable(item, application) {
			continue
		}
		targets = append(targets, targetOf(application))
	}
	return map[string]any{"applications": targets}, nil
}

// mayWriteApplication applies KOSD-C-59 to one application: the global Owner
// writes every application, a Member only the applications it owns, and a Guest
// none. An application without an owner belongs to the global Owner alone
// (KOSD-C-60).
func mayWriteApplication(session identity.Session, application v1beta1.Application) bool {
	switch session.Role {
	case write.RoleOwner:
		return true
	case write.RoleMember:
		return application.Spec.Owner != "" && application.Spec.Owner == session.Username
	default:
		return false
	}
}

// actionable reports whether one entry can change an application now: an
// application is not uninstalling, and stop, start and restart follow the state
// the control plane accepts them in (KOSD-C-33, KOSD-C-66).
func actionable(item string, application v1beta1.Application) bool {
	if application.DeletionTimestamp != nil || application.Status.Phase == app.PhaseUninstalling {
		return false
	}
	switch item {
	case ItemStop:
		return !application.Spec.Paused
	case ItemStart:
		return application.Spec.Paused
	case ItemRestart:
		return application.Status.Phase == app.PhaseReady || application.Status.Phase == app.PhaseUpgrading
	default:
		return true
	}
}

// targetOf renders one application for an entry. The resource values come from
// the spec the entry writes; the unrestricted marker and the LimitRange name
// come from the projection the reconciler writes (KOSD-C-67).
func targetOf(application v1beta1.Application) entryTarget {
	target := entryTarget{
		Namespace:       application.Namespace,
		Name:            application.Name,
		Phase:           application.Status.Phase,
		Paused:          application.Spec.Paused,
		Replicas:        application.Spec.Replicas,
		Resources:       application.Spec.Resources,
		ResourceVersion: application.ResourceVersion,
	}
	if application.Spec.Resources == nil {
		target.Unlimited = true
	}
	if application.Status.Resources != nil {
		target.Unlimited = application.Status.Resources.Unlimited
		target.LimitRange = application.Status.Resources.LimitRange
	}
	return target
}
