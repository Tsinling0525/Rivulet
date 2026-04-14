package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Tsinling0525/rivulet/engine"
	"github.com/Tsinling0525/rivulet/format/n8n"
	"github.com/Tsinling0525/rivulet/infra"
	apiinfra "github.com/Tsinling0525/rivulet/infra/api"
	_ "github.com/Tsinling0525/rivulet/nodes/echo"
	_ "github.com/Tsinling0525/rivulet/nodes/files"
	_ "github.com/Tsinling0525/rivulet/nodes/fs"
	_ "github.com/Tsinling0525/rivulet/nodes/http"
	_ "github.com/Tsinling0525/rivulet/nodes/logic"
	_ "github.com/Tsinling0525/rivulet/nodes/merge"
	_ "github.com/Tsinling0525/rivulet/nodes/ollama"
	_ "github.com/Tsinling0525/rivulet/nodes/openai"
	"github.com/Tsinling0525/rivulet/plugin"
)

// APIRequest represents the request to start a workflow
type APIRequest = n8n.N8nRequest

// APIResponse represents the API response
type APIResponse struct {
	Success bool                   `json:"success"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

// Helper function to send JSON response
func sendResponse(c *gin.Context, statusCode int, success bool, data map[string]interface{}, errorMsg string) {
	response := APIResponse{Success: success, Data: data, Error: errorMsg}
	c.JSON(statusCode, response)
}

func sendSuccess(c *gin.Context, data map[string]interface{}) {
	sendResponse(c, http.StatusOK, true, data, "")
}
func sendError(c *gin.Context, statusCode int, errorMsg string) {
	sendResponse(c, statusCode, false, nil, errorMsg)
}

// Handlers
func handleHealth(c *gin.Context) {
	sendSuccess(c, map[string]interface{}{"status": "healthy", "timestamp": time.Now().Unix(), "version": "1.0.0"})
}

func handleStartWorkflow(c *gin.Context) {
	var req APIRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}
	workflow, inputData := n8n.ToRivulet(req)
	deps := plugin.Deps{State: apiinfra.MemState{}, Bus: apiinfra.NullBus{}, Files: infra.NewLocalFiles()}
	eng := engine.New(deps)
	executionID := fmt.Sprintf("exec-%d", time.Now().Unix())
	result, err := eng.Run(c.Request.Context(), executionID, workflow, inputData)
	if err != nil {
		sendError(c, http.StatusInternalServerError, err.Error())
		return
	}
	sendSuccess(c, map[string]interface{}{"executionId": executionID, "result": result})
}

func listWorkflowFiles() ([]map[string]any, error) {
	dir := infra.WorkflowsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	workflows := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		fullPath := filepath.Join(dir, entry.Name())
		item := map[string]any{
			"file_name": entry.Name(),
			"path":      fullPath,
		}

		if raw, err := os.ReadFile(fullPath); err == nil {
			var req n8n.N8nRequest
			if err := json.Unmarshal(raw, &req); err == nil {
				item["workflow_id"] = req.Workflow.ID
				item["name"] = req.Workflow.Name
				item["active"] = req.Workflow.Active
				item["sample_data"] = req.Data
				item["node_count"] = len(req.Workflow.Nodes)
			}
		}

		workflows = append(workflows, item)
	}

	return workflows, nil
}

func avgDurationMS(stats infra.InstanceStats) int64 {
	if stats.SuccessfulExecutions == 0 {
		return 0
	}
	return stats.TotalSuccessDuration.Milliseconds() / int64(stats.SuccessfulExecutions)
}

// NewRouter builds the Gin router with routes and middleware
func NewRouter() *gin.Engine {
	r := gin.Default()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	// CORS
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// Routes (start-only API)
	r.GET("/health", handleHealth)
	r.POST("/workflow/start", handleStartWorkflow)
	r.GET("/workflows/files", func(c *gin.Context) {
		workflows, err := listWorkflowFiles()
		if err != nil {
			sendError(c, http.StatusInternalServerError, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"workflows": workflows})
	})

	// Instance Manager
	mgr := infra.NewInstanceManager()

	frontendDir := infra.FrontendDir()
	if stat, err := os.Stat(frontendDir); err == nil && stat.IsDir() {
		r.StaticFS("/app", gin.Dir(frontendDir, true))
		r.GET("/", func(c *gin.Context) {
			c.File(filepath.Join(frontendDir, "index.html"))
		})
	}

	r.POST("/instances", func(c *gin.Context) {
		var payload struct {
			WorkflowPath string `json:"workflow_path"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil || payload.WorkflowPath == "" {
			sendError(c, http.StatusBadRequest, "workflow_path is required")
			return
		}
		inst, err := mgr.CreateFromWorkflowPath(payload.WorkflowPath)
		if err != nil {
			sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		sendSuccess(c, map[string]interface{}{"id": inst.ID, "state": inst.State, "name": inst.Name})
	})

	r.GET("/instances", func(c *gin.Context) {
		list := mgr.List()
		out := make([]map[string]any, 0, len(list))
		for _, it := range list {
			snapshot := it.Snapshot()
			out = append(out, map[string]any{
				"id":            it.ID,
				"name":          it.Name,
				"state":         it.State,
				"created_at":    it.CreatedAt.Unix(),
				"workflow_path": it.WorkflowPath,
				"queue_length":  snapshot.QueueLength,
				"is_executing":  snapshot.Active.IsExecuting,
			})
		}
		sendSuccess(c, map[string]any{"instances": out})
	})

	r.GET("/instances/:id", func(c *gin.Context) {
		id := c.Param("id")
		inst, ok := mgr.Get(id)
		if !ok {
			sendError(c, http.StatusNotFound, "not found")
			return
		}
		snapshot := inst.Snapshot()
		sendSuccess(c, map[string]any{
			"id":            inst.ID,
			"name":          inst.Name,
			"state":         inst.State,
			"created_at":    inst.CreatedAt.Unix(),
			"workflow_path": inst.WorkflowPath,
			"workflow": map[string]any{
				"id":         inst.Workflow.ID,
				"name":       inst.Workflow.Name,
				"node_count": len(inst.Workflow.Nodes),
				"edge_count": len(inst.Workflow.Edges),
				"nodes": func() []map[string]any {
					nodes := make([]map[string]any, 0, len(inst.Workflow.Nodes))
					for _, node := range inst.Workflow.Nodes {
						nodes = append(nodes, map[string]any{
							"id":   node.ID,
							"name": node.Name,
							"type": node.Type,
						})
					}
					return nodes
				}(),
			},
			"stats": map[string]any{
				"total_executions":      snapshot.Stats.TotalExecutions,
				"successful_executions": snapshot.Stats.SuccessfulExecutions,
				"failed_executions":     snapshot.Stats.FailedExecutions,
				"last_run_at":           snapshot.Stats.LastRunAt,
				"average_duration_ms":   avgDurationMS(snapshot.Stats),
				"queue_length":          snapshot.QueueLength,
			},
			"execution_status": snapshot.Active,
			"last_execution":   snapshot.LastRun,
		})
	})

	r.POST("/instances/:id/stop", func(c *gin.Context) {
		id := c.Param("id")
		if err := mgr.Stop(id); err != nil {
			sendError(c, http.StatusNotFound, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"stopped": true})
	})

	r.GET("/instances/:id/logs", func(c *gin.Context) {
		id := c.Param("id")
		logs, err := mgr.Logs(id)
		if err != nil {
			sendError(c, http.StatusNotFound, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"logs": logs})
	})

	r.POST("/instances/:id/enqueue", func(c *gin.Context) {
		id := c.Param("id")
		// Expect {"data": {nodeID: [{...}]}}
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil {
			sendError(c, http.StatusBadRequest, "invalid json")
			return
		}
		if raw, ok := body["data"]; ok {
			// inputs is map[string][]map[string]any for compatibility (avoid model import error)
			inputs := map[string][]map[string]any{}
			if m, ok := raw.(map[string]any); ok {
				for k, v := range m {
					if arr, ok := v.([]any); ok {
						items := make([]map[string]any, 0, len(arr))
						for _, it := range arr {
							if obj, ok := it.(map[string]any); ok {
								items = append(items, obj)
							}
						}
						inputs[k] = items
					}
				}
			}
			if err := mgr.Enqueue(id, inputs); err != nil {
				sendError(c, http.StatusBadRequest, err.Error())
				return
			}
			sendSuccess(c, map[string]any{"enqueued": true})
			return
		}
		sendError(c, http.StatusBadRequest, "missing data field: expected {data: {...}}")
	})

	r.GET("/dashboard/metrics", func(c *gin.Context) {
		metrics := mgr.DashboardMetrics()
		sendSuccess(c, map[string]any{"metrics": metrics})
	})

	return r
}
