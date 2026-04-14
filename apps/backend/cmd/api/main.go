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
	fmt.Printf("   POST   /workflow/start         - Run a workflow immediately\n")
	fmt.Printf("   GET    /workflows/files        - List workflow JSON files\n")
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
