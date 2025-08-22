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
	fmt.Printf("   GET    /health              - Health check\n")
	fmt.Printf("   POST   /workflow/start      - Start a workflow (legacy)\n")
	fmt.Printf("   POST   /workflows           - Create workflow\n")
	fmt.Printf("   PUT    /workflows/:id       - Update workflow\n")
	fmt.Printf("   DELETE /workflows/:id       - Delete workflow\n")
	fmt.Printf("   POST   /workflows/:id/start - Start existing workflow\n")
	fmt.Printf("🔗 n8n-compatible API with Gin ready!\n")

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
