// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	coredb "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain"
	blockdevice "github.com/juju/juju/domain/blockdevice/state"
	"github.com/juju/juju/domain/life"
)

// State describes retrieval and persistence methods for storage.
type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState returns a new state reference.
func NewState(factory coredb.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// CreateMachine creates or updates the specified machine.
// TODO - this just creates a minimal row for now.
func (st *State) CreateMachine(ctx context.Context, machineId coremachine.ID, nodeUUID, machineUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	machineIDParam := sqlair.M{"machine_id": machineId}
	query := `SELECT &M.uuid FROM machine WHERE machine_id = $M.machine_id`
	queryStmt, err := st.Prepare(query, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	createMachine := `
INSERT INTO machine (uuid, net_node_uuid, machine_id, life_id)
VALUES ($M.machine_uuid, $M.net_node_uuid, $M.machine_id, $M.life_id)
`
	createMachineStmt, err := st.Prepare(createMachine, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	createNode := `INSERT INTO net_node (uuid) VALUES ($M.net_node_uuid)`
	createNodeStmt, err := st.Prepare(createNode, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := sqlair.M{}
		err := tx.Query(ctx, queryStmt, machineIDParam).Get(&result)
		if err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(err, "querying machine %q", machineId)
			}
		}
		// For now, we just care if the minimal machine row already exists.
		if err == nil {
			return nil
		}

		createParams := sqlair.M{
			"machine_uuid":  machineUUID,
			"net_node_uuid": nodeUUID,
			"machine_id":    machineId,
			"life_id":       life.Alive,
		}
		if err := tx.Query(ctx, createNodeStmt, createParams).Run(); err != nil {
			return errors.Annotatef(err, "creating net node row for machine %q", machineId)
		}
		if err := tx.Query(ctx, createMachineStmt, createParams).Run(); err != nil {
			return errors.Annotatef(err, "creating machine row for machine %q", machineId)
		}
		return nil
	})
	return errors.Annotatef(err, "upserting machine %q", machineId)
}

// DeleteMachine deletes the specified machine and any dependent child records.
// TODO - this just deals with child block devices for now.
func (st *State) DeleteMachine(ctx context.Context, machineId coremachine.ID) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	machineIDParam := sqlair.M{"machine_id": machineId}

	queryMachine := `SELECT &M.uuid FROM machine WHERE machine_id = $M.machine_id`
	queryMachineStmt, err := st.Prepare(queryMachine, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	deleteMachine := `DELETE FROM machine WHERE machine_id = $M.machine_id`
	deleteMachineStmt, err := st.Prepare(deleteMachine, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	deleteNode := `
DELETE FROM net_node WHERE uuid IN
(SELECT net_node_uuid FROM machine WHERE machine_id = $M.machine_id) 
`
	deleteNodeStmt, err := st.Prepare(deleteNode, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := sqlair.M{}
		err = tx.Query(ctx, queryMachineStmt, machineIDParam).Get(result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "looking up UUID for machine %q", machineId)
		}
		// Machine already deleted is a no op.
		if len(result) == 0 {
			return nil
		}
		machineUUID := result["uuid"].(string)

		if err := blockdevice.RemoveMachineBlockDevices(ctx, tx, machineUUID); err != nil {
			return errors.Annotatef(err, "deleting block devices for machine %q", machineId)
		}

		if err := tx.Query(ctx, deleteMachineStmt, machineIDParam).Run(); err != nil {
			return errors.Annotatef(err, "deleting machine %q", machineId)
		}
		if err := tx.Query(ctx, deleteNodeStmt, machineIDParam).Run(); err != nil {
			return errors.Annotatef(err, "deleting net node for machine  %q", machineId)
		}

		return nil
	})
	return errors.Annotatef(err, "deleting machine %q", machineId)
}

// InitialWatchStatement returns the table and the initial watch statement
// for the machines.
func (s *State) InitialWatchStatement() (string, string) {
	return "machine", "SELECT machine_id FROM machine"
}

// GetMachineLife returns the life status of the specified machine.
func (st *State) GetMachineLife(ctx context.Context, machineId coremachine.ID) (*life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	queryForLife := `SELECT life_id as &machineLife.life_id FROM machine WHERE machine_id = $M.machine_id`
	lifeStmt, err := st.Prepare(queryForLife, sqlair.M{}, machineLife{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var lifeResult life.Life
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := machineLife{}
		err := tx.Query(ctx, lifeStmt, sqlair.M{"machine_id": machineId}).Get(&result)
		if err != nil {
			return errors.Annotatef(err, "looking up life for machine %q", machineId)
		}

		lifeResult = result.ID

		return nil
	})

	if err != nil && errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.NotFoundf("machine %q", machineId)
	}

	return &lifeResult, errors.Annotatef(err, "getting life status for machines %q", machineId)
}

// ListAllMachines retrieves the ids of all machines in the model.
func (st *State) ListAllMachines(ctx context.Context) ([]coremachine.ID, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	query := `SELECT machine_id AS &machineID.* FROM machine`
	queryStmt, err := st.Prepare(query, machineID{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []machineID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, queryStmt).GetAll(&results)
	})
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Annotate(err, "querying all machines")
	}

	var machineIds []coremachine.ID
	for _, result := range results {
		machineIds = append(machineIds, coremachine.ID(result.ID))
	}
	return machineIds, nil
}
