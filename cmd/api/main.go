package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Tsinling0525/rivulet/engine"
	"github.com/Tsinling0525/rivulet/model"
	_ "github.com/Tsinling0525/rivulet/nodes/echo" // Import to register the echo node
	"github.com/Tsinling0525/rivulet/plugin"
)

// n8nWorkflow represents the n8n workflow format
type n8nWorkflow struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Active      bool                      `json:"active"`
	Nodes       []n8nNode                 `json:"nodes"`
	Connections map[string]n8nConnections `json:"connections"`
	Settings    map[string]interface{}    `json:"settings"`
}

type n8nNode struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	TypeVersion float64                `json:"typeVersion"`
	Position    []float64              `json:"position"`
	Parameters  map[string]interface{} `json:"parameters"`
	Credentials map[string]interface{} `json:"credentials"`
}

type n8nConnections struct {
	Main [][]n8nConnection `json:"main"`
}

type n8nConnection struct {
	Node  string `json:"node"`
	Type  string `json:"type"`
	Index int    `json:"index"`
}

// APIRequest represents the request to start a workflow
type APIRequest struct {
	Workflow n8nWorkflow            `json:"workflow"`
	Data     map[string]interface{} `json:"data,omitempty"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

// APIResponse represents the API response
type APIResponse struct {
	Success bool                   `json:"success"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

// Convert n8n workflow to Rivulet workflow
func convertWorkflow(n8nWF n8nWorkflow) model.Workflow {
	nodes := make([]model.Node, len(n8nWF.Nodes))
	edges := []model.Edge{}

	// Convert nodes
	for i, n8nNode := range n8nWF.Nodes {
		nodes[i] = model.Node{
			ID:          model.ID(n8nNode.ID),
			Type:        n8nNode.Type,
			Name:        n8nNode.Name,
			Config:      n8nNode.Parameters,
			Timeout:     30 * time.Second, // Default timeout
			Concurrency: 1,                // Default concurrency
		}
	}

	// Convert connections to edges
	for fromNodeID, connections := range n8nWF.Connections {
		mainConns := connections.Main
		if len(mainConns) > 0 {
			for _, connGroup := range mainConns {
				for _, conn := range connGroup {
					edges = append(edges, model.Edge{
						FromNode: model.ID(fromNodeID),
						FromPort: model.PortMain,
						ToNode:   model.ID(conn.Node),
						ToPort:   model.PortMain,
					})
				}
			}
		}
	}

	return model.Workflow{
		ID:    model.ID(n8nWF.ID),
		Name:  n8nWF.Name,
		Nodes: nodes,
		Edges: edges,
	}
}

// Convert input data to Rivulet format
func convertInputData(data map[string]interface{}) map[model.ID]model.Items {
	result := make(map[model.ID]model.Items)

	for nodeID, nodeData := range data {
		if items, ok := nodeData.([]interface{}); ok {
			rivuletItems := make(model.Items, len(items))
			for i, item := range items {
				if itemMap, ok := item.(map[string]interface{}); ok {
					rivuletItems[i] = model.Item(itemMap)
				}
			}
			result[model.ID(nodeID)] = rivuletItems
		}
	}

	return result
}

// Dependencies
type nullBus struct{}

func (n nullBus) Emit(ctx context.Context, event string, fields map[string]any) error {
	return nil
}

type memState struct{}

func (m memState) SaveNodeState(context.Context, string, model.ID, map[string]any) error {
	return nil
}

func (m memState) LoadNodeState(context.Context, string, model.ID) (map[string]any, error) {
	return map[string]any{}, nil
}

// Test functions
func testN8nWorkflow() {
	// n8n-style workflow JSON
	workflowJSON := `{
		"workflow": {
			"id": "test-workflow",
			"name": "Test Echo Workflow",
			"active": true,
			"nodes": [
				{
					"id": "node1",
					"name": "Echo Node",
					"type": "echo",
					"typeVersion": 1.0,
					"position": [100, 100],
					"parameters": {
						"label": "Hello from n8n!"
					}
				}
			],
			"connections": {},
			"settings": {}
		},
		"data": {
			"node1": [
				{
					"message": "Hello World",
					"timestamp": "2024-01-01T00:00:00Z"
				}
			]
		}
	}`

	// Send request to API
	resp, err := http.Post("http://localhost:8080/workflow/start", "application/json", bytes.NewBufferString(workflowJSON))
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	// Read response
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("📡 Response Status: %s\n", resp.Status)
	fmt.Printf("📄 Response Body: %s\n", string(body))
}

func testHealth() {
	resp, err := http.Get("http://localhost:8080/health")
	if err != nil {
		fmt.Printf("❌ Health check failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("🏥 Health Status: %s\n", resp.Status)
	fmt.Printf("📄 Health Response: %s\n", string(body))
}

func runTests() {
	fmt.Println("🧪 Testing Rivulet n8n-compatible API")
	fmt.Println("=====================================")

	// Test health endpoint
	fmt.Println("\n1. Testing health endpoint:")
	testHealth()

	// Test workflow execution
	fmt.Println("\n2. Testing workflow execution:")
	testN8nWorkflow()

	fmt.Println("\n✅ Test completed!")
}

// HTTP handlers
func handleStartWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req APIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Convert n8n workflow to Rivulet workflow
	workflow := convertWorkflow(req.Workflow)

	// Convert input data
	inputData := convertInputData(req.Data)
	if len(inputData) == 0 {
		// Provide default input for start nodes
		for _, node := range workflow.Nodes {
			inputData[node.ID] = model.Items{{"trigger": "manual"}}
		}
	}

	// Setup engine
	deps := plugin.Deps{State: memState{}, Bus: nullBus{}}
	eng := engine.New(deps)

	// Execute workflow
	executionID := fmt.Sprintf("exec-%d", time.Now().Unix())
	result, err := eng.Run(context.Background(), executionID, workflow, inputData)

	response := APIResponse{}
	if err != nil {
		response.Success = false
		response.Error = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		response.Success = true
		response.Data = map[string]interface{}{
			"executionId": executionID,
			"result":      result,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	response := APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"status":    "healthy",
			"timestamp": time.Now().Unix(),
			"version":   "1.0.0",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func main() {
	// Check if we should run tests
	if len(os.Args) > 1 && os.Args[1] == "test" {
		runTests()
		return
	}

	http.HandleFunc("/workflow/start", handleStartWorkflow)
	http.HandleFunc("/health", handleHealth)

	port := "8080"
	fmt.Printf("🚀 Starting Rivulet API Server on port %s\n", port)
	fmt.Printf("📡 Endpoints:\n")
	fmt.Printf("   POST /workflow/start - Start a workflow\n")
	fmt.Printf("   GET  /health         - Health check\n")
	fmt.Printf("🔗 n8n-compatible API ready!\n")

	log.Fatal(http.ListenAndServe(":"+port, nil))
}
