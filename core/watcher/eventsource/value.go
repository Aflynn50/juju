// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
)

// ValueWatcher watches for events associated with a single value
// from a namespace.
// Any time the identified change value has an associated event,
// a notification is emitted.
type ValueWatcher struct {
	*BaseWatcher

	out         chan struct{}
	namespace   string
	changeValue string
	changeMask  changestream.ChangeType
}

// NewValueWatcher returns a new watcher that receives changes from the input
// base watcher's db/queue when change-log events occur for a specific changeValue
// from the input namespace.
func NewValueWatcher(base *BaseWatcher, namespace, changeValue string, changeMask changestream.ChangeType) *ValueWatcher {
	w := &ValueWatcher{
		BaseWatcher: base,
		out:         make(chan struct{}),
		namespace:   namespace,
		changeValue: changeValue,
		changeMask:  changeMask,
	}

	w.tomb.Go(w.loop)
	return w
}

// Changes returns the channel on which notifications
// are sent when the watched database row changes.
func (w *ValueWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *ValueWatcher) loop() error {
	defer close(w.out)

	opt := changestream.FilteredNamespace(w.namespace, w.changeMask, func(e changestream.ChangeEvent) bool {
		return e.Changed() == w.changeValue
	})
	subscription, err := w.watchableDB.Subscribe(opt)
	if err != nil {
		// TODO(wallyworld) - remove when we have dqlite watchers on k8s
		w.logger.Warningf("error subscribing to entity %q in namespace %q: %v", w.changeValue, w.namespace, err)
		subscription = noopSubscription{}
	}
	defer subscription.Unsubscribe()

	// By reassigning the in and out channels, we effectively ticktock between
	// read mode and dispatch mode. This ensures we always dispatch
	// notifications for changes we received before reading more, and every
	// channel read/write is guarded by checks of the tomb and subscription
	// liveness.
	// We begin in dispatch mode in order to send the initial notification.
	in := subscription.Changes()
	out := w.out

	w.drainInitialEvent(in)

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-subscription.Done():
			return ErrSubscriptionClosed
		case _, ok := <-in:
			if !ok {
				w.logger.Debugf("change channel closed for %q; terminating watcher for %q", w.namespace, w.changeValue)
				return nil
			}

			// We have changes. Tick over to dispatch mode.
			out = w.out
		case out <- struct{}{}:
			// We have dispatched. Tick over to read mode.
			out = nil
		}
	}
}

func (w *ValueWatcher) drainInitialEvent(in <-chan []changestream.ChangeEvent) {
	select {
	case _, ok := <-in:
		if !ok {
			w.logger.Debugf("change channel closed for %q; terminating watcher for %q", w.namespace, w.changeValue)
			return
		}
	default:
	}
}