// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/internal/errors"
)

type migrationSuite struct {
	testing.IsolationSuite

	state *MockMigrationState
}

var _ = tc.Suite(&migrationSuite{})

func (s *migrationSuite) TestMigrationService(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.state.EXPECT().GetApplicationLeadershipForModel(gomock.Any(), modelUUID).Return(map[string]string{
		"foo": "bar",
	}, nil)

	leadershipService := NewMigrationService(s.state)
	leaders, err := leadershipService.GetApplicationLeadershipForModel(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(leaders, jc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *migrationSuite) TestMigrationServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.state.EXPECT().GetApplicationLeadershipForModel(gomock.Any(), modelUUID).Return(map[string]string{
		"foo": "bar",
	}, errors.Errorf("boom"))

	leadershipService := NewMigrationService(s.state)
	_, err := leadershipService.GetApplicationLeadershipForModel(context.Background(), modelUUID)
	c.Assert(err, tc.ErrorMatches, "boom")

}

func (s *migrationSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockMigrationState(ctrl)

	return ctrl
}
