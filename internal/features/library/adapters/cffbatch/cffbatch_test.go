package cffbatch

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"testing/synctest"
)

func TestEmptySingleAndCanceledBatches(t *testing.T) {
	executor := New()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := executor.Run(ctx, nil); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("single task failed")
	if err := executor.Run(t.Context(), []func(context.Context) error{
		func(context.Context) error { return failure },
	}); !errors.Is(err, failure) {
		t.Fatalf("single error: %v", err)
	}
	for _, size := range []int{1, 8} {
		jobs := make([]func(context.Context) error, size)
		for index := range jobs {
			jobs[index] = func(context.Context) error { t.Error("canceled job ran"); return nil }
		}
		if err := executor.Run(ctx, jobs); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled batch: %v", err)
		}
	}
}

func TestConcurrencyBoundAndCancellationDrain(t *testing.T) {
	for _, size := range []int{2, 12} {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			release := make(chan struct{})
			var started, completed atomic.Int32
			jobs := make([]func(context.Context) error, size)
			for index := range jobs {
				jobs[index] = func(context.Context) error {
					started.Add(1)
					<-release
					completed.Add(1)
					return nil
				}
			}
			done := make(chan error, 1)
			go func() { done <- New().Run(ctx, jobs) }()
			synctest.Wait()
			want := int32(min(size, 4))
			if count := started.Load(); count != want {
				t.Fatalf("running jobs=%d, want %d", count, want)
			}
			cancel()
			synctest.Wait()
			select {
			case err := <-done:
				t.Fatalf("returned before drain: %v", err)
			default:
			}
			close(release)
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if started.Load() != want || completed.Load() != want {
				t.Fatalf("started=%d completed=%d", started.Load(), completed.Load())
			}
		})
	}
}

func TestErrorsPreserveInputOrderAndDrainOtherJobs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		first, second := errors.New("first"), errors.New("second")
		release := make(chan struct{})
		var completed atomic.Int32
		done := make(chan error, 1)
		go func() {
			done <- New().Run(t.Context(), []func(context.Context) error{
				func(context.Context) error { <-release; return first },
				func(context.Context) error { return second },
				func(context.Context) error { completed.Add(1); return nil },
			})
		}()
		synctest.Wait()
		select {
		case err := <-done:
			t.Fatalf("returned before first job finished: %v", err)
		default:
		}
		close(release)
		if err := <-done; !errors.Is(err, first) || completed.Load() != 1 {
			t.Fatalf("error=%v completed=%d", err, completed.Load())
		}
	})
}

func TestPanicsBecomeErrorsAndDoNotSkipJobs(t *testing.T) {
	for _, size := range []int{1, 6} {
		var completed atomic.Int32
		jobs := make([]func(context.Context) error, size)
		jobs[0] = func(context.Context) error { panic("broken task") }
		for index := 1; index < size; index++ {
			jobs[index] = func(context.Context) error { completed.Add(1); return nil }
		}
		if err := New().Run(t.Context(), jobs); !errors.Is(err, errTaskPanicked) {
			t.Fatalf("panic: %v", err)
		}
		if completed.Load() != int32(size-1) {
			t.Fatalf("completed=%d", completed.Load())
		}
	}
}

func TestSuccessfulBatchesJoinResults(t *testing.T) {
	for _, size := range []int{1, 2, 12} {
		values := make([]int, size)
		jobs := make([]func(context.Context) error, size)
		for index := range jobs {
			jobs[index] = func(context.Context) error { values[index] = index + 1; return nil }
		}
		if err := New().Run(t.Context(), jobs); err != nil {
			t.Fatal(err)
		}
		for index, value := range values {
			if value != index+1 {
				t.Fatalf("result[%d]=%d", index, value)
			}
		}
	}
}

func TestUnexpectedJobExitDrainsOtherJobs(t *testing.T) {
	var completed atomic.Bool
	err := New().Run(t.Context(), []func(context.Context) error{
		func(context.Context) error { runtime.Goexit(); return nil },
		func(context.Context) error { completed.Store(true); return nil },
	})
	if err == nil || !completed.Load() {
		t.Fatalf("unexpected exit: error=%v completed=%t", err, completed.Load())
	}
}
