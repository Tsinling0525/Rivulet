package memory

import (
	"sort"
	"strings"
	"time"
)

func (g *Graph) AddObservation(text string, opts WriteOptions) WriteResult {
	inputs := ExtractPropositions(text)
	for i := range inputs {
		if inputs[i].Source == "" {
			inputs[i].Source = opts.Source
		}
	}
	return g.AddPropositions(inputs, opts)
}

func (g *Graph) AddPropositions(inputs []PropositionInput, opts WriteOptions) WriteResult {
	now := resolveNow(opts.Now)
	state := opts.State
	if state == "" {
		state = StateActive
	}
	confidence := opts.Confidence
	if confidence <= 0 || confidence > 1 {
		confidence = 1
	}
	g.ensure(now)

	result := WriteResult{}
	for _, input := range inputs {
		text := strings.TrimSpace(input.Text)
		if text == "" {
			continue
		}
		kind := input.Kind
		if kind == "" {
			kind = NodeKindProposition
		}
		if input.Source == "" {
			input.Source = opts.Source
		}
		node, created := g.upsertNode(PropositionInput{
			Text:        text,
			Kind:        kind,
			Assumptions: uniqueStrings(input.Assumptions),
			Domains:     uniqueStrings(input.Domains),
			Horizon:     input.Horizon,
			Evidence:    uniqueStrings(input.Evidence),
			Source:      input.Source,
			Metadata:    input.Metadata,
		}, state, confidence, now)
		if created {
			result.Nodes = append(result.Nodes, node)
		}

		if kind != NodeKindProposition {
			continue
		}
		for _, assumption := range input.Assumptions {
			assumptionInput := PropositionInput{
				Text:     assumption,
				Kind:     NodeKindAssumption,
				Domains:  input.Domains,
				Evidence: []string{text},
				Source:   input.Source,
			}
			assumptionNode, created := g.upsertNode(assumptionInput, StateActive, confidence, now)
			if created {
				result.Nodes = append(result.Nodes, assumptionNode)
			}
			edge := Edge{
				From:       assumptionNode.ID,
				To:         node.ID,
				Type:       dependencyTypeForAssumption(assumption, input.Domains),
				Confidence: 0.95,
				Evidence:   assumption,
				Polarity:   PolarityPositive,
				UpdatedAt:  now,
			}
			if saved, changed := g.upsertEdge(edge); changed {
				result.Edges = append(result.Edges, saved)
			}
		}
	}
	g.UpdatedAt = now
	return result
}

