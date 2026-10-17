package lifetime_test

import (
	"context"
	"testing"
	"time"

	"raven/internal/shared/lifetime"
)

func TestCancellationAndParentSubscription(t *testing.T) {
	for _, attach := range []bool{false, true} {
		runtime := lifetime.New()
		if runtime.Context().Err() != nil {
			t.Fatal("new runtime is canceled")
		}
		if attach {
			parent, cancel := context.WithCancel(t.Context())
			defer cancel()
			runtime.Attach(parent)
			cancel()
			select {
			case <-runtime.Context().Done():
			case <-time.After(time.Second):
				t.Fatal("parent cancellation did not reach runtime")
			}
		}
		runtime.Cancel()
		runtime.Cancel()
		if runtime.Context().Err() != context.Canceled {
			t.Fatal("runtime retained its context")
		}
	}
}
