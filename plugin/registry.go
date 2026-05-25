package plugin

import "sync"

type factory func() NodeHandler
type Resolver func(nodeType string) (NodeHandler, bool)

var (
	mu        sync.RWMutex
	registry  = map[string]factory{}
	resolvers []Resolver
)

func Register(nodeType string, f factory) {
	mu.Lock()
	defer mu.Unlock()
	registry[nodeType] = f
}

func RegisterResolver(r Resolver) {
	mu.Lock()
	defer mu.Unlock()
	resolvers = append(resolvers, r)
}

func New(nodeType string) (NodeHandler, bool) {
	mu.RLock()
	f, ok := registry[nodeType]
	if !ok {
		copiedResolvers := append([]Resolver(nil), resolvers...)
		mu.RUnlock()
		for _, r := range copiedResolvers {
			if handler, ok := r(nodeType); ok {
				return handler, true
			}
		}
		return nil, false
	}
	mu.RUnlock()
	return f(), true
}