func (g *Graph) UpdateWithObservation(text string, opts UpdateOptions) UpdateResult {
	opts = opts.withDefaults()
	now := resolveNow(opts.Now)
	g.ensure(now)

	existingIDs := map[string]bool{}
	existing := make([]Node, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		existingIDs[node.ID] = true
		existing = append(existing, node)
	}

	result := UpdateResult{}
	if opts.StoreObservation {
		write := g.AddObservation(text, WriteOptions{Now: opts.Now, Source: opts.Source})
		result.Observations = write.Nodes
	}

	score := triggerScore(text, existing)
	result.TriggerScore = score
	result.Triggered = score > opts.TriggerThreshold
	if !result.Triggered {
		g.UpdatedAt = now
		return result
	}

	previous := map[string]ValidityState{}
	for _, node := range g.Nodes {
		previous[node.ID] = node.State
	}

	directRisk := map[string]float64{}
	riskReason := map[string]string{}
	for _, node := range existing {
		if node.State == StateStale {
			continue
		}
		risk, reason := directInvalidationScore(text, node)
		result.NodesExamined++
		if risk < opts.DirectThreshold {
			continue
		}
		directRisk[node.ID] = risk
		riskReason[node.ID] = reason
	}

	edgesUpdated := g.refineEdges(directRisk, existingIDs, now, opts.EdgeThreshold)
	result.EdgesUpdated = append(result.EdgesUpdated, edgesUpdated...)

	risk := map[string]float64{}
	for id, value := range directRisk {
		risk[id] = value
	}
	for depth := 0; depth < opts.MaxDepth; depth++ {
		next := cloneRisk(risk)
		for _, edge := range g.Edges {
			if edge.Polarity == PolarityNegative {
				continue
			}
			sourceRisk := risk[edge.From]
			if sourceRisk <= 0 {
				continue
			}
			transfer := edge.Confidence * propagationCoefficient(edge.Type) * opts.PathDecay
			if transfer <= 0 {
				continue
			}
			prior := next[edge.To]
			next[edge.To] = 1 - ((1 - prior) * (1 - transfer*sourceRisk))
		}
		risk = next
	}

	directSet := map[string]bool{}
	for id := range directRisk {
		directSet[id] = true
	}

	for i := range g.Nodes {
		nodeRisk := risk[g.Nodes[i].ID]
		if nodeRisk <= 0 {
			continue
		}
		prev := g.Nodes[i].State
		if directSet[g.Nodes[i].ID] {
			g.Nodes[i].State = stateForDirectRisk(nodeRisk, opts)
			g.Nodes[i].Risk = nodeRisk
			g.Nodes[i].Confidence = clamp01(1 - nodeRisk)
			g.Nodes[i].UpdatedAt = now
			result.Direct = append(result.Direct, Invalidation{
				NodeID:        g.Nodes[i].ID,
				Proposition:   g.Nodes[i].Proposition,
				PreviousState: previous[g.Nodes[i].ID],
				State:         g.Nodes[i].State,
				Risk:          nodeRisk,
				Reason:        riskReason[g.Nodes[i].ID],
				Direct:        true,
			})
			continue
		}
		if !existingIDs[g.Nodes[i].ID] {
			continue
		}
		nextState := stateForPropagatedRisk(g.Nodes[i].ID, nodeRisk, g.Edges, opts)
		if nextState == prev && nodeRisk < opts.SuspicionThreshold {
			continue
		}
		g.Nodes[i].State = nextState
		g.Nodes[i].Risk = nodeRisk
		g.Nodes[i].Confidence = clamp01(1 - nodeRisk)
		g.Nodes[i].UpdatedAt = now
		result.Propagated = append(result.Propagated, Invalidation{
			NodeID:        g.Nodes[i].ID,
			Proposition:   g.Nodes[i].Proposition,
			PreviousState: previous[g.Nodes[i].ID],
			State:         g.Nodes[i].State,
			Risk:          nodeRisk,
			Reason:        "dependency risk propagated from an invalidated premise",
			Direct:        false,
		})
	}
	sortInvalidations(result.Direct)
	sortInvalidations(result.Propagated)
	g.UpdatedAt = now
	return result
}

func (g Graph) Query(query string, opts QueryOptions) QueryResult {
	if opts.MaxResults <= 0 {
		opts.MaxResults = 8
	}
	if len(opts.States) == 0 {
		opts.States = []ValidityState{StateActive}
	}
	if len(opts.WarningStates) == 0 {
		opts.WarningStates = []ValidityState{StateSuspect, StateUnknownCurrent, StateStale}
	}
	stateSet := validitySet(opts.States)
	warningSet := validitySet(opts.WarningStates)

	matches := []QueryMatch{}
	warnings := []QueryMatch{}
	for _, node := range g.Nodes {
		if node.Kind != NodeKindProposition && node.Kind != NodeKindObservation {
			continue
		}
		score := queryScore(query, node)
		if score <= 0 && strings.TrimSpace(query) != "" {
			continue
		}
		match := QueryMatch{Node: node, Score: score, Reason: "lexical/domain match"}
		if stateSet[node.State] {
			matches = append(matches, match)
			continue
		}
		if opts.IncludeWarnings && warningSet[node.State] {
			warnings = append(warnings, match)
		}
	}
	sortMatches(matches)
	sortMatches(warnings)
	if len(matches) > opts.MaxResults {
		matches = matches[:opts.MaxResults]
	}
	if len(warnings) > opts.MaxResults {
		warnings = warnings[:opts.MaxResults]
	}
	return QueryResult{Matches: matches, Warnings: warnings}
}

