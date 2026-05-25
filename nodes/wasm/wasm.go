package wasm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

const defaultTimeout = 30 * time.Second

type Node struct {
	deps     plugin.Deps
	defaults map[string]any
}

type wasmRequest struct {
	Workflow wasmWorkflow   `json:"workflow"`
	Node     wasmNode       `json:"node"`
	Config   map[string]any `json:"config,omitempty"`
	Items    model.Items    `json:"items"`
}

type wasmWorkflow struct {
	ID   model.ID           `json:"id,omitempty"`
	Name string             `json:"name,omitempty"`
	Kind model.WorkflowKind `json:"kind,omitempty"`
}

type wasmNode struct {
	ID   model.ID `json:"id,omitempty"`
	Name string   `json:"name,omitempty"`
	Type string   `json:"type,omitempty"`
}

type wasmResponse struct {
	Items model.Items                `json:"items,omitempty"`
	Ports map[model.Port]model.Items `json:"ports,omitempty"`
	Error string                     `json:"error,omitempty"`
}

type moduleConfig struct {
	Path           string
	Args           []string
	Env            map[string]string
	Mounts         []mountConfig
	Timeout        time.Duration
	MaxMemoryPages uint32
}

type mountConfig struct {
	Host     string `json:"host"`
	Guest    string `json:"guest"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

type pluginManifest struct {
	Nodes []manifestNode `json:"nodes"`
}

type manifestNode struct {
	Type           string            `json:"type"`
	Module         string            `json:"module"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Mounts         []mountConfig     `json:"mounts,omitempty"`
	TimeoutSeconds float64           `json:"timeout_seconds,omitempty"`
	MaxMemoryPages uint32            `json:"max_memory_pages,omitempty"`
}

func (n *Node) Init(ctx context.Context, deps plugin.Deps) error {
	n.deps = deps
	return nil
}

func (n *Node) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
	ports, err := n.ProcessPorted(ctx, wf, node, in)
	if err != nil {
		return nil, err
	}
	return ports[model.PortMain], nil
}

