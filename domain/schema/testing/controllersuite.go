// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/database/testing"
	jujutesting "github.com/juju/juju/testing"
)

// ControllerSuite is used to provide a sql.DB reference to tests.
// It is pre-populated with the controller schema.
type ControllerSuite struct {
	testing.DqliteSuite
}

// SetUpTest is responsible for setting up a testing database suite initialised
// with the controller schema.
func (s *ControllerSuite) SetUpTest(c *gc.C) {
	s.DqliteSuite.SetUpTest(c)
	s.DqliteSuite.ApplyDDL(c, &SchemaApplier{
		Schema:  schema.ControllerDDL(),
		Verbose: s.Verbose,
	})
	err := database.InsertControllerNodeID(context.Background(), s.DqliteSuite.TxnRunner(), 0x2dc171858c3155be)
	c.Assert(err, jc.ErrorIsNil)
}

// ApplyDDLForRunner is responsible for applying the controller schema to the
// given database.
func (s *ControllerSuite) ApplyDDLForRunner(c *gc.C, runner coredatabase.TxnRunner) {
	s.DqliteSuite.ApplyDDLForRunner(c, &SchemaApplier{
		Schema:  schema.ControllerDDL(),
		Verbose: s.Verbose,
	}, runner)
	err := database.InsertControllerNodeID(context.Background(), runner, 0x2dc171858c3155be)
	c.Assert(err, jc.ErrorIsNil)
}

// ControllerTxnRunner returns a txn runner attached to the controller database.
func (s *ControllerSuite) ControllerTxnRunner() coredatabase.TxnRunner {
	return s.TxnRunner()
}

// SeedControllerUUID sets the uuid in the controller table to the default
// testing value. It does not add any other controller config.
func (s *ControllerSuite) SeedControllerUUID(c *gc.C) string {
	controllerUUID := jujutesting.ControllerTag.Id()
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO controller (uuid) VALUES (?)`, controllerUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	return controllerUUID
}
