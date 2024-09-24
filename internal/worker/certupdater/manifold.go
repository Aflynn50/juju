// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	jujuagent "github.com/juju/juju/agent"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/pki"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/common"
	workerstate "github.com/juju/juju/internal/worker/state"
	"github.com/juju/juju/state"
)

// ManifoldConfig holds the information necessary to run a certupdater
// in a dependency.Engine.
type ManifoldConfig struct {
	AgentName                string
	AuthorityName            string
	StateName                string
	DomainServicesName       string
	NewWorker                func(Config) (worker.Worker, error)
	NewMachineAddressWatcher func(st *state.State, machineId string) (AddressWatcher, error)
	Logger                   logger.Logger
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.AuthorityName == "" {
		return errors.NotValidf("empty AuthorityName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewMachineAddressWatcher == nil {
		return errors.NotValidf("nil NewMachineAddressWatcher")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a pki Authority.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.AuthorityName,
			config.StateName,
			config.DomainServicesName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent jujuagent.Agent
	if err := getter.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var authority pki.Authority
	if err := getter.Get(config.AuthorityName, &authority); err != nil {
		return nil, errors.Trace(err)
	}

	var controllerDomainServices services.ControllerDomainServices
	if err := getter.Get(config.DomainServicesName, &controllerDomainServices); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := getter.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	_, st, err := stTracker.Use()
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}

	agentConfig := agent.CurrentConfig()

	addressWatcher, err := config.NewMachineAddressWatcher(st, agentConfig.Tag().Id())
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}

	w, err := config.NewWorker(Config{
		AddressWatcher:         addressWatcher,
		Authority:              authority,
		APIHostPortsGetter:     st,
		ControllerConfigGetter: controllerDomainServices.ControllerConfig(),
		Logger:                 config.Logger,
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}

// NewMachineAddressWatcher is the function that non-test code should
// pass into ManifoldConfig.NewMachineAddressWatcher.
func NewMachineAddressWatcher(st *state.State, machineId string) (AddressWatcher, error) {
	machine, err := st.Machine(machineId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return machineShim{
		machine: machine,
	}, nil
}

type machineShim struct {
	machine *state.Machine
}

func (s machineShim) WatchAddresses() watcher.NotifyWatcher {
	return watcherShim{
		watcher: s.machine.WatchAddresses(),
	}
}

func (s machineShim) Addresses() network.SpaceAddresses {
	return s.machine.Addresses()
}

type watcherShim struct {
	watcher state.NotifyWatcher
}

func (s watcherShim) Changes() watcher.NotifyChannel {
	return s.watcher.Changes()
}

func (s watcherShim) Kill() {
	s.watcher.Kill()
}

func (s watcherShim) Wait() error {
	return s.watcher.Wait()
}
