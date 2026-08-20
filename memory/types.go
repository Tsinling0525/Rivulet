package memory

import "time"

type ValidityState string

const (
	StateActive         ValidityState = "active"
	StateSuspect        ValidityState = "suspect"
	StateStale          ValidityState = "stale"
	StateUnknownCurrent ValidityState = "unknown-current"
)

type NodeKind string

const (
	NodeKindProposition NodeKind = "proposition"
	NodeKindAssumption  NodeKind = "assumption"
	NodeKindObservation NodeKind = "observation"
)

type DependencyType string

const (
	DependencyLogical    DependencyType = "logical"
	DependencyFunctional DependencyType = "functional"
	DependencyCausal     DependencyType = "causal"
	DependencyTemporal   DependencyType = "temporal"
	DependencyContextual DependencyType = "contextual"
)

type Polarity string

const (
	PolarityPositive Polarity = "positive"
	PolarityNegative Polarity = "negative"
)

type Node struct {
	ID          string         `json:"id"`
	Kind        NodeKind       `json:"kind"`
	Proposition string         `json:"proposition"`
	WrittenAt   time.Time      `json:"written_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	State       ValidityState  `json:"state"`
	Confidence  float64        `json:"confidence"`
	Risk        float64        `json:"risk,omitempty"`
	Domains     []string       `json:"domains,omitempty"`
	Assumptions []string       `json:"assumptions,omitempty"`
	Horizon     string         `json:"horizon,omitempty"`
	Evidence    []string       `json:"evidence,omitempty"`
	Source      string         `json:"source,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type Edge struct {
	From       string         `json:"from"`
	To         string         `json:"to"`
	Type       DependencyType `json:"type"`
	Confidence float64        `json:"confidence"`
	Evidence   string         `json:"evidence,omitempty"`
	Polarity   Polarity       `json:"polarity"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

type Graph struct {
	Version   int       `json:"version"`
	UserID    string    `json:"user_id"`
	Nodes     []Node    `json:"nodes"`
	Edges     []Edge    `json:"edges"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PropositionInput struct {
	Text        string
	Kind        NodeKind
	Assumptions []string
	Domains     []string
	Horizon     string
	Evidence    []string
	Source      string
	Metadata    map[string]any
}

type WriteOptions struct {
	Now        func() time.Time
	Source     string
	State      ValidityState
	Confidence float64
}

type WriteResult struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type UpdateOptions struct {
	Now                  func() time.Time
	Source               string
	TriggerThreshold     float64
	DirectThreshold      float64
	SuspicionThreshold   float64
	StaleThreshold       float64
	UncertaintyThreshold float64
	EdgeThreshold        float64
	PathDecay            float64
	MaxDepth             int
	StoreObservation     bool
}

type Invalidation struct {
	NodeID        string        `json:"node_id"`
	Proposition   string        `json:"proposition"`
	PreviousState ValidityState `json:"previous_state"`
	State         ValidityState `json:"state"`
	Risk          float64       `json:"risk"`
	Reason        string        `json:"reason,omitempty"`
	Direct        bool          `json:"direct"`
}

type UpdateResult struct {
	Triggered     bool           `json:"triggered"`
	TriggerScore  float64        `json:"trigger_score"`
	Observations  []Node         `json:"observations,omitempty"`
	Direct        []Invalidation `json:"direct_invalidations,omitempty"`
	Propagated    []Invalidation `json:"propagated_invalidations,omitempty"`
	EdgesUpdated  []Edge         `json:"edges_updated,omitempty"`
	NodesExamined int            `json:"nodes_examined"`
}

type QueryOptions struct {
	MaxResults      int
	States          []ValidityState
	WarningStates   []ValidityState
	IncludeWarnings bool
}

type QueryMatch struct {
	Node   Node    `json:"node"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason,omitempty"`
}

type QueryResult struct {
	Matches  []QueryMatch `json:"matches"`
	Warnings []QueryMatch `json:"warnings,omitempty"`
}
