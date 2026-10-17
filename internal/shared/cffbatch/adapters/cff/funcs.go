// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package cff

import (
	"context"
	"fmt"

	"go.uber.org/cff/scheduler"
)

// New constructs an executor; workers exist only while a batch runs.
func New() *Executor { return &Executor{} }

// Run executes all jobs unless canceled, preserving input-order error precedence.
func (*Executor) Run(ctx context.Context, jobs []func(context.Context) error) error {
	execute := batchRunner(len(jobs))
	if execute == nil {
		return nil
	}

	err := execute(ctx, jobs)
	if err != nil {
		return fmt.Errorf("run media batch: %w", err)
	}

	return nil
}

func batchRunner(count int) func(context.Context, []func(context.Context) error) error {
	if count == zero {
		return nil
	}

	if count == one {
		return runSingle
	}

	return runBatch
}

func runSingle(ctx context.Context, jobs []func(context.Context) error) error {
	err := runJob(ctx, jobs[zero])
	if err == nil {
		err = contextError(ctx)
	}

	if err != nil {
		return fmt.Errorf("run single media task: %w", err)
	}

	return nil
}

func runBatch(ctx context.Context, jobs []func(context.Context) error) error {
	results := emptyResults(len(jobs))
	batch := schedule(ctx, jobs, results)
	// Drain independently of cancellation before callers inspect results or
	// roll back filesystem effects. Cancellation is checked inside each job.
	err := batch.Wait(context.WithoutCancel(ctx))
	if err == nil {
		err = batchError(ctx, results)
	}

	if err != nil {
		return fmt.Errorf("drain media batch: %w", err)
	}

	return nil
}

func emptyResults(count int) []error {
	results := make([]error, zero, count)
	for range count {
		results = append(results, nil)
	}

	return results
}

func schedule(
	ctx context.Context,
	jobs []func(context.Context) error,
	results []error,
) *scheduler.Scheduler {
	config := scheduler.Config{
		Concurrency: min(concurrency, len(jobs)), ContinueOnError: true,
		Emitter: nil, StateFlushFrequency: zero,
	}
	batch := config.New()

	for index := range jobs {
		batch.Enqueue(context.WithoutCancel(ctx), scheduler.Job{
			Run: indexedJob(ctx, jobs[index], results[index:index+one]), Dependencies: nil,
		})
	}

	return batch
}

func indexedJob(
	ctx context.Context,
	job func(context.Context) error,
	result []error,
) func(context.Context) error {
	return func(context.Context) error {
		result[zero] = runJob(ctx, job)

		return nil
	}
}

func batchError(ctx context.Context, results []error) error {
	for index := range results {
		if results[index] != nil {
			return results[index]
		}
	}

	err := contextError(ctx)
	if err != nil {
		return fmt.Errorf("finish media batch: %w", err)
	}

	return nil
}

func contextError(ctx context.Context) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("media batch canceled: %w", err)
	}

	return nil
}

func runJob(ctx context.Context, job func(context.Context) error) (err error) {
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("%w: %v", errTaskPanicked, value)
		}
	}()

	contextErr := contextError(ctx)
	if contextErr != nil {
		return fmt.Errorf("start media task: %w", contextErr)
	}

	jobErr := job(ctx)
	if jobErr != nil {
		return fmt.Errorf("run media task: %w", jobErr)
	}

	return nil
}
