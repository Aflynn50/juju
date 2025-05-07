// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/internal/testing"
)

type consumeSuite struct {
	testing.BaseSuite

	watcher *MockWatcher[[]string]
}

var _ = tc.Suite(&consumeSuite{})

func (s *consumeSuite) TestConsumeInitialEventReturnsChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	contents := []string{"a", "b"}
	changes := make(chan []string, 1)
	changes <- contents
	s.watcher.EXPECT().Changes().Return(changes)

	res, err := ConsumeInitialEvent[[]string](context.Background(), s.watcher)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.SameContents, contents)
}

func (s *consumeSuite) TestConsumeInitialEventWorkerKilled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 1)
	s.watcher.EXPECT().Changes().Return(changes)

	// We close the channel to make sure the worker is killed by ConsumeInitialEvent
	close(changes)
	s.watcher.EXPECT().Kill()
	s.watcher.EXPECT().Wait().Return(tomb.ErrDying)

	res, err := ConsumeInitialEvent[[]string](context.Background(), s.watcher)
	c.Assert(err, tc.ErrorMatches, tomb.ErrDying.Error())
	c.Assert(res, tc.IsNil)
}

func (s *consumeSuite) TestConsumeInitialEventWatcherStoppedNilErr(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 1)
	s.watcher.EXPECT().Changes().Return(changes)

	// We close the channel to make sure the worker is killed by ConsumeInitialEvent
	close(changes)
	s.watcher.EXPECT().Kill()
	s.watcher.EXPECT().Wait().Return(nil)

	res, err := ConsumeInitialEvent[[]string](context.Background(), s.watcher)
	c.Assert(err, tc.ErrorMatches, "expected an error from .* got nil.*")
	c.Assert(err, jc.ErrorIs, ErrWorkerStopped)
	c.Assert(res, tc.IsNil)
}

func (s *consumeSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.watcher = NewMockWatcher[[]string](ctrl)

	return ctrl
}