func (g *Graph) ensure(now time.Time) {
	if g.Version == 0 {
		g.Version = 1
	}
	if g.UpdatedAt.IsZero() {
		g.UpdatedAt = now
	}
	for i := range g.Nodes {
		if g.Nodes[i].State == "" {
			g.Nodes[i].State = StateActive
		}
		if g.Nodes[i].Kind == "" {
			g.Nodes[i].Kind = NodeKindProposition
		}
		if g.Nodes[i].Confidence <= 0 || g.Nodes[i].Confidence > 1 {
			g.Nodes[i].Confidence = 1
		}
	}
}

func (g *Graph) upsertNode(input PropositionInput, state ValidityState, confidence float64, now time.Time) (Node, bool) {
	norm := normalizeText(input.Text)
	for i := range g.Nodes {
		if g.Nodes[i].Kind == input.Kind && normalizeText(g.Nodes[i].Proposition) == norm {
			g.Nodes[i].Assumptions = uniqueStrings(append(g.Nodes[i].Assumptions, input.Assumptions...))
			g.Nodes[i].Domains = uniqueStrings(append(g.Nodes[i].Domains, input.Domains...))
			g.Nodes[i].Evidence = uniqueStrings(append(g.Nodes[i].Evidence, input.Evidence...))
			if input.Horizon != "" {
				g.Nodes[i].Horizon = input.Horizon
			}
			if input.Source != "" {
				g.Nodes[i].Source = input.Source
			}
			if input.Metadata != nil {
				if g.Nodes[i].Metadata == nil {
					g.Nodes[i].Metadata = map[string]any{}
				}
				for key, value := range input.Metadata {
					g.Nodes[i].Metadata[key] = value
				}
			}
			g.Nodes[i].UpdatedAt = now
			if g.Nodes[i].State == StateStale && state == StateActive {
				g.Nodes[i].State = StateSuspect
				g.Nodes[i].Risk = 0.5
				g.Nodes[i].Confidence = 0.5
			}
			return g.Nodes[i], false
		}
	}

	prefix := "m"
	if input.Kind == NodeKindAssumption {
		prefix = "a"
	} else if input.Kind == NodeKindObservation {
		prefix = "o"
	}
	node := Node{
		ID:          prefix + "_" + hashString(string(input.Kind)+"\n"+norm),
		Kind:        input.Kind,
		Proposition: input.Text,
		WrittenAt:   now,
		UpdatedAt:   now,
		State:       state,
		Confidence:  confidence,
		Domains:     uniqueStrings(input.Domains),
		Assumptions: uniqueStrings(input.Assumptions),
		Horizon:     input.Horizon,
		Evidence:    uniqueStrings(input.Evidence),
		Source:      input.Source,
		Metadata:    cloneMetadata(input.Metadata),
	}
	node.ID = g.uniqueNodeID(node.ID)
	g.Nodes = append(g.Nodes, node)
	return node, true
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	copy := make(map[string]any, len(metadata))
	for key, value := range metadata {
		copy[key] = value
	}
	return copy
}

func (g *Graph) uniqueNodeID(candidate string) string {
	used := map[string]bool{}
	for _, node := range g.Nodes {
		used[node.ID] = true
	}
	if !used[candidate] {
		return candidate
	}
	for i := 2; ; i++ {
		next := candidate + "_" + strings.TrimLeft(time.Unix(int64(i), 0).Format("150405"), "0")
		if !used[next] {
			return next
		}
	}
}

func (g *Graph) upsertEdge(edge Edge) (Edge, bool) {
	if edge.Polarity == "" {
		edge.Polarity = PolarityPositive
	}
	if edge.Confidence <= 0 || edge.Confidence > 1 {
		edge.Confidence = 0.5
	}
	for i := range g.Edges {
		if g.Edges[i].From == edge.From && g.Edges[i].To == edge.To && g.Edges[i].Type == edge.Type {
			changed := false
			if edge.Confidence > g.Edges[i].Confidence {
				g.Edges[i].Confidence = edge.Confidence
				changed = true
			}
			if edge.Evidence != "" && edge.Evidence != g.Edges[i].Evidence {
				g.Edges[i].Evidence = edge.Evidence
				changed = true
			}
			if !edge.UpdatedAt.IsZero() {
				g.Edges[i].UpdatedAt = edge.UpdatedAt
			}
			return g.Edges[i], changed
		}
	}
	g.Edges = append(g.Edges, edge)
	return edge, true
}

