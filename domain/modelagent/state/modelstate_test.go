// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type modelStateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&modelStateSuite{})

// TestCheckMachineDoesNotExist is asserting that if no machine exists we get
// back an error satisfying [machineerrors.MachineNotFound].
func (s *modelStateSuite) TestCheckMachineDoesNotExist(c *gc.C) {
	err := NewModelState(s.TxnRunnerFactory()).CheckMachineExists(
		context.Background(),
		machine.Name("0"),
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestCheckUnitDoesNotExist is asserting that if no unit exists we get back an
// error satisfying [applicationerrors.UnitNotFound].
func (s *modelStateSuite) TestCheckUnitDoesNotExist(c *gc.C) {
	err := NewModelState(s.TxnRunnerFactory()).CheckUnitExists(
		context.Background(),
		"foo/0",
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestGetModelAgentVersionSuccess tests that State.GetModelAgentVersion is
// correct in the expected case when the model exists.
func (s *modelStateSuite) TestGetModelAgentVersionSuccess(c *gc.C) {
	expectedVersion, err := version.Parse("4.21.54")
	c.Assert(err, jc.ErrorIsNil)

	st := NewModelState(s.TxnRunnerFactory())
	s.setAgentVersion(c, expectedVersion.String())

	obtainedVersion, err := st.GetTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(obtainedVersion, jc.DeepEquals, expectedVersion)
}

// TestGetModelAgentVersionModelNotFound tests that State.GetModelAgentVersion
// returns modelerrors.NotFound when the model does not exist in the DB.
func (s *modelStateSuite) TestGetModelAgentVersionModelNotFound(c *gc.C) {
	st := NewModelState(s.TxnRunnerFactory())

	_, err := st.GetTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.AgentVersionNotFound)
}

// TestGetModelAgentVersionCantParseVersion tests that State.GetModelAgentVersion
// returns an appropriate error when the agent version in the DB is invalid.
func (s *modelStateSuite) TestGetModelAgentVersionCantParseVersion(c *gc.C) {
	s.setAgentVersion(c, "invalid-version")

	st := NewModelState(s.TxnRunnerFactory())
	_, err := st.GetTargetAgentVersion(context.Background())
	c.Check(err, gc.ErrorMatches, `parsing agent version: invalid version "invalid-version".*`)
}

// Set the agent version for the given model in the DB.
func (s *modelStateSuite) setAgentVersion(c *gc.C, vers string) {
	db, err := domain.NewStateBase(s.TxnRunnerFactory()).DB()
	c.Assert(err, jc.ErrorIsNil)

	q := "INSERT INTO agent_version (target_version) values ($M.target_version)"

	args := sqlair.M{"target_version": vers}
	stmt, err := sqlair.Prepare(q, args)
	c.Assert(err, jc.ErrorIsNil)

	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, args).Run()
	})
	c.Assert(err, jc.ErrorIsNil)
}
