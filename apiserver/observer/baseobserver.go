// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer

import (
	"context"

	"github.com/juju/juju/core/model"
	"github.com/juju/names/v6"
)

// BaseObserver provides a common state between different observers.
type BaseObserver struct {
	tag            names.Tag
	model          names.ModelTag
	modelUUID      model.UUID
	agent          bool
	fromController bool
}

// Login implements Observer.
func (n *BaseObserver) Login(ctx context.Context, entity names.Tag, model names.ModelTag, modelUUID model.UUID, fromController bool, userData string) {
	n.tag = entity
	n.fromController = fromController
	n.agent = n.isAgent(entity)
	n.model = model
	n.modelUUID = modelUUID
}

// IsAgent returns whether the entity is an agent during the current login.
// If the entity has not logged in, it returns false.
func (n *BaseObserver) IsAgent() bool {
	return n.agent
}

// AgentTag returns the tag of the agent that has logged in.
// If no agent has logged in, it returns an empty string.
func (n *BaseObserver) AgentTag() names.Tag {
	return n.tag
}

// ModelTag returns the model tag of the agent that has logged in.
// If no agent has logged in, it returns an empty string.
func (n *BaseObserver) ModelTag() names.ModelTag {
	return n.model
}

// FromController returns whether the agent has logged in from the controller.
func (n *BaseObserver) FromController() bool {
	return n.fromController
}

func (n *BaseObserver) isAgent(entity names.Tag) bool {
	switch entity.(type) {
	case names.UnitTag, names.MachineTag, names.ApplicationTag:
		return true
	default:
		return false
	}
}
