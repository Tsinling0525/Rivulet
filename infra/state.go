package infra

import (
	"context"
	"sync"
)

type memState struct {
	mu sync.RWMutex
	m  map[string]map[string]map[string]any
}

func NewMemState() *memState { return &memState{m: map[string]map[string]map[string]any{}} }

func (s *memState) SaveNodeState(ctx context.Context, execID, nodeID string, state map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[execID]; !ok {
		s.m[execID] = map[string]map[string]any{}
	}
	s.m[execID][nodeID] = state
	return nil
}

func (s *memState) LoadNodeState(ctx context.Context, execID, nodeID string) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.m[execID]; ok {
		if st, ok := e[nodeID]; ok {
			return st, nil
		}
	}
	return map[string]any{}, nil
}
