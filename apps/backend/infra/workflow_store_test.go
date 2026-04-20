package infra

import (
	"path/filepath"
	"testing"

	"github.com/Tsinling0525/rivulet/format/n8n"
)

func TestWorkflowStoreCreateVersionAndActivate(t *testing.T) {
	store := &WorkflowStore{dir: filepath.Join(t.TempDir(), "workflows")}

	req := n8n.N8nRequest{
		Workflow: n8n.N8nWorkflow{
			ID:   "wf-demo",
			Name: "Demo",
			Nodes: []n8n.N8nNode{
				{ID: "echo1", Name: "Echo", Type: "echo", Parameters: map[string]any{"label": "hello"}},
			},
		},
		Data: map[string]any{
			"echo1": []any{map[string]any{"message": "hello"}},
		},
	}

	record, err := store.Create(req, "first version", true)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if record.ID != "wf-demo" {
		t.Fatalf("expected workflow id wf-demo, got %q", record.ID)
	}
	if record.ActiveVersion != 1 {
		t.Fatalf("expected active version 1, got %d", record.ActiveVersion)
	}

	req.Workflow.Name = "Demo v2"
	req.Workflow.Nodes = append(req.Workflow.Nodes, n8n.N8nNode{ID: "echo2", Name: "Echo 2", Type: "echo"})
	record, err = store.AddVersion("wf-demo", req, false)
	if err != nil {
		t.Fatalf("AddVersion returned error: %v", err)
	}
	if len(record.Versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(record.Versions))
	}
	if record.ActiveVersion != 1 {
		t.Fatalf("expected active version to remain 1, got %d", record.ActiveVersion)
	}

	record, err = store.ActivateVersion("wf-demo", 2)
	if err != nil {
		t.Fatalf("ActivateVersion returned error: %v", err)
	}
	if record.ActiveVersion != 2 {
		t.Fatalf("expected active version 2, got %d", record.ActiveVersion)
	}

	loaded, versionReq, err := store.LoadVersionRequest("wf-demo", 0)
	if err != nil {
		t.Fatalf("LoadVersionRequest returned error: %v", err)
	}
	if loaded.ActiveVersion != 2 {
		t.Fatalf("expected loaded active version 2, got %d", loaded.ActiveVersion)
	}
	if got := len(versionReq.Workflow.Nodes); got != 2 {
		t.Fatalf("expected 2 nodes in active version request, got %d", got)
	}
}
