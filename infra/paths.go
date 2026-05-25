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

// StoreDir is the base directory for file-backed metadata persistence.
func StoreDir() string { return filepath.Join(DataDir(), "store") }

// WorkflowStoreDir is the directory storing versioned workflow definitions.
func WorkflowStoreDir() string { return filepath.Join(StoreDir(), "workflows") }

// RunHistoryDir is the directory storing persisted execution history.
func RunHistoryDir() string { return filepath.Join(StoreDir(), "runs") }

// ScheduleStoreDir is the directory storing persisted workflow schedules.
func ScheduleStoreDir() string { return filepath.Join(StoreDir(), "schedules") }

// ReviewStoreDir is the directory storing human review requests.
func ReviewStoreDir() string { return filepath.Join(StoreDir(), "reviews") }

// CheckpointStoreDir is the directory storing paused execution checkpoints.
func CheckpointStoreDir() string { return filepath.Join(StoreDir(), "checkpoints") }

// ScriptsDir is the directory to store Python scripts
func ScriptsDir() string { return filepath.Join(DataDir(), "scripts") }

// FilesDir returns directory for attachments under a workflow
func FilesDir(workflowID string) string { return filepath.Join(DataDir(), "files", workflowID) }
