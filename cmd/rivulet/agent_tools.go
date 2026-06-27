package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Tsinling0525/rivulet/agent"
)

const (
	defaultFileReadLimit = 20000
	maxToolOutputBytes   = 24000
)

func newCodingToolRegistry(cwd string, out io.Writer) *agent.Registry {
	root, err := filepath.Abs(cwd)
	if err != nil {
		root = cwd
	}
	return agent.NewRegistry(
		newListFilesTool(root, out),
		newReadFileTool(root, out),
		newEditFileTool(root, out),
		newWriteFileTool(root, out),
		newShellTool(root, out),
	)
}

func newListFilesTool(root string, out io.Writer) agent.Tool {
	return agent.NewToolFunc("list_files", func(ctx context.Context, call agent.ToolCall) (agent.Observation, error) {
		path, _ := stringArg(call.Args, "path")
		if strings.TrimSpace(path) == "" {
			path = "."
		}
		dir, err := resolveWorkspacePath(root, path)
		if err != nil {
			return agent.Observation{}, err
		}
		recursive, _ := boolArg(call.Args, "recursive")
		maxEntries, _ := intArg(call.Args, "max_entries")
		if maxEntries <= 0 {
			maxEntries = 100
		}
		fmt.Fprintf(out, "tool:list_files %s\n", displayPath(root, dir))

		entries := make([]string, 0, maxEntries)
		if recursive {
			err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if path == dir {
					return nil
				}
				if entry.IsDir() && shouldSkipDir(entry.Name()) {
					return filepath.SkipDir
				}
				entries = append(entries, displayPath(root, path))
				if len(entries) >= maxEntries {
					return filepath.SkipAll
				}
				return nil
			})
		} else {
			var raw []os.DirEntry
			raw, err = os.ReadDir(dir)
			for _, entry := range raw {
				name := entry.Name()
				if entry.IsDir() {
					name += "/"
				}
				entries = append(entries, name)
				if len(entries) >= maxEntries {
					break
				}
			}
		}
		if err != nil {
			return agent.Observation{}, err
		}
		return agent.Observation{
			ToolName: "list_files",
			Summary:  fmt.Sprintf("listed %d entries", len(entries)),
			Output:   map[string]any{"entries": entries},
		}, nil
	})
}

func newReadFileTool(root string, out io.Writer) agent.Tool {
	return agent.NewToolFunc("read_file", func(ctx context.Context, call agent.ToolCall) (agent.Observation, error) {
		path, ok := stringArg(call.Args, "path")
		if !ok {
			return agent.Observation{}, fmt.Errorf("path is required")
		}
		resolved, err := resolveWorkspacePath(root, path)
		if err != nil {
			return agent.Observation{}, err
		}
		offset, _ := intArg(call.Args, "offset")
		limit, _ := intArg(call.Args, "limit")
		if limit <= 0 || limit > defaultFileReadLimit {
			limit = defaultFileReadLimit
		}
		fmt.Fprintf(out, "tool:read_file %s\n", displayPath(root, resolved))

		data, err := os.ReadFile(resolved)
		if err != nil {
			return agent.Observation{}, err
		}
		if offset < 0 {
			offset = 0
		}
		if offset > len(data) {
			offset = len(data)
		}
		end := offset + limit
		if end > len(data) {
			end = len(data)
		}
		content := string(data[offset:end])
		return agent.Observation{
			ToolName: "read_file",
			Summary:  fmt.Sprintf("read %d bytes from %s", len(content), displayPath(root, resolved)),
			Output: map[string]any{
				"path":      displayPath(root, resolved),
				"content":   content,
				"offset":    offset,
				"truncated": end < len(data),
			},
		}, nil
	})
}

func newEditFileTool(root string, out io.Writer) agent.Tool {
	return agent.NewToolFunc("edit_file", func(ctx context.Context, call agent.ToolCall) (agent.Observation, error) {
		path, ok := stringArg(call.Args, "path")
		if !ok {
			return agent.Observation{}, fmt.Errorf("path is required")
		}
		oldText, ok := stringArg(call.Args, "old")
		if !ok {
			return agent.Observation{}, fmt.Errorf("old is required")
		}
		newText, ok := stringArg(call.Args, "new")
		if !ok {
			return agent.Observation{}, fmt.Errorf("new is required")
		}
		replaceAll, _ := boolArg(call.Args, "replace_all")
		resolved, err := resolveWorkspacePath(root, path)
		if err != nil {
			return agent.Observation{}, err
		}
		fmt.Fprintf(out, "tool:edit_file %s\n", displayPath(root, resolved))

		data, err := os.ReadFile(resolved)
		if err != nil {
			return agent.Observation{}, err
		}
		content := string(data)
		count := strings.Count(content, oldText)
		if count == 0 {
			return agent.Observation{}, fmt.Errorf("old text not found in %s", displayPath(root, resolved))
		}
		n := 1
		if replaceAll {
			n = -1
		}
		updated := strings.Replace(content, oldText, newText, n)
		if err := os.WriteFile(resolved, []byte(updated), 0o644); err != nil {
			return agent.Observation{}, err
		}
		replacements := 1
		if replaceAll {
			replacements = count
		}
		return agent.Observation{
			ToolName: "edit_file",
			Summary:  fmt.Sprintf("updated %s (%d replacement(s))", displayPath(root, resolved), replacements),
			Output: map[string]any{
				"path":         displayPath(root, resolved),
				"replacements": replacements,
			},
		}, nil
	})
}

