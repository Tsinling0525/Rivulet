package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Tsinling0525/rivulet/cmd/api/server"
	"github.com/Tsinling0525/rivulet/engine"
	"github.com/Tsinling0525/rivulet/format/n8n"
	"github.com/Tsinling0525/rivulet/infra"
	apiinfra "github.com/Tsinling0525/rivulet/infra/api"
	"github.com/Tsinling0525/rivulet/model"
	_ "github.com/Tsinling0525/rivulet/nodes/echo"
	_ "github.com/Tsinling0525/rivulet/nodes/http"
	_ "github.com/Tsinling0525/rivulet/nodes/logic"
	_ "github.com/Tsinling0525/rivulet/nodes/merge"
	_ "github.com/Tsinling0525/rivulet/nodes/ollama"
	"github.com/Tsinling0525/rivulet/plugin"
)

func runServer() error {
	r := server.NewRouter()
	port := os.Getenv("RIV_API_PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Printf("🚀 Starting Rivulet API Server on :%s\n", port)
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
	return srv.Shutdown(ctx)
}

func runFlowFromFile(path string) error {
	f, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var req n8n.N8nRequest
	if err := json.Unmarshal(f, &req); err != nil {
		return err
	}
	wf, inputs := n8n.ToRivulet(req)
	deps := pluginDeps()
	eng := engine.New(deps)
	execID := fmt.Sprintf("exec-%d", time.Now().UnixNano())
	res, err := eng.Run(context.Background(), execID, wf, inputs)
	if err != nil {
		return err
	}
	fmt.Printf("✅ Execution %s result: %+v\n", execID, res)
	return nil
}

func pluginDeps() plugin.Deps {
	return plugin.Deps{State: apiinfra.MemState{}, Bus: apiinfra.NullBus{}, Files: infra.NewMemFiles()}
}

func runEchoSample() error {
	deps := pluginDeps()
	eng := engine.New(deps)
	wf := model.Workflow{
		ID:   "wf_echo",
		Name: "EchoFlow",
		Nodes: []model.Node{{
			ID: "n1", Type: "echo", Name: "Echo", Timeout: 2 * time.Second,
			Config: map[string]any{"label": "hello"},
		}},
	}
	inputs := map[model.ID]model.Items{"n1": {{"msg": "hello world"}}}
	res, err := eng.Run(context.Background(), "exec-echo-001", wf, inputs)
	if err != nil {
		return err
	}
	fmt.Printf("✅ Result: %+v\n", res)
	return nil
}

func main() {
	if len(os.Args) < 2 {
		_ = runServer()
		return
	}
	sub := os.Args[1]
	switch sub {
	case "server":
		_ = runServer()
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		file := fs.String("file", "", "Path to n8n workflow JSON")
		_ = fs.Parse(os.Args[2:])
		if *file == "" {
			fmt.Println("--file is required")
			os.Exit(2)
		}
		if err := runFlowFromFile(*file); err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
	default:
		fmt.Println("Usage:")
		fmt.Println("  rivulet server             # start API server")
		fmt.Println("  rivulet run --file path    # run workflow JSON once")
	}
}
