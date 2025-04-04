// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/model"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead pass one passed as manifold config.
type logger interface{}

var _ logger = struct{}{}

// ManifoldConfig holds the information necessary to run a FlagWorker in
// a dependency.Engine.
type ManifoldConfig struct {
	LeaseManagerName string

	Clock    clock.Clock
	Duration time.Duration
	// TODO(controlleragent) - claimaint should be a ControllerAgentTag
	Claimant  names.Tag
	Entity    names.Tag
	ModelUUID model.UUID

	NewWorker func(context.Context, FlagConfig) (worker.Worker, error)
}

// Validate ensures the required values are set.
func (config *ManifoldConfig) Validate() error {
	if config.LeaseManagerName == "" {
		return errors.NotValidf("empty LeaseManagerName")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var leaseManager lease.Manager
	if err := getter.Get(config.LeaseManagerName, &leaseManager); err != nil {
		return nil, errors.Trace(err)
	}

	if !names.IsValidMachine(config.Claimant.Id()) && !names.IsValidControllerAgent(config.Claimant.Id()) {
		return nil, errors.NotValidf("claimant tag")
	}

	flag, err := config.NewWorker(ctx, FlagConfig{
		Clock:        config.Clock,
		LeaseManager: leaseManager,
		Claimant:     config.Claimant,
		Entity:       config.Entity,
		ModelUUID:    config.ModelUUID,
		Duration:     config.Duration,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return flag, nil
}

// Manifold returns a dependency.Manifold that will run a FlagWorker and
// expose it to clients as a engine.Flag resource.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.LeaseManagerName,
		},
		Start: config.start,
		Output: func(in worker.Worker, out interface{}) error {
			return engine.FlagOutput(in, out)
		},
		Filter: func(err error) error {
			if errors.Is(err, ErrRefresh) {
				return dependency.ErrBounce
			}
			return err
		},
	}
}
