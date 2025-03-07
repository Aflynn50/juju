// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/unitstate"
	unitstateerrors "github.com/juju/juju/domain/unitstate/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type stateSuite struct {
	testing.ModelSuite

	unitUUID coreunit.UUID
	unitName coreunit.Name
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	appState := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	appArg := application.AddApplicationArg{
		Charm: charm.Charm{
			Metadata: charm.Metadata{
				Name: "app",
			},
			Manifest: charm.Manifest{
				Bases: []charm.Base{{
					Name:          "ubuntu",
					Channel:       charm.Channel{Risk: charm.RiskStable},
					Architectures: []string{"amd64"},
				}},
			},
			ReferenceName: "app",
			Source:        charm.LocalSource,
			Architecture:  architecture.AMD64,
		},
	}

	s.unitName = unittesting.GenNewName(c, "app/0")
	unitArgs := []application.AddUnitArg{{UnitName: s.unitName}}

	ctx := context.Background()
	_, err := appState.CreateApplication(ctx, "app", appArg, unitArgs)
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit").Scan(&s.unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestSetUnitState(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	agentState := unitstate.UnitState{
		Name:          s.unitName,
		CharmState:    ptr(map[string]string{"one-key": "one-value"}),
		UniterState:   ptr("some-uniter-state-yaml"),
		RelationState: ptr(map[int]string{1: "one-value"}),
		StorageState:  ptr("some-storage-state-yaml"),
		SecretState:   ptr("some-secret-state-yaml"),
	}
	st.SetUnitState(context.Background(), agentState)

	expectedAgentState := unitstate.RetrievedUnitState{
		CharmState:    *agentState.CharmState,
		UniterState:   *agentState.UniterState,
		RelationState: *agentState.RelationState,
		StorageState:  *agentState.StorageState,
		SecretState:   *agentState.SecretState,
	}

	state, err := st.GetUnitState(context.Background(), s.unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(state, gc.DeepEquals, expectedAgentState)
}

func (s *stateSuite) TestSetUnitStateJustUniterState(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	agentState := unitstate.UnitState{
		Name:        s.unitName,
		UniterState: ptr("some-uniter-state-yaml"),
	}
	st.SetUnitState(context.Background(), agentState)

	expectedAgentState := unitstate.RetrievedUnitState{
		UniterState: *agentState.UniterState,
	}

	state, err := st.GetUnitState(context.Background(), s.unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(state, gc.DeepEquals, expectedAgentState)
}

func (s *stateSuite) TestGetUnitStateUnitNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetUnitState(context.Background(), "bad-uuid")
	c.Assert(err, jc.ErrorIs, unitstateerrors.UnitNotFound)
}

func (s *stateSuite) TestEnsureUnitStateRecord(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.ensureUnitStateRecord(ctx, tx, unitUUID{UUID: s.unitUUID})
	})
	c.Assert(err, jc.ErrorIsNil)

	var uuid coreunit.UUID
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit").Scan(&uuid)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, s.unitUUID)

	// Running again makes no change.
	err = s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.ensureUnitStateRecord(ctx, tx, unitUUID{UUID: s.unitUUID})
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit").Scan(&uuid)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, s.unitUUID)
}

func (s *stateSuite) TestUpdateUnitStateUniter(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	expState := "some uniter state YAML"

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.ensureUnitStateRecord(ctx, tx, unitUUID{UUID: s.unitUUID}); err != nil {
			return err
		}
		return st.updateUnitStateUniter(ctx, tx, unitUUID{UUID: s.unitUUID}, expState)
	})
	c.Assert(err, jc.ErrorIsNil)

	var gotState string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT uniter_state FROM unit_state where unit_uuid = ?"
		return tx.QueryRowContext(ctx, q, s.unitUUID).Scan(&gotState)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotState, gc.Equals, expState)
}

func (s *stateSuite) TestUpdateUnitStateStorage(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	expState := "some storage state YAML"

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.ensureUnitStateRecord(ctx, tx, unitUUID{UUID: s.unitUUID}); err != nil {
			return err
		}
		return st.updateUnitStateStorage(ctx, tx, unitUUID{UUID: s.unitUUID}, expState)
	})
	c.Assert(err, jc.ErrorIsNil)

	var gotState string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT storage_state FROM unit_state where unit_uuid = ?"
		return tx.QueryRowContext(ctx, q, s.unitUUID).Scan(&gotState)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotState, gc.Equals, expState)
}

func (s *stateSuite) TestUpdateUnitStateSecret(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	expState := "some secret state YAML"

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.ensureUnitStateRecord(ctx, tx, unitUUID{UUID: s.unitUUID}); err != nil {
			return err
		}
		return st.updateUnitStateSecret(ctx, tx, unitUUID{UUID: s.unitUUID}, expState)
	})
	c.Assert(err, jc.ErrorIsNil)

	var gotState string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT secret_state FROM unit_state where unit_uuid = ?"
		return tx.QueryRowContext(ctx, q, s.unitUUID).Scan(&gotState)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotState, gc.Equals, expState)
}

func (s *stateSuite) TestUpdateUnitStateCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	// Set some initial state. This should be overwritten.
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "INSERT INTO unit_state_charm VALUES (?, 'one-key', 'one-val')"
		_, err := tx.ExecContext(ctx, q, s.unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	expState := map[string]string{
		"two-key":   "two-val",
		"three-key": "three-val",
	}

	err = s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setUnitStateCharm(ctx, tx, unitUUID{UUID: s.unitUUID}, expState)
	})
	c.Assert(err, jc.ErrorIsNil)

	gotState := make(map[string]string)
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT key, value FROM unit_state_charm WHERE unit_uuid = ?"
		rows, err := tx.QueryContext(ctx, q, s.unitUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				return err
			}
			gotState[k] = v
		}
		return rows.Err()
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(gotState, gc.DeepEquals, expState)
}

func (s *stateSuite) TestUpdateUnitStateRelation(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	// Set some initial state. This should be overwritten.
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "INSERT INTO unit_state_relation VALUES (?, 1, 'one-val')"
		_, err := tx.ExecContext(ctx, q, s.unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	expState := map[int]string{
		2: "two-val",
		3: "three-val",
	}

	err = s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setUnitStateRelation(ctx, tx, unitUUID{UUID: s.unitUUID}, expState)
	})
	c.Assert(err, jc.ErrorIsNil)

	gotState := make(map[int]string)
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT key, value FROM unit_state_relation WHERE unit_uuid = ?"
		rows, err := tx.QueryContext(ctx, q, s.unitUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var k int
			var v string
			if err := rows.Scan(&k, &v); err != nil {
				return err
			}
			gotState[k] = v
		}
		return rows.Err()
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(gotState, gc.DeepEquals, expState)
}

func (s *stateSuite) TestUpdateUnitStateRelationEmptyMap(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	// Set some initial state. This should be overwritten.
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "INSERT INTO unit_state_relation VALUES (?, 1, 'one-val')"
		_, err := tx.ExecContext(ctx, q, s.unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setUnitStateRelation(ctx, tx, unitUUID{UUID: s.unitUUID}, map[int]string{})
	})
	c.Assert(err, jc.ErrorIsNil)

	var rowCount int
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT key, value FROM unit_state_relation WHERE unit_uuid = ?"
		rows, err := tx.QueryContext(ctx, q, s.unitUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			rowCount++
		}
		return rows.Err()
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(rowCount, gc.DeepEquals, 0)
}

func ptr[T any](v T) *T {
	return &v
}
