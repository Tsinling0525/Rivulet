package memory

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestFileStoreSeparatesUserIDsWithSimilarFilenames(t *testing.T) {
	store := NewFileStore(t.TempDir())
	ctx := context.Background()

	for _, tc := range []struct {
		userID string
		text   string
	}{
		{userID: "team/alice", text: "Alice prefers tea"},
		{userID: "team?alice", text: "Alice prefers coffee"},
	} {
		if _, err := store.Update(ctx, tc.userID, func(graph *Graph) error {
			graph.AddObservation(tc.text, WriteOptions{Source: "test"})
			return nil
		}); err != nil {
			t.Fatalf("update %q: %v", tc.userID, err)
		}
	}

	if store.path("team/alice") == store.path("team?alice") {
		t.Fatal("distinct user IDs must not share a storage path")
	}
	first, err := store.Load(ctx, "team/alice")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Load(ctx, "team?alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Nodes) != 1 || first.Nodes[0].Proposition != "Alice prefers tea" {
		t.Fatalf("unexpected first graph: %+v", first.Nodes)
	}
	if len(second.Nodes) != 1 || second.Nodes[0].Proposition != "Alice prefers coffee" {
		t.Fatalf("unexpected second graph: %+v", second.Nodes)
	}
}

func TestFileStoreConcurrentUpdatesRetainAllWrites(t *testing.T) {
	store := NewFileStore(t.TempDir())
	ctx := context.Background()
	const writers = 16

	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := store.Update(ctx, "shared-user", func(graph *Graph) error {
				graph.AddObservation(fmt.Sprintf("Memory number %d", i), WriteOptions{Source: "test"})
				return nil
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	graph, err := store.Load(ctx, "shared-user")
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != writers {
		t.Fatalf("expected %d stored nodes, got %d", writers, len(graph.Nodes))
	}
}
