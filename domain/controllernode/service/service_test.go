// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/tc"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
)

type serviceSuite struct {
	testing.IsolationSuite

	state *MockState
}

var _ = tc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}

func (s *serviceSuite) TestUpdateExternalControllerSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CurateNodes(gomock.Any(), []string{"3", "4"}, []string{"1"})

	err := NewService(s.state).CurateNodes(context.Background(), []string{"3", "4"}, []string{"1"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateDqliteNode(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().UpdateDqliteNode(gomock.Any(), "0", uint64(12345), "192.168.5.60")

	err := NewService(s.state).UpdateDqliteNode(context.Background(), "0", 12345, "192.168.5.60")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestIsModelKnownToController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	knownID := "known"
	fakeID := "fake"

	exp := s.state.EXPECT()
	gomock.InOrder(
		exp.SelectDatabaseNamespace(gomock.Any(), fakeID).Return("", controllernodeerrors.NotFound),
		exp.SelectDatabaseNamespace(gomock.Any(), knownID).Return(knownID, nil),
	)

	svc := NewService(s.state)

	known, err := svc.IsKnownDatabaseNamespace(context.Background(), fakeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(known, tc.IsFalse)

	known, err = svc.IsKnownDatabaseNamespace(context.Background(), knownID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(known, tc.IsTrue)
}

func (s *serviceSuite) TestSetControllerNodeAgentVersionSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	controllerID := "1"
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("1.2.3"),
		Arch:   corearch.ARM64,
	}

	s.state.EXPECT().SetRunningAgentBinaryVersion(gomock.Any(), controllerID, ver).Return(nil)

	svc := NewService(s.state)
	err := svc.SetControllerNodeReportedAgentVersion(
		context.Background(),
		controllerID,
		ver,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetControllerNodeAgentVersionNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state)

	controllerID := "1"

	ver := coreagentbinary.Version{
		Number: semversion.Zero,
	}
	err := svc.SetControllerNodeReportedAgentVersion(
		context.Background(),
		controllerID,
		ver,
	)
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	ver = coreagentbinary.Version{
		Number: semversion.MustParse("1.2.3"),
		Arch:   corearch.UnsupportedArches[0],
	}
	err = svc.SetControllerNodeReportedAgentVersion(
		context.Background(),
		controllerID,
		ver,
	)
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestSetControllerNodeAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state)

	controllerID := "1"
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("1.2.3"),
		Arch:   corearch.ARM64,
	}

	s.state.EXPECT().SetRunningAgentBinaryVersion(gomock.Any(), controllerID, ver).Return(controllernodeerrors.NotFound)

	err := svc.SetControllerNodeReportedAgentVersion(
		context.Background(),
		controllerID,
		ver,
	)
	c.Assert(err, tc.ErrorIs, controllernodeerrors.NotFound)
}
