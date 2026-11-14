package fixtures

import (
	"context"
	"sort"
	"time"

	"github.com/tsic404/kosd/pkg/controlplane"
)

// LogSource is the cluster's log endpoint stand-in (KOSD-C-64): it answers the
// lines a case wrote for one pod container, oldest first, bounded by the
// requested window and line count the way the kubelet does. The control plane
// owns the request bounds; this only serves what it asks for.
type LogSource struct {
	// ByContainer maps one namespace/pod/container key to its lines.
	ByContainer map[string][]controlplane.LogLine
	// Err makes every read fail the way an unreachable cluster does.
	Err error
}

// LogKey is the map key of one container's lines.
func LogKey(namespace, pod, container string) string {
	return namespace + "/" + pod + "/" + container
}

// Lines implements controlplane.LogSource.
func (s *LogSource) Lines(_ context.Context, namespace, pod, container string, since time.Time, limit int) ([]controlplane.LogLine, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	read := make([]controlplane.LogLine, 0)
	for _, line := range s.ByContainer[LogKey(namespace, pod, container)] {
		if line.Timestamp.Before(since) {
			continue
		}
		read = append(read, line)
	}
	sort.SliceStable(read, func(left, right int) bool { return read[left].Timestamp.Before(read[right].Timestamp) })
	if len(read) > limit {
		read = read[len(read)-limit:]
	}
	return read, nil
}
