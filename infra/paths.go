package infra

import (
	"os"
	"path/filepath"
)

// DataDir returns the base directory to persist data. Defaults to ./data
func DataDir() string {
	if v := os.Getenv("RIV_DATA_DIR"); v != "" {
		return v
	}
	return "data"
}

func ensureDir(path string) error { return os.MkdirAll(path, 0o755) }

// WorkflowsDir is the directory storing workflow JSON files
func WorkflowsDir() string { return filepath.Join(DataDir(), "workflows") }

// ScriptsDir is the directory to store Python scripts
func ScriptsDir() string { return filepath.Join(DataDir(), "scripts") }

// FilesDir returns directory for attachments under a workflow
func FilesDir(workflowID string) string { return filepath.Join(DataDir(), "files", workflowID) }
