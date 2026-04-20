package model

import "time"

type ID string

type Port string

const (
	PortMain Port = "main"
)

type Edge struct {
	FromNode ID
	FromPort Port
	ToNode   ID
	ToPort   Port
}

type Node struct {
	ID          ID
	Type        string
	Name        string
	Concurrency int           // 0 = default
	Timeout     time.Duration // 0 = none
	Config      map[string]any
	Credentials string // reference key
}

type WorkflowKind string

const (
	WorkflowKindAutomation WorkflowKind = "automation"
	WorkflowKindAI         WorkflowKind = "ai_workflow"
	WorkflowKindData       WorkflowKind = "data_pipeline"
)

type AIWorkflowMetadata struct {
	Purpose             string   `json:"purpose,omitempty"`
	Models              []string `json:"models,omitempty"`
	RiskLevel           string   `json:"risk_level,omitempty"`
	HumanReviewRequired bool     `json:"human_review_required,omitempty"`
}

type AINodeMetadata struct {
	Provider            string   `json:"provider,omitempty"`
	Model               string   `json:"model,omitempty"`
	PromptTemplate      string   `json:"prompt_template,omitempty"`
	InputFields         []string `json:"input_fields,omitempty"`
	OutputSchema        any      `json:"output_schema,omitempty"`
	HumanReviewRequired bool     `json:"human_review_required,omitempty"`
}

type Workflow struct {
	ID    ID
	Name  string
	Kind  WorkflowKind
	AI    *AIWorkflowMetadata
	Nodes []Node
	Edges []Edge
}

type Item = map[string]any

type Items = []Item

type ReviewStatus string

const (
	ReviewPending  ReviewStatus = "pending"
	ReviewApproved ReviewStatus = "approved"
	ReviewRejected ReviewStatus = "rejected"
)

type ReviewRequest struct {
	ID             string       `json:"id"`
	RunID          string       `json:"run_id"`
	WorkflowID     ID           `json:"workflow_id"`
	WorkflowName   string       `json:"workflow_name,omitempty"`
	WorkflowKind   WorkflowKind `json:"workflow_kind,omitempty"`
	NodeID         ID           `json:"node_id"`
	NodeName       string       `json:"node_name,omitempty"`
	Status         ReviewStatus `json:"status"`
	Input          Item         `json:"input,omitempty"`
	ProposedOutput any          `json:"proposed_output,omitempty"`
	Context        Item         `json:"context,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	DecidedAt      time.Time    `json:"decided_at,omitempty"`
	Reviewer       string       `json:"reviewer,omitempty"`
	Comment        string       `json:"comment,omitempty"`
}

type ReviewCreate struct {
	RunID          string
	WorkflowID     ID
	WorkflowName   string
	WorkflowKind   WorkflowKind
	NodeID         ID
	NodeName       string
	Input          Item
	ProposedOutput any
	Context        Item
}

// FileMeta describes an attached file
type FileMeta struct {
	ID        string
	Name      string
	Size      int64
	MediaType string
	CreatedAt time.Time
}
