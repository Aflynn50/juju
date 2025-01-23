// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"context"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type upgraderWatchSuite struct {
	testing.IsolationSuite

	agentService    *MockModelAgentService
	watcherRegistry *facademocks.MockWatcherRegistry
}

var _ = gc.Suite(&upgraderWatchSuite{})

func (s *upgraderWatchSuite) TestWatchAPIVersionNothing(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Not an error to watch nothing
	results, err := s.api().WatchAPIVersion(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *upgraderWatchSuite) TestWatchAPIVersionMachine(c *gc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	defer close(done)
	ch := make(chan struct{})
	w := watchertest.NewMockNotifyWatcher(ch)

	tag := names.NewMachineTag("2")

	s.agentService.EXPECT().WatchMachineTargetAgentVersion(gomock.Any(), coremachine.Name(tag.Id())).DoAndReturn(func(_ context.Context, _ coremachine.Name) (watcher.Watcher[struct{}], error) {
		time.AfterFunc(coretesting.ShortWait, func() {
			// Send initial event.
			select {
			case ch <- struct{}{}:
			case <-done:
				c.ExpectFailure("watcher (unit) did not fire")
			}
		})
		return w, nil
	})
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("87", nil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		}}
	results, err := s.api().WatchAPIVersion(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "87"},
		},
	})
}

func (s *upgraderWatchSuite) TestWatchAPIVersionUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	defer close(done)
	ch := make(chan struct{})
	w := watchertest.NewMockNotifyWatcher(ch)

	tag := names.NewUnitTag("test/1")

	s.agentService.EXPECT().WatchUnitTargetAgentVersion(gomock.Any(), tag.Id()).DoAndReturn(func(_ context.Context, _ string) (watcher.Watcher[struct{}], error) {
		time.AfterFunc(coretesting.ShortWait, func() {
			// Send initial event.
			select {
			case ch <- struct{}{}:
			case <-done:
				c.ExpectFailure("watcher (unit) did not fire")
			}
		})
		return w, nil
	})
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("4", nil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		}}
	results, err := s.api().WatchAPIVersion(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "4"},
		},
	})
}

func (s *upgraderWatchSuite) TestWatchAPIVersionControllerModelAgent(c *gc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	defer close(done)
	chC := make(chan struct{})
	chM := make(chan struct{})
	wc := watchertest.NewMockNotifyWatcher(chC)
	wm := watchertest.NewMockNotifyWatcher(chM)

	s.agentService.EXPECT().WatchModelTargetAgentVersion(gomock.Any()).DoAndReturn(func(_ context.Context) (watcher.Watcher[struct{}], error) {
		time.AfterFunc(coretesting.ShortWait, func() {
			// Send initial event.
			select {
			case chC <- struct{}{}:
			case <-done:
				c.ExpectFailure("watcher (controller) did not fire")
			}
		})
		return wc, nil
	})
	s.agentService.EXPECT().WatchModelTargetAgentVersion(gomock.Any()).DoAndReturn(func(_ context.Context) (watcher.Watcher[struct{}], error) {
		time.AfterFunc(coretesting.ShortWait, func() {
			// Send initial event.
			select {
			case chM <- struct{}{}:
			case <-done:
				c.ExpectFailure("watcher (model) did not fire")
			}
		})
		return wm, nil
	})
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("2", nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: coretesting.ControllerTag.String()},
			{Tag: coretesting.ModelTag.String()},
		}}
	results, err := s.api().WatchAPIVersion(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "2"},
			{NotifyWatcherId: "1"},
		},
	})
}

func (s *upgraderWatchSuite) TestWatchAPIVersionTagInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	args := params.Entities{
		Entities: []params.Entity{{Tag: "unknow-tag-type"}},
	}
	results, err := s.api().WatchAPIVersion(context.Background(), args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].NotifyWatcherId, gc.Equals, "")
	c.Assert(results.Results[0].Error.Code, gc.Equals, params.CodeTagInvalid)
}

func (s *upgraderWatchSuite) TestWatchAPIVersionWrongTypeTag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Application can be a valid tag, however it's not valid for
	// this method.
	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewApplicationTag("testme").String()}},
	}
	results, err := s.api().WatchAPIVersion(context.Background(), args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].NotifyWatcherId, gc.Equals, "")
	c.Assert(results.Results[0].Error.Code, gc.Equals, params.CodeNotValid)
}

func (s *upgraderWatchSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agentService = NewMockModelAgentService(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	return ctrl
}

func (s *upgraderWatchSuite) api() *UpgraderAPI {
	return &UpgraderAPI{
		modelAgentService: s.agentService,
		watcherRegistry:   s.watcherRegistry,
	}
}
