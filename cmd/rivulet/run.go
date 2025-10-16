
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Tsinling0525/rivulet/engine"
	"github.com/Tsinling0525/rivulet/format/n8n"
	"github.com/Tsinling0525/rivulet/infra"
	apiinfra "github.com/Tsinling0525/rivulet/infra/api"
	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

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
	return plugin.Deps{State: apiinfra.MemState{}, Bus: apiinfra.NullBus{}, Files: infra.NewLocalFiles()}
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