func dependencyTypeForAssumption(assumption string, domains []string) DependencyType {
	norm := normalizeText(assumption)
	if containsAny(norm, "able", "access", "commute", "location", "tool", "resource") {
		return DependencyFunctional
	}
	for _, domain := range domains {
		if normalizeText(domain) == "preference" {
			return DependencyContextual
		}
	}
	return DependencyCausal
}

func (g *Graph) refineEdges(directRisk map[string]float64, existingIDs map[string]bool, now time.Time, threshold float64) []Edge {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.55
	}
	updated := []Edge{}
	byID := g.nodeMap()
	for sourceID := range directRisk {
		source := byID[sourceID]
		for _, target := range g.Nodes {
			if target.ID == sourceID || !existingIDs[target.ID] {
				continue
			}
			conf, depType, evidence := heuristicDependency(source, target)
			if conf < threshold {
				continue
			}
			edge := Edge{
				From:       sourceID,
				To:         target.ID,
				Type:       depType,
				Confidence: conf,
				Evidence:   evidence,
				Polarity:   PolarityPositive,
				UpdatedAt:  now,
			}
			if saved, changed := g.upsertEdge(edge); changed {
				updated = append(updated, saved)
			}
		}
	}
	return updated
}

func (g Graph) nodeMap() map[string]Node {
	out := make(map[string]Node, len(g.Nodes))
	for _, node := range g.Nodes {
		out[node.ID] = node
	}
	return out
}

func heuristicDependency(source, target Node) (float64, DependencyType, string) {
	if source.Kind == NodeKindAssumption {
		for _, assumption := range target.Assumptions {
			if normalizeText(assumption) == normalizeText(source.Proposition) {
				return 0.95, dependencyTypeForAssumption(assumption, target.Domains), assumption
			}
		}
	}
	if len(intersectStrings(source.Domains, target.Domains)) > 0 && tokenSimilarity(source.Proposition, target.Proposition) > 0.12 {
		return 0.58, DependencyContextual, "shared domain and semantic overlap"
	}
	return 0, "", ""
}

func triggerScore(observation string, nodes []Node) float64 {
	norm := normalizeText(observation)
	score := 0.0
	if containsAny(norm, "no longer", "moved", "relocated", "quit", "stopped", "started", "changed", "broke", "injured", "cast", "recovered", "cleared", "new") {
		score += 0.55
	}
	best := 0.0
	for _, node := range nodes {
		if node.State == StateStale {
			continue
		}
		if sim := tokenSimilarity(observation, node.Proposition); sim > best {
			best = sim
		}
	}
	score += best * 0.45
	return clamp01(score)
}

func directInvalidationScore(observation string, node Node) (float64, string) {
	obs := normalizeText(observation)
	prop := normalizeText(node.Proposition)

	if isHealthLimitation(obs) && impliesPhysicalAbility(prop) {
		return 0.95, "new health or mobility observation invalidates an ability premise"
	}
	if containsAny(obs, "sold bike", "bike stolen", "no bike") && strings.Contains(prop, "bike") {
		return 0.9, "new observation removes bike access"
	}
	if containsAny(obs, "moved", "relocated", "new address") && containsAny(prop, "address", "home", "location", "reachable", "commute", "work") {
		return 0.88, "location change may supersede an older location-dependent memory"
	}
	if containsAny(obs, "quit", "left job", "changed job", "changed jobs", "new job") && containsAny(prop, "work", "job", "office", "commute") {
		return 0.88, "work change may supersede an older work-dependent memory"
	}
	if containsAny(obs, "no longer", "not", "never", "stopped") && tokenSimilarity(observation, node.Proposition) > 0.18 {
		return 0.82, "new negated observation overlaps the older memory"
	}
	if containsAny(obs, "changed", "started", "stopped") && tokenSimilarity(observation, node.Proposition) > 0.22 {
		return 0.62, "state-change cue overlaps the older memory"
	}
	return 0, ""
}

func isHealthLimitation(value string) bool {
	return containsAny(value, "broke", "broken", "injured", "injury", "cast", "surgery", "sprain", "sprained")
}

