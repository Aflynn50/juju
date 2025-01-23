// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/agent/machiner"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

type MachineAccessor interface {
	Machine(context.Context, names.MachineTag) (Machine, error)
}

type Machine interface {
	Refresh(context.Context) error
	Life() life.Value
	EnsureDead(context.Context) error
	SetMachineAddresses(ctx context.Context, addresses []network.MachineAddress) error
	SetStatus(ctx context.Context, machineStatus status.Status, info string, data map[string]interface{}) error
	Watch(context.Context) (watcher.NotifyWatcher, error)
	SetObservedNetworkConfig(ctx context.Context, netConfig []params.NetworkConfig) error
}

type APIMachineAccessor struct {
	State *machiner.Client
}

func (a APIMachineAccessor) Machine(ctx context.Context, tag names.MachineTag) (Machine, error) {
	m, err := a.State.Machine(ctx, tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return m, nil
}
