package api

import "github.com/Tsinling0525/rivulet/model"

// WorkflowStore is a simple in-memory store for workflows.
type WorkflowStore struct{ M map[string]model.Workflow }

func NewWorkflowStore() *WorkflowStore { return &WorkflowStore{M: make(map[string]model.Workflow)} }

func (s *WorkflowStore) Put(wf model.Workflow)                { s.M[string(wf.ID)] = wf }
func (s *WorkflowStore) Get(id string) (model.Workflow, bool) { wf, ok := s.M[id]; return wf, ok }
func (s *WorkflowStore) Delete(id string)                     { delete(s.M, id) }
