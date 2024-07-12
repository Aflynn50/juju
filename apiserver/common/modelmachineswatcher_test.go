// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type modelMachinesWatcherSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&modelMachinesWatcherSuite{})

type fakeModelMachinesWatcher struct {
	state.ModelMachinesWatcher
	initial []string
}

func (f *fakeModelMachinesWatcher) WatchModelMachines() state.StringsWatcher {
	changes := make(chan []string, 1)
	// Simulate initial event.
	changes <- f.initial
	return &fakeStringsWatcher{changes: changes}
}

func (s *modelMachinesWatcherSuite) TestWatchModelMachines(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	e := common.NewModelMachinesWatcher(
		&fakeModelMachinesWatcher{initial: []string{"foo"}},
		resources,
		authorizer,
	)
	result, err := e.WatchModelMachines(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResult{StringsWatcherId: "1", Changes: []string{"foo"}, Error: nil})
	c.Assert(resources.Count(), gc.Equals, 1)
}

func (s *modelMachinesWatcherSuite) TestWatchAuthError(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("1"),
		Controller: false,
	}
	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	e := common.NewModelMachinesWatcher(
		&fakeModelMachinesWatcher{},
		resources,
		authorizer,
	)
	_, err := e.WatchModelMachines(context.Background())
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(resources.Count(), gc.Equals, 0)
}
