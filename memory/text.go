package memory

import (
	"crypto/sha1"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
)

var sentenceSplitRE = regexp.MustCompile(`[.!?]\s+|\n+`)

func ExtractPropositions(text string) []PropositionInput {
	parts := sentenceSplitRE.Split(strings.TrimSpace(text), -1)
	out := make([]PropositionInput, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.Trim(part, " \t\r\n.!?"))
		if part == "" {
			continue
		}
		assumptions, domains, horizon := inferAssumptions(part)
		out = append(out, PropositionInput{
			Text:        part,
			Kind:        NodeKindProposition,
			Assumptions: assumptions,
			Domains:     domains,
			Horizon:     horizon,
			Evidence:    []string{text},
		})
	}
	return out
}

func inferAssumptions(text string) ([]string, []string, string) {
	norm := normalizeText(text)
	assumptions := []string{}
	domains := []string{}
	horizon := ""

	if containsAny(norm, "bike", "bikes", "biking", "cycle", "cycling") {
		assumptions = append(assumptions,
			"The user is physically able to bike",
			"The user has access to a bike",
		)
		domains = append(domains, "mobility", "health", "transportation")
		horizon = "current routine; re-evaluate if health, work, address, or transportation changes"
	}
	if containsAny(norm, "commute", "work", "job", "office") {
		assumptions = append(assumptions, "The user currently has a commute")
		domains = append(domains, "work")
		if horizon == "" {
			horizon = "current work context; re-evaluate if job, address, or routine changes"
		}
	}
	if containsAny(norm, "home", "address", "moved", "relocated", "location") {
		domains = append(domains, "location")
	}
	if containsAny(norm, "broke", "injured", "cast", "surgery", "recovered", "cleared") {
		domains = append(domains, "health", "mobility")
	}
	if containsAny(norm, "prefer", "likes", "dislikes", "favorite") {
		domains = append(domains, "preference")
	}

	return uniqueStrings(assumptions), uniqueStrings(domains), horizon
}

func normalizeText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
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

func tokenSet(value string) map[string]bool {
	tokens := map[string]bool{}
	for _, token := range strings.Fields(normalizeText(value)) {
		if len(token) < 2 || stopWords[token] {
			continue
		}
		tokens[token] = true
	}
	return tokens
}

func tokenSimilarity(a, b string) float64 {
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

func containsAny(value string, needles ...string) bool {
	padded := " " + value + " "
	for _, needle := range needles {
		if strings.Contains(padded, " "+needle+" ") {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := normalizeText(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func hashString(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}

var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "by": true, "for": true, "from": true, "has": true, "have": true,
	"i": true, "in": true, "is": true, "it": true, "my": true, "of": true,
	"on": true, "or": true, "the": true, "their": true, "to": true, "user": true,
	"was": true, "will": true, "with": true,
}
