package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

// LocalFiles stores attachments on the local filesystem under data/files/<workflowID>.
type LocalFiles struct{}

// NewLocalFiles returns a new LocalFiles store.
func NewLocalFiles() *LocalFiles { return &LocalFiles{} }

func (l *LocalFiles) Put(ctx context.Context, workflowID, filename string, contents []byte, mediaType string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	dir := FilesDir(workflowID)
	if err := ensureDir(dir); err != nil {
		return "", err
	}
	id := fmt.Sprintf("f_%d", time.Now().UnixNano())
	dataPath := filepath.Join(dir, id)
	if err := os.WriteFile(dataPath, contents, 0o644); err != nil {
		return "", err
	}
	meta := model.FileMeta{ID: id, Name: filename, Size: int64(len(contents)), MediaType: mediaType, CreatedAt: time.Now().UTC()}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(dataPath+".json", metaBytes, 0o644); err != nil {
		return "", err
	}
	return id, nil
}

func (l *LocalFiles) Get(ctx context.Context, workflowID, fileID string) (string, string, []byte, error) {
	select {
	case <-ctx.Done():
		return "", "", nil, ctx.Err()
	default:
	}
	dir := FilesDir(workflowID)
	metaBytes, err := os.ReadFile(filepath.Join(dir, fileID+".json"))
	if err != nil {
		return "", "", nil, err
	}
	var meta model.FileMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return "", "", nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, fileID))
	if err != nil {
		return "", "", nil, err
	}
	return meta.Name, meta.MediaType, data, nil
}

func (l *LocalFiles) List(ctx context.Context, workflowID string) ([]model.FileMeta, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	dir := FilesDir(workflowID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	metas := []model.FileMeta{}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var m model.FileMeta
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		metas = append(metas, m)
	}
	return metas, nil
}

func (l *LocalFiles) Delete(ctx context.Context, workflowID, fileID string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	dir := FilesDir(workflowID)
	_ = os.Remove(filepath.Join(dir, fileID))
	_ = os.Remove(filepath.Join(dir, fileID+".json"))
	return nil
}

var _ plugin.FileStore = (*LocalFiles)(nil)
