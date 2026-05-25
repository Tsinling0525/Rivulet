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

func TestWorkflowStoreRollbackPromptToHash(t *testing.T) {
	store := &WorkflowStore{dir: filepath.Join(t.TempDir(), "workflows")}
	reqV1 := n8n.N8nRequest{Workflow: n8n.N8nWorkflow{
		ID:   "wf-prompt",
		Name: "Prompt Workflow",
		Nodes: []n8n.N8nNode{{
			ID:         "llm1",
			Name:       "LLM",
			Type:       "chatgpt",
			Parameters: map[string]interface{}{"prompt": "old prompt"},
		}},
	}}
	if _, err := store.Create(reqV1, "", true); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	reqV2 := reqV1
	reqV2.Workflow.Nodes[0].Parameters = map[string]interface{}{"prompt": "new prompt"}
	if _, err := store.AddVersion("wf-prompt", reqV2, true); err != nil {
		t.Fatalf("AddVersion returned error: %v", err)
	}

	record, err := store.RollbackPromptToHash("wf-prompt", "llm1", promptTemplateHash("old prompt"), true)
	if err != nil {
		t.Fatalf("RollbackPromptToHash returned error: %v", err)
	}
	if record.ActiveVersion != 3 {
		t.Fatalf("expected rollback version 3 to be active, got %d", record.ActiveVersion)
	}
	_, active, err := store.LoadVersionRequest("wf-prompt", 0)
	if err != nil {
		t.Fatalf("LoadVersionRequest returned error: %v", err)
	}
	prompt, _ := active.Workflow.Nodes[0].Parameters["prompt"].(string)
	if prompt != "old prompt" {
		t.Fatalf("expected active prompt to roll back, got %q", prompt)
	}
}
