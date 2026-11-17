package app

import (
	"context"
	"sort"
	"time"

	"github.com/tsic404/kosd/api"
	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/pages/cluster"
	"k8s.io/apimachinery/pkg/labels"
)

// detailEventLimit bounds the events one application's row carries. The window
// already decides what is readable; the bound only keeps one row from repeating
// a noisy cluster (KOSD-C-64).
const detailEventLimit = 20

// detailRow names the application a row belongs to. A page read is not scoped
// to one object, so every detail item resolves for each application the session
// may see and each row states which application its facts are about
// (KOSD-C-53, KOSD-C-59).
type detailRow struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// detailHealth is one application's own health facts (DASH-C-52): the phase,
// the availability axis, the conditions that explain a failure, and the pause
// and replica values the object carries.
type detailHealth struct {
	detailRow
	Owner      string           `json:"owner,omitempty"`
	Phase      string           `json:"phase"`
	Health     string           `json:"health"`
	Conditions []map[string]any `json:"conditions"`
	Paused     bool             `json:"paused"`
	Replicas   *int32           `json:"replicas,omitempty"`
}

// detailVolume is one volume object an application declares. A declaration the
// cluster cannot resolve stays visible as unresolved instead of disappearing,
// so the page never presents a declared volume as absent (KOSD-C-40).
type detailVolume struct {
	Name         string `json:"name"`
	Resolved     bool   `json:"resolved"`
	Phase        string `json:"phase,omitempty"`
	Health       string `json:"health,omitempty"`
	Mobility     string `json:"mobility,omitempty"`
	Capacity     string `json:"capacity,omitempty"`
	StorageClass string `json:"storageClass,omitempty"`
	Node         string `json:"node,omitempty"`
	BoundClaim   string `json:"boundClaim,omitempty"`
	Provider     string `json:"provider,omitempty"`
}

// detailVolumes is one application and the volumes it declares.
type detailVolumes struct {
	detailRow
	Volumes []detailVolume `json:"volumes"`
}

// detailIngress is one endpoint that exposes an application (KOSD-C-43).
type detailIngress struct {
	Name        string                       `json:"name"`
	Kind        string                       `json:"kind"`
	Host        string                       `json:"host,omitempty"`
	Path        string                       `json:"path,omitempty"`
	Port        int32                        `json:"port,omitempty"`
	Phase       string                       `json:"phase"`
	Health      string                       `json:"health"`
	Certificate *v1beta1.EndpointCertificate `json:"certificate,omitempty"`
}

// detailIngresses is one application and the endpoints whose target it is.
type detailIngresses struct {
	detailRow
	Endpoints []detailIngress `json:"endpoints"`
}

// detailBackup is one backup policy whose selector covers an application
// (KOSD-C-41).
type detailBackup struct {
	Name        string `json:"name"`
	Selector    string `json:"selector,omitempty"`
	Schedule    string `json:"schedule,omitempty"`
	Consistency string `json:"consistency,omitempty"`
	Retention   int32  `json:"retention,omitempty"`
	Drill       bool   `json:"drill,omitempty"`
	Phase       string `json:"phase"`
	Health      string `json:"health"`
	Invalid     string `json:"invalid,omitempty"`
}

// detailBackups is one application and the policies that cover it.
type detailBackups struct {
	detailRow
	Policies []detailBackup `json:"policies"`
}

// detailDrift is one application's external-change record (KOSD-C-35).
type detailDrift struct {
	detailRow
	Policy  string               `json:"policy,omitempty"`
	Entries []v1beta1.DriftEntry `json:"entries"`
}

// detailEvent is one event of the window whose subject is the application.
type detailEvent struct {
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	Reason   string    `json:"reason"`
	Message  string    `json:"message"`
	Count    int64     `json:"count"`
	LastSeen time.Time `json:"lastSeen"`
}

// detailEvents is one application and the events about it, newest first.
type detailEvents struct {
	detailRow
	Events []detailEvent `json:"events"`
}

// detailLogs is the log read one application's row names. The row carries the
// read's address, not its lines: lines are read at read time through that
// address and never enter the page projection (KOSD-C-64).
type detailLogs struct {
	detailRow
	Path string `json:"path"`
}

