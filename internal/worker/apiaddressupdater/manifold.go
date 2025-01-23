// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddressupdater

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api/agent/machiner"
	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/logger"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	Logger        logger.Logger
}

// Manifold returns a dependency manifold that runs an API address updater worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := engine.AgentAPIManifoldConfig{
		AgentName:     config.AgentName,
		APICallerName: config.APICallerName,
	}
	return engine.AgentAPIManifold(typedConfig, config.newWorker)
}

// newWorker trivially wraps NewAPIAddressUpdater for use in a engine.AgentAPIManifold.
// It's not tested at the moment, because the scaffolding necessary is too
// unwieldy/distracting to introduce at this point.
func (config ManifoldConfig) newWorker(_ context.Context, a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	tag := a.CurrentConfig().Tag()

	// TODO(fwereade): use appropriate facade!
	var facade APIAddresser
	switch apiTag := tag.(type) {
	case names.UnitTag:
		facade = uniter.NewClient(apiCaller, apiTag)
	case names.MachineTag:
		facade = machiner.NewClient(apiCaller)
	default:
		return nil, errors.Errorf("expected a unit or machine tag; got %q", tag)
	}

	setter := agent.APIHostPortsSetter{Agent: a}
	w, err := NewAPIAddressUpdater(Config{
		Addresser: facade,
		Setter:    setter,
		Logger:    config.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