func impliesPhysicalAbility(value string) bool {
	return strings.Contains(value, "physically able") ||
		(strings.Contains(value, "able") && strings.Contains(value, "bike")) ||
		strings.Contains(value, "can bike")
}

func (opts UpdateOptions) withDefaults() UpdateOptions {
	if opts.TriggerThreshold <= 0 || opts.TriggerThreshold > 1 {
		opts.TriggerThreshold = 0.25
	}
	if opts.DirectThreshold <= 0 || opts.DirectThreshold > 1 {
		opts.DirectThreshold = 0.7
	}
	if opts.SuspicionThreshold <= 0 || opts.SuspicionThreshold > 1 {
		opts.SuspicionThreshold = 0.35
	}
	if opts.StaleThreshold <= 0 || opts.StaleThreshold > 1 {
		opts.StaleThreshold = 0.72
	}
	if opts.UncertaintyThreshold <= 0 || opts.UncertaintyThreshold > 1 {
		opts.UncertaintyThreshold = 0.45
	}
	if opts.EdgeThreshold <= 0 || opts.EdgeThreshold > 1 {
		opts.EdgeThreshold = 0.55
	}
	if opts.PathDecay <= 0 || opts.PathDecay > 1 {
		opts.PathDecay = 0.95
	}
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 2
	}
	return opts
}

func stateForDirectRisk(risk float64, opts UpdateOptions) ValidityState {
	if risk >= opts.StaleThreshold {
		return StateStale
	}
	return StateSuspect
}

func stateForPropagatedRisk(nodeID string, risk float64, edges []Edge, opts UpdateOptions) ValidityState {
	if risk < opts.SuspicionThreshold {
		return StateActive
	}
	uncertainty := propagationUncertainty(nodeID, edges)
	if risk >= opts.StaleThreshold && uncertainty < opts.UncertaintyThreshold {
		return StateUnknownCurrent
	}
	return StateSuspect
}

func propagationUncertainty(nodeID string, edges []Edge) float64 {
	best := 0.0
	for _, edge := range edges {
		if edge.To == nodeID && edge.Polarity == PolarityPositive && edge.Confidence > best {
			best = edge.Confidence
		}
	}
	if best == 0 {
		return 1
	}
	return 1 - best
}

func propagationCoefficient(depType DependencyType) float64 {
	switch depType {
	case DependencyLogical:
		return 1
	case DependencyFunctional:
		return 1
	case DependencyTemporal:
		return 0.9
	case DependencyCausal:
		return 0.75
	case DependencyContextual:
		return 0.45
	default:
		return 0.5
	}
}

func queryScore(query string, node Node) float64 {
	if strings.TrimSpace(query) == "" {
		return 0.1
	}
	score := tokenSimilarity(query, node.Proposition)
	_, queryDomains, _ := inferAssumptions(query)
	if len(intersectStrings(queryDomains, node.Domains)) > 0 {
		score += 0.2
	}
	if containsAny(normalizeText(query), "bike", "biking", "route", "commute") && strings.Contains(normalizeText(node.Proposition), "bike") {
		score += 0.25
	}
	return clamp01(score)
}

func validitySet(states []ValidityState) map[ValidityState]bool {
	out := map[ValidityState]bool{}
	for _, state := range states {
		out[state] = true
	}
	return out
}

func intersectStrings(a, b []string) []string {
	left := map[string]bool{}
	for _, value := range a {
		left[normalizeText(value)] = true
	}
	out := []string{}
	for _, value := range b {
		if left[normalizeText(value)] {
			out = append(out, value)
		}
	}
	return out
}

func cloneRisk(src map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func resolveNow(fn func() time.Time) time.Time {
	if fn != nil {
		return fn().UTC()
	}
	return time.Now().UTC()
}

func sortInvalidations(values []Invalidation) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Risk == values[j].Risk {
			return values[i].NodeID < values[j].NodeID
		}
		return values[i].Risk > values[j].Risk
	})
}

func sortMatches(values []QueryMatch) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Score == values[j].Score {
			return values[i].Node.WrittenAt.After(values[j].Node.WrittenAt)
		}
		return values[i].Score > values[j].Score
	})
}
