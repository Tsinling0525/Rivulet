package memory

import (
	"testing"
	"time"
)

func TestWriteBuildsAssumptionEdges(t *testing.T) {
	now := fixedTime()
	graph := Graph{UserID: "u1"}

	result := graph.AddObservation("The user bikes to work every weekday.", WriteOptions{
		Now:    func() time.Time { return now },
		Source: "test",
	})

	if len(result.Nodes) < 2 {
		t.Fatalf("expected proposition plus assumption nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) == 0 {
		t.Fatalf("expected assumption dependency edge")
	}

	var bikeMemory Node
	var ability Node
	for _, node := range graph.Nodes {
		switch node.Proposition {
		case "The user bikes to work every weekday":
			bikeMemory = node
		case "The user is physically able to bike":
			ability = node
		}
	}
	if bikeMemory.ID == "" {
		t.Fatalf("bike memory was not stored")
	}
	if ability.ID == "" {
		t.Fatalf("ability assumption was not stored")
	}

	found := false
	for _, edge := range graph.Edges {
		if edge.From == ability.ID && edge.To == bikeMemory.ID && edge.Type == DependencyFunctional {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected functional ability -> bike memory edge, edges=%+v", graph.Edges)
	}
}

func TestUpdateCascadesToDependentUnknownCurrent(t *testing.T) {
	now := fixedTime()
	graph := Graph{UserID: "u1"}
	graph.AddObservation("The user bikes to work every weekday.", WriteOptions{
		Now:    func() time.Time { return now },
		Source: "test",
	})

	result := graph.UpdateWithObservation("I broke my leg yesterday and will be in a cast for six weeks.", UpdateOptions{
		Now:              func() time.Time { return now.Add(time.Hour) },
		Source:           "test",
		StoreObservation: true,
	})

	if !result.Triggered {
		t.Fatalf("expected update to trigger maintenance")
	}
	if len(result.Direct) == 0 {
		t.Fatalf("expected direct invalidation of ability assumption")
	}
	if len(result.Propagated) == 0 {
		t.Fatalf("expected propagated invalidation")
	}

	var ability Node
	var bikeMemory Node
	for _, node := range graph.Nodes {
		switch node.Proposition {
		case "The user is physically able to bike":
			ability = node
		case "The user bikes to work every weekday":
			bikeMemory = node
		}
	}
	if ability.State != StateStale {
		t.Fatalf("expected ability assumption stale, got %s", ability.State)
	}
	if bikeMemory.State != StateUnknownCurrent {
		t.Fatalf("expected bike memory unknown-current, got %s risk=%.3f", bikeMemory.State, bikeMemory.Risk)
	}
}

func TestQueryExcludesUnknownCurrentAndReturnsWarning(t *testing.T) {
	now := fixedTime()
	graph := Graph{UserID: "u1"}
	graph.AddObservation("The user bikes to work every weekday.", WriteOptions{
		Now:    func() time.Time { return now },
		Source: "test",
	})
	graph.UpdateWithObservation("I broke my leg yesterday and will be in a cast for six weeks.", UpdateOptions{
		Now:              func() time.Time { return now.Add(time.Hour) },
		Source:           "test",
		StoreObservation: true,
	})

	result := graph.Query("What bike route should the user take to work tomorrow?", QueryOptions{
		IncludeWarnings: true,
	})

	for _, match := range result.Matches {
		if match.Node.Proposition == "The user bikes to work every weekday" {
			t.Fatalf("unknown-current bike memory should not be returned as active match")
		}
	}
	foundWarning := false
	for _, warning := range result.Warnings {
		if warning.Node.Proposition == "The user bikes to work every weekday" && warning.Node.State == StateUnknownCurrent {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("expected unknown-current bike memory warning, warnings=%+v", result.Warnings)
	}
}

func TestWriteIsIdempotentAndCopiesMetadata(t *testing.T) {
	now := fixedTime()
	metadata := map[string]any{"source_id": "first"}
	input := PropositionInput{Text: "The user prefers tea", Metadata: metadata}
	graph := Graph{UserID: "u1"}

	first := graph.AddPropositions([]PropositionInput{input}, WriteOptions{Now: func() time.Time { return now }})
	second := graph.AddPropositions([]PropositionInput{input}, WriteOptions{Now: func() time.Time { return now.Add(time.Hour) }})
	metadata["source_id"] = "mutated-after-write"

	if len(first.Nodes) != 1 {
		t.Fatalf("expected first write to create one node, got %d", len(first.Nodes))
	}
	if len(second.Nodes) != 0 {
		t.Fatalf("expected duplicate write not to create a node, got %d", len(second.Nodes))
	}
	if len(graph.Nodes) != 1 {
		t.Fatalf("expected one graph node after duplicate write, got %d", len(graph.Nodes))
	}
	if got := graph.Nodes[0].Metadata["source_id"]; got != "first" {
		t.Fatalf("memory metadata was aliased, got %#v", got)
	}
}

func TestUpdateEmptyGraphStoresObservationWithoutInvalidation(t *testing.T) {
	graph := Graph{UserID: "u1"}
	result := graph.UpdateWithObservation("I moved to a new address.", UpdateOptions{
		Now:              fixedTime,
		StoreObservation: true,
	})

	if !result.Triggered {
		t.Fatal("expected state-change observation to trigger maintenance")
	}
	if len(result.Observations) != 1 || len(graph.Nodes) != 1 {
		t.Fatalf("expected observation to be stored in an empty graph, got result=%+v graph=%+v", result.Observations, graph.Nodes)
	}
	if len(result.Direct) != 0 || len(result.Propagated) != 0 {
		t.Fatalf("empty graph must not invalidate nodes, got direct=%+v propagated=%+v", result.Direct, result.Propagated)
	}
}

func fixedTime() time.Time {
	return time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
}
