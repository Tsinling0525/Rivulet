package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Scope owns effects and goroutines created by one runtime component. Closing
// it cancels workers, waits for them, then disposes effects in reverse order.
type Scope struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	closed  bool
	effects []func() error
	wg      sync.WaitGroup
}

func NewScope(parent context.Context) *Scope {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	return &Scope{ctx: ctx, cancel: cancel}
}

func (s *Scope) Context() context.Context { return s.ctx }

// Effect records a cleanup action for a completed registration or mutation.
// Callers should add the cleanup immediately after the matching effect occurs.
func (s *Scope) Effect(dispose func() error) error {
	if s == nil {
		return fmt.Errorf("runtime scope is required")
	}
	if dispose == nil {
		return fmt.Errorf("runtime effect disposer is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("runtime scope is closed")
	}
	s.effects = append(s.effects, dispose)
	return nil
}

// Go starts a scope-owned worker. The worker must respect the supplied
// context; Close cancels it before waiting for completion.
func (s *Scope) Go(worker func(context.Context)) error {
	if s == nil {
		return fmt.Errorf("runtime scope is required")
	}
	if worker == nil {
		return fmt.Errorf("runtime worker is required")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("runtime scope is closed")
	}
	s.wg.Add(1)
	ctx := s.ctx
	s.mu.Unlock()

	go func() {
		defer s.wg.Done()
		worker(ctx)
	}()
	return nil
}

// Provide makes a capability available for the lifetime of this scope.
func ProvideInScope[T any](s *Scope, c *Context, capability T) error {
	dispose, err := Provide(c, capability)
	if err != nil {
		return err
	}
	if err := s.Effect(func() error {
		dispose()
		return nil
	}); err != nil {
		dispose()
		return err
	}
	return nil
}

// Close is idempotent. It returns every cleanup failure after attempting all
// disposers, preserving deterministic reverse-order cleanup.
func (s *Scope) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	effects := append([]func() error(nil), s.effects...)
	s.mu.Unlock()

	s.cancel()
	s.wg.Wait()

	var errs []error
	for index := len(effects) - 1; index >= 0; index-- {
		if err := effects[index](); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
