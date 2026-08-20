package memory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type FileStore struct {
	dir string
}

var fileStoreMu sync.Mutex

func NewFileStore(dir string) *FileStore {
	if strings.TrimSpace(dir) == "" {
		dir = DefaultStoreDir()
	}
	return &FileStore{dir: dir}
}

func DefaultStoreDir() string {
	dataDir := os.Getenv("RIV_DATA_DIR")
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "data"
	}
	return filepath.Join(dataDir, "store", "memory")
}

func (s *FileStore) Load(ctx context.Context, userID string) (Graph, error) {
	select {
	case <-ctx.Done():
		return Graph{}, ctx.Err()
	default:
	}

	fileStoreMu.Lock()
	defer fileStoreMu.Unlock()
	return s.loadLocked(userID)
}

func (s *FileStore) Save(ctx context.Context, userID string, graph Graph) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	fileStoreMu.Lock()
	defer fileStoreMu.Unlock()
	return s.saveLocked(userID, graph)
}

func (s *FileStore) Update(ctx context.Context, userID string, mutate func(*Graph) error) (Graph, error) {
	select {
	case <-ctx.Done():
		return Graph{}, ctx.Err()
	default:
	}

	fileStoreMu.Lock()
	defer fileStoreMu.Unlock()

	graph, err := s.loadLocked(userID)
	if err != nil {
		return Graph{}, err
	}
	if err := mutate(&graph); err != nil {
		return Graph{}, err
	}
	if err := s.saveLocked(userID, graph); err != nil {
		return Graph{}, err
	}
	return graph, nil
}

func (s *FileStore) loadLocked(userID string) (Graph, error) {
	path := s.path(userID)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			now := time.Now().UTC()
			return Graph{
				Version:   1,
				UserID:    normalizeUserID(userID),
				UpdatedAt: now,
			}, nil
		}
		return Graph{}, err
	}
	var graph Graph
	if err := json.Unmarshal(raw, &graph); err != nil {
		return Graph{}, err
	}
	if graph.Version == 0 {
		graph.Version = 1
	}
	if graph.UserID == "" {
		graph.UserID = normalizeUserID(userID)
	}
	return graph, nil
}

func (s *FileStore) saveLocked(userID string, graph Graph) error {
	if graph.Version == 0 {
		graph.Version = 1
	}
	graph.UserID = normalizeUserID(userID)
	graph.UpdatedAt = time.Now().UTC()

	path := s.path(userID)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".memory-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	if parent, err := os.Open(dir); err == nil {
		defer parent.Close()
		_ = parent.Sync()
	}
	return nil
}

func (s *FileStore) path(userID string) string {
	userID = normalizeUserID(userID)
	name := sanitizeFilename(userID)
	if len(name) > 64 {
		name = name[:64]
	}
	digest := sha256.Sum256([]byte(userID))
	return filepath.Join(s.dir, fmt.Sprintf("%s-%x.json", name, digest[:8]))
}

func normalizeUserID(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "default"
	}
	return userID
}

func sanitizeFilename(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}
