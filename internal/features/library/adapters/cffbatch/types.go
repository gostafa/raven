// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package cffbatch executes independent library jobs with bounded concurrency.
package cffbatch

// Executor owns the policy for finite batches without retaining runtime resources.
type (
	Executor struct{}
)
