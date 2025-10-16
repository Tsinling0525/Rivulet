package server

import (
	"fmt"
	"net/http"
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

	// Instance Manager
	mgr := infra.NewInstanceManager()

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
			out = append(out, map[string]any{
				"id": it.ID, "name": it.Name, "state": it.State, "created_at": it.CreatedAt.Unix(), "workflow_path": it.WorkflowPath,
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
		sendSuccess(c, map[string]any{
			"id": inst.ID, "name": inst.Name, "state": inst.State, "created_at": inst.CreatedAt.Unix(), "workflow_path": inst.WorkflowPath,
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

	return r
}
