package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type testCapability interface{ Value() string }

type testProvider string

func (p testProvider) Value() string { return string(p) }

func TestCapabilityBecomesAvailableAndIsRemovedWithScope(t *testing.T) {
	capabilities := NewContext()
	if _, err := Require[testCapability](capabilities); err == nil {
		t.Fatal("expected a missing capability error")
	}

	scope := NewScope(context.Background())
	if err := ProvideInScope[testCapability](scope, capabilities, testProvider("provider-a")); err != nil {
		t.Fatalf("provide capability: %v", err)
	}
	capability, err := Require[testCapability](capabilities)
	if err != nil {
		t.Fatalf("require capability: %v", err)
	}
	if got := capability.Value(); got != "provider-a" {
		t.Fatalf("capability value = %q, want provider-a", got)
	}
	if err := scope.Close(); err != nil {
		t.Fatalf("close scope: %v", err)
	}
	if _, err := Require[testCapability](capabilities); err == nil {
		t.Fatal("expected capability to disappear after scope close")
	}
}

func TestScopeDisposesEffectsInReverseOrder(t *testing.T) {
	scope := NewScope(context.Background())
	var order []string
	for _, name := range []string{"A", "B", "C"} {
		name := name
		if err := scope.Effect(func() error {
			order = append(order, name)
			return nil
		}); err != nil {
			t.Fatalf("add effect: %v", err)
		}
	}
	if err := scope.Close(); err != nil {
		t.Fatalf("close scope: %v", err)
	}
	if want := []string{"C", "B", "A"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("cleanup order = %v, want %v", order, want)
	}
}

func TestScopeCancelsAndWaitsForWorkers(t *testing.T) {
	scope := NewScope(context.Background())
	stopped := make(chan struct{})
	if err := scope.Go(func(ctx context.Context) {
		<-ctx.Done()
		close(stopped)
	}); err != nil {
		t.Fatalf("start worker: %v", err)
	}
	if err := scope.Close(); err != nil {
		t.Fatalf("close scope: %v", err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("scope returned before its worker stopped")
	}
	if err := scope.Effect(func() error { return errors.New("unreachable") }); err == nil {
		t.Fatal("expected adding an effect after close to fail")
	}
}
