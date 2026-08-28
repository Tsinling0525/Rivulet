// Package runtime provides the small amount of composition infrastructure used
// to assemble independently-owned Rivulet runtime capabilities.
//
// It deliberately does not contain agent or workflow behavior. Consumers use
// typed Require calls at composition boundaries; ordinary implementation
// details should continue to use explicit constructor arguments.
package runtime

import (
	"fmt"
	"reflect"
	"sync"
)

// Context records capabilities made available to a runtime scope. It is not a
// service locator for application logic: use it while composing components and
// inject the required contract into the constructed consumer.
type Context struct {
	mu           sync.RWMutex
	capabilities map[reflect.Type]entry
	nextID       uint64
}

type entry struct {
	value any
	id    uint64
}

func NewContext() *Context {
	return &Context{capabilities: make(map[reflect.Type]entry)}
}

// Provide makes a typed capability available. Its disposer only removes the
// capability if it is still the provider it installed, making cleanup safe if
// a later provider replaces it.
func Provide[T any](c *Context, capability T) (func(), error) {
	if c == nil {
		return nil, fmt.Errorf("runtime capability context is required")
	}
	key := capabilityType[T]()
	if isNilCapability(capability) {
		return nil, fmt.Errorf("runtime capability %s is nil", key)
	}

	c.mu.Lock()
	c.nextID++
	providerID := c.nextID
	c.capabilities[key] = entry{value: capability, id: providerID}
	c.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			if current, ok := c.capabilities[key]; ok && current.id == providerID {
				delete(c.capabilities, key)
			}
		})
	}, nil
}

// Require returns a capability by its contract type. A missing capability is
// always an explicit error rather than a nil dereference later in startup.
func Require[T any](c *Context) (T, error) {
	var zero T
	if c == nil {
		return zero, fmt.Errorf("runtime capability context is required")
	}
	key := capabilityType[T]()
	c.mu.RLock()
	value, ok := c.capabilities[key]
	c.mu.RUnlock()
	if !ok {
		return zero, fmt.Errorf("required runtime capability %s is unavailable", key)
	}
	capability, ok := value.value.(T)
	if !ok {
		return zero, fmt.Errorf("runtime capability %s has incompatible provider %T", key, value.value)
	}
	return capability, nil
}

func capabilityType[T any]() reflect.Type {
	return reflect.TypeOf((*T)(nil)).Elem()
}

func isNilCapability[T any](value T) bool {
	v := reflect.ValueOf(value)
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