// detailApplications reads the applications the session may see, which every
// detail item is keyed by (KOSD-C-59).
func detailApplications(ctx context.Context, reader *cluster.Reader, session identity.Session) ([]v1beta1.Application, *controlplane.Error) {
	return visibleApplications(ctx, reader, session)
}

// detailHealthOf renders one application's health row.
func detailHealthOf(application v1beta1.Application) detailHealth {
	return detailHealth{
		detailRow:  detailRow{Namespace: application.Namespace, Name: application.Name},
		Owner:      application.Spec.Owner,
		Phase:      application.Status.Phase,
		Health:     string(application.Status.Health.State),
		Conditions: conditionsOf(application.Status.Conditions),
		Paused:     application.Spec.Paused,
		Replicas:   application.Spec.Replicas,
	}
}

// detailHealthItem resolves the health item: each visible application's own
// phase, health and conditions (DASH-C-52).
func detailHealthItem(ctx context.Context, reader *cluster.Reader, session identity.Session) (any, *controlplane.Error) {
	applications, failure := detailApplications(ctx, reader, session)
	if failure != nil {
		return nil, failure
	}
	rows := make([]detailHealth, 0, len(applications))
	for _, application := range applications {
		rows = append(rows, detailHealthOf(application))
	}
	return map[string]any{"applications": rows}, nil
}

// detailVolumesItem resolves the volume item: per application, the volume
// objects its spec declares, each with the object's own state (KOSD-C-40).
func detailVolumesItem(ctx context.Context, reader *cluster.Reader, session identity.Session) (any, *controlplane.Error) {
	applications, failure := detailApplications(ctx, reader, session)
	if failure != nil {
		return nil, failure
	}
	volumes, failure := volumesByKey(ctx, reader)
	if failure != nil {
		return nil, failure
	}
	rows := make([]detailVolumes, 0, len(applications))
	for _, application := range applications {
		row := detailVolumes{detailRow: detailRow{Namespace: application.Namespace, Name: application.Name},
			Volumes: make([]detailVolume, 0, len(application.Spec.Volumes))}
		for _, reference := range application.Spec.Volumes {
			volume, known := volumes[application.Namespace+"/"+reference]
			if !known {
				row.Volumes = append(row.Volumes, detailVolume{Name: reference})
				continue
			}
			row.Volumes = append(row.Volumes, detailVolumeOf(volume))
		}
		sort.Slice(row.Volumes, func(left, right int) bool { return row.Volumes[left].Name < row.Volumes[right].Name })
		rows = append(rows, row)
	}
	return map[string]any{"applications": rows}, nil
}

// volumesByKey indexes the cluster's volume objects by namespace and name.
func volumesByKey(ctx context.Context, reader *cluster.Reader) (map[string]v1beta1.Volume, *controlplane.Error) {
	objects, failure := reader.List(ctx, "Volume", "")
	if failure != nil {
		return nil, failure
	}
	indexed := make(map[string]v1beta1.Volume, len(objects))
	for _, object := range objects {
		var volume v1beta1.Volume
		if failure := cluster.Decode(object, &volume); failure != nil {
			return nil, failure
		}
		indexed[volume.Namespace+"/"+volume.Name] = volume
	}
	return indexed, nil
}

// detailVolumeOf renders one resolved volume's own facts.
func detailVolumeOf(volume v1beta1.Volume) detailVolume {
	return detailVolume{
		Name:         volume.Name,
		Resolved:     true,
		Phase:        volume.Status.Phase,
		Health:       string(volume.Status.Health.State),
		Mobility:     string(volume.Spec.Mobility),
		Capacity:     volume.Spec.Capacity,
		StorageClass: volume.Spec.StorageClass,
		Node:         volume.Status.Node,
		BoundClaim:   volume.Status.BoundClaim,
		Provider:     volume.Status.Provider.Name,
	}
}

