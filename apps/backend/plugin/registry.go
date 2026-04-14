package plugin

import "sync"

type factory func() NodeHandler

var (
	mu       sync.RWMutex
	registry = map[string]factory{}
)

func Register(nodeType string, f factory) {
	mu.Lock()
	defer mu.Unlock()
	registry[nodeType] = f
}

func New(nodeType string) (NodeHandler, bool) {
	mu.RLock()
	defer mu.RUnlock()
	f, ok := registry[nodeType]
	if !ok {
		return nil, false
	}
	return f(), true
}
