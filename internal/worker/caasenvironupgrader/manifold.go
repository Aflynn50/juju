// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasenvironupgrader

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/internal/worker/gate"
)

// ManifoldConfig describes how to configure and construct a Worker,
// and what registered resources it may depend upon.
type ManifoldConfig struct {
	GateName string
	ModelTag names.ModelTag

	NewWorker func(Config) (worker.Worker, error)
}

func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	var gate gate.Unlocker
	if err := getter.Get(config.GateName, &gate); err != nil {
		return nil, errors.Trace(err)
	}

	worker, err := config.NewWorker(Config{
		GateUnlocker: gate,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

// Manifold returns a dependency.Manifold that will run a Worker as
// configured.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.GateName,
		},
		Start: config.start,
	}
}