func (n *Node) ProcessPorted(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (map[model.Port]model.Items, error) {
	nodeConfig := mergeConfig(n.defaults, node.Config)
	cfg, err := parseModuleConfig(nodeConfig)
	if err != nil {
		return nil, err
	}
	ports, stderr, duration, err := runWASI(ctx, cfg, nodeConfig, wf, node, in)
	if err != nil {
		n.emit(ctx, wf, node, false, duration, stderr, err)
		return nil, err
	}
	n.emit(ctx, wf, node, true, duration, stderr, nil)
	return ports, nil
}

func runWASI(ctx context.Context, cfg moduleConfig, nodeConfig map[string]any, wf model.Workflow, node model.Node, in model.Items) (map[model.Port]model.Items, string, time.Duration, error) {
	wasmBytes, err := os.ReadFile(cfg.Path)
	if err != nil {
		return nil, "", 0, err
	}
	req := wasmRequest{
		Workflow: wasmWorkflow{ID: wf.ID, Name: wf.Name, Kind: wf.Kind},
		Node:     wasmNode{ID: node.ID, Name: node.Name, Type: node.Type},
		Config:   publicConfig(nodeConfig),
		Items:    cloneItems(in),
	}
	stdin, err := json.Marshal(req)
	if err != nil {
		return nil, "", 0, err
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	runtimeConfig := wazero.NewRuntimeConfig().WithCloseOnContextDone(true)
	if cfg.MaxMemoryPages > 0 {
		runtimeConfig = runtimeConfig.WithMemoryLimitPages(cfg.MaxMemoryPages)
	}
	r := wazero.NewRuntimeWithConfig(runCtx, runtimeConfig)
	defer r.Close(runCtx)
	if _, err := wasi_snapshot_preview1.Instantiate(runCtx, r); err != nil {
		return nil, "", 0, err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	moduleCfg := wazero.NewModuleConfig().
		WithName("").
		WithArgs(append([]string{filepath.Base(cfg.Path)}, cfg.Args...)...).
		WithStdin(bytes.NewReader(stdin)).
		WithStdout(&stdout).
		WithStderr(&stderr).
		WithSysWalltime().
		WithSysNanotime().
		WithSysNanosleep()
	for key, value := range cfg.Env {
		moduleCfg = moduleCfg.WithEnv(key, value)
	}
	if len(cfg.Mounts) > 0 {
		fsCfg := wazero.NewFSConfig()
		for _, mount := range cfg.Mounts {
			if mount.ReadOnly {
				fsCfg = fsCfg.WithReadOnlyDirMount(mount.Host, mount.Guest)
			} else {
				fsCfg = fsCfg.WithDirMount(mount.Host, mount.Guest)
			}
		}
		moduleCfg = moduleCfg.WithFSConfig(fsCfg)
	}

	start := time.Now()
	_, err = r.InstantiateWithConfig(runCtx, wasmBytes, moduleCfg)
	duration := time.Since(start)
	if err != nil {
		return nil, stderr.String(), duration, fmt.Errorf("wasm module failed: %w", err)
	}
	ports, err := decodeResponse(stdout.Bytes())
	if err != nil {
		if text := strings.TrimSpace(stderr.String()); text != "" {
			return nil, text, duration, fmt.Errorf("%w; stderr=%s", err, text)
		}
		return nil, stderr.String(), duration, err
	}
	return ports, stderr.String(), duration, nil
}

func decodeResponse(raw []byte) (map[model.Port]model.Items, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return map[model.Port]model.Items{model.PortMain: {}}, nil
	}
	var directItems model.Items
	if err := json.Unmarshal(raw, &directItems); err == nil {
		return map[model.Port]model.Items{model.PortMain: directItems}, nil
	}
	var single model.Item
	if err := json.Unmarshal(raw, &single); err == nil {
		if _, hasItems := single["items"]; !hasItems {
			if _, hasPorts := single["ports"]; !hasPorts {
				if _, hasError := single["error"]; !hasError {
					return map[model.Port]model.Items{model.PortMain: {single}}, nil
				}
			}
		}
	}
	var resp wasmResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("wasm stdout must be JSON array, item, or response object: %w", err)
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	ports := map[model.Port]model.Items{}
	for port, items := range resp.Ports {
		ports[port] = cloneItems(items)
	}
	if resp.Items != nil {
		ports[model.PortMain] = cloneItems(resp.Items)
	}
	if _, ok := ports[model.PortMain]; !ok {
		ports[model.PortMain] = model.Items{}
	}
	return ports, nil
}

func parseModuleConfig(raw map[string]any) (moduleConfig, error) {
	if raw == nil {
		return moduleConfig{}, errors.New("wasm node requires module path")
	}
	path := firstString(raw, "module", "path", "wasm_path")
	if path == "" {
		return moduleConfig{}, errors.New("wasm node requires module, path, or wasm_path")
	}
	path = os.ExpandEnv(path)
	if !filepath.IsAbs(path) {
		path = filepath.Clean(path)
	}
	timeout := durationSeconds(raw["timeout_seconds"], defaultTimeout)
	maxPages := uint32(0)
	if pages, ok := floatValue(raw["max_memory_pages"]); ok && pages > 0 {
		maxPages = uint32(pages)
	}
	return moduleConfig{
		Path:           path,
		Args:           stringSlice(raw["args"]),
		Env:            stringMap(raw["env"]),
		Mounts:         parseMounts(raw["mounts"]),
		Timeout:        timeout,
		MaxMemoryPages: maxPages,
	}, nil
}

func parseMounts(raw any) []mountConfig {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]mountConfig, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		host := os.ExpandEnv(firstString(m, "host", "path"))
		guest := firstString(m, "guest", "guest_path")
		if host == "" || guest == "" {
			continue
		}
		readOnly, _ := m["read_only"].(bool)
		out = append(out, mountConfig{Host: host, Guest: guest, ReadOnly: readOnly})
	}
	return out
}

func publicConfig(cfg map[string]any) map[string]any {
	out := make(map[string]any, len(cfg))
	for key, value := range cfg {
		if strings.HasPrefix(key, "_") {
			continue
		}
		switch key {
		case "env", "mounts":
			continue
		default:
			out[key] = value
		}
	}
	return out
}

