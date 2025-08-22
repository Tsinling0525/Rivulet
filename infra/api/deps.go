package api

import (
	"context"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

// NullBus is a no-op event bus implementation.
type NullBus struct{}

func (n NullBus) Emit(ctx context.Context, event string, fields map[string]any) error { return nil }

// MemState is an in-memory state store implementation.
type MemState struct{}

func (m MemState) SaveNodeState(context.Context, string, model.ID, map[string]any) error { return nil }
func (m MemState) LoadNodeState(context.Context, string, model.ID) (map[string]any, error) {
	return map[string]any{}, nil
}

// Ensure interface implementation at compile time
var _ plugin.EventBus = (*NullBus)(nil)
var _ plugin.StateStore = (*MemState)(nil)
