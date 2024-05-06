// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
)

// Applier is a function that applies a change to a value.
type Applier[T any] func(T, T) T

// MultiWatcher implements Watcher, combining multiple Watchers.
type MultiWatcher[T any] struct {
	catacomb         catacomb.Catacomb
	staging, changes chan T
	applier          Applier[T]
}

// NewMultiNotifyWatcher creates a NotifyWatcher that combines
// each of the NotifyWatchers passed in. Each watcher's initial
// event is consumed, and a single initial event is sent.
func NewMultiNotifyWatcher(ctx context.Context, watchers ...Watcher[struct{}]) (*MultiWatcher[struct{}], error) {
	applier := func(_, _ struct{}) struct{} {
		return struct{}{}
	}
	return NewMultiWatcher[struct{}](ctx, applier, watchers...)
}

// NewMultiStringsWatcher creates a strings watcher (Watcher[[]string]) that
// combines each of the (strings) watchers passed in. Each watcher's initial
// event is consumed, and a single initial event is sent.
func NewMultiStringsWatcher(ctx context.Context, watchers ...Watcher[[]string]) (*MultiWatcher[[]string], error) {
	applier := func(staging, in []string) []string {
		return append(staging, in...)
	}
	return NewMultiWatcher[[]string](ctx, applier, watchers...)
}

// NewMultiWatcher creates a NotifyWatcher that combines
// each of the NotifyWatchers passed in. Each watcher's initial
// event is consumed, and a single initial event is sent.
// Subsequent events are not coalesced.
func NewMultiWatcher[T any](ctx context.Context, applier Applier[T], watchers ...Watcher[T]) (*MultiWatcher[T], error) {
	workers := make([]worker.Worker, len(watchers))
	for i, w := range watchers {
		_, err := ConsumeInitialEvent[T](ctx, w)
		if err != nil {
			return nil, errors.Trace(err)
		}

		workers[i] = w
	}

	w := &MultiWatcher[T]{
		staging: make(chan T),
		changes: make(chan T),
		applier: applier,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: workers,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	for _, watcher := range watchers {
		// Copy events from the watcher to the staging channel.
		go w.copyEvents(watcher.Changes())
	}

	return w, nil
}

// loop copies events from the input channel to the output channel.
func (w *MultiWatcher[T]) loop() error {
	defer close(w.changes)

	out := w.changes
	var payload T
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case v := <-w.staging:
			payload = w.applier(payload, v)
			out = w.changes
		case out <- payload:
			out = nil

			// Ensure we reset the payload to the initial value after
			// sending it to the channel.
			var init T
			payload = init
		}
	}
}

// copyEvents copies channel events from "in" to "out".
func (w *MultiWatcher[T]) copyEvents(in <-chan T) {
	var (
		outC    chan<- T
		payload T
	)
	for {
		select {
		case <-w.catacomb.Dying():
			return
		case v, ok := <-in:
			if !ok {
				return
			}
			payload = w.applier(payload, v)
			outC = w.staging
		case outC <- payload:
			outC = nil

			// Ensure we reset the payload to the initial value after
			// sending it to the channel.
			var init T
			payload = init
		}
	}
}

func (w *MultiWatcher[T]) Kill() {
	w.catacomb.Kill(nil)
}

func (w *MultiWatcher[T]) Wait() error {
	return w.catacomb.Wait()
}

func (w *MultiWatcher[T]) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *MultiWatcher[T]) Err() error {
	return w.catacomb.Err()
}

func (w *MultiWatcher[T]) Changes() <-chan T {
	return w.changes
}
