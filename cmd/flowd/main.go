package main

import (
	"context"
	"fmt"
	"time"

	// register node
	"rivulet/engine"
	"rivulet/model"
	"rivulet/plugin"
)

type nullBus struct{}

func (n nullBus) Emit(ctx context.Context, event string, fields map[string]any) error { return nil }

type memState struct{}

func (m memState) SaveNodeState(context.Context, string, model.ID, map[string]any) error { return nil }
func (m memState) LoadNodeState(context.Context, string, model.ID) (map[string]any, error) {
	return map[string]any{}, nil
}

func main() {
	deps := plugin.Deps{State: memState{}, Bus: nullBus{}}
	eng := engine.New(deps)

	wf := model.Workflow{
		ID:    "wf1",
		Name:  "EchoFlow",
		Nodes: []model.Node{{ID: "n1", Type: "echo", Name: "Echo", Timeout: 2 * time.Second, Config: map[string]any{"label": "hi"}}},
	}

	res, err := eng.Run(context.Background(), "exec-001", wf, map[model.ID]model.Items{"n1": {{"msg": "hello"}}})
	if err != nil {
		panic(err)
	}
	fmt.Println(res)
}
