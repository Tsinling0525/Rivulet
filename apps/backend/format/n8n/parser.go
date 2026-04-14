package n8n

import (
	"time"

	"github.com/Tsinling0525/rivulet/model"
)

// N8nWorkflow represents the n8n workflow format
type N8nWorkflow struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Active      bool                      `json:"active"`
	Nodes       []N8nNode                 `json:"nodes"`
	Connections map[string]N8nConnections `json:"connections"`
	Settings    map[string]interface{}    `json:"settings"`
}

// N8nNode represents an n8n node
type N8nNode struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	TypeVersion float64                `json:"typeVersion"`
	Position    []float64              `json:"position"`
	Parameters  map[string]interface{} `json:"parameters"`
	Credentials map[string]interface{} `json:"credentials"`
}

// N8nConnections represents n8n node connections
type N8nConnections struct {
	Main [][]N8nConnection `json:"main"`
}

// N8nConnection represents a single connection
type N8nConnection struct {
	Node  string `json:"node"`
	Type  string `json:"type"`
	Index int    `json:"index"`
}

// N8nRequest represents the full n8n API request
type N8nRequest struct {
	Workflow N8nWorkflow            `json:"workflow"`
	Data     map[string]interface{} `json:"data,omitempty"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

// ParseWorkflow converts n8n workflow to Rivulet workflow
func ParseWorkflow(n8nWF N8nWorkflow) model.Workflow {
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

		// Handle credentials if present
		if len(n8nNode.Credentials) > 0 {
			// Store credentials reference in config
			nodes[i].Config["_credentials"] = n8nNode.Credentials
		}

		// Store n8n specific metadata in config
		if nodes[i].Config == nil {
			nodes[i].Config = make(map[string]interface{})
		}
		nodes[i].Config["_n8n_typeVersion"] = n8nNode.TypeVersion
		nodes[i].Config["_n8n_position"] = n8nNode.Position
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

// ParseInputData converts n8n input data to Rivulet format
func ParseInputData(data map[string]interface{}) map[model.ID]model.Items {
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

// ToRivulet converts a full n8n request to Rivulet format
func ToRivulet(n8nReq N8nRequest) (model.Workflow, map[model.ID]model.Items) {
	workflow := ParseWorkflow(n8nReq.Workflow)
	inputData := ParseInputData(n8nReq.Data)

	// Provide default input for start nodes if no data provided
	if len(inputData) == 0 {
		for _, node := range workflow.Nodes {
			inputData[node.ID] = model.Items{{"trigger": "manual"}}
		}
	}

	return workflow, inputData
}

// GetN8nMetadata extracts n8n-specific metadata from a Rivulet node
func GetN8nMetadata(node model.Node) (typeVersion float64, position []float64, credentials map[string]interface{}) {
	if node.Config != nil {
		if tv, ok := node.Config["_n8n_typeVersion"].(float64); ok {
			typeVersion = tv
		}
		if pos, ok := node.Config["_n8n_position"].([]float64); ok {
			position = pos
		}
		if creds, ok := node.Config["_credentials"].(map[string]interface{}); ok {
			credentials = creds
		}
	}
	return typeVersion, position, credentials
}
