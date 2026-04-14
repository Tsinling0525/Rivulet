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
	r := server.NewRouter()

	port := os.Getenv("RIV_API_PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Rivulet flowd listening on :%s\n", port)
	fmt.Printf("Dashboard: http://localhost:%s/\n", port)

	srv := &http.Server{Addr: ":" + port, Handler: r}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("server error: %v\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
