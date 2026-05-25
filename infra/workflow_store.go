package infra

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Tsinling0525/rivulet/format/n8n"
	"github.com/Tsinling0525/rivulet/model"
)

var ErrWorkflowNotFound = errors.New("workflow not found")

type StoredWorkflowVersion struct {
	Number    int             `json:"number"`
	CreatedAt time.Time       `json:"created_at"`
	NodeCount int             `json:"node_count"`
	Request   json.RawMessage `json:"request"`
}

type StoredWorkflow struct {
	ID            string                    `json:"id"`
	Name          string                    `json:"name"`
	Kind          model.WorkflowKind        `json:"kind,omitempty"`
	AI            *model.AIWorkflowMetadata `json:"ai,omitempty"`
	Description   string                    `json:"description,omitempty"`
	ActiveVersion int                       `json:"active_version"`
	CreatedAt     time.Time                 `json:"created_at"`
	UpdatedAt     time.Time                 `json:"updated_at"`
	Versions      []StoredWorkflowVersion   `json:"versions"`
}

type WorkflowStore struct {
	mu  sync.Mutex
	dir string
}

func NewWorkflowStore() *WorkflowStore {
	return &WorkflowStore{dir: WorkflowStoreDir()}
}

func (s *WorkflowStore) workflowPath(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func (s *WorkflowStore) List() ([]StoredWorkflow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked()
}

func (s *WorkflowStore) Get(id string) (StoredWorkflow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked(id)
}

func (s *WorkflowStore) Create(req n8n.N8nRequest, description string, activate bool) (StoredWorkflow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ensureDir(s.dir); err != nil {
		return StoredWorkflow{}, err
	}

	id, name := normalizeWorkflowIdentity(req)
	req.Workflow.ID = id
	req.Workflow.Name = name
	if _, err := os.Stat(s.workflowPath(id)); err == nil {
		return StoredWorkflow{}, fmt.Errorf("workflow %s already exists", id)
	}

	now := time.Now().UTC()
	record := StoredWorkflow{
		ID:            id,
		Name:          name,
		Kind:          workflowKindFromRequest(req),
		AI:            req.Workflow.AI,
		Description:   description,
		ActiveVersion: 1,
		CreatedAt:     now,
		UpdatedAt:     now,
		Versions: []StoredWorkflowVersion{
			buildWorkflowVersion(1, req, now),
		},
	}
	if !activate {
		record.ActiveVersion = 1
	}

	if err := s.saveLocked(record); err != nil {
		return StoredWorkflow{}, err
	}
	return record, nil
}

func (s *WorkflowStore) AddVersion(id string, req n8n.N8nRequest, activate bool) (StoredWorkflow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.loadLocked(id)
	if err != nil {
		return StoredWorkflow{}, err
	}

	now := time.Now().UTC()
	req.Workflow.ID = id
	if req.Workflow.Name == "" {
		req.Workflow.Name = record.Name
	}
	number := 1
	if len(record.Versions) > 0 {
		number = record.Versions[len(record.Versions)-1].Number + 1
	}
	record.Name = req.Workflow.Name
	record.Kind = workflowKindFromRequest(req)
	record.AI = req.Workflow.AI
	record.UpdatedAt = now
	record.Versions = append(record.Versions, buildWorkflowVersion(number, req, now))
	if activate || record.ActiveVersion == 0 {
		record.ActiveVersion = number
	}

	if err := s.saveLocked(record); err != nil {
		return StoredWorkflow{}, err
	}
	return record, nil
}

func (s *WorkflowStore) ActivateVersion(id string, version int) (StoredWorkflow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.loadLocked(id)
	if err != nil {
		return StoredWorkflow{}, err
	}
	if _, ok := findWorkflowVersion(record, version); !ok {
		return StoredWorkflow{}, fmt.Errorf("workflow %s version %d not found", id, version)
	}
	record.ActiveVersion = version
	record.UpdatedAt = time.Now().UTC()
	if err := s.saveLocked(record); err != nil {
		return StoredWorkflow{}, err
	}
	return record, nil
}

func (s *WorkflowStore) LoadVersionRequest(id string, version int) (StoredWorkflow, n8n.N8nRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.loadLocked(id)
	if err != nil {
		return StoredWorkflow{}, n8n.N8nRequest{}, err
	}
	if version == 0 {
		version = record.ActiveVersion
	}
	storedVersion, ok := findWorkflowVersion(record, version)
	if !ok {
		return StoredWorkflow{}, n8n.N8nRequest{}, fmt.Errorf("workflow %s version %d not found", id, version)
	}
	var req n8n.N8nRequest
	if err := json.Unmarshal(storedVersion.Request, &req); err != nil {
		return StoredWorkflow{}, n8n.N8nRequest{}, err
	}
	return record, req, nil
}

func (s *WorkflowStore) RollbackPromptToHash(id, nodeID, promptHash string, activate bool) (StoredWorkflow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.loadLocked(id)
	if err != nil {
		return StoredWorkflow{}, err
	}
	targetPrompt := ""
	for _, version := range record.Versions {
		var req n8n.N8nRequest
		if err := json.Unmarshal(version.Request, &req); err != nil {
			return StoredWorkflow{}, err
		}
		for _, node := range req.Workflow.Nodes {
			if node.ID != nodeID {
				continue
			}
			prompt, _ := node.Parameters["prompt"].(string)
			if prompt != "" && promptTemplateHash(prompt) == promptHash {
				targetPrompt = prompt
				break
			}
		}
		if targetPrompt != "" {
			break
		}
	}
	if targetPrompt == "" {
		return StoredWorkflow{}, fmt.Errorf("prompt hash %s not found for node %s", promptHash, nodeID)
	}

	_, req, err := s.loadVersionRequestLocked(record, record.ActiveVersion)
	if err != nil {
		return StoredWorkflow{}, err
	}
	replaced := false
	for i := range req.Workflow.Nodes {
		if req.Workflow.Nodes[i].ID != nodeID {
			continue
		}
		if req.Workflow.Nodes[i].Parameters == nil {
			req.Workflow.Nodes[i].Parameters = map[string]interface{}{}
		}
		req.Workflow.Nodes[i].Parameters["prompt"] = targetPrompt
		replaced = true
		break
	}
	if !replaced {
		return StoredWorkflow{}, fmt.Errorf("node %s not found in active workflow", nodeID)
	}

	now := time.Now().UTC()
	number := 1
	if len(record.Versions) > 0 {
		number = record.Versions[len(record.Versions)-1].Number + 1
	}
	record.Kind = workflowKindFromRequest(req)
	record.AI = req.Workflow.AI
	record.UpdatedAt = now
	record.Versions = append(record.Versions, buildWorkflowVersion(number, req, now))
	if activate || record.ActiveVersion == 0 {
		record.ActiveVersion = number
	}
	if err := s.saveLocked(record); err != nil {
		return StoredWorkflow{}, err
	}
	return record, nil
}

func (s *WorkflowStore) listLocked() ([]StoredWorkflow, error) {
	if err := ensureDir(s.dir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	out := make([]StoredWorkflow, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var record StoredWorkflow
		if err := json.Unmarshal(raw, &record); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func (s *WorkflowStore) loadLocked(id string) (StoredWorkflow, error) {
	raw, err := os.ReadFile(s.workflowPath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return StoredWorkflow{}, ErrWorkflowNotFound
		}
		return StoredWorkflow{}, err
	}
	var record StoredWorkflow
	if err := json.Unmarshal(raw, &record); err != nil {
		return StoredWorkflow{}, err
	}
	return record, nil
}

func (s *WorkflowStore) saveLocked(record StoredWorkflow) error {
	if err := ensureDir(s.dir); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.workflowPath(record.ID), raw, 0o644)
}

func findWorkflowVersion(record StoredWorkflow, version int) (StoredWorkflowVersion, bool) {
	for _, item := range record.Versions {
		if item.Number == version {
			return item, true
		}
	}
	return StoredWorkflowVersion{}, false
}

func (s *WorkflowStore) loadVersionRequestLocked(record StoredWorkflow, version int) (StoredWorkflowVersion, n8n.N8nRequest, error) {
	if version == 0 {
		version = record.ActiveVersion
	}
	storedVersion, ok := findWorkflowVersion(record, version)
	if !ok {
		return StoredWorkflowVersion{}, n8n.N8nRequest{}, fmt.Errorf("workflow %s version %d not found", record.ID, version)
	}
	var req n8n.N8nRequest
	if err := json.Unmarshal(storedVersion.Request, &req); err != nil {
		return StoredWorkflowVersion{}, n8n.N8nRequest{}, err
	}
	return storedVersion, req, nil
}

func buildWorkflowVersion(number int, req n8n.N8nRequest, now time.Time) StoredWorkflowVersion {
	return StoredWorkflowVersion{
		Number:    number,
		CreatedAt: now,
		NodeCount: len(req.Workflow.Nodes),
		Request:   marshalWorkflowRequest(req),
	}
}

func normalizeWorkflowRequest(raw []byte, req n8n.N8nRequest) []byte {
	if len(raw) > 0 {
		return append([]byte(nil), raw...)
	}
	b, _ := json.Marshal(req)
	return b
}

func marshalWorkflowRequest(req n8n.N8nRequest) []byte {
	b, _ := json.Marshal(req)
	return b
}

func normalizeWorkflowIdentity(req n8n.N8nRequest) (string, string) {
	id := string(req.Workflow.ID)
	if id == "" {
		id = fmt.Sprintf("wf-%d", time.Now().UnixNano())
	}
	name := req.Workflow.Name
	if name == "" {
		name = id
	}
	return id, name
}

func workflowKindFromRequest(req n8n.N8nRequest) model.WorkflowKind {
	if req.Workflow.Kind != "" {
		return model.WorkflowKind(req.Workflow.Kind)
	}
	if req.Workflow.AI != nil {
		return model.WorkflowKindAI
	}
	for _, node := range req.Workflow.Nodes {
		switch node.Type {
		case "chatgpt", "ollama", "llm:route":
			return model.WorkflowKindAI
		}
	}
	return model.WorkflowKindAutomation
}

func promptTemplateHash(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return "sha256:" + hex.EncodeToString(sum[:])
}
