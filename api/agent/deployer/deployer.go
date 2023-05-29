// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/rpc/params"
)

const deployerFacade = "Deployer"

// Client provides access to the deployer worker's idea of the state.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a new Client instance that makes API calls
// through the given caller.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, deployerFacade)
	return &Client{facade: facadeCaller}

}

// unitLife returns the lifecycle state of the given unit.
func (c *Client) unitLife(tag names.UnitTag) (life.Value, error) {
	return common.OneLife(c.facade, tag)
}

// Unit returns the unit with the given tag.
func (c *Client) Unit(tag names.UnitTag) (*Unit, error) {
	life, err := c.unitLife(tag)
	if err != nil {
		return nil, err
	}
	return &Unit{
		tag:    tag,
		life:   life,
		client: c,
	}, nil
}

// Machine returns the machine with the given tag.
func (c *Client) Machine(tag names.MachineTag) (*Machine, error) {
	// TODO(dfc) this cannot return an error any more
	return &Machine{
		tag:    tag,
		client: c,
	}, nil
}

// ConnectionInfo returns all the address information that the deployer task
// needs in one call.
func (c *Client) ConnectionInfo() (result params.DeployerConnectionValues, err error) {
	err = c.facade.FacadeCall("ConnectionInfo", nil, &result)
	return result, err
}