func newWriteFileTool(root string, out io.Writer) agent.Tool {
	return agent.NewToolFunc("write_file", func(ctx context.Context, call agent.ToolCall) (agent.Observation, error) {
		path, ok := stringArg(call.Args, "path")
		if !ok {
			return agent.Observation{}, fmt.Errorf("path is required")
		}
		content, ok := stringArg(call.Args, "content")
		if !ok {
			return agent.Observation{}, fmt.Errorf("content is required")
		}
		appendMode, _ := boolArg(call.Args, "append")
		resolved, err := resolveWorkspacePath(root, path)
		if err != nil {
			return agent.Observation{}, err
		}
		fmt.Fprintf(out, "tool:write_file %s\n", displayPath(root, resolved))

		if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
			return agent.Observation{}, err
		}
		flag := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		if appendMode {
			flag = os.O_CREATE | os.O_WRONLY | os.O_APPEND
		}
		f, err := os.OpenFile(resolved, flag, 0o644)
		if err != nil {
			return agent.Observation{}, err
		}
		defer f.Close()
		if _, err := f.WriteString(content); err != nil {
			return agent.Observation{}, err
		}
		return agent.Observation{
			ToolName: "write_file",
			Summary:  fmt.Sprintf("wrote %d bytes to %s", len(content), displayPath(root, resolved)),
			Output:   map[string]any{"path": displayPath(root, resolved), "bytes": len(content)},
		}, nil
	})
}

func newShellTool(root string, out io.Writer) agent.Tool {
	return agent.NewToolFunc("shell", func(ctx context.Context, call agent.ToolCall) (agent.Observation, error) {
		command, ok := stringArg(call.Args, "command")
		if !ok {
			return agent.Observation{}, fmt.Errorf("command is required")
		}
		timeoutSeconds, _ := intArg(call.Args, "timeout_seconds")
		if timeoutSeconds <= 0 {
			timeoutSeconds = 30
		}
		if timeoutSeconds > 300 {
			timeoutSeconds = 300
		}
		fmt.Fprintf(out, "tool:shell %s\n", command)

		runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
		defer cancel()
		cmd := exec.CommandContext(runCtx, "sh", "-lc", command)
		cmd.Dir = root
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		err := cmd.Run()
		text := truncateText(output.String(), maxToolOutputBytes)
		obs := agent.Observation{
			ToolName: "shell",
			Summary:  fmt.Sprintf("ran command: %s", command),
			Output: map[string]any{
				"command": command,
				"output":  text,
			},
		}
		if runCtx.Err() == context.DeadlineExceeded {
			return obs, fmt.Errorf("command timed out after %ds", timeoutSeconds)
		}
		if err != nil {
			return obs, err
		}
		return obs, nil
	})
}

func resolveWorkspacePath(root, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("path is required")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path := raw
	if !filepath.IsAbs(path) {
		path = filepath.Join(rootAbs, path)
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, resolved)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %q is outside workspace %q", raw, rootAbs)
	}
	return resolved, nil
}

func displayPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return path
	}
	return rel
}

func stringArg(args map[string]any, key string) (string, bool) {
	if args == nil {
		return "", false
	}
	value, ok := args[key]
	if !ok {
		return "", false
	}
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return "", false
		}
		return v, true
	default:
		return fmt.Sprint(v), true
	}
}

func boolArg(args map[string]any, key string) (bool, bool) {
	if args == nil {
		return false, false
	}
	value, ok := args[key]
	if !ok {
		return false, false
	}
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		parsed, err := strconv.ParseBool(v)
		return parsed, err == nil
	default:
		return false, false
	}
}

func intArg(args map[string]any, key string) (int, bool) {
	if args == nil {
		return 0, false
	}
	value, ok := args[key]
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		parsed, err := strconv.Atoi(v)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "bin", "node_modules", "vendor":
		return true
	default:
		return false
	}
}

func truncateText(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 32 {
		return value[:limit]
	}
	head := limit / 2
	tail := limit - head - len("\n...[truncated]...\n")
	if tail <= 0 {
		return value[:limit]
	}
	return value[:head] + "\n...[truncated]...\n" + value[len(value)-tail:]
}
