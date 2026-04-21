package llm

import (
	"path/filepath"
	"testing"
)

func TestSemanticCacheReturnsNearbyPrompt(t *testing.T) {
	t.Setenv("RIV_SEMANTIC_CACHE_PATH", filepath.Join(t.TempDir(), "cache.json"))
	opts := SemanticCacheOptions{Enabled: true, Threshold: 0.5, Scope: "node"}
	StoreSemanticCache("openai", "gpt-test", "node1", "classify invoice status", "paid", opts)

	hit, ok := LookupSemanticCache("openai", "gpt-test", "node1", "classify the invoice status", opts)
	if !ok {
		t.Fatalf("expected semantic cache hit")
	}
	if hit.Output != "paid" {
		t.Fatalf("expected cached output paid, got %q", hit.Output)
	}
}

func TestSemanticCacheMissesDifferentScope(t *testing.T) {
	t.Setenv("RIV_SEMANTIC_CACHE_PATH", filepath.Join(t.TempDir(), "cache.json"))
	opts := SemanticCacheOptions{Enabled: true, Threshold: 0.5, Scope: "node"}
	StoreSemanticCache("openai", "gpt-test", "node1", "classify invoice status", "paid", opts)

	if _, ok := LookupSemanticCache("openai", "gpt-test", "node2", "classify invoice status", opts); ok {
		t.Fatalf("expected cache miss for different node scope")
	}
}
