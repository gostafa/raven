package retry

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestRetryClassificationAndBudgets(t *testing.T) {
	for _, failure := range []error{os.ErrNotExist, os.ErrPermission, os.ErrClosed, io.EOF, io.ErrUnexpectedEOF, context.Canceled, errors.New("unknown")} {
		if Transient(failure) {
			t.Fatalf("permanent error retried: %v", failure)
		}
	}
	for _, failure := range []error{syscall.EINTR, syscall.EAGAIN, syscall.EMFILE, syscall.ENFILE, syscall.ENOBUFS} {
		if !Transient(failure) {
			t.Fatalf("temporary error rejected: %v", failure)
		}
	}
	settings := policy{
		attempts: 3,
		timeout:  time.Second,
		initial:  time.Nanosecond,
		maximum:  time.Nanosecond,
	}
	for _, succeed := range []bool{true, false} {
		calls := 0
		value, err := testRetry(
			t.Context(),
			settings,
			slog.New(slog.NewTextHandler(io.Discard, nil)),
			slog.LevelDebug,
			"test",
			Transient,
			func(context.Context) (int, error) {
				calls++
				if succeed && calls == 3 {
					return 42, nil
				}
				return 0, syscall.EAGAIN
			},
		)
		if calls != 3 || succeed && (err != nil || value != 42) ||
			!succeed && !errors.Is(err, syscall.EAGAIN) {
			t.Fatalf("calls=%d value=%d error=%v", calls, value, err)
		}
	}
	calls := 0
	_, err := testRetry(
		t.Context(),
		settings,
		nil,
		slog.LevelDebug,
		"test",
		Transient,
		func(context.Context) (int, error) {
			calls++
			return 0, os.ErrPermission
		},
	)
	if calls != 1 || !errors.Is(err, os.ErrPermission) {
		t.Fatal(calls, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = testRetry(
		ctx,
		settings,
		nil,
		slog.LevelDebug,
		"test",
		Transient,
		func(context.Context) (int, error) { t.Error("ran canceled operation"); return 0, nil },
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestRetryDeadlineAndOwnerCancellation(t *testing.T) {
	for _, operation := range []Operation{nil, NewReads(t.Context(), nil).Do} {
		if err := operation.Do(
			t.Context(),
			func(context.Context) error { return nil },
		); err != nil {
			t.Fatal(err)
		}
	}
	if err := NewReads(
		t.Context(),
		nil,
	).Do(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	settings := policy{
		attempts: 5,
		timeout:  10 * time.Millisecond,
		initial:  time.Second,
		maximum:  time.Second,
	}
	_, err := testRetry(
		t.Context(),
		settings,
		nil,
		slog.LevelDebug,
		"test",
		Transient,
		func(context.Context) (int, error) { return 0, syscall.EAGAIN },
	)
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, syscall.EAGAIN) {
		t.Fatal(err)
	}
	owner, cancel := context.WithCancel(t.Context())
	reads := NewReads(owner, nil)
	calls := 0
	err = reads.Do(
		t.Context(),
		func(context.Context) error { calls++; cancel(); return syscall.EAGAIN },
	)
	if calls != 1 || !errors.Is(err, context.Canceled) {
		t.Fatal(calls, err)
	}
	err = reads.Do(
		t.Context(),
		func(context.Context) error { t.Error("read after shutdown"); return nil },
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestInitializationSingleFlightAndCallerIsolation(t *testing.T) {
	entered, unblock := make(chan struct{}), make(chan struct{})
	var calls, releases atomic.Int32
	resource := testInitialization(t.Context(), nil, "test", func(context.Context) (*int, error) {
		calls.Add(1)
		close(entered)
		<-unblock
		value := 42
		return &value, nil
	}, func(*int) error { releases.Add(1); return nil }, Transient)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { _, err := resource.Get(ctx); done <- err }()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(unblock)
	var workers sync.WaitGroup
	for range 20 {
		workers.Go(func() {
			value, err := resource.Get(t.Context())
			if err != nil || *value != 42 {
				t.Error(value, err)
			}
		})
	}
	workers.Wait()
	if err := resource.Close(); err != nil {
		t.Fatal(err)
	}
	if err := resource.Close(); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || releases.Load() != 1 {
		t.Fatal(calls.Load(), releases.Load())
	}
	if _, err := resource.Get(t.Context()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestInitializationShutdownDrainsLateResource(t *testing.T) {
	entered, unblock := make(chan struct{}), make(chan struct{})
	var released atomic.Bool
	resource := testInitialization(t.Context(), nil, "test", func(context.Context) (int, error) {
		close(entered)
		<-unblock
		return 42, nil
	}, func(int) error { released.Store(true); return nil }, Transient)
	getDone := make(chan struct{})
	go func() { _, _ = resource.Get(t.Context()); close(getDone) }()
	<-entered
	stopDone := make(chan struct{})
	go func() { _ = resource.Close(); close(stopDone) }()
	select {
	case <-stopDone:
		t.Fatal("shutdown did not drain initialization")
	case <-time.After(10 * time.Millisecond):
	}
	close(unblock)
	<-stopDone
	<-getDone
	if !released.Load() {
		t.Fatal("late resource leaked")
	}
}

func TestInitializationFailureIsCachedAndDormantClose(t *testing.T) {
	calls := 0
	resource := testInitialization(t.Context(), nil, "test", func(context.Context) (int, error) {
		calls++
		return 0, os.ErrPermission
	}, func(int) error { t.Error("released failed initialization"); return nil }, Transient)
	for range 2 {
		if _, err := resource.Get(t.Context()); !errors.Is(err, os.ErrPermission) {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatal(calls)
	}
	if err := resource.Close(); err != nil {
		t.Fatal(err)
	}
	dormant := testInitialization(t.Context(), nil, "dormant", func(context.Context) (int, error) {
		t.Error("dormant factory ran")
		return 0, nil
	}, func(int) error { return nil }, Transient)
	if err := dormant.Close(); err != nil {
		t.Fatal(err)
	}
}

func testInitialization[Value any](
	ctx context.Context,
	logger *slog.Logger,
	name string,
	factory func(context.Context) (Value, error),
	release func(Value) error,
	transient func(error) bool,
) *Initialization[Value] {
	return NewInitialization(
		ctx,
		factory,
		&Options[Value]{Name: name, Logger: logger, Release: release, Transient: transient},
	)
}

func testRetry[Value any](
	ctx context.Context,
	settings policy,
	logger *slog.Logger,
	level slog.Level,
	name string,
	transient func(error) bool,
	operation func(context.Context) (Value, error),
) (Value, error) {
	settings.logger, settings.level, settings.name, settings.transient = logger, level, name, transient
	return retry(ctx, &settings, operation)
}