// detailIngressItem resolves the endpoint item: per application, the endpoints
// whose target names it in the same namespace (KOSD-C-43, KOSD-C-45). An
// endpoint of another application never appears on this row.
func detailIngressItem(ctx context.Context, reader *cluster.Reader, session identity.Session) (any, *controlplane.Error) {
	applications, failure := detailApplications(ctx, reader, session)
	if failure != nil {
		return nil, failure
	}
	endpoints, failure := endpointsByTarget(ctx, reader)
	if failure != nil {
		return nil, failure
	}
	rows := make([]detailIngresses, 0, len(applications))
	for _, application := range applications {
		row := detailIngresses{detailRow: detailRow{Namespace: application.Namespace, Name: application.Name},
			Endpoints: endpoints[application.Namespace+"/"+application.Name]}
		if row.Endpoints == nil {
			row.Endpoints = []detailIngress{}
		}
		rows = append(rows, row)
	}
	return map[string]any{"applications": rows}, nil
}

// endpointsByTarget indexes the endpoints an application is the target of, by
// the target application's namespace and name.
func endpointsByTarget(ctx context.Context, reader *cluster.Reader) (map[string][]detailIngress, *controlplane.Error) {
	objects, failure := reader.List(ctx, "Endpoint", "")
	if failure != nil {
		return nil, failure
	}
	indexed := make(map[string][]detailIngress, len(objects))
	for _, object := range objects {
		var endpoint v1beta1.Endpoint
		if failure := cluster.Decode(object, &endpoint); failure != nil {
			return nil, failure
		}
		indexed[endpoint.Namespace+"/"+endpoint.Spec.TargetApp] = append(
			indexed[endpoint.Namespace+"/"+endpoint.Spec.TargetApp], detailIngressOf(endpoint))
	}
	for key := range indexed {
		sort.Slice(indexed[key], func(left, right int) bool { return indexed[key][left].Name < indexed[key][right].Name })
	}
	return indexed, nil
}

// detailIngressOf renders one endpoint's own facts.
func detailIngressOf(endpoint v1beta1.Endpoint) detailIngress {
	return detailIngress{
		Name:        endpoint.Name,
		Kind:        string(endpoint.Spec.Kind),
		Host:        endpoint.Spec.Host,
		Path:        endpoint.Spec.Path,
		Port:        endpoint.Spec.Port,
		Phase:       endpoint.Status.Phase,
		Health:      string(endpoint.Status.Health.State),
		Certificate: endpoint.Status.Certificate,
	}
}

// detailBackupItem resolves the backup item: per application, the policies whose
// selector covers it (KOSD-C-41). A selector the rule cannot parse is reported
// with its reason rather than silently matching nothing.
func detailBackupItem(ctx context.Context, reader *cluster.Reader, session identity.Session) (any, *controlplane.Error) {
	applications, failure := detailApplications(ctx, reader, session)
	if failure != nil {
		return nil, failure
	}
	policies, failure := policiesByNamespace(ctx, reader)
	if failure != nil {
		return nil, failure
	}
	rows := make([]detailBackups, 0, len(applications))
	for _, application := range applications {
		row := detailBackups{detailRow: detailRow{Namespace: application.Namespace, Name: application.Name},
			Policies: make([]detailBackup, 0)}
		for _, policy := range policies[application.Namespace] {
			covered, invalid := covers(policy, application)
			if !covered && invalid == "" {
				continue
			}
			entry := detailBackup{
				Name: policy.Name, Selector: policy.Spec.Selector, Schedule: policy.Spec.Schedule,
				Consistency: string(policy.Spec.Consistency), Retention: policy.Spec.Retention.Keep,
				Drill: policy.Spec.Drill.Enabled, Phase: policy.Status.Phase,
				Health: string(policy.Status.Health.State), Invalid: invalid,
			}
			row.Policies = append(row.Policies, entry)
		}
		sort.Slice(row.Policies, func(left, right int) bool { return row.Policies[left].Name < row.Policies[right].Name })
		rows = append(rows, row)
	}
	return map[string]any{"applications": rows}, nil
}

