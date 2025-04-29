// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/names/v6"

	"github.com/juju/juju/state"
)

// State provides the subset of global state required by the
// action facade.
type State interface {
	AllMachines() ([]*state.Machine, error)
	Model() (Model, error)
	WatchActionLogs(actionId string) state.StringsWatcher
	ActionByTag(tag names.ActionTag) (state.Action, error)
}

// Model describes model state used by the action facade.
type Model interface {
	AddAction(receiver state.ActionReceiver, operationID, name string, payload map[string]interface{}, parallel *bool, executionGroup *string) (state.Action, error)
	EnqueueOperation(summary string, count int) (string, error)
	FailOperationEnqueuing(operationID, failMessage string, count int) error
	FindActionsByName(name string) ([]state.Action, error)
	ListOperations(actionNames []string, actionReceivers []names.Tag, operationStatus []state.ActionStatus,
		offset, limit int,
	) ([]state.OperationInfo, bool, error)
	OperationWithActions(id string) (*state.OperationInfo, error)
}

type stateShim struct {
	st *state.State
}

func (s *stateShim) ActionByTag(tag names.ActionTag) (state.Action, error) {
	return s.st.ActionByTag(tag)
}

func (s *stateShim) AllMachines() ([]*state.Machine, error) {
	return s.st.AllMachines()
}

func (s *stateShim) Model() (Model, error) {
	return s.st.Model()
}

func (s *stateShim) WatchActionLogs(actionId string) state.StringsWatcher {
	return s.st.WatchActionLogs(actionId)
}
