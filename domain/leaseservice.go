// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"

	"github.com/juju/juju/core/lease"
	internalerrors "github.com/juju/juju/internal/errors"
)

// LeaseChecker is an interface that checks if a lease is held by a holder.
type LeaseChecker interface {
	lease.Waiter
	lease.Checker
}

// LeaseService creates a base service that offers lease capabilities.
type LeaseService struct {
	leaseChecker func() LeaseChecker
}

// WithLease executes the closure function if the holder to the lease is
// held. As soon as that isn't the case, the context is cancelled and the
// function returns.
// The context must be passed to the closure function to ensure that the
// cancellation is propagated to the closure.
func (s *LeaseService) WithLease(ctx context.Context, leaseName, holderName string, fn func(context.Context) error) error {
	// Holding the lease is quite a complex operation, so we need to ensure that
	// the context is not cancelled before we start the operation.
	if err := ctx.Err(); err != nil {
		return internalerrors.Errorf("lease prechecking").Add(ctx.Err())
	}

	leaseChecker := s.leaseChecker()

	// The leaseCtx will be cancelled when the lease is no longer held by the
	// lease holder. This may or may not be the same as the holderName for the
	// lease. That check is done by the Token checker.
	leaseCtx, leaseCancel := context.WithCancel(ctx)
	defer leaseCancel()

	// Start will be closed when we start waiting for the lease to expire.
	// If the lease is not held, the function will return immediately and
	// the context will be cancelled.
	start := make(chan struct{})

	// WaitUntilExpired will be run against the leaseName. To ensure that after
	// we've waited that we still hold the lease, we need to check that the
	// lease is still held by the holder. Then we can guarantee that the lease
	// is held by the holder for the duration of the function. Although
	// convoluted this is necessary to ensure that the lease is held by the
	// holder for the duration of the function. The context will be cancelled
	// when the lease is no longer held by the lease holder for the lease name.

	waitCtx, waitCancel := context.WithCancel(ctx)
	defer waitCancel()

	waitErr := make(chan error)
	go func() {
		// This guards against the case that the lease has changed state
		// before we run the function.
		err := leaseChecker.WaitUntilExpired(waitCtx, leaseName, start)

		// Ensure that the lease context is cancelled when the wait has
		// completed. We do this as quick as possible to ensure that the
		// function is cancelled as soon as possible.
		leaseCancel()

		// The waitErr might not be read, so we need to provide another way
		// to collapse the goroutine. Using the waitCtx this goroutine will
		// be cancelled when the function is complete.
		select {
		case <-waitCtx.Done():
			return
		case waitErr <- internalerrors.Errorf("waiting for lease to expire: %w", err):
		}
	}()

	select {
	case <-leaseCtx.Done():
		// If the leaseCtx is cancelled, then the waiting for the lease to
		// expire finished unexpectedly. Return the context error.
		return internalerrors.Errorf("waiting for lease finished before execution").Add(leaseCtx.Err())
	case err := <-waitErr:
		if err == nil {
			// This shouldn't happen, but if it does, we need to return an
			// error. If we're attempting to wait whilst holding the lease,
			// before running the function and then wait return nil, we don't
			// know if the lease is held by the holder or what state we're in.
			return internalerrors.Errorf("unable to wait for lease to expire whilst holding lease")
		}
		return err
	case <-start:
	}

	// Ensure that the lease is held by the holder before proceeding.
	// We're guaranteed that the lease is held by the holder, otherwise the
	// context will have been cancelled.
	token := leaseChecker.Token(leaseName, holderName)
	if err := token.Check(); err != nil {
		return internalerrors.Errorf("checking lease token: %w", err)
	}

	// The leaseCtx will be cancelled when the lease is no longer held. This
	// will ensure that the function is cancelled when the lease is no longer
	// held.
	if err := fn(leaseCtx); err != nil {
		return internalerrors.Errorf("executing lease func: %w", err)
	}
	return nil
}
