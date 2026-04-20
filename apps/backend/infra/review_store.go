package infra

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Tsinling0525/rivulet/model"
	"github.com/Tsinling0525/rivulet/plugin"
)

var ErrReviewNotFound = errors.New("review not found")

type ReviewStore struct {
	mu  sync.Mutex
	dir string
}

func NewReviewStore() *ReviewStore {
	return &ReviewStore{dir: ReviewStoreDir()}
}

func (s *ReviewStore) reviewPath(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func (s *ReviewStore) Create(ctx context.Context, req model.ReviewCreate) (model.ReviewRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	review := model.ReviewRequest{
		ID:             newReviewID(),
		RunID:          req.RunID,
		WorkflowID:     req.WorkflowID,
		WorkflowName:   req.WorkflowName,
		WorkflowKind:   req.WorkflowKind,
		NodeID:         req.NodeID,
		NodeName:       req.NodeName,
		Status:         model.ReviewPending,
		Input:          cloneItem(req.Input),
		ProposedOutput: req.ProposedOutput,
		Context:        cloneItem(req.Context),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.saveLocked(review); err != nil {
		return model.ReviewRequest{}, err
	}
	return review, nil
}

func (s *ReviewStore) List(ctx context.Context, status model.ReviewStatus) ([]model.ReviewRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := s.listLocked()
	if err != nil {
		return nil, err
	}
	if status == "" {
		return items, nil
	}
	filtered := make([]model.ReviewRequest, 0, len(items))
	for _, item := range items {
		if item.Status == status {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (s *ReviewStore) Get(ctx context.Context, id string) (model.ReviewRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked(id)
}

func (s *ReviewStore) Approve(ctx context.Context, id, reviewer, comment string) (model.ReviewRequest, error) {
	return s.decide(id, model.ReviewApproved, reviewer, comment)
}

func (s *ReviewStore) Reject(ctx context.Context, id, reviewer, comment string) (model.ReviewRequest, error) {
	return s.decide(id, model.ReviewRejected, reviewer, comment)
}

func (s *ReviewStore) decide(id string, status model.ReviewStatus, reviewer, comment string) (model.ReviewRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	review, err := s.loadLocked(id)
	if err != nil {
		return model.ReviewRequest{}, err
	}
	now := time.Now().UTC()
	review.Status = status
	review.Reviewer = reviewer
	review.Comment = comment
	review.DecidedAt = now
	review.UpdatedAt = now
	if err := s.saveLocked(review); err != nil {
		return model.ReviewRequest{}, err
	}
	return review, nil
}

func (s *ReviewStore) listLocked() ([]model.ReviewRequest, error) {
	if err := ensureDir(s.dir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	out := make([]model.ReviewRequest, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var review model.ReviewRequest
		if err := json.Unmarshal(raw, &review); err != nil {
			return nil, err
		}
		out = append(out, review)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *ReviewStore) loadLocked(id string) (model.ReviewRequest, error) {
	raw, err := os.ReadFile(s.reviewPath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.ReviewRequest{}, ErrReviewNotFound
		}
		return model.ReviewRequest{}, err
	}
	var review model.ReviewRequest
	if err := json.Unmarshal(raw, &review); err != nil {
		return model.ReviewRequest{}, err
	}
	return review, nil
}

func (s *ReviewStore) saveLocked(review model.ReviewRequest) error {
	if err := ensureDir(s.dir); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.reviewPath(review.ID), raw, 0o644)
}

func cloneItem(src model.Item) model.Item {
	if src == nil {
		return nil
	}
	out := model.Item{}
	for key, value := range src {
		out[key] = value
	}
	return out
}

func newReviewID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("review-%d", time.Now().UnixNano())
	}
	return "review-" + hex.EncodeToString(buf[:])
}

var _ plugin.ReviewStore = (*ReviewStore)(nil)
