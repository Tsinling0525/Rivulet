package wasm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

type eventRecorder struct {
	events []string
	fields []map[string]any
}

func (r *eventRecorder) Emit(ctx context.Context, event string, fields map[string]any) error {
	r.events = append(r.events, event)
	r.fields = append(r.fields, fields)
	return nil
}

func TestWASMNodeRunsWASIPluginAndRoutesPorts(t *testing.T) {
	wasmPath := buildWASIPlugin(t)
	bus := &eventRecorder{}
	node := &Node{}
	if err := node.Init(context.Background(), plugin.Deps{Bus: bus}); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	out, err := node.ProcessPorted(context.Background(), model.Workflow{ID: "wf-wasm"}, model.Node{
		ID:   "custom1",
		Type: "wasm:node",
		Config: map[string]any{
			"module": wasmPath,
			"args":   []any{"tag=checked"},
		},
	}, model.Items{{"message": "hello"}})
	if err != nil {
		t.Fatalf("ProcessPorted returned error: %v", err)
	}

	main := out[model.PortMain]
	if len(main) != 1 {
		t.Fatalf("expected 1 main item, got %d", len(main))
	}
	if main[0]["wasm_checked"] != true {
		t.Fatalf("expected wasm_checked=true, got %#v", main[0]["wasm_checked"])
	}
	if main[0]["arg_count"] != float64(2) {
		t.Fatalf("expected arg_count=2 from argv, got %#v", main[0]["arg_count"])
	}
	if len(out[model.Port("pass")]) != 1 {
		t.Fatalf("expected pass port to receive item")
	}
	if len(bus.events) != 1 || bus.events[0] != "wasm_node_call" {
		t.Fatalf("expected wasm_node_call event, got %+v", bus.events)
	}
}

func TestDecodeResponseAcceptsDirectItems(t *testing.T) {
	ports, err := decodeResponse([]byte(`[{"answer":42}]`))
	if err != nil {
		t.Fatalf("decodeResponse returned error: %v", err)
	}
	if len(ports[model.PortMain]) != 1 || ports[model.PortMain][0]["answer"] != float64(42) {
		t.Fatalf("unexpected decoded items: %+v", ports)
	}
}

func TestManifestResolverLoadsCustomNodeType(t *testing.T) {
	wasmPath := buildWASIPlugin(t)
	dir := t.TempDir()
	manifest := `{
  "nodes": [
    {
      "type": "custom:wasm_fixture",
      "module": "` + filepath.ToSlash(wasmPath) + `",
      "args": ["from=manifest"]
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	t.Setenv("RIV_WASM_PLUGIN_DIR", dir)

	handler, ok := plugin.New("custom:wasm_fixture")
	if !ok {
		t.Fatalf("expected manifest resolver to create custom wasm node")
	}
	if err := handler.Init(context.Background(), plugin.Deps{}); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	out, err := handler.Process(context.Background(), model.Workflow{ID: "wf"}, model.Node{
		ID:   "custom1",
		Type: "custom:wasm_fixture",
	}, model.Items{{"message": "manifest"}})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(out) != 1 || out[0]["wasm_checked"] != true {
		t.Fatalf("unexpected output from manifest node: %+v", out)
	}
}

func buildWASIPlugin(t *testing.T) string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go compiler not available for WASI fixture")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "plugin.go")
	wasmPath := filepath.Join(dir, "plugin.wasm")
	if err := os.WriteFile(src, []byte(wasiPluginSource), 0o644); err != nil {
		t.Fatalf("write fixture source: %v", err)
	}
	cmd := exec.Command(goBin, "build", "-o", wasmPath, src)
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("go could not build WASI fixture: %v\n%s", err, string(out))
	}
	return wasmPath
}

const wasiPluginSource = `package main

import (
	"encoding/json"
	"io"
	"os"
)

type request struct {
	Config map[string]any   ` + "`json:\"config\"`" + `
	Items  []map[string]any ` + "`json:\"items\"`" + `
}

func main() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fail(err)
	}
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		fail(err)
	}
	for _, item := range req.Items {
		item["wasm_checked"] = true
		item["arg_count"] = len(os.Args)
		item["module_config_seen"] = req.Config["module"] != ""
	}
	resp := map[string]any{
		"ports": map[string]any{
			"main": req.Items,
			"pass": req.Items,
		},
	}
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}

func fail(err error) {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": err.Error()})
	os.Exit(1)
}
`
