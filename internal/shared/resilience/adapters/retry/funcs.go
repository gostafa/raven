// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package retry

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v7"
	"github.com/gostafa/singleton"
)

// Transient recognizes only temporary OS resource failures.
func Transient(err error) bool {
	permanent := []error{
		context.Canceled,
		context.DeadlineExceeded,
		os.ErrNotExist,
		os.ErrPermission,
		os.ErrClosed,
		io.EOF,
		io.ErrUnexpectedEOF,
	}
	temporary := []error{
		syscall.EINTR,
		syscall.EAGAIN,
		syscall.EMFILE,
		syscall.ENFILE,
		syscall.ENOBUFS,
	}

	return !matches(err, permanent) && (matches(err, temporary) || platformTransient(err))
}

func matches(err error, candidates []error) bool {
	for index := range candidates {
		if errors.Is(err, candidates[index]) {
			return true
		}
	}

	return false
}

// NewReads supplies the bounded read policy without retaining request state.
func NewReads(owner context.Context, logger *slog.Logger) *Reads {
	return &Reads{owner: func() context.Context { return owner }, logger: logger}
}

// Do runs a supplied retry callback, or loads directly when no policy is installed.
func (operation Operation) Do(ctx context.Context, load func(context.Context) error) error {
	if operation == nil {
		return errors.Join(load(ctx))
	}

	return errors.Join(operation(ctx, load))
}

// Do repeats only explicitly transient read failures.
//
//nolint:contextcheck // Both the request and independent application lifetime bound reads.
func (reads *Reads) Do(ctx context.Context, operation func(context.Context) error) error {
	owned, cancel := context.WithCancel(ctx)
	defer cancel()

	detach := context.AfterFunc(reads.owner(), cancel)

	defer detach()

	if reads.owner().Err() != nil {
		cancel()
	}

	result, err := retry(owned, readPolicy(reads.logger), readAttempt(operation))
	if result {
		return nil
	}

	return errors.Join(err)
}

func readAttempt(operation func(context.Context) error) func(context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		err := errors.Join(operation(ctx), ctx.Err())

		return err == nil, err
	}
}

func readPolicy(logger *slog.Logger) *policy {
	return &policy{
		attempts:  readAttempts,
		timeout:   readTimeout,
		initial:   readInitial,
		maximum:   readMaximum,
		logger:    logger,
		level:     slog.LevelDebug,
		name:      readOperation,
		transient: Transient,
	}
}

func initializationPolicy[Value any](options *Options[Value]) *policy {
	return &policy{
		attempts:  initAttempts,
		timeout:   initTimeout,
		initial:   initInitial,
		maximum:   initMaximum,
		logger:    options.Logger,
		level:     slog.LevelWarn,
		name:      options.Name,
		transient: options.Transient,
	}
}

// NewInitialization allocates a dormant provider under the owner lifetime.
//
//nolint:gosec // The returned Initialization owns cancel and calls it from Close.
func NewInitialization[Value any](
	owner context.Context,
	factory func(context.Context) (Value, error),
	options *Options[Value],
) *Initialization[Value] {
	ctx, cancel := context.WithCancel(owner)
	resource := new(initializationState[Value])

	resource.cancel, resource.release, resource.done = cancel, options.Release, make(chan struct{})
	resource.provider = singleton.MustNew(
		func(context.Context) (Value, error) { return retry(ctx, initializationPolicy(options), factory) },
		singleton.WithMaxAttempts(singleAttempt),
		singleton.WithInitializationTimeout(zeroAttempts),
	)

	return &Initialization[Value]{
		get:  func(ctx context.Context) (Value, error) { return getInitialization(ctx, resource) },
		stop: func() error { return closeInitialization(resource) },
	}
}

// Get isolates caller cancellation from shared initialization.
func getInitialization[Value any](
	ctx context.Context,
	resource *initializationState[Value],
) (Value, error) {
	if !beginInitialization(ctx, resource) {
		var zero Value

		return zero, context.Canceled
	}

	value, err := resource.provider.Get(ctx)
	if initializationStopped(resource) {
		var zero Value

		return zero, errors.Join(initializationError(err), context.Canceled)
	}

	return value, errors.Join(initializationError(err))
}

func initializationStopped[Value any](resource *initializationState[Value]) bool {
	resource.mu.Lock()
	defer resource.mu.Unlock()

	return resource.stopped
}

func initializationError(err error) error {
	failure := new(singleton.InitError)
	if errors.As(err, &failure) {
		return errors.Join(failure.Err())
	}

	return errors.Join(err)
}

