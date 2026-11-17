package fixtures

import (
	"context"
	"sync"

	"github.com/tsic404/kosd/pkg/controlplane"
)

// SwitchableSource is the control plane's read source with a runtime that can
// become unreachable and recover: while failed is set every read names the
// failure the way an unreachable cluster does, and clearing it restores the
// live source. A case uses it to prove an unreadable projection is not an empty
// one (KOSD-C-76, KOSD-C-64).
type SwitchableSource struct {
	// Live is the source every read reaches while the runtime is reachable.
	Live controlplane.ObjectSource

	mu     sync.Mutex
	failed error
}

// Fail makes every read fail with err until Recover is called.
func (s *SwitchableSource) Fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed = err
}

// Recover makes reads reach the live source again.
func (s *SwitchableSource) Recover() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed = nil
}

// List implements controlplane.ObjectSource.
func (s *SwitchableSource) List(ctx context.Context, objectType controlplane.ObjectType, namespace string) ([]controlplane.ObservedObject, error) {
	if failure := s.failure(); failure != nil {
		return nil, failure
	}
	return s.Live.List(ctx, objectType, namespace)
}

// Get implements controlplane.ObjectSource.
func (s *SwitchableSource) Get(ctx context.Context, objectType controlplane.ObjectType, namespace, name string) (controlplane.ObservedObject, error) {
	if failure := s.failure(); failure != nil {
		return controlplane.ObservedObject{}, failure
	}
	return s.Live.Get(ctx, objectType, namespace, name)
}

// failure reports the current outage, if any.
func (s *SwitchableSource) failure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failed
}
