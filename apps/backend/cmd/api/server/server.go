package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Tsinling0525/rivulet/format/n8n"
	"github.com/Tsinling0525/rivulet/infra"
	apiinfra "github.com/Tsinling0525/rivulet/infra/api"
	"github.com/Tsinling0525/rivulet/model"
	_ "github.com/Tsinling0525/rivulet/nodes/echo"
	_ "github.com/Tsinling0525/rivulet/nodes/eval"
	_ "github.com/Tsinling0525/rivulet/nodes/files"
	_ "github.com/Tsinling0525/rivulet/nodes/fs"
	_ "github.com/Tsinling0525/rivulet/nodes/http"
	_ "github.com/Tsinling0525/rivulet/nodes/llmroute"
	_ "github.com/Tsinling0525/rivulet/nodes/logic"
	_ "github.com/Tsinling0525/rivulet/nodes/merge"
	ollama "github.com/Tsinling0525/rivulet/nodes/ollama"
	_ "github.com/Tsinling0525/rivulet/nodes/openai"
	_ "github.com/Tsinling0525/rivulet/nodes/review"
	_ "github.com/Tsinling0525/rivulet/nodes/wasm"
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

func handleStartWorkflow(deps plugin.Deps, runs *infra.RunStore, checkpoints *infra.CheckpointStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req APIRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			sendError(c, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		outcome, err := infra.ExecuteWorkflow(c.Request.Context(), deps, runs, infra.ExecuteRequest{
			WorkflowRequest: req,
			Source:          "ad_hoc",
			Trigger:         "api_start",
			Checkpoints:     checkpoints,
		})
		if err != nil {
			if outcome.Run.Status == "paused" {
				sendSuccess(c, map[string]interface{}{"executionId": outcome.Run.ID, "result": outcome.Result, "run": outcome.Run})
				return
			}
			sendError(c, http.StatusInternalServerError, err.Error())
			return
		}
		sendSuccess(c, map[string]interface{}{"executionId": outcome.Run.ID, "result": outcome.Result, "run": outcome.Run})
	}
}

