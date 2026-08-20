package memory

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	memcore "github.com/Tsinling0525/rivulet/memory"
	"github.com/Tsinling0525/rivulet/model"
)

func storeFor(node model.Node) *memcore.FileStore {
	storeDir, _ := node.Config["store_dir"].(string)
	return memcore.NewFileStore(storeDir)
}

func userIDFor(node model.Node, item model.Item) string {
	if value, ok := node.Config["user_id"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	field := stringConfig(node, "user_id_field", "user_id")
	if value, ok := item[field].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return stringConfig(node, "default_user_id", "default")
}

func textFor(node model.Node, item model.Item, configKey, fallbackField string) string {
	field := stringConfig(node, configKey, fallbackField)
	if value := stringFromAny(item[field]); value != "" {
		return value
	}
	for _, candidate := range []string{"proposition", "memory", "observation", "text", "body", "query"} {
		if value := stringFromAny(item[candidate]); value != "" {
			return value
		}
	}
	if value, ok := node.Config[fallbackField].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func propositionInputs(node model.Node, item model.Item, text string) []memcore.PropositionInput {
	propositions := stringsFromAny(firstPresent(item["propositions"], node.Config["propositions"]))
	if len(propositions) == 0 && strings.TrimSpace(text) != "" {
		propositions = []string{text}
	}
	assumptions := stringsFromAny(firstPresent(item["assumptions"], node.Config["assumptions"]))
	domains := stringsFromAny(firstPresent(item["domains"], node.Config["domains"]))
	horizon := stringFromAny(firstPresent(item["horizon"], node.Config["horizon"]))
	source := stringFromAny(firstPresent(item["source"], node.Config["source"]))
	if source == "" {
		source = stringFromAny(node.ID)
	}
	metadata := map[string]any{}
	if raw, ok := item["metadata"].(map[string]any); ok {
		for key, value := range raw {
			metadata[key] = value
		}
	}

	out := make([]memcore.PropositionInput, 0, len(propositions))
	for _, proposition := range propositions {
		input := memcore.PropositionInput{
			Text:        proposition,
			Kind:        memcore.NodeKindProposition,
			Assumptions: assumptions,
			Domains:     domains,
			Horizon:     horizon,
			Evidence:    []string{text},
			Source:      source,
			Metadata:    metadata,
		}
		if len(input.Assumptions) == 0 && len(input.Domains) == 0 && input.Horizon == "" {
			extracted := memcore.ExtractPropositions(proposition)
			if len(extracted) > 0 {
				input.Text = extracted[0].Text
				input.Assumptions = extracted[0].Assumptions
				input.Domains = extracted[0].Domains
				input.Horizon = extracted[0].Horizon
			}
		}
		out = append(out, input)
	}
	return out
}

func cloneItem(item model.Item) model.Item {
	out := model.Item{}
	for key, value := range item {
		out[key] = value
	}
	return out
}

func invalidationsToItems(values []memcore.Invalidation) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		out = append(out, map[string]any{
			"node_id":        value.NodeID,
			"proposition":    value.Proposition,
			"previous_state": string(value.PreviousState),
			"state":          string(value.State),
			"risk":           value.Risk,
			"reason":         value.Reason,
			"direct":         value.Direct,
		})
	}
	return out
}

func edgesToItems(values []memcore.Edge) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		out = append(out, map[string]any{
			"from":       value.From,
			"to":         value.To,
			"type":       string(value.Type),
			"confidence": value.Confidence,
			"evidence":   value.Evidence,
			"polarity":   string(value.Polarity),
			"updated_at": value.UpdatedAt,
		})
	}
	return out
}

func nodesToItems(values []memcore.Node) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		out = append(out, nodeToItem(value, 0))
	}
	return out
}

func matchesToItems(values []memcore.QueryMatch) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		out = append(out, nodeToItem(value.Node, value.Score))
	}
	return out
}

func nodeToItem(value memcore.Node, score float64) map[string]any {
	item := map[string]any{
		"id":          value.ID,
		"kind":        string(value.Kind),
		"proposition": value.Proposition,
		"state":       string(value.State),
		"confidence":  value.Confidence,
		"risk":        value.Risk,
		"domains":     value.Domains,
		"assumptions": value.Assumptions,
		"horizon":     value.Horizon,
		"source":      value.Source,
		"written_at":  value.WrittenAt,
		"updated_at":  value.UpdatedAt,
	}
	if score > 0 {
		item["score"] = score
	}
	return item
}

func memoryContext(matches []memcore.QueryMatch) string {
	lines := make([]string, 0, len(matches))
	for _, match := range matches {
		lines = append(lines, fmt.Sprintf("- [%s %.2f] %s", match.Node.State, match.Score, match.Node.Proposition))
	}
	return strings.Join(lines, "\n")
}

func parseStates(raw any, fallback []memcore.ValidityState) []memcore.ValidityState {
	values := stringsFromAny(raw)
	if len(values) == 0 {
		return fallback
	}
	out := make([]memcore.ValidityState, 0, len(values))
	for _, value := range values {
		switch memcore.ValidityState(value) {
		case memcore.StateActive, memcore.StateSuspect, memcore.StateStale, memcore.StateUnknownCurrent:
			out = append(out, memcore.ValidityState(value))
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func stringConfig(node model.Node, key, fallback string) string {
	if value, ok := node.Config[key].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func boolConfig(node model.Node, key string, fallback bool) bool {
	if value, ok := node.Config[key].(bool); ok {
		return value
	}
	return fallback
}

func intConfig(node model.Node, key string, fallback int) int {
	switch value := node.Config[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return int(parsed)
		}
	}
	return fallback
}

func floatConfig(node model.Node, key string, fallback float64) float64 {
	switch value := node.Config[key].(type) {
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case float64:
		return value
	case float32:
		return float64(value)
	case json.Number:
		if parsed, err := value.Float64(); err == nil {
			return parsed
		}
	}
	return fallback
}

func stringFromAny(raw any) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		return ""
	}
}

func stringsFromAny(raw any) []string {
	switch value := raw.(type) {
	case []string:
		return uniqueSorted(value)
	case []any:
		out := make([]string, 0, len(value))
		for _, entry := range value {
			if text := stringFromAny(entry); text != "" {
				out = append(out, text)
			}
		}
		return uniqueSorted(out)
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		parts := strings.Split(value, ",")
		if len(parts) == 1 {
			parts = strings.Split(value, "\n")
		}
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		return uniqueSorted(out)
	default:
		return nil
	}
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func firstPresent(values ...any) any {
	for _, value := range values {
		if value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return value
			}
		case []string:
			if len(typed) > 0 {
				return value
			}
		case []any:
			if len(typed) > 0 {
				return value
			}
		default:
			return value
		}
	}
	return nil
}
