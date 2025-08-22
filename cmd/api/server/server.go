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

	return r
}
