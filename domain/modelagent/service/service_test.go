// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/uuid"
)

type suite struct {
	state *MockState
}

var _ = gc.Suite(&suite{})

func (s *suite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

// TestGetModelAgentVersionSuccess tests the happy path for
// Service.GetModelAgentVersion.
func (s *suite) TestGetModelAgentVersionSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	expectedVersion, err := semversion.Parse("4.21.65")
	c.Assert(err, jc.ErrorIsNil)
	s.state.EXPECT().GetTargetAgentVersion(gomock.Any()).Return(expectedVersion, nil)

	svc := NewService(s.state, nil)
	ver, err := svc.GetModelTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(ver, jc.DeepEquals, expectedVersion)
}

// TestGetModelAgentVersionNotFound tests that Service.GetModelAgentVersion
// returns an appropriate error when the agent version cannot be found.
func (s *suite) TestGetModelAgentVersionModelNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetTargetAgentVersion(gomock.Any()).Return(semversion.Zero, modelerrors.AgentVersionNotFound)

	svc := NewService(s.state, nil)
	_, err := svc.GetModelTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.AgentVersionNotFound)
}

// TestGetMachineTargetAgentVersion is asserting the happy path for getting
// a machine's target agent version.
func (s *suite) TestGetMachineTargetAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machineName := coremachine.Name("0")
	ver := semversion.MustParse("4.0.0")

	s.state.EXPECT().CheckMachineExists(gomock.Any(), machineName).Return(nil)
	s.state.EXPECT().GetTargetAgentVersion(gomock.Any()).Return(ver, nil)

	rval, err := NewService(s.state, nil).GetMachineTargetAgentVersion(context.Background(), machineName)
	c.Check(err, jc.ErrorIsNil)
	c.Check(rval, gc.Equals, ver)
}

// TestGetMachineTargetAgentVersionNotFound is testing that the service
// returns a [machineerrors.MachineNotFound] error when no machine exists for
// a given name.
func (s *suite) TestGetMachineTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CheckMachineExists(gomock.Any(), coremachine.Name("0")).Return(
		machineerrors.MachineNotFound,
	)

	_, err := NewService(s.state, nil).GetMachineTargetAgentVersion(
		context.Background(),
		coremachine.Name("0"),
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetUnitTargetAgentVersion is asserting the happy path for getting
// a unit's target agent version.
func (s *suite) TestGetUnitTargetAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ver := semversion.MustParse("4.0.0")

	s.state.EXPECT().CheckUnitExists(gomock.Any(), "foo/0").Return(nil)
	s.state.EXPECT().GetTargetAgentVersion(gomock.Any()).Return(ver, nil)

	rval, err := NewService(s.state, nil).GetUnitTargetAgentVersion(context.Background(), "foo/0")
	c.Check(err, jc.ErrorIsNil)
	c.Check(rval, gc.Equals, ver)
}

