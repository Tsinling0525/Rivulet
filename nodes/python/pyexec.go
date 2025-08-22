package python

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

// ScriptNode runs a local python script against an attached file and emits the script stdout as text
// Config:
// - script: string (required) absolute or relative path to the python script
// - args: []string (optional) additional args passed before the input file path
// - file_id_field: string (optional, default: "file_id") item field containing FileStore file ID
// - output_field: string (optional, default: "latex") item field to write stdout
// - python_bin: string (optional, default: "python3") interpreter to use
// - workdir: string (optional) process working directory
type ScriptNode struct {
	deps plugin.Deps
}

func (n *ScriptNode) Init(ctx context.Context, deps plugin.Deps) error {
	n.deps = deps
	return nil
}

func (n *ScriptNode) Process(ctx context.Context, wf model.Workflow, node model.Node, in model.Items) (model.Items, error) {
	if n.deps.Files == nil {
		return nil, fmt.Errorf("files store not configured")
	}

	scriptPath, _ := node.Config["script"].(string)
	if scriptPath == "" {
		return nil, fmt.Errorf("config.script is required")
	}

	pythonBin, _ := node.Config["python_bin"].(string)
	if pythonBin == "" {
		pythonBin = "python3"
	}

	workdir, _ := node.Config["workdir"].(string)

	var extraArgs []string
	if raw, ok := node.Config["args"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				extraArgs = append(extraArgs, s)
			}
		}
	}

	fileIDField := "file_id"
	if s, ok := node.Config["file_id_field"].(string); ok && s != "" {
		fileIDField = s
	}
	outField := "latex"
	if s, ok := node.Config["output_field"].(string); ok && s != "" {
		outField = s
	}

	// Prepare temp dir per node execution
	tempBase := os.TempDir()
	execDir := filepath.Join(tempBase, fmt.Sprintf("rivulet_py_%d", time.Now().UnixNano()))
	if err := os.MkdirAll(execDir, 0o755); err != nil {
		return nil, err
	}

	outItems := make(model.Items, 0, len(in))

	for _, item := range in {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if item == nil {
			item = model.Item{}
		}

		// fetch file by id
		rawID, ok := item[fileIDField]
		if !ok {
			return nil, fmt.Errorf("missing %s in item", fileIDField)
		}
		fileID, ok := rawID.(string)
		if !ok || strings.TrimSpace(fileID) == "" {
			return nil, fmt.Errorf("%s must be string", fileIDField)
		}

		name, _, content, err := n.deps.Files.Get(ctx, string(wf.ID), fileID)
		if err != nil {
			return nil, fmt.Errorf("get file %s: %w", fileID, err)
		}

		// write temp input file preserving extension if any
		ext := filepath.Ext(name)
		if ext == "" {
			ext = ".bin"
		}
		inputPath := filepath.Join(execDir, fmt.Sprintf("input_%s%s", fileID, ext))
		if err := os.WriteFile(inputPath, content, 0o644); err != nil {
			return nil, err
		}

		// build command: pythonBin scriptPath [args...] inputPath
		args := append([]string{scriptPath}, append(extraArgs, inputPath)...)
		cmd := exec.CommandContext(ctx, pythonBin, args...)
		if workdir != "" {
			cmd.Dir = workdir
		}
		// Inherit minimal env
		cmd.Env = os.Environ()

		outBytes, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("python script failed: %w; output: %s", err, string(outBytes))
		}

		// store stdout into item
		enriched := model.Item{}
		for k, v := range item {
			enriched[k] = v
		}
		enriched[outField] = strings.TrimSpace(string(outBytes))
		enriched["script"] = scriptPath
		enriched["python_bin"] = pythonBin
		outItems = append(outItems, enriched)
	}

	return outItems, nil
}

func init() { plugin.Register("python:script", func() plugin.NodeHandler { return &ScriptNode{} }) }