// policiesByNamespace indexes the cluster's backup policies by namespace.
func policiesByNamespace(ctx context.Context, reader *cluster.Reader) (map[string][]v1beta1.BackupPolicy, *controlplane.Error) {
	objects, failure := reader.List(ctx, "BackupPolicy", "")
	if failure != nil {
		return nil, failure
	}
	indexed := make(map[string][]v1beta1.BackupPolicy, len(objects))
	for _, object := range objects {
		var policy v1beta1.BackupPolicy
		if failure := cluster.Decode(object, &policy); failure != nil {
			return nil, failure
		}
		indexed[policy.Namespace] = append(indexed[policy.Namespace], policy)
	}
	return indexed, nil
}

// covers applies KOSD-C-41's binding to one policy and one application: the
// selector decides the range, and an unparseable selector is a reported reason
// rather than a match.
func covers(policy v1beta1.BackupPolicy, application v1beta1.Application) (bool, string) {
	selector, err := labels.Parse(policy.Spec.Selector)
	if err != nil {
		return false, "选择器无法解析：" + err.Error()
	}
	return selector.Matches(labels.Set(application.Labels)), ""
}

// detailDriftItem resolves the drift item: per application, the downstream
// values an external write changed, with the policy the application declared
// (KOSD-C-35).
func detailDriftItem(ctx context.Context, reader *cluster.Reader, session identity.Session) (any, *controlplane.Error) {
	applications, failure := detailApplications(ctx, reader, session)
	if failure != nil {
		return nil, failure
	}
	rows := make([]detailDrift, 0, len(applications))
	for _, application := range applications {
		entries := application.Status.Drift
		if entries == nil {
			entries = []v1beta1.DriftEntry{}
		}
		rows = append(rows, detailDrift{
			detailRow: detailRow{Namespace: application.Namespace, Name: application.Name},
			Policy:    string(application.Spec.DriftPolicy),
			Entries:   entries,
		})
	}
	return map[string]any{"applications": rows}, nil
}

// detailEventsItem resolves the event item: the events of KOSD-C-64's window,
// newest first, each grouped under the application it is about. The read goes
// through the control plane's one window implementation and applies its
// visibility rule before a row sees an event (KOSD-AC-59).
func detailEventsItem(ctx context.Context, reader *cluster.Reader, session identity.Session) (any, *controlplane.Error) {
	applications, failure := detailApplications(ctx, reader, session)
	if failure != nil {
		return nil, failure
	}
	events, failure := reader.Events(ctx, session, "", 0)
	if failure != nil {
		return nil, failure
	}
	rows := make([]detailEvents, 0, len(applications))
	for _, application := range applications {
		row := detailEvents{detailRow: detailRow{Namespace: application.Namespace, Name: application.Name},
			Events: make([]detailEvent, 0)}
		filter := controlplane.EventFilter{Ref: controlplane.ObjectRef{
			Group: api.Group, Kind: "Application", Namespace: application.Namespace, Name: application.Name,
		}}
		for _, event := range events {
			if !filter.Matches(event) || len(row.Events) >= detailEventLimit {
				continue
			}
			row.Events = append(row.Events, detailEvent{
				Name: event.Name, Type: event.Type, Reason: event.Reason, Message: event.Message,
				Count: event.Count, LastSeen: event.LastSeen.UTC(),
			})
		}
		rows = append(rows, row)
	}
	return map[string]any{"window": controlplane.EventWindow.String(), "applications": rows}, nil
}

// detailLogsItem resolves the log item: per application, the control-plane read
// its lines come from. The row states the address and nothing else, because a
// page read covers every visible application while a log read is one
// application's own (KOSD-C-64).
func detailLogsItem(ctx context.Context, reader *cluster.Reader, session identity.Session) (any, *controlplane.Error) {
	applications, failure := detailApplications(ctx, reader, session)
	if failure != nil {
		return nil, failure
	}
	rows := make([]detailLogs, 0, len(applications))
	for _, application := range applications {
		rows = append(rows, detailLogs{
			detailRow: detailRow{Namespace: application.Namespace, Name: application.Name},
			Path:      applicationLogPath(application.Namespace, application.Name),
		})
	}
	return map[string]any{"rule": LogRule, "applications": rows}, nil
}
