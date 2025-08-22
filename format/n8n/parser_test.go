package n8n

import (
	"testing"

	"github.com/Tsinling0525/rivulet/model"
)

func TestParseWorkflow(t *testing.T) {
	// Test n8n workflow
	n8nWF := N8nWorkflow{
		ID:   "test-workflow",
		Name: "Test Workflow",
		Nodes: []N8nNode{
			{
				ID:   "node1",
				Name: "Test Node",
				Type: "echo",
				Parameters: map[string]interface{}{
					"label": "test-label",
				},
				TypeVersion: 1.0,
				Position:    []float64{100, 200},
			},
		},
		Connections: map[string]N8nConnections{
			"node1": {
				Main: [][]N8nConnection{
					{
						{Node: "node2", Type: "main", Index: 0},
					},
				},
			},
		},
	}

	// Parse to Rivulet format
	rivuletWF := ParseWorkflow(n8nWF)

	// Assertions
	if string(rivuletWF.ID) != "test-workflow" {
		t.Errorf("Expected ID 'test-workflow', got '%s'", rivuletWF.ID)
	}

	if rivuletWF.Name != "Test Workflow" {
		t.Errorf("Expected Name 'Test Workflow', got '%s'", rivuletWF.Name)
	}

	if len(rivuletWF.Nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(rivuletWF.Nodes))
	}

	node := rivuletWF.Nodes[0]
	if string(node.ID) != "node1" {
		t.Errorf("Expected node ID 'node1', got '%s'", node.ID)
	}

	if node.Type != "echo" {
		t.Errorf("Expected node type 'echo', got '%s'", node.Type)
	}

	if label, ok := node.Config["label"].(string); !ok || label != "test-label" {
		t.Errorf("Expected label 'test-label', got '%v'", node.Config["label"])
	}

	// Check n8n metadata is preserved
	if typeVersion, ok := node.Config["_n8n_typeVersion"].(float64); !ok || typeVersion != 1.0 {
		t.Errorf("Expected typeVersion 1.0, got '%v'", node.Config["_n8n_typeVersion"])
	}

	if len(rivuletWF.Edges) != 1 {
		t.Errorf("Expected 1 edge, got %d", len(rivuletWF.Edges))
	}

	edge := rivuletWF.Edges[0]
	if string(edge.FromNode) != "node1" || string(edge.ToNode) != "node2" {
		t.Errorf("Edge connection mismatch: %s -> %s", edge.FromNode, edge.ToNode)
	}
}

func TestParseInputData(t *testing.T) {
	inputData := map[string]interface{}{
		"node1": []interface{}{
			map[string]interface{}{
				"message": "hello",
				"count":   42,
			},
			map[string]interface{}{
				"message": "world",
				"count":   24,
			},
		},
	}

	result := ParseInputData(inputData)

	if len(result) != 1 {
		t.Errorf("Expected 1 node data, got %d", len(result))
	}

	node1Data, exists := result[model.ID("node1")]
	if !exists {
		t.Errorf("Expected node1 data to exist")
	}

	if len(node1Data) != 2 {
		t.Errorf("Expected 2 items for node1, got %d", len(node1Data))
	}

	if msg, ok := node1Data[0]["message"].(string); !ok || msg != "hello" {
		t.Errorf("Expected first item message 'hello', got '%v'", node1Data[0]["message"])
	}
}

func TestToRivulet(t *testing.T) {
	n8nReq := N8nRequest{
		Workflow: N8nWorkflow{
			ID:    "test-wf",
			Name:  "Test",
			Nodes: []N8nNode{{ID: "n1", Type: "echo", Name: "Echo"}},
		},
		Data: map[string]interface{}{
			"n1": []interface{}{
				map[string]interface{}{"msg": "test"},
			},
		},
	}

	workflow, inputData := ToRivulet(n8nReq)

	if string(workflow.ID) != "test-wf" {
		t.Errorf("Expected workflow ID 'test-wf', got '%s'", workflow.ID)
	}

	if len(inputData) != 1 {
		t.Errorf("Expected 1 input data entry, got %d", len(inputData))
	}

	if data, exists := inputData[model.ID("n1")]; !exists || len(data) != 1 {
		t.Errorf("Expected input data for n1 with 1 item")
	}
}

func TestToRivuletWithDefaultData(t *testing.T) {
	n8nReq := N8nRequest{
		Workflow: N8nWorkflow{
			ID:    "test-wf",
			Name:  "Test",
			Nodes: []N8nNode{{ID: "n1", Type: "echo", Name: "Echo"}},
		},
		// No data provided
	}

	_, inputData := ToRivulet(n8nReq)

	if len(inputData) != 1 {
		t.Errorf("Expected default input data to be created, got %d entries", len(inputData))
	}

	if data, exists := inputData[model.ID("n1")]; !exists || len(data) != 1 {
		t.Errorf("Expected default input data for n1")
	} else if trigger, ok := data[0]["trigger"].(string); !ok || trigger != "manual" {
		t.Errorf("Expected default trigger 'manual', got '%v'", data[0]["trigger"])
	}
}

func TestGetN8nMetadata(t *testing.T) {
	node := model.Node{
		ID:   "test",
		Type: "echo",
		Config: map[string]any{
			"label":            "test",
			"_n8n_typeVersion": 1.5,
			"_n8n_position":    []float64{150, 250},
			"_credentials":     map[string]interface{}{"api_key": "secret"},
		},
	}

	typeVersion, position, credentials := GetN8nMetadata(node)

	if typeVersion != 1.5 {
		t.Errorf("Expected typeVersion 1.5, got %f", typeVersion)
	}

	if len(position) != 2 || position[0] != 150 || position[1] != 250 {
		t.Errorf("Expected position [150, 250], got %v", position)
	}

	if credentials == nil {
		t.Errorf("Expected credentials to be extracted")
	}
}
