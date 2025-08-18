package engine

import "rivulet/model"

// Build adjacency + in-degree for topological scheduling
func topo(wf model.Workflow) (order []model.ID, indeg map[model.ID]int, out map[model.ID][]model.ID) {
	indeg = map[model.ID]int{}      // Track incoming edges per node
	out = map[model.ID][]model.ID{} // Track outgoing edges per node
	for _, n := range wf.Nodes {
		indeg[n.ID] = 0
	}
	for _, e := range wf.Edges {
		out[e.FromNode] = append(out[e.FromNode], e.ToNode)
		indeg[e.ToNode]++
	}
	// Kahn
	q := []model.ID{}
	for id, d := range indeg {
		if d == 0 {
			q = append(q, id)
		}
	}
	for len(q) > 0 {
		v := q[0]
		q = q[1:]
		order = append(order, v)
		for _, u := range out[v] {
			indeg[u]--
			if indeg[u] == 0 {
				q = append(q, u)
			}
		}
	}
	return
}
