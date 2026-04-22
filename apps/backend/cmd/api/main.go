package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Tsinling0525/rivulet/cmd/api/server"
	_ "github.com/Tsinling0525/rivulet/nodes/review"
)

func main() {
	// Setup router via server package
	r := server.NewRouter()

	port := os.Getenv("RIV_API_PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Printf("🚀 Starting Rivulet API Server with Gin on port %s\n", port)
	fmt.Printf("📡 Endpoints:\n")
	fmt.Printf("   GET    /health                 - Health check\n")
	fmt.Printf("   POST   /api/chat/ollama        - Chat with local Ollama\n")
	fmt.Printf("   POST   /research/arxiv-papers  - Search arXiv papers\n")
	fmt.Printf("   POST   /research/summarize-papers - Summarize papers with local Gemma4\n")
	fmt.Printf("   POST   /research/agentic-papers - Search arXiv and summarize with local Gemma4\n")
	fmt.Printf("   POST   /workflow/start         - Run a workflow immediately\n")
	fmt.Printf("   GET    /workflows/files        - List workflow JSON files\n")
	fmt.Printf("   GET    /workflows              - List persisted workflows\n")
	fmt.Printf("   POST   /workflows              - Create a persisted workflow\n")
	fmt.Printf("   GET    /workflows/:id          - Inspect one persisted workflow\n")
	fmt.Printf("   POST   /workflows/:id/versions - Create a new workflow version\n")
	fmt.Printf("   POST   /workflows/:id/activate - Activate a workflow version\n")
	fmt.Printf("   POST   /workflows/:id/prompts/:node_id/rollback - Roll back a prompt by hash\n")
	fmt.Printf("   GET    /runs                   - List persisted runs\n")
	fmt.Printf("   GET    /runs/:id               - Inspect one persisted run\n")
	fmt.Printf("   POST   /runs/:id/replay        - Replay a persisted run\n")
	fmt.Printf("   GET    /schedules              - List workflow schedules\n")
	fmt.Printf("   POST   /schedules              - Create a workflow schedule\n")
	fmt.Printf("   GET    /schedules/:id          - Inspect one workflow schedule\n")
	fmt.Printf("   POST   /schedules/:id/pause    - Pause a workflow schedule\n")
	fmt.Printf("   POST   /schedules/:id/resume   - Resume a workflow schedule\n")
	fmt.Printf("   DELETE /schedules/:id          - Delete a workflow schedule\n")
	fmt.Printf("   GET    /reviews                - List human review requests\n")
	fmt.Printf("   GET    /reviews/:id            - Inspect one review request\n")
	fmt.Printf("   POST   /reviews/:id/approve    - Approve one review request\n")
	fmt.Printf("   POST   /reviews/:id/reject     - Reject one review request\n")
	fmt.Printf("   POST   /instances              - Create a managed workflow instance\n")
	fmt.Printf("   GET    /instances              - List workflow instances\n")
	fmt.Printf("   GET    /instances/:id          - Inspect one workflow instance\n")
	fmt.Printf("   POST   /instances/:id/stop     - Stop a workflow instance\n")
	fmt.Printf("   GET    /instances/:id/logs     - Read workflow instance logs\n")
	fmt.Printf("   POST   /instances/:id/enqueue  - Enqueue execution data\n")
	fmt.Printf("   GET    /dashboard/metrics      - Dashboard metrics\n")
	fmt.Printf("🌐 Dashboard: http://localhost:%s/\n", port)

	srv := &http.Server{Addr: ":" + port, Handler: r}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("server error: %v\n", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
