package service

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"
)

type Service interface {
	Run(ctx context.Context) error
}

// Manager manages a collection of services.
type Manager struct {
	services []Service
	mu       sync.Mutex // Protects access to the services slice
	wg       *errgroup.Group
}

func NewManager() *Manager {
	return &Manager{services: make([]Service, 0)}
}

// Register adds a new service to the ServiceManager.
func (sm *Manager) Register(s ...Service) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.services = append(sm.services, s...)
}

// Run runs all registered services concurrently using an errgroup.
func (sm *Manager) Run(ctx context.Context) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if len(sm.services) == 0 {
		return
	}

	group, ctx := errgroup.WithContext(ctx)
	for _, s := range sm.services {
		s := s // Capture the service in the loop.
		group.Go(func() error {
			return s.Run(ctx)
		})
	}
	sm.wg = group
}

func (sm *Manager) Wait() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if len(sm.services) == 0 {
		return nil
	}
	return sm.wg.Wait()
}