func handleOllamaChat(client *ollama.Client) gin.HandlerFunc {
	if client == nil {
		client = ollama.NewClient()
	}
	return func(c *gin.Context) {
		var payload struct {
			Model    string               `json:"model"`
			Messages []ollama.ChatMessage `json:"messages"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			sendError(c, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		model := strings.TrimSpace(payload.Model)
		if model == "" {
			model = ollama.DefaultModel
		}
		if len(payload.Messages) == 0 {
			sendError(c, http.StatusBadRequest, "messages must contain at least one message")
			return
		}

		messages := make([]ollama.ChatMessage, 0, len(payload.Messages))
		for _, message := range payload.Messages {
			role := strings.ToLower(strings.TrimSpace(message.Role))
			if role == "" {
				sendError(c, http.StatusBadRequest, "message role is required")
				return
			}
			if role != "system" && role != "user" && role != "assistant" && role != "tool" {
				sendError(c, http.StatusBadRequest, "message role must be one of: system, user, assistant, tool")
				return
			}
			if strings.TrimSpace(message.Content) == "" {
				sendError(c, http.StatusBadRequest, "message content is required")
				return
			}
			messages = append(messages, ollama.ChatMessage{Role: role, Content: message.Content})
		}

		completion, err := client.Chat(c.Request.Context(), model, messages)
		if err != nil {
			sendError(c, http.StatusBadGateway, err.Error())
			return
		}
		sendSuccess(c, map[string]any{
			"model":   completion.Model,
			"message": completion.Message,
			"reply":   completion.Reply,
			"usage":   completion.Usage,
		})
	}
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

func workflowSummary(record infra.StoredWorkflow) map[string]any {
	return map[string]any{
		"id":             record.ID,
		"name":           record.Name,
		"kind":           record.Kind,
		"ai":             record.AI,
		"description":    record.Description,
		"active_version": record.ActiveVersion,
		"created_at":     record.CreatedAt,
		"updated_at":     record.UpdatedAt,
		"versions":       len(record.Versions),
		"node_count":     workflowNodeCount(record, record.ActiveVersion),
	}
}

func workflowDetail(record infra.StoredWorkflow) map[string]any {
	versions := make([]map[string]any, 0, len(record.Versions))
	for _, version := range record.Versions {
		versions = append(versions, map[string]any{
			"number":     version.Number,
			"created_at": version.CreatedAt,
			"node_count": version.NodeCount,
			"active":     version.Number == record.ActiveVersion,
			"request":    json.RawMessage(version.Request),
		})
	}
	return map[string]any{
		"id":             record.ID,
		"name":           record.Name,
		"kind":           record.Kind,
		"ai":             record.AI,
		"description":    record.Description,
		"active_version": record.ActiveVersion,
		"created_at":     record.CreatedAt,
		"updated_at":     record.UpdatedAt,
		"versions":       versions,
	}
}

func workflowNodeCount(record infra.StoredWorkflow, version int) int {
	for _, item := range record.Versions {
		if item.Number == version {
			return item.NodeCount
		}
	}
	return 0
}

func scheduleResponse(schedule infra.Schedule) map[string]any {
	return map[string]any{
		"id":               schedule.ID,
		"workflow_id":      schedule.WorkflowID,
		"workflow_version": schedule.WorkflowVersion,
		"interval_seconds": schedule.IntervalSeconds,
		"input":            schedule.Input,
		"enabled":          schedule.Enabled,
		"running":          schedule.Running,
		"created_at":       schedule.CreatedAt,
		"updated_at":       schedule.UpdatedAt,
		"next_run_at":      schedule.NextRunAt,
		"last_run_at":      schedule.LastRunAt,
		"last_run_id":      schedule.LastRunID,
		"last_status":      schedule.LastStatus,
		"last_error":       schedule.LastError,
	}
}

func reviewStatusFromQuery(value string) model.ReviewStatus {
	switch model.ReviewStatus(value) {
	case model.ReviewPending, model.ReviewApproved, model.ReviewRejected:
		return model.ReviewStatus(value)
	default:
		return ""
	}
}

func parseInputData(raw any) (map[model.ID]model.Items, error) {
	if raw == nil {
		return nil, nil
	}
	out := map[model.ID]model.Items{}
	root, ok := raw.(map[string]any)
	if !ok {
		return nil, http.ErrBodyNotAllowed
	}
	for key, value := range root {
		arr, ok := value.([]any)
		if !ok {
			return nil, http.ErrBodyNotAllowed
		}
		items := make(model.Items, 0, len(arr))
		for _, entry := range arr {
			obj, ok := entry.(map[string]any)
			if !ok {
				return nil, http.ErrBodyNotAllowed
			}
			items = append(items, model.Item(obj))
		}
		out[model.ID(key)] = items
	}
	return out, nil
}

// NewRouter builds the Gin router with routes and middleware
func NewRouter() *gin.Engine {
	reviews := infra.NewReviewStore()
	baseDeps := plugin.Deps{State: apiinfra.MemState{}, Bus: apiinfra.NullBus{}, Files: infra.NewLocalFiles(), Reviews: reviews}
	workflows := infra.NewWorkflowStore()
	runs := infra.NewRunStore()
	schedules := infra.NewScheduleStore()
	checkpoints := infra.NewCheckpointStore()
	scheduleRunner := infra.NewScheduleRunner(baseDeps, workflows, runs, schedules, checkpoints)
	scheduleRunner.Start(context.Background(), time.Second)
	r := gin.Default()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	// CORS
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// Routes (start-only API)
	r.GET("/health", handleHealth)
	r.POST("/api/chat/ollama", handleOllamaChat(ollama.NewClient()))
	r.POST("/workflow/start", handleStartWorkflow(baseDeps, runs, checkpoints))
	r.GET("/workflows/files", func(c *gin.Context) {
		workflows, err := listWorkflowFiles()
		if err != nil {
			sendError(c, http.StatusInternalServerError, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"workflows": workflows})
	})

	r.GET("/workflows", func(c *gin.Context) {
		records, err := workflows.List()
		if err != nil {
			sendError(c, http.StatusInternalServerError, err.Error())
			return
		}
		out := make([]map[string]any, 0, len(records))
		for _, record := range records {
			out = append(out, workflowSummary(record))
		}
		sendSuccess(c, map[string]any{"workflows": out})
	})

	r.POST("/workflows", func(c *gin.Context) {
		var payload struct {
			Description string                 `json:"description"`
			Activate    *bool                  `json:"activate"`
			Workflow    n8n.N8nWorkflow        `json:"workflow"`
			Data        map[string]interface{} `json:"data,omitempty"`
			Options     map[string]interface{} `json:"options,omitempty"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			sendError(c, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		req := n8n.N8nRequest{Workflow: payload.Workflow, Data: payload.Data, Options: payload.Options}
		record, err := workflows.Create(req, payload.Description, payload.Activate == nil || *payload.Activate)
		if err != nil {
			sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"workflow": workflowDetail(record)})
	})

	r.GET("/workflows/:id", func(c *gin.Context) {
		record, err := workflows.Get(c.Param("id"))
		if err != nil {
			status := http.StatusInternalServerError
			if err == infra.ErrWorkflowNotFound {
				status = http.StatusNotFound
			}
			sendError(c, status, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"workflow": workflowDetail(record)})
	})

	r.POST("/workflows/:id/versions", func(c *gin.Context) {
		var payload struct {
			Activate *bool                  `json:"activate"`
			Workflow n8n.N8nWorkflow        `json:"workflow"`
			Data     map[string]interface{} `json:"data,omitempty"`
			Options  map[string]interface{} `json:"options,omitempty"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			sendError(c, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		req := n8n.N8nRequest{Workflow: payload.Workflow, Data: payload.Data, Options: payload.Options}
		record, err := workflows.AddVersion(c.Param("id"), req, payload.Activate == nil || *payload.Activate)
		if err != nil {
			status := http.StatusBadRequest
			if err == infra.ErrWorkflowNotFound {
				status = http.StatusNotFound
			}
			sendError(c, status, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"workflow": workflowDetail(record)})
	})

	r.POST("/workflows/:id/activate", func(c *gin.Context) {
		var payload struct {
			Version int `json:"version"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil || payload.Version <= 0 {
			sendError(c, http.StatusBadRequest, "version is required")
			return
		}
		record, err := workflows.ActivateVersion(c.Param("id"), payload.Version)
		if err != nil {
			status := http.StatusBadRequest
			if err == infra.ErrWorkflowNotFound {
				status = http.StatusNotFound
			}
			sendError(c, status, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"workflow": workflowDetail(record)})
	})

	r.POST("/workflows/:id/prompts/:node_id/rollback", func(c *gin.Context) {
		var payload struct {
			PromptHash string `json:"prompt_hash"`
			Activate   *bool  `json:"activate"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil || payload.PromptHash == "" {
			sendError(c, http.StatusBadRequest, "prompt_hash is required")
			return
		}
		record, err := workflows.RollbackPromptToHash(c.Param("id"), c.Param("node_id"), payload.PromptHash, payload.Activate == nil || *payload.Activate)
		if err != nil {
			status := http.StatusBadRequest
			if err == infra.ErrWorkflowNotFound {
				status = http.StatusNotFound
			}
			sendError(c, status, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"workflow": workflowDetail(record)})
	})

	r.GET("/workflows/:id/versions/:version", func(c *gin.Context) {
		version, err := strconv.Atoi(c.Param("version"))
		if err != nil || version <= 0 {
			sendError(c, http.StatusBadRequest, "invalid version")
			return
		}
		record, req, err := workflows.LoadVersionRequest(c.Param("id"), version)
		if err != nil {
			status := http.StatusBadRequest
			if err == infra.ErrWorkflowNotFound {
				status = http.StatusNotFound
			}
			sendError(c, status, err.Error())
			return
		}
		sendSuccess(c, map[string]any{
			"workflow_id": record.ID,
			"version":     version,
			"request":     req,
		})
	})

	r.GET("/runs", func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.Query("limit"))
		items, err := runs.List(c.Query("workflow_id"), limit)
		if err != nil {
			sendError(c, http.StatusInternalServerError, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"runs": items})
	})

	r.GET("/runs/:id", func(c *gin.Context) {
		run, err := runs.Get(c.Param("id"))
		if err != nil {
			status := http.StatusInternalServerError
			if err == infra.ErrRunNotFound {
				status = http.StatusNotFound
			}
			sendError(c, status, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"run": run})
	})

	r.POST("/runs/:id/replay", func(c *gin.Context) {
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
			sendError(c, http.StatusBadRequest, "invalid json")
			return
		}
		inputs, err := parseInputData(body["data"])
		if err != nil {
			sendError(c, http.StatusBadRequest, "data must look like {nodeId: [{...}]}")
			return
		}
		outcome, err := runs.Replay(c.Request.Context(), baseDeps, c.Param("id"), inputs)
		if err != nil {
			status := http.StatusInternalServerError
			if err == infra.ErrRunNotFound {
				status = http.StatusNotFound
			}
			sendError(c, status, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"executionId": outcome.Run.ID, "result": outcome.Result, "run": outcome.Run})
	})

	r.GET("/schedules", func(c *gin.Context) {
		items, err := schedules.List()
		if err != nil {
			sendError(c, http.StatusInternalServerError, err.Error())
			return
		}
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			out = append(out, scheduleResponse(item))
		}
		sendSuccess(c, map[string]any{"schedules": out})
	})

	r.POST("/schedules", func(c *gin.Context) {
		var payload struct {
			WorkflowID      string    `json:"workflow_id"`
			Version         int       `json:"version"`
			IntervalSeconds int       `json:"interval_seconds"`
			Data            any       `json:"data"`
			Enabled         *bool     `json:"enabled"`
			NextRunAt       time.Time `json:"next_run_at"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			sendError(c, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		if _, _, err := workflows.LoadVersionRequest(payload.WorkflowID, payload.Version); err != nil {
			status := http.StatusBadRequest
			if err == infra.ErrWorkflowNotFound {
				status = http.StatusNotFound
			}
			sendError(c, status, err.Error())
			return
		}
		inputs, err := parseInputData(payload.Data)
		if err != nil {
			sendError(c, http.StatusBadRequest, "data must look like {nodeId: [{...}]}")
			return
		}
		enabled := true
		if payload.Enabled != nil {
			enabled = *payload.Enabled
		}
		schedule, err := schedules.Create(infra.ScheduleCreate{
			WorkflowID:      payload.WorkflowID,
			WorkflowVersion: payload.Version,
			IntervalSeconds: payload.IntervalSeconds,
			Input:           inputs,
			Enabled:         enabled,
			NextRunAt:       payload.NextRunAt,
		})
		if err != nil {
			sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"schedule": scheduleResponse(schedule)})
	})

	r.GET("/schedules/:id", func(c *gin.Context) {
		schedule, err := schedules.Get(c.Param("id"))
		if err != nil {
			status := http.StatusInternalServerError
			if err == infra.ErrScheduleNotFound {
				status = http.StatusNotFound
			}
			sendError(c, status, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"schedule": scheduleResponse(schedule)})
	})

	r.POST("/schedules/:id/pause", func(c *gin.Context) {
		schedule, err := schedules.Pause(c.Param("id"))
		if err != nil {
			status := http.StatusInternalServerError
			if err == infra.ErrScheduleNotFound {
				status = http.StatusNotFound
			}
			sendError(c, status, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"schedule": scheduleResponse(schedule)})
	})

	r.POST("/schedules/:id/resume", func(c *gin.Context) {
		schedule, err := schedules.Resume(c.Param("id"))
		if err != nil {
			status := http.StatusInternalServerError
			if err == infra.ErrScheduleNotFound {
				status = http.StatusNotFound
			}
			sendError(c, status, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"schedule": scheduleResponse(schedule)})
	})

	r.DELETE("/schedules/:id", func(c *gin.Context) {
		if err := schedules.Delete(c.Param("id")); err != nil {
			status := http.StatusInternalServerError
			if err == infra.ErrScheduleNotFound {
				status = http.StatusNotFound
			}
			sendError(c, status, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"deleted": true})
	})

	r.GET("/reviews", func(c *gin.Context) {
		items, err := reviews.List(c.Request.Context(), reviewStatusFromQuery(c.Query("status")))
		if err != nil {
			sendError(c, http.StatusInternalServerError, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"reviews": items})
	})

	r.GET("/reviews/:id", func(c *gin.Context) {
		review, err := reviews.Get(c.Request.Context(), c.Param("id"))
		if err != nil {
			status := http.StatusInternalServerError
			if err == infra.ErrReviewNotFound {
				status = http.StatusNotFound
			}
			sendError(c, status, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"review": review})
	})

	r.POST("/reviews/:id/approve", func(c *gin.Context) {
		var payload struct {
			Reviewer string `json:"reviewer"`
			Comment  string `json:"comment"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil && !errors.Is(err, io.EOF) {
			sendError(c, http.StatusBadRequest, "invalid json")
			return
		}
		review, err := reviews.Approve(c.Request.Context(), c.Param("id"), payload.Reviewer, payload.Comment)
		if err != nil {
			status := http.StatusInternalServerError
			if err == infra.ErrReviewNotFound {
				status = http.StatusNotFound
			}
			sendError(c, status, err.Error())
			return
		}
		resp := map[string]any{"review": review}
		checkpoint, err := checkpoints.FindActiveByReviewID(review.ID)
		if err == nil {
			outcome, resumeErr := infra.ResumeCheckpoint(c.Request.Context(), baseDeps, runs, checkpoints, checkpoint.ID)
			if resumeErr != nil && outcome.Run.Status != "paused" {
				sendError(c, http.StatusInternalServerError, resumeErr.Error())
				return
			}
			resp["run"] = outcome.Run
			resp["result"] = outcome.Result
			resp["resumed"] = outcome.Run.Status != "paused"
		} else if err != infra.ErrCheckpointNotFound {
			sendError(c, http.StatusInternalServerError, err.Error())
			return
		}
		sendSuccess(c, resp)
	})

	r.POST("/reviews/:id/reject", func(c *gin.Context) {
		var payload struct {
			Reviewer string `json:"reviewer"`
			Comment  string `json:"comment"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil && !errors.Is(err, io.EOF) {
			sendError(c, http.StatusBadRequest, "invalid json")
			return
		}
		review, err := reviews.Reject(c.Request.Context(), c.Param("id"), payload.Reviewer, payload.Comment)
		if err != nil {
			status := http.StatusInternalServerError
			if err == infra.ErrReviewNotFound {
				status = http.StatusNotFound
			}
			sendError(c, status, err.Error())
			return
		}
		resp := map[string]any{"review": review}
		checkpoint, err := checkpoints.FindActiveByReviewID(review.ID)
		if err == nil {
			_, _ = checkpoints.MarkRejected(checkpoint.ID)
			run, cancelErr := runs.MarkCancelled(checkpoint.RunID, "review rejected")
			if cancelErr != nil {
				sendError(c, http.StatusInternalServerError, cancelErr.Error())
				return
			}
			resp["run"] = run
		} else if err != infra.ErrCheckpointNotFound {
			sendError(c, http.StatusInternalServerError, err.Error())
			return
		}
		sendSuccess(c, resp)
	})

	// Instance Manager
	mgr := infra.NewInstanceManager(workflows, runs, checkpoints)

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
			WorkflowID   string `json:"workflow_id"`
			Version      int    `json:"version"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			sendError(c, http.StatusBadRequest, "invalid json")
			return
		}
		var (
			inst *infra.Instance
			err  error
		)
		switch {
		case payload.WorkflowID != "":
			inst, err = mgr.CreateFromWorkflowID(payload.WorkflowID, payload.Version)
		case payload.WorkflowPath != "":
			inst, err = mgr.CreateFromWorkflowPath(payload.WorkflowPath)
		default:
			sendError(c, http.StatusBadRequest, "workflow_path or workflow_id is required")
			return
		}
		if err != nil {
			sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		sendSuccess(c, map[string]interface{}{"id": inst.ID, "state": inst.State, "name": inst.Name, "workflow_id": inst.WorkflowID, "workflow_version": inst.WorkflowVersion})
	})

	r.GET("/instances", func(c *gin.Context) {
		list := mgr.List()
		out := make([]map[string]any, 0, len(list))
		for _, it := range list {
			snapshot := it.Snapshot()
			out = append(out, map[string]any{
				"id":               it.ID,
				"name":             it.Name,
				"state":            it.State,
				"created_at":       it.CreatedAt.Unix(),
				"workflow_path":    it.WorkflowPath,
				"workflow_id":      it.WorkflowID,
				"workflow_version": it.WorkflowVersion,
				"queue_length":     snapshot.QueueLength,
				"is_executing":     snapshot.Active.IsExecuting,
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
			"id":               inst.ID,
			"name":             inst.Name,
			"state":            inst.State,
			"created_at":       inst.CreatedAt.Unix(),
			"workflow_path":    inst.WorkflowPath,
			"workflow_id":      inst.WorkflowID,
			"workflow_version": inst.WorkflowVersion,
			"workflow": map[string]any{
				"id":         inst.Workflow.ID,
				"name":       inst.Workflow.Name,
				"kind":       inst.Workflow.Kind,
				"ai":         inst.Workflow.AI,
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
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil {
			sendError(c, http.StatusBadRequest, "invalid json")
			return
		}
		raw, ok := body["data"]
		if !ok {
			sendError(c, http.StatusBadRequest, "missing data field: expected {data: {...}}")
			return
		}
		inputs, err := parseInputData(raw)
		if err != nil {
			sendError(c, http.StatusBadRequest, "data must look like {nodeId: [{...}]}")
			return
		}
		converted := map[string]model.Items{}
		for k, v := range inputs {
			converted[string(k)] = v
		}
		if err := mgr.Enqueue(id, converted); err != nil {
			sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		sendSuccess(c, map[string]any{"enqueued": true})
	})

	r.GET("/dashboard/metrics", func(c *gin.Context) {
		metrics := mgr.DashboardMetrics()
		sendSuccess(c, map[string]any{"metrics": metrics})
	})

	return r
}