func (n *Node) emit(ctx context.Context, wf model.Workflow, node model.Node, ok bool, duration time.Duration, stderr string, err error) {
	if n.deps.Bus == nil {
		return
	}
	fields := map[string]any{
		"exec":          plugin.ExecutionIDFromContext(ctx),
		"workflow":      wf.ID,
		"workflow_kind": wf.Kind,
		"node":          node.ID,
		"module":        firstString(mergeConfig(n.defaults, node.Config), "module", "path", "wasm_path"),
		"status":        "succeeded",
		"duration_ms":   duration.Milliseconds(),
	}
	if !ok {
		fields["status"] = "failed"
	}
	if stderr = strings.TrimSpace(stderr); stderr != "" {
		fields["stderr"] = truncate(stderr, 500)
	}
	if err != nil {
		fields["error"] = err.Error()
	}
	_ = n.deps.Bus.Emit(ctx, "wasm_node_call", fields)
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func stringSlice(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, fmt.Sprint(item))
		}
		return out
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	default:
		return nil
	}
}

func stringMap(raw any) map[string]string {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for key, value := range m {
		out[key] = fmt.Sprint(value)
	}
	return out
}

func durationSeconds(raw any, def time.Duration) time.Duration {
	if value, ok := floatValue(raw); ok {
		return time.Duration(value * float64(time.Second))
	}
	return def
}

func floatValue(raw any) (float64, bool) {
	switch v := raw.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case json.Number:
		value, err := v.Float64()
		return value, err == nil
	default:
		return 0, false
	}
}

func cloneItems(items model.Items) model.Items {
	out := make(model.Items, len(items))
	for i, item := range items {
		if item == nil {
			out[i] = model.Item{}
			continue
		}
		copied := make(model.Item, len(item))
		for key, value := range item {
			copied[key] = value
		}
		out[i] = copied
	}
	return out
}

func mergeConfig(defaults, overrides map[string]any) map[string]any {
	if len(defaults) == 0 {
		return overrides
	}
	out := make(map[string]any, len(defaults)+len(overrides))
	for key, value := range defaults {
		out[key] = value
	}
	for key, value := range overrides {
		out[key] = value
	}
	return out
}

func truncate(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "..."
}

func resolveManifestNode(nodeType string) (plugin.NodeHandler, bool) {
	dir := os.Getenv("RIV_WASM_PLUGIN_DIR")
	if dir == "" {
		dataDir := os.Getenv("RIV_DATA_DIR")
		if dataDir == "" {
			dataDir = "data"
		}
		dir = filepath.Join(dataDir, "plugins")
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, false
	}
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var manifest pluginManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			continue
		}
		baseDir := filepath.Dir(path)
		for _, node := range manifest.Nodes {
			if node.Type != nodeType || node.Module == "" {
				continue
			}
			modulePath := os.ExpandEnv(node.Module)
			if !filepath.IsAbs(modulePath) {
				modulePath = filepath.Join(baseDir, modulePath)
			}
			defaults := map[string]any{"module": modulePath}
			if len(node.Args) > 0 {
				defaults["args"] = node.Args
			}
			if len(node.Env) > 0 {
				env := map[string]any{}
				for key, value := range node.Env {
					env[key] = value
				}
				defaults["env"] = env
			}
			if len(node.Mounts) > 0 {
				mounts := make([]any, 0, len(node.Mounts))
				for _, mount := range node.Mounts {
					host := os.ExpandEnv(mount.Host)
					if !filepath.IsAbs(host) {
						host = filepath.Join(baseDir, host)
					}
					mounts = append(mounts, map[string]any{
						"host":      host,
						"guest":     mount.Guest,
						"read_only": mount.ReadOnly,
					})
				}
				defaults["mounts"] = mounts
			}
			if node.TimeoutSeconds > 0 {
				defaults["timeout_seconds"] = node.TimeoutSeconds
			}
			if node.MaxMemoryPages > 0 {
				defaults["max_memory_pages"] = node.MaxMemoryPages
			}
			return &Node{defaults: defaults}, true
		}
	}
	return nil, false
}

func init() {
	plugin.Register("wasm:node", func() plugin.NodeHandler { return &Node{} })
	plugin.Register("wasm", func() plugin.NodeHandler { return &Node{} })
	plugin.RegisterResolver(resolveManifestNode)
}
