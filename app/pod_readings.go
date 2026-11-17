package app

import (
	"context"

	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/app"
	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/pages/cluster"
)

// PodsStoppedReason is why a stopped application's Pod list is empty: a stopped
// application projects no Pods (KOSD-C-76), so the empty list is an answer and
// this states it instead of leaving the reader to guess (TASK-C-3).
const PodsStoppedReason = "应用已停止：停止的应用没有 Pod（KOSD-C-76）"

// podRow is one application the Pod page can open. The row names the
// application's own projection read rather than carrying the Pods themselves:
// the projection is the control-plane API of KOSD-C-76, so the window consumes
// that answer instead of a second rendering of the same facts.
type podRow struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Paused    bool   `json:"paused"`
	Path      string `json:"path"`
	Reason    string `json:"reason,omitempty"`
}

// podLogsRow is one application and the log read its Pods are narrowed through
// (KOSD-C-64). The read is per application; the window adds the Pod it narrows
// to, so the address stays the control plane's one route.
type podLogsRow struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Path      string `json:"path"`
}

// podDeleteTarget is one application whose Pods this session may delete
// (KOSD-C-77, KOSD-C-59). The entry's path names the Pod, so the page offers the
// delete only for rows of an application this list carries.
type podDeleteTarget struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// podProjectionItem resolves the Pod list item: per application the session may
// see, the address of its own KOSD-C-76 projection and the stop marker that
// explains an empty list. A page read is not scoped to one object, so each row
// names the application it belongs to (KOSD-C-53, KOSD-C-59).
func podProjectionItem(ctx context.Context, reader *cluster.Reader, session identity.Session) (any, *controlplane.Error) {
	applications, failure := visibleApplications(ctx, reader, session)
	if failure != nil {
		return nil, failure
	}
	rows := make([]podRow, 0, len(applications))
	for _, application := range applications {
		row := podRow{
			Namespace: application.Namespace,
			Name:      application.Name,
			Paused:    application.Spec.Paused,
			Path:      applicationPodsPath(application.Namespace, application.Name),
		}
		if application.Spec.Paused {
			row.Reason = PodsStoppedReason
		}
		rows = append(rows, row)
	}
	return map[string]any{"applications": rows}, nil
}

// podLogsItem resolves the log item: per application the session may see, the
// control-plane log read its Pods are narrowed through. The row carries the
// address and nothing else, because lines are read at read time (KOSD-C-64).
func podLogsItem(ctx context.Context, reader *cluster.Reader, session identity.Session) (any, *controlplane.Error) {
	applications, failure := visibleApplications(ctx, reader, session)
	if failure != nil {
		return nil, failure
	}
	rows := make([]podLogsRow, 0, len(applications))
	for _, application := range applications {
		rows = append(rows, podLogsRow{
			Namespace: application.Namespace,
			Name:      application.Name,
			Path:      applicationLogPath(application.Namespace, application.Name),
		})
	}
	return map[string]any{"rule": PodLogRule, "applications": rows}, nil
}

// podDeleteItem resolves the delete item: the applications whose Pods this
// session may delete. The role and ownership rule of KOSD-C-59 judges it — the
// global Owner writes every application, a Member only the ones it owns, and a
// Guest none — so an identity without that right never receives a target. A
// stopped or uninstalling application projects no Pods, so it is no target
// either (KOSD-C-76, KOSD-C-33).
func podDeleteItem(ctx context.Context, reader *cluster.Reader, session identity.Session) (any, *controlplane.Error) {
	applications, failure := visibleApplications(ctx, reader, session)
	if failure != nil {
		return nil, failure
	}
	targets := make([]podDeleteTarget, 0, len(applications))
	for _, application := range applications {
		if !mayWriteApplication(session, application) || !podDeletable(application) {
			continue
		}
		targets = append(targets, podDeleteTarget{
			Namespace: application.Namespace,
			Name:      application.Name,
		})
	}
	return map[string]any{"applications": targets}, nil
}

// podDeletable reports whether one application can carry a Pod delete now: a
// stopped application has no Pods, and an application on its way out is not a
// place to act on a replica (KOSD-C-76, KOSD-C-33).
func podDeletable(application v1beta1.Application) bool {
	return !application.Spec.Paused &&
		application.DeletionTimestamp == nil &&
		application.Status.Phase != app.PhaseUninstalling
}