// TestGetUnitTargetAgentVersionNotFound is testing that the service
// returns a [applicationerrors.UnitNotFound] error when no unit exists for
// a given name.
func (s *suite) TestGetUnitTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CheckUnitExists(gomock.Any(), "foo/0").Return(
		applicationerrors.UnitNotFound,
	)

	_, err := NewService(s.state, nil).GetUnitTargetAgentVersion(
		context.Background(),
		"foo/0",
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestWatchUnitTargetAgentVersionNotFound is testing that the service
// returns a [applicationerrors.UnitNotFound] error when no unit exists for
// a given name.
func (s *suite) TestWatchUnitTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CheckUnitExists(gomock.Any(), "foo/0").Return(
		applicationerrors.UnitNotFound,
	)

	_, err := NewService(s.state, nil).WatchUnitTargetAgentVersion(
		context.Background(),
		"foo/0",
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestWatchMachineTargetAgentVersionNotFound is testing that the service
// returns a [machineerrors.MachineNotFound] error when no machine exists for
// a given name.
func (s *suite) TestWatchMachineTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CheckMachineExists(gomock.Any(), coremachine.Name("0")).Return(
		machineerrors.MachineNotFound,
	)

	_, err := NewService(s.state, nil).WatchMachineTargetAgentVersion(context.Background(), "0")
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestSetReportedMachineAgentVersionInvalid is here to assert that if pass a
// junk agent binary version to [Service.SetReportedMachineAgentVersion] we get
// back an error that satisfies [coreerrors.NotValid].
func (s *suite) TestSetReportedMachineAgentVersionInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state, nil).SetReportedMachineAgentVersion(
		context.Background(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.Zero,
		},
	)
	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestSetReportedMachineAgentVersionSuccess asserts that if we try to set the
// reported agent version for a machine that doesn't exist we get an error
// satisfying [machineerrors.MachineNotFound]. Because the service relied on
// state for producing this error we need to simulate this in two different
// locations to assert the full functionality.
func (s *suite) TestSetReportedMachineAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// MachineNotFound error location 1.
	s.state.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return(
		"", machineerrors.MachineNotFound,
	)

	err := NewService(s.state, nil).SetReportedMachineAgentVersion(
		context.Background(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)

	// MachineNotFound error location 2.
	machineUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return(
		machineUUID.String(), nil,
	)

	s.state.EXPECT().SetMachineRunningAgentBinaryVersion(
		gomock.Any(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(machineerrors.MachineNotFound)

	err = NewService(s.state, nil).SetReportedMachineAgentVersion(
		context.Background(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *suite) TestSetReportedMachineAgentVersionDead(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return(
		machineUUID.String(), nil,
	)

	s.state.EXPECT().SetMachineRunningAgentBinaryVersion(
		gomock.Any(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(machineerrors.MachineIsDead)

	err = NewService(s.state, nil).SetReportedMachineAgentVersion(
		context.Background(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineIsDead)
}

// TestSetReportedMachineAgentVersion asserts the happy path of
// [Service.SetReportedMachineAgentVersion].
func (s *suite) TestSetReportedMachineAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return(
		machineUUID.String(), nil,
	)
	s.state.EXPECT().SetMachineRunningAgentBinaryVersion(
		gomock.Any(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(nil)

	err = NewService(s.state, nil).SetReportedMachineAgentVersion(
		context.Background(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIsNil)
}

// TestSetReportedUnitAgentVersionInvalid is here to assert that if pass a
// junk agent binary version to [Service.SetReportedUnitAgentVersion] we get
// back an error that satisfies [coreerrors.NotValid].
func (s *suite) TestSetReportedUnitAgentVersionInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state, nil).SetUnitReportedUnitAgentVersion(
		context.Background(),
		coreunit.Name("foo/666"),
		coreagentbinary.Version{
			Number: semversion.Zero,
		},
	)
	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestSetReportedUnitAgentVersionNotFound asserts that if we try to set the
// reported agent version for a unit that doesn't exist we get an error
// satisfying [applicationerrors.UnitNotFound]. Because the service relied on
// state for producing this error we need to simulate this in two different
// locations to assert the full functionality.
func (s *suite) TestSetReportedUnitAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// UnitNotFound error location 1.
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		"", applicationerrors.UnitNotFound,
	)

	err := NewService(s.state, nil).SetUnitReportedUnitAgentVersion(
		context.Background(),
		coreunit.Name("foo/666"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)

	// UnitNotFound error location 2.
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		unitUUID, nil,
	)

	s.state.EXPECT().SetUnitRunningAgentBinaryVersion(
		gomock.Any(),
		unitUUID,
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(applicationerrors.UnitNotFound)

	err = NewService(s.state, nil).SetUnitReportedUnitAgentVersion(
		context.Background(),
		coreunit.Name("foo/666"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestSetReportedUnitAgentVersionDead asserts that if we try to set the
// reported agent version for a dead unit we get an error satisfying
// [applicationerrors.UnitIsDead].
func (s *suite) TestSetReportedUnitAgentVersionDead(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		coreunit.UUID(unitUUID.String()), nil,
	)

	s.state.EXPECT().SetUnitRunningAgentBinaryVersion(
		gomock.Any(),
		coreunit.UUID(unitUUID.String()),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(applicationerrors.UnitIsDead)

	err := NewService(s.state, nil).SetUnitReportedUnitAgentVersion(
		context.Background(),
		coreunit.Name("foo/666"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitIsDead)
}

// TestSetReportedUnitAgentVersion asserts the happy path of
// [Service.SetReportedUnitAgentVersion].
func (s *suite) TestSetReportedUnitAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		coreunit.UUID(unitUUID.String()), nil,
	)

	s.state.EXPECT().SetUnitRunningAgentBinaryVersion(
		gomock.Any(),
		coreunit.UUID(unitUUID.String()),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(nil)

	err := NewService(s.state, nil).SetUnitReportedUnitAgentVersion(
		context.Background(),
		coreunit.Name("foo/666"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIsNil)
}