func beginInitialization[Value any](
	ctx context.Context,
	resource *initializationState[Value],
) bool {
	resource.mu.Lock()
	defer resource.mu.Unlock()

	if resource.stopped {
		return false
	}

	if !resource.started {
		resource.started = true
		go settleInitialization(context.WithoutCancel(ctx), resource)
	}

	return true
}

func settleInitialization[Value any](ctx context.Context, resource *initializationState[Value]) {
	resource.value, resource.err = resource.provider.Get(ctx)
	close(resource.done)
}

// Close cancels initialization, drains its factory, and releases once.
func closeInitialization[Value any](resource *initializationState[Value]) error {
	resource.closeOnce.Do(func() { releaseInitialization(resource) })

	return errors.Join(resource.closeErr)
}

func releaseInitialization[Value any](resource *initializationState[Value]) {
	resource.mu.Lock()

	resource.stopped = true

	started := resource.started
	resource.cancel()
	resource.mu.Unlock()

	if !started {
		return
	}

	<-resource.done
	releaseInitializedValue(resource)
}

func releaseInitializedValue[Value any](resource *initializationState[Value]) {
	if resource.err == nil {
		resource.closeErr = resource.release(resource.value)
	}
}

func retry[Value any](
	parent context.Context,
	settings *policy,
	operation func(context.Context) (Value, error),
) (Value, error) {
	ctx, cancel := context.WithTimeout(parent, settings.timeout)
	defer cancel()

	value, err := runRetry(ctx, settings, operation)

	return value, errors.Join(err)
}

func runRetry[Value any](
	ctx context.Context,
	settings *policy,
	operation func(context.Context) (Value, error),
) (Value, error) {
	attempt := zeroAttempts
	value, err := backoff.Retry(
		ctx,
		func() (Value, error) {
			attempt++

			return execute(ctx, settings, operation)
		},
		retryOptions(ctx, settings, &attempt)...)

	err = retryError(ctx, err)
	logAttempt(ctx, settings, &event{attempt: attempt, delay: zeroAttempts, err: err})

	return value, errors.Join(err)
}

func retryOptions(ctx context.Context, settings *policy, attempt *int) []backoff.RetryOption {
	strategy := backoff.NewExponentialBackOff()

	strategy.InitialInterval, strategy.MaxInterval = settings.initial, settings.maximum

	return []backoff.RetryOption{
		backoff.WithBackOff(strategy),
		backoff.WithMaxTries(settings.attempts),
		backoff.WithMaxElapsedTime(zeroAttempts),
		backoff.WithNotify(func(err error, delay time.Duration) {
			logAttempt(ctx, settings, &event{attempt: *attempt, delay: delay, err: err})
		}),
	}
}

func execute[Value any](
	ctx context.Context,
	settings *policy,
	operation func(context.Context) (Value, error),
) (Value, error) {
	var zero Value

	err := ctx.Err()
	if err != nil {
		return zero, errors.Join(backoff.Permanent(err))
	}

	value, err := operation(ctx)

	return value, errors.Join(classifiedError(err, settings.transient))
}

func classifiedError(err error, transient func(error) bool) error {
	if err != nil && !transient(err) {
		return errors.Join(backoff.Permanent(err))
	}

	return errors.Join(err)
}

func retryError(ctx context.Context, err error) error {
	failure := backoff.AsRetryError(err)
	if failure == nil {
		return errors.Join(err)
	}

	return errors.Join(failure.LastErr, ctx.Err())
}

func logAttempt(ctx context.Context, settings *policy, result *event) {
	if settings.logger == nil {
		return
	}

	settings.logger.Log(
		ctx,
		attemptLogLevel(settings, result),
		operationMessage,
		slog.String(operationKey, settings.name),
		slog.Int(attemptKey, result.attempt),
		slog.Duration(delayKey, result.delay),
		slog.Any(errorKey, result.err),
	)
}

func attemptLogLevel(settings *policy, result *event) slog.Level {
	if result.err == nil {
		return slog.LevelDebug
	}

	return settings.level
}

// Get waits under the caller context without canceling the shared factory.
func (resource *Initialization[Value]) Get(ctx context.Context) (Value, error) {
	value, err := resource.get(ctx)

	return value, errors.Join(err)
}

// Close cancels initialization, drains its factory, and releases once.
func (resource *Initialization[Value]) Close() error { return errors.Join(resource.stop()) }
