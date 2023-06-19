// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/worker/common"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...interface{})
	Warningf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Debugf(message string, args ...interface{})
	Tracef(message string, args ...interface{})
	IsTraceEnabled() bool
}

// WatchableDBFn is an alias function that allows the creation of
// EventQueueWorker.
type WatchableDBFn = func(string, coredatabase.TxnRunner, FileNotifier, clock.Clock, Logger) (WatchableDBWorker, error)

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	AgentName         string
	DBAccessor        string
	FileNotifyWatcher string

	Clock          clock.Clock
	Logger         Logger
	NewWatchableDB WatchableDBFn
}

func (cfg ManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.DBAccessor == "" {
		return errors.NotValidf("empty DBAccessorName")
	}
	if cfg.FileNotifyWatcher == "" {
		return errors.NotValidf("empty FileNotifyWatcherName")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewWatchableDB == nil {
		return errors.NotValidf("nil NewWatchableDB")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the changestream
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.DBAccessor,
			config.FileNotifyWatcher,
		},
		Output: changeStreamOutput,
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var agent agent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}

			agentConfig := agent.CurrentConfig()

			var dbGetter DBGetter
			if err := context.Get(config.DBAccessor, &dbGetter); err != nil {
				return nil, errors.Trace(err)
			}

			var fileNotifyWatcher FileNotifyWatcher
			if err := context.Get(config.FileNotifyWatcher, &fileNotifyWatcher); err != nil {
				return nil, errors.Trace(err)
			}

			cfg := WorkerConfig{
				AgentTag:          agentConfig.Tag().Id(),
				DBGetter:          dbGetter,
				FileNotifyWatcher: fileNotifyWatcher,
				Clock:             config.Clock,
				Logger:            config.Logger,
				NewWatchableDB:    config.NewWatchableDB,
			}

			w, err := newWorker(cfg)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

func changeStreamOutput(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*changeStreamWorker)
	if !ok {
		return errors.Errorf("in should be a *changeStreamWorker; got %T", in)
	}

	switch out := out.(type) {
	case *changestream.WatchableDBGetter:
		var target changestream.WatchableDBGetter = w
		*out = target
	default:
		return errors.Errorf("out should be a *changestream.WatchableDBGetter; got %T", out)
	}
	return nil
}
