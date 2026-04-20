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

// FileMeta describes an attached file
type FileMeta struct {
	ID        string
	Name      string
	Size      int64
	MediaType string
	CreatedAt time.Time
}
