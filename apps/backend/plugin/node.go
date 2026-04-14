package plugin

import (
	"context"

	"github.com/Tsinling0525/rivulet/model"
)

type Deps struct {
	State StateStore
	Bus   EventBus
	Files FileStore
}

type NodeHandler interface {
	Init(ctx context.Context, deps Deps) error
	Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error)
}

type StateStore interface {
	SaveNodeState(ctx context.Context, execID string, nodeID model.ID, state map[string]any) error
	LoadNodeState(ctx context.Context, execID string, nodeID model.ID) (map[string]any, error)
}

type EventBus interface {
	Emit(ctx context.Context, event string, fields map[string]any) error
}

// FileStore provides blob/file attachment access scoped by workflow ID
type FileStore interface {
	Put(ctx context.Context, workflowID string, filename string, contents []byte, mediaType string) (fileID string, err error)
	Get(ctx context.Context, workflowID string, fileID string) (filename string, mediaType string, contents []byte, err error)
	List(ctx context.Context, workflowID string) ([]model.FileMeta, error)
	Delete(ctx context.Context, workflowID string, fileID string) error
}
