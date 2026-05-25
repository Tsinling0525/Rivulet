package infra

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

// MemFiles is an in-memory FileStore implementation
type MemFiles struct {
	mu   sync.RWMutex
	data map[string]map[string]memFile // workflowID -> fileID -> file
}

type memFile struct {
	name      string
	mediaType string
	content   []byte
	createdAt time.Time
}

func NewMemFiles() *MemFiles { return &MemFiles{data: make(map[string]map[string]memFile)} }

func (m *MemFiles) Put(ctx context.Context, workflowID string, filename string, contents []byte, mediaType string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[workflowID]; !ok {
		m.data[workflowID] = make(map[string]memFile)
	}
	id := fmt.Sprintf("f_%d", time.Now().UnixNano())
	m.data[workflowID][id] = memFile{name: filename, mediaType: mediaType, content: append([]byte(nil), contents...), createdAt: time.Now().UTC()}
	return id, nil
}

func (m *MemFiles) Get(ctx context.Context, workflowID string, fileID string) (string, string, []byte, error) {
	select {
	case <-ctx.Done():
		return "", "", nil, ctx.Err()
	default:
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if wf, ok := m.data[workflowID]; ok {
		if f, ok := wf[fileID]; ok {
			return f.name, f.mediaType, append([]byte(nil), f.content...), nil
		}
	}
	return "", "", nil, fmt.Errorf("file not found")
}

func (m *MemFiles) List(ctx context.Context, workflowID string) ([]model.FileMeta, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []model.FileMeta
	if wf, ok := m.data[workflowID]; ok {
		for id, f := range wf {
			out = append(out, model.FileMeta{ID: id, Name: f.name, Size: int64(len(f.content)), MediaType: f.mediaType, CreatedAt: f.createdAt})
		}
	}
	return out, nil
}

func (m *MemFiles) Delete(ctx context.Context, workflowID string, fileID string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if wf, ok := m.data[workflowID]; ok {
		delete(wf, fileID)
	}
	return nil
}

var _ plugin.FileStore = (*MemFiles)(nil)
