package otter

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLoadingCacheLifecycleFailuresAndInvalidation(t *testing.T) {
	cache := New(&Options[string, string]{MaximumSize: 2})
	var loads atomic.Int64
	load := func(context.Context) (string, error) { loads.Add(1); return "value", nil }
	get := func() {
		t.Helper()
		if value, err := cache.Get(t.Context(), "key", load); err != nil || value != "value" {
			t.Fatalf("%q %v", value, err)
		}
	}
	cache.InvalidateAll()
	get()
	get()
	if loads.Load() != 2 {
		t.Fatal("dormant cache retained values")
	}
	if err := cache.Start(); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 20 {
		workers.Go(get)
	}
	workers.Wait()
	if loads.Load() != 3 {
		t.Fatalf("concurrent loads=%d", loads.Load())
	}
	cache.InvalidateAll()
	get()
	if loads.Load() != 4 {
		t.Fatal("invalidation did not load again")
	}
	if err := cache.Start(); err != nil {
		t.Fatal(err)
	}
	get()
	if loads.Load() != 5 {
		t.Fatal("restart retained entries")
	}
	want := errors.New("load failed")
	for range 2 {
		if _, err := cache.Get(
			t.Context(),
			"failure",
			func(context.Context) (string, error) { loads.Add(1); return "", want },
		); !errors.Is(
			err,
			want,
		) {
			t.Fatal(err)
		}
	}
	if loads.Load() != 7 {
		t.Fatal("cached a failure")
	}
	cache.Close()
	cache.Close()
	cache.InvalidateAll()
	get()
	if loads.Load() != 8 {
		t.Fatal("closed cache retained values")
	}
	bad := New(&Options[string, string]{MaximumSize: -1})
	if err := bad.Start(); err == nil {
		t.Fatal("accepted invalid options")
	}
	bad.Close()
}

func TestWeightedCacheDoesNotRetainOversizedValues(t *testing.T) {
	cache := New(
		&Options[string, string]{
			MaximumWeight: 4,
			Weigher:       func(_ string, value string) uint32 { return uint32(len(value)) },
		},
	)
	if err := cache.Start(); err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	loads := 0
	for range 2 {
		value, err := cache.Get(
			t.Context(),
			"large",
			func(context.Context) (string, error) { loads++; return "oversized", nil },
		)
		if err != nil || value != "oversized" {
			t.Fatalf("%q %v", value, err)
		}
	}
	if loads != 2 {
		t.Fatal("oversized value retained")
	}
	for _, key := range []string{"one", "two", "three"} {
		if _, err := cache.Get(
			t.Context(),
			key,
			func(context.Context) (string, error) { return "1234", nil },
		); err != nil {
			t.Fatal(err)
		}
	}
	cache.state.cache.CleanUp()
	if cache.state.cache.WeightedSize() > 4 {
		t.Fatal("weighted capacity exceeded after maintenance")
	}
}

func TestCloseDrainsLoadsAndWaiterCancellation(t *testing.T) {
	cache := New(&Options[string, string]{MaximumSize: 2})
	if err := cache.Start(); err != nil {
		t.Fatal(err)
	}
	started, release, ownerDone, closed := make(
		chan struct{},
	), make(
		chan struct{},
	), make(
		chan struct{},
	), make(
		chan struct{},
	)
	go func() {
		defer close(ownerDone)
		_, err := cache.Get(
			t.Context(),
			"key",
			func(context.Context) (string, error) { close(started); <-release; return "ready", nil },
		)
		if err != nil {
			t.Error(err)
		}
	}()
	<-started
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := cache.Get(
		ctx,
		"key",
		func(context.Context) (string, error) { t.Error("canceled caller loaded"); return "", nil },
	); !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatal(err)
	}
	go func() { cache.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("close did not drain load")
	default:
	}
	close(release)
	<-ownerDone
	<-closed
}
