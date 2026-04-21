package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const defaultSemanticThreshold = 0.92

var semanticCacheMu sync.Mutex

type SemanticCacheOptions struct {
	Enabled   bool
	Threshold float64
	Scope     string
}

type SemanticCacheHit struct {
	Output     string
	Similarity float64
	PromptHash string
	CreatedAt  time.Time
}

type semanticCacheEntry struct {
	Key        string    `json:"key"`
	Scope      string    `json:"scope"`
	Provider   string    `json:"provider"`
	Model      string    `json:"model"`
	NodeID     string    `json:"node_id,omitempty"`
	PromptHash string    `json:"prompt_hash"`
	Prompt     string    `json:"prompt"`
	Output     string    `json:"output"`
	CreatedAt  time.Time `json:"created_at"`
}

func SemanticCacheConfig(raw any) SemanticCacheOptions {
	opts := SemanticCacheOptions{Threshold: defaultSemanticThreshold, Scope: "node"}
	switch value := raw.(type) {
	case bool:
		opts.Enabled = value
	case map[string]any:
		opts.Enabled = boolValue(value["enabled"], true)
		opts.Threshold = floatValue(value["threshold"], defaultSemanticThreshold)
		opts.Scope = stringValue(value["scope"], "node")
	default:
		opts.Enabled = false
	}
	if opts.Threshold <= 0 || opts.Threshold > 1 {
		opts.Threshold = defaultSemanticThreshold
	}
	if opts.Scope == "" {
		opts.Scope = "node"
	}
	return opts
}

func LookupSemanticCache(provider, modelName, nodeID, prompt string, opts SemanticCacheOptions) (SemanticCacheHit, bool) {
	if !opts.Enabled {
		return SemanticCacheHit{}, false
	}
	semanticCacheMu.Lock()
	defer semanticCacheMu.Unlock()

	entries, err := loadSemanticCacheLocked()
	if err != nil {
		return SemanticCacheHit{}, false
	}
	scope := cacheScope(provider, modelName, nodeID, opts.Scope)
	var best semanticCacheEntry
	bestScore := 0.0
	for _, entry := range entries {
		if entry.Scope != scope {
			continue
		}
		score := semanticSimilarity(prompt, entry.Prompt)
		if score > bestScore {
			bestScore = score
			best = entry
		}
	}
	if bestScore < opts.Threshold {
		return SemanticCacheHit{}, false
	}
	return SemanticCacheHit{
		Output:     best.Output,
		Similarity: bestScore,
		PromptHash: best.PromptHash,
		CreatedAt:  best.CreatedAt,
	}, true
}

func StoreSemanticCache(provider, modelName, nodeID, prompt, output string, opts SemanticCacheOptions) {
	if !opts.Enabled || strings.TrimSpace(prompt) == "" {
		return
	}
	semanticCacheMu.Lock()
	defer semanticCacheMu.Unlock()

	entries, err := loadSemanticCacheLocked()
	if err != nil {
		entries = nil
	}
	entry := semanticCacheEntry{
		Key:        semanticCacheKey(provider, modelName, nodeID, prompt, opts.Scope),
		Scope:      cacheScope(provider, modelName, nodeID, opts.Scope),
		Provider:   provider,
		Model:      modelName,
		NodeID:     nodeID,
		PromptHash: PromptHash(prompt),
		Prompt:     prompt,
		Output:     output,
		CreatedAt:  time.Now().UTC(),
	}
	for i := range entries {
		if entries[i].Key == entry.Key {
			entries[i] = entry
			_ = saveSemanticCacheLocked(entries)
			return
		}
	}
	entries = append(entries, entry)
	if len(entries) > 1000 {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].CreatedAt.After(entries[j].CreatedAt)
		})
		entries = entries[:1000]
	}
	_ = saveSemanticCacheLocked(entries)
}

func semanticCacheKey(provider, modelName, nodeID, prompt, scopeMode string) string {
	sum := sha256.Sum256([]byte(cacheScope(provider, modelName, nodeID, scopeMode) + "\n" + normalizedPrompt(prompt)))
	return hex.EncodeToString(sum[:])
}

func cacheScope(provider, modelName, nodeID, scopeMode string) string {
	switch scopeMode {
	case "global":
		return provider + "::" + modelName
	case "workflow":
		return provider + "::" + modelName + "::" + strings.Split(nodeID, ":")[0]
	default:
		return provider + "::" + modelName + "::" + nodeID
	}
}

func loadSemanticCacheLocked() ([]semanticCacheEntry, error) {
	raw, err := os.ReadFile(semanticCachePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var entries []semanticCacheEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func saveSemanticCacheLocked(entries []semanticCacheEntry) error {
	path := semanticCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func semanticCachePath() string {
	if path := os.Getenv("RIV_SEMANTIC_CACHE_PATH"); path != "" {
		return path
	}
	dataDir := os.Getenv("RIV_DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	return filepath.Join(dataDir, "store", "semantic_cache.json")
}

func semanticSimilarity(a, b string) float64 {
	ta := tokenSet(a)
	tb := tokenSet(b)
	if len(ta) == 0 && len(tb) == 0 {
		return 1
	}
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	intersection := 0
	union := map[string]bool{}
	for token := range ta {
		union[token] = true
		if tb[token] {
			intersection++
		}
	}
	for token := range tb {
		union[token] = true
	}
	return float64(intersection) / float64(len(union))
}

func tokenSet(value string) map[string]bool {
	value = normalizedPrompt(value)
	out := map[string]bool{}
	for _, token := range strings.Fields(value) {
		if len(token) < 2 {
			continue
		}
		out[token] = true
	}
	return out
}

func normalizedPrompt(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 127 {
			b.WriteRune(r)
			continue
		}
		b.WriteByte(' ')
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func boolValue(raw any, def bool) bool {
	if value, ok := raw.(bool); ok {
		return value
	}
	return def
}

func floatValue(raw any, def float64) float64 {
	switch value := raw.(type) {
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case float64:
		return value
	case float32:
		return float64(value)
	case json.Number:
		if out, err := value.Float64(); err == nil {
			return out
		}
	}
	return def
}

func stringValue(raw any, def string) string {
	if value, ok := raw.(string); ok && value != "" {
		return value
	}
	return def
}
