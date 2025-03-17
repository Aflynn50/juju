// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	coredb "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain"
	blockdevice "github.com/juju/juju/domain/blockdevice/state"
	"github.com/juju/juju/domain/life"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	internalerrors "github.com/juju/juju/internal/errors"
)

// State describes retrieval and persistence methods for storage.
type State struct {
	*domain.StateBase
	clock  clock.Clock
	logger logger.Logger
}

// NewState returns a new state reference.
func NewState(factory coredb.TxnRunnerFactory, clock clock.Clock, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		clock:     clock,
		logger:    logger,
	}
}

// CreateMachine creates or updates the specified machine.
// Adds a row to machine table, as well as a row to the net_node table.
// It returns a MachineAlreadyExists error if a machine with the same name
// already exists.
func (st *State) CreateMachine(ctx context.Context, machineName machine.Name, nodeUUID, machineUUID string) error {
	return st.createMachine(ctx, createMachineArgs{
		name:        machineName,
		netNodeUUID: nodeUUID,
		machineUUID: machineUUID,
	})
}

// CreateMachineWithParent creates or updates the specified machine with a
// parent.
// Adds a row to machine table, as well as a row to the net_node table, and adds
// a row to the machine_parent table for associating with the specified parent.
// It returns a MachineNotFound error if the parent machine does not exist.
// It returns a MachineAlreadyExists error if a machine with the same name
// already exists.
func (st *State) CreateMachineWithParent(ctx context.Context, machineName, parentName machine.Name, nodeUUID, machineUUID string) error {
	return st.createMachine(ctx, createMachineArgs{
		name:        machineName,
		netNodeUUID: nodeUUID,
		machineUUID: machineUUID,
		parentName:  parentName,
	})
}

// createMachine creates or updates the specified machine.
// Adds a row to machine table, as well as a row to the net_node table.
// It returns the uuid of the created machine.
// It returns a MachineAlreadyExists error if a machine with the same name
// already exists.
func (st *State) createMachine(ctx context.Context, args createMachineArgs) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	mName := args.name

	// Prepare query for machine uuid.
	machineNameParam := machineName{Name: mName}
	machineUUIDout := machineUUID{}
	machineUUIDQuery := `SELECT &machineUUID.uuid FROM machine WHERE name = $machineName.name`
	machineUUIDStmt, err := st.Prepare(machineUUIDQuery, machineNameParam, machineUUIDout)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for creating machine row.
	createParams := sqlair.M{
		"machine_uuid":  args.machineUUID,
		"net_node_uuid": args.netNodeUUID,
		"name":          mName,
		"life_id":       life.Alive,
	}
	createMachineQuery := `
INSERT INTO machine (uuid, net_node_uuid, name, life_id)
VALUES ($M.machine_uuid, $M.net_node_uuid, $M.name, $M.life_id)
`
	createMachineStmt, err := st.Prepare(createMachineQuery, createParams)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for creating net node row.
	createNodeQuery := `INSERT INTO net_node (uuid) VALUES ($M.net_node_uuid)`
	createNodeStmt, err := st.Prepare(createNodeQuery, createParams)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for associating/verifying parent machine.
	var parentNameParam machineName
	var associateParentStmt *sqlair.Statement
	var associateParentParam machineParent
	var parentQueryStmt *sqlair.Statement
	if args.parentName != "" {
		parentNameParam = machineName{Name: args.parentName}
		associateParentParam = machineParent{MachineUUID: args.machineUUID}
		associateParentQuery := `
INSERT INTO machine_parent (machine_uuid, parent_uuid)
VALUES ($machineParent.machine_uuid, $machineParent.parent_uuid)
`
		associateParentStmt, err = st.Prepare(associateParentQuery, associateParentParam)
		if err != nil {
			return errors.Trace(err)
		}

		// Prepare query for verifying there's no grandparent.
		outputMachineParent := machineParent{}
		inputParentMachineUUID := machineUUID{}
		parentQuery := `
SELECT parent_uuid AS &machineParent.parent_uuid
FROM   machine_parent
WHERE  machine_uuid = $machineUUID.uuid`
		parentQueryStmt, err = st.Prepare(parentQuery, outputMachineParent, inputParentMachineUUID)
		if err != nil {
			return errors.Trace(err)
		}
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine uuid. If the machine already exists, return a
		// MachineAlreadyExists error.
		err := tx.Query(ctx, machineUUIDStmt, machineNameParam).Get(&machineUUIDout)
		// No error means we found the machine with the given name.
		if err == nil {
			return errors.Annotatef(machineerrors.MachineAlreadyExists, "machine %q", mName)
		}
		if !errors.Is(err, sqlair.ErrNoRows) {
			// Return error if the query failed for any reason other than not
			// found.
			return errors.Annotatef(err, "querying machine %q", mName)
		}

		// Run query to create net node row.
		if err := tx.Query(ctx, createNodeStmt, createParams).Run(); err != nil {
			return errors.Annotatef(err, "creating net node row for machine %q", mName)
		}

		// Run query to create machine row.
		if err := tx.Query(ctx, createMachineStmt, createParams).Run(); err != nil {
			return errors.Annotatef(err, "creating machine row for machine %q", mName)
		}

		// Associate a parent machine if parentName is provided.
		if args.parentName != "" {
			// Query for the parent uuid.
			// Reusing the machineUUIDout variable for the parent.
			err := tx.Query(ctx, machineUUIDStmt, parentNameParam).Get(&machineUUIDout)
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(machineerrors.MachineNotFound, "parent machine %q for %q", args.parentName, mName)
			}
			if err != nil {
				return errors.Annotatef(err, "querying parent machine %q for machine %q", args.parentName, mName)
			}

			// Protect against a grandparent
			machineParentUUID := machineUUID{}
			machineParentUUID.UUID = machineUUIDout.UUID
			machineParent := machineParent{}
			err = tx.Query(ctx, parentQueryStmt, machineParentUUID).Get(&machineParent)
			// No error means we found a grandparent.
			if err == nil {
				return errors.Annotatef(machineerrors.GrandParentNotSupported, "machine %q", mName)
			}
			if !errors.Is(err, sqlair.ErrNoRows) {
				// Return error if the query failed for any reason other than not
				// found.
				return errors.Annotatef(err, "querying for grandparent UUID for machine %q", mName)
			}

			// Run query to associate parent machine.
			associateParentParam.ParentUUID = machineUUIDout.UUID
			if err := tx.Query(ctx, associateParentStmt, associateParentParam).Run(); err != nil {
				return errors.Annotatef(err, "associating parent machine %q for machine %q", args.parentName, mName)
			}
		}

		return nil
	})
	return errors.Annotatef(err, "inserting machine %q", mName)
}

// DeleteMachine deletes the specified machine and any dependent child records.
// TODO - this just deals with child block devices for now.
func (st *State) DeleteMachine(ctx context.Context, mName machine.Name) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for machine uuid.
	machineNameParam := machineName{Name: mName}
	machineUUIDParam := machineUUID{}
	queryMachine := `SELECT uuid AS &machineUUID.* FROM machine WHERE name = $machineName.name`
	queryMachineStmt, err := st.Prepare(queryMachine, machineNameParam, machineUUIDParam)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for deleting machine row.
	deleteMachine := `DELETE FROM machine WHERE name = $machineName.name`
	deleteMachineStmt, err := st.Prepare(deleteMachine, machineNameParam)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for deleting net node row.
	deleteNode := `
DELETE FROM net_node WHERE uuid IN
(SELECT net_node_uuid FROM machine WHERE name = $machineName.name)
`
	deleteNodeStmt, err := st.Prepare(deleteNode, machineNameParam)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for deleting status and status data for the machine.
	deleteStatus := `DELETE FROM machine_status WHERE machine_uuid = $machineUUID.uuid`
	deleteStatusStmt, err := st.Prepare(deleteStatus, machineUUIDParam)
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, queryMachineStmt, machineNameParam).Get(&machineUUIDParam)
		if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		}
		if err != nil {
			return errors.Annotatef(err, "looking up UUID for machine %q", mName)
		}

		// Remove block devices for the machine.
		if err := blockdevice.RemoveMachineBlockDevices(ctx, tx, machineUUIDParam.UUID); err != nil {
			return errors.Annotatef(err, "deleting block devices for machine %q", mName)
		}

		// Remove the status for the machine. No need to return error if no
		// status is set for the machine while deleting.
		if err := tx.Query(ctx, deleteStatusStmt, machineUUIDParam).Run(); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "deleting status for machine %q", mName)
		}

		// Remove the machine.
		if err := tx.Query(ctx, deleteMachineStmt, machineNameParam).Run(); err != nil {
			return errors.Annotatef(err, "deleting machine %q", mName)
		}

		// Remove the net node for the machine.
		if err := tx.Query(ctx, deleteNodeStmt, machineNameParam).Run(); err != nil {
			return errors.Annotatef(err, "deleting net node for machine  %q", mName)
		}

		return nil
	})
	return errors.Annotatef(err, "deleting machine %q", mName)
}

// InitialWatchModelMachinesStatement returns the table and the initial watch
// statement for watching life changes of non-container machines.
func (st *State) InitialWatchModelMachinesStatement() (string, string) {
	return "machine", "SELECT name FROM machine WHERE name NOT LIKE '%/%'"
}

// InitialWatchStatement returns the table and the initial watch statement
// for the machines.
func (st *State) InitialWatchStatement() (string, string) {
	return "machine", "SELECT name FROM machine"
}

// GetMachineLife returns the life status of the specified machine.
// It returns a MachineNotFound if the given machine doesn't exist.
func (st *State) GetMachineLife(ctx context.Context, mName machine.Name) (*life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	machineNameParam := machineName{Name: mName}
	queryForLife := `SELECT life_id as &machineLife.life_id FROM machine WHERE name = $machineName.name`
	lifeStmt, err := st.Prepare(queryForLife, machineNameParam, machineLife{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var lifeResult life.Life
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := machineLife{}
		err := tx.Query(ctx, lifeStmt, machineNameParam).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		}
		if err != nil {
			return errors.Annotatef(err, "looking up life for machine %q", mName)
		}

		lifeResult = result.LifeID

		return nil
	})
	if err != nil {
		return nil, errors.Annotatef(err, "getting life status for machine %q", mName)
	}
	return &lifeResult, nil
}

// GetMachineStatus returns the status of the specified machine.
// It returns MachineNotFound if the machine does not exist.
// It returns a StatusNotSet if the status is not set.
// Idempotent.
func (st *State) GetMachineStatus(ctx context.Context, mName machine.Name) (domainmachine.StatusInfo[domainmachine.MachineStatusType], error) {
	db, err := st.DB()
	if err != nil {
		return domainmachine.StatusInfo[domainmachine.MachineStatusType]{}, errors.Trace(err)
	}

	nameIdent := machineName{Name: mName}

	var uuid machineUUID
	uuidQuery := `SELECT uuid AS &machineUUID.uuid FROM machine WHERE name = $machineName.name;`
	uuidQueryStmt, err := st.Prepare(uuidQuery, nameIdent, uuid)
	if err != nil {
		return domainmachine.StatusInfo[domainmachine.MachineStatusType]{}, errors.Trace(err)
	}

	var status machineStatus
	statusQuery := `
SELECT &machineStatus.*
FROM v_machine_status AS st
WHERE st.machine_uuid = $machineUUID.uuid;
`
	statusCombinedQueryStmt, err := st.Prepare(statusQuery, uuid, status)
	if err != nil {
		return domainmachine.StatusInfo[domainmachine.MachineStatusType]{}, errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine uuid
		err := tx.Query(ctx, uuidQueryStmt, nameIdent).Get(&uuid)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(machineerrors.MachineNotFound, "machine %q", mName)
		} else if err != nil {
			return errors.Annotatef(err, "querying uuid for machine %q", mName)
		}

		// Query for the machine cloud instance status and status data combined
		err = tx.Query(ctx, statusCombinedQueryStmt, uuid).Get(&status)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(machineerrors.StatusNotSet, "machine: %q", mName)
		} else if err != nil {
			return errors.Annotatef(err, "querying machine status for machine %q", mName)
		}

		return nil
	})

	if err != nil {
		return domainmachine.StatusInfo[domainmachine.MachineStatusType]{}, errors.Trace(err)
	}

	// Convert the internal status id from the (machine_status_value table)
	// into the core status.Status type.
	machineStatus, err := decodeMachineStatus(status.Status)
	if err != nil {
		return domainmachine.StatusInfo[domainmachine.MachineStatusType]{}, errors.Annotatef(err, "decoding machine status for machine %q", mName)
	}

	var since time.Time
	if status.Updated.Valid {
		since = status.Updated.Time
	} else {
		since = st.clock.Now()
	}

	var data []byte
	if len(status.Data) > 0 {
		data = status.Data
	}

	return domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status:  machineStatus,
		Message: status.Message,
		Since:   &since,
		Data:    data,
	}, nil
}

// SetMachineStatus sets the status of the specified machine.
// It returns MachineNotFound if the machine does not exist.
func (st *State) SetMachineStatus(ctx context.Context, mName machine.Name, newStatus domainmachine.StatusInfo[domainmachine.MachineStatusType]) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	statusID, err := encodeMachineStatus(newStatus.Status)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare the new status to be set.
	status := setMachineStatus{
		StatusID: statusID,
		Message:  newStatus.Message,
		Data:     newStatus.Data,
		Updated:  newStatus.Since,
	}

	// Prepare query for machine uuid
	nameIdent := machineName{Name: mName}

	var mUUID machineUUID
	queryMachine := `SELECT uuid AS &machineUUID.* FROM machine WHERE name = $machineName.name;`
	queryMachineStmt, err := st.Prepare(queryMachine, nameIdent, mUUID)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for setting machine status
	statusQuery := `
INSERT INTO machine_status (*)
VALUES ($setMachineStatus.*)
  ON CONFLICT (machine_uuid)
  DO UPDATE SET 
  	status_id = excluded.status_id, 
	message = excluded.message, 
	updated_at = excluded.updated_at,
	data = excluded.data;
`
	statusQueryStmt, err := st.Prepare(statusQuery, status)
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine uuid.
		err := tx.Query(ctx, queryMachineStmt, nameIdent).Get(&mUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(machineerrors.MachineNotFound, "machine %q", mName)
		} else if err != nil {
			return errors.Annotatef(err, "querying uuid for machine %q", mName)
		}

		// Set the machine uuid in the status struct.
		status.MachineUUID = mUUID.UUID

		// Query for setting the machine status.
		err = tx.Query(ctx, statusQueryStmt, status).Run()
		if err != nil {
			return errors.Annotatef(err, "setting machine status for machine %q", mName)
		}

		return nil
	})
}

// SetMachineLife sets the life status of the specified machine.
// It returns a MachineNotFound if the provided machine doesn't exist.
func (st *State) SetMachineLife(ctx context.Context, mName machine.Name, life life.Life) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for machine UUID.
	machineNameParam := machineName{Name: mName}
	machineUUIDoutput := machineUUID{}
	uuidQuery := `SELECT &machineUUID.uuid FROM machine WHERE name = $machineName.name`
	uuidQueryStmt, err := st.Prepare(uuidQuery, machineNameParam, machineUUIDoutput)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for updating machine life.
	machineLifeParam := machineLife{LifeID: life}
	query := `UPDATE machine SET life_id = $machineLife.life_id WHERE uuid = $machineLife.uuid`
	queryStmt, err := st.Prepare(query, machineLifeParam)
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for machine uuid, return MachineNotFound if machine doesn't exist.
		err := tx.Query(ctx, uuidQueryStmt, machineNameParam).Get(&machineUUIDoutput)
		if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		}
		if err != nil {
			return errors.Annotatef(err, "querying UUID for machine %q", mName)
		}

		// Update machine life status.
		machineLifeParam.UUID = machineUUIDoutput.UUID
		err = tx.Query(ctx, queryStmt, machineLifeParam).Run()
		if err != nil {
			return errors.Annotatef(err, "setting life for machine %q", mName)
		}
		return nil
	})
}

// IsMachineController returns whether the machine is a controller machine.
// It returns a NotFound if the given machine doesn't exist.
func (st *State) IsMachineController(ctx context.Context, mName machine.Name) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Trace(err)
	}

	machineNameParam := machineName{Name: mName}
	result := machineIsController{}
	query := `SELECT &machineIsController.is_controller FROM machine WHERE name = $machineName.name`
	queryStmt, err := st.Prepare(query, machineNameParam, result)
	if err != nil {
		return false, errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, queryStmt, machineNameParam).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		}
		if err != nil {
			return errors.Annotatef(err, "querying if machine %q is a controller", mName)
		}
		return nil
	})
	if err != nil {
		return false, errors.Annotatef(err, "checking if machine %q is a controller", mName)
	}

	return result.IsController, nil
}

// AllMachineNames retrieves the names of all machines in the model.
func (st *State) AllMachineNames(ctx context.Context) ([]machine.Name, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	query := `SELECT name AS &machineName.* FROM machine`
	queryStmt, err := st.Prepare(query, machineName{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []machineName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, queryStmt).GetAll(&results)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errors.Annotate(err, "querying all machines")
		}
		return nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Transform the results ([]machineName) into a slice of machine.Name.
	machineNames := transform.Slice(results, machineName.nameSliceTransform)

	return machineNames, nil
}

// GetMachineParentUUID returns the parent UUID of the specified machine.
// It returns a MachineNotFound if the machine does not exist.
// It returns a MachineHasNoParent if the machine has no parent.
func (st *State) GetMachineParentUUID(ctx context.Context, uuid string) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	// Prepare query for checking that the machine exists.
	currentMachineUUID := machineUUID{UUID: uuid}
	query := `SELECT uuid AS &machineUUID.uuid FROM machine WHERE uuid = $machineUUID.uuid`
	queryStmt, err := st.Prepare(query, currentMachineUUID)
	if err != nil {
		return "", errors.Trace(err)
	}

	// Prepare query for parent UUID.
	parentUUID := ""
	parentUUIDParam := machineParent{}
	parentQuery := `
SELECT parent_uuid AS &machineParent.parent_uuid
FROM machine_parent WHERE machine_uuid = $machineUUID.uuid`
	parentQueryStmt, err := st.Prepare(parentQuery, currentMachineUUID, parentUUIDParam)
	if err != nil {
		return "", errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine UUID.
		outUUID := machineUUID{} // This value doesn't really matter, it is just a way to check existence
		err := tx.Query(ctx, queryStmt, currentMachineUUID).Get(&outUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(machineerrors.MachineNotFound, "machine %q", uuid)
		}
		if err != nil {
			return errors.Annotatef(err, "checking existence of machine %q", uuid)
		}

		// Query for the parent UUID.
		err = tx.Query(ctx, parentQueryStmt, currentMachineUUID).Get(&parentUUIDParam)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(machineerrors.MachineHasNoParent, "machine %q", uuid)
		}
		if err != nil {
			return errors.Annotatef(err, "querying parent UUID for machine %q", uuid)
		}

		parentUUID = parentUUIDParam.ParentUUID

		return nil
	})
	return parentUUID, errors.Annotatef(err, "getting parent UUID for machine %q", uuid)
}

// MarkMachineForRemoval marks the specified machine for removal.
// It returns NotFound if the machine does not exist.
// TODO(cderici): use machineerrors.MachineNotFound on rebase after #17759
// lands.
// Idempotent.
func (st *State) MarkMachineForRemoval(ctx context.Context, mName machine.Name) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for getting the machine UUID.
	machineNameParam := machineName{Name: mName}
	markForRemovalUUID := machineMarkForRemoval{}
	machineUUIDQuery := `SELECT uuid AS &machineMarkForRemoval.machine_uuid FROM machine WHERE name = $machineName.name`
	machineUUIDStmt, err := st.Prepare(machineUUIDQuery, machineNameParam, markForRemovalUUID)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for adding the machine to the machine_removals table.
	markForRemovalUpdateQuery := `
INSERT OR IGNORE INTO machine_removals (machine_uuid)
VALUES ($machineMarkForRemoval.machine_uuid)`
	markForRemovalStmt, err := st.Prepare(markForRemovalUpdateQuery, markForRemovalUUID)
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine UUID.
		err := tx.Query(ctx, machineUUIDStmt, machineNameParam).Get(&markForRemovalUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(machineerrors.MachineNotFound, "machine %q", mName)
		}
		if err != nil {
			return errors.Annotatef(err, "querying UUID for machine %q", mName)
		}

		// Run query for adding the machine to the removals table.
		return tx.Query(ctx, markForRemovalStmt, markForRemovalUUID).Run()
	})

	return errors.Annotatef(err, "marking machine %q for removal", mName)
}

// GetAllMachineRemovals returns the UUIDs of all of the machines that need to
// be removed but need provider-level cleanup.
func (st *State) GetAllMachineRemovals(ctx context.Context) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Prepare query for uuids of all machines marked for removal.
	markForRemovalParam := machineMarkForRemoval{}
	machinesMarkedForRemovalQuery := `SELECT &machineMarkForRemoval.machine_uuid FROM machine_removals`
	machinesMarkedForRemovalStmt, err := st.Prepare(machinesMarkedForRemovalQuery, markForRemovalParam)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []machineMarkForRemoval
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Run query to get all machines marked for removal.
		err := tx.Query(ctx, machinesMarkedForRemovalStmt).GetAll(&results)
		// No errors if there's no machine marked for removal.
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})

	if err != nil {
		return nil, errors.Annotate(err, "querying all machines marked for removal")
	}

	// Transform the results ([]machineUUID) into a slice of machine UUIDs.
	machineUUIDs := transform.Slice(
		results,
		machineMarkForRemoval.uuidSliceTransform,
	)

	return machineUUIDs, nil
}

// GetMachineUUID returns the UUID of a machine identified by its name.
// It returns a MachineNotFound if the machine does not exist.
func (st *State) GetMachineUUID(ctx context.Context, name machine.Name) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	var uuid machineUUID
	currentMachineName := machineName{Name: name}
	query := `SELECT uuid AS &machineUUID.uuid FROM machine WHERE name = $machineName.name`
	queryStmt, err := st.Prepare(query, uuid, currentMachineName)
	if err != nil {
		return "", errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine UUID.
		err := tx.Query(ctx, queryStmt, currentMachineName).Get(&uuid)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(machineerrors.MachineNotFound, "machine %q", name)
		}
		if err != nil {
			return errors.Annotatef(err, "querying uuid for machine %q", name)
		}
		return nil
	})
	return uuid.UUID, errors.Annotatef(err, "getting UUID for machine %q", name)
}

// ShouldKeepInstance reports whether a machine, when removed from Juju, should cause
// the corresponding cloud instance to be stopped.
func (st *State) ShouldKeepInstance(ctx context.Context, mName machine.Name) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Trace(err)
	}

	machineNameParam := machineName{Name: mName}
	result := keepInstance{}
	query := `
SELECT &keepInstance.keep_instance
FROM   machine
WHERE  name = $machineName.name`
	queryStmt, err := st.Prepare(query, machineNameParam, result)
	if err != nil {
		return false, errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, queryStmt, machineNameParam).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		}
		if err != nil {
			return fmt.Errorf("querying machine %q keep instance: %w", mName, err)
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("check for machine %q keep instance: %w", mName, err)
	}

	return result.KeepInstance, nil
}

// SetKeepInstance sets whether the machine cloud instance will be retained
// when the machine is removed from Juju. This is only relevant if an instance
// exists.
func (st *State) SetKeepInstance(ctx context.Context, mName machine.Name, keep bool) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for machine uuid.
	machineUUID := machineUUID{}
	machineNameParam := machineName{Name: mName}
	machineExistsQuery := `
SELECT uuid AS &machineUUID.uuid
FROM   machine
WHERE  name = $machineName.name`
	machineExistsStmt, err := st.Prepare(machineExistsQuery, machineUUID, machineNameParam)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for updating machine keep instance.
	keepInstanceParam := keepInstance{KeepInstance: keep}
	keepInstanceQuery := `
UPDATE machine
SET    keep_instance = $keepInstance.keep_instance
WHERE  name = $machineName.name`
	keepInstanceStmt, err := st.Prepare(keepInstanceQuery, keepInstanceParam, machineNameParam)
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine uuid before attempting to update it,
		// and return an error if it doesn't.
		err := tx.Query(ctx, machineExistsStmt, machineNameParam).Get(&machineUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		}
		// Update machine keep instance.
		err = tx.Query(ctx, keepInstanceStmt, keepInstanceParam, machineNameParam).Run()
		if err != nil {
			return fmt.Errorf("setting keep instance for machine %q: %w", mName, err)
		}
		return nil
	})
}

// AppliedLXDProfileNames returns the names of the LXD profiles on the machine.
func (st *State) AppliedLXDProfileNames(ctx context.Context, mUUID string) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	instanceDataQuery := instanceData{MachineUUID: mUUID}
	isProvisionedStmt, err := st.Prepare(`
SELECT   &instanceData.instance_id
FROM     machine_cloud_instance
WHERE    machine_uuid = $instanceData.machine_uuid`, instanceDataQuery)
	if err != nil {
		return nil, errors.Trace(err)
	}

	lxdProfileQuery := lxdProfile{MachineUUID: mUUID}
	queryStmt, err := st.Prepare(`
SELECT   &lxdProfile.name
FROM     machine_lxd_profile
WHERE    machine_uuid = $lxdProfile.machine_uuid
ORDER BY array_index ASC`, lxdProfileQuery)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result []lxdProfile
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var instanceData instanceData
		err := tx.Query(ctx, isProvisionedStmt, instanceDataQuery).Get(&instanceData)
		if errors.Is(err, sqlair.ErrNoRows) {
			return internalerrors.Errorf("machine %q: %w", mUUID, machineerrors.NotProvisioned)
		}
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return internalerrors.Errorf("checking machine cloud instance for machine %q: %w", mUUID, err)
		}
		err = tx.Query(ctx, queryStmt, lxdProfileQuery).GetAll(&result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return internalerrors.Errorf("retrieving lxd profiles for machine %q: %w", mUUID, err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	lxdProfiles := make([]string, len(result))
	for i, lxdProfile := range result {
		lxdProfiles[i] = lxdProfile.Name
	}
	return lxdProfiles, nil
}

// SetAppliedLXDProfileNames sets the list of LXD profile names to the
// lxd_profile table for the given machine. This method will overwrite the list
// of profiles for the given machine without any checks.
// [machineerrors.MachineNotFound] will be returned if the machine does not
// exist.
func (st *State) SetAppliedLXDProfileNames(ctx context.Context, mUUID string, profileNames []string) error {
	if len(profileNames) == 0 {
		return nil
	}

	db, err := st.DB()
	if err != nil {
		return internalerrors.Errorf("cannot get database to set lxd profiles %v for machine %q: %w", profileNames, mUUID, err)
	}

	queryMachineUUID := machineUUID{UUID: mUUID}
	checkMachineExistsStmt, err := st.Prepare(`
SELECT uuid AS &machineUUID.uuid
FROM   machine
WHERE  machine.uuid = $machineUUID.uuid`, queryMachineUUID)
	if err != nil {
		return errors.Trace(err)
	}

	instanceDataQuery := instanceData{MachineUUID: mUUID}
	isProvisionedStmt, err := st.Prepare(`
SELECT   &instanceData.instance_id
FROM     machine_cloud_instance
WHERE    machine_uuid = $instanceData.machine_uuid`, instanceDataQuery)
	if err != nil {
		return errors.Trace(err)
	}

	queryLXDProfile := lxdProfile{MachineUUID: mUUID}
	retrievePreviousProfilesStmt, err := st.Prepare(`
SELECT   name AS &lxdProfile.name
FROM     machine_lxd_profile
WHERE    machine_uuid = $lxdProfile.machine_uuid
ORDER BY array_index ASC`, queryLXDProfile)
	if err != nil {
		return errors.Trace(err)
	}

	removePreviousProfilesStmt, err := st.Prepare(
		`DELETE FROM machine_lxd_profile WHERE machine_uuid = $machineUUID.uuid`,
		machineUUID{},
	)
	if err != nil {
		return errors.Trace(err)
	}

	setLXDProfileStmt, err := st.Prepare(`
INSERT INTO machine_lxd_profile (*)
VALUES      ($lxdProfile.*)`, lxdProfile{})
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var machineExists machineUUID
		err = tx.Query(ctx, checkMachineExistsStmt, queryMachineUUID).Get(&machineExists)
		if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		} else if err != nil {
			return internalerrors.Errorf("checking if machine %q exists: %w", mUUID, err)
		}

		// Check if the machine is provisioned
		var instanceData instanceData
		err = tx.Query(ctx, isProvisionedStmt, instanceDataQuery).Get(&instanceData)
		if errors.Is(err, sqlair.ErrNoRows) {
			return internalerrors.Errorf("machine %q: %w", mUUID, machineerrors.NotProvisioned)
		}
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return internalerrors.Errorf("checking machine cloud instance for machine %q: %w", mUUID, err)
		}
		// Retrieve the existing profiles to check if the input is the exactly
		// the same as the existing ones and in that case no insert is needed.
		var existingProfiles []lxdProfile
		err = tx.Query(ctx, retrievePreviousProfilesStmt, queryLXDProfile).GetAll(&existingProfiles)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return internalerrors.Errorf("retrieving previous profiles for machine %q: %w", mUUID, err)
		}
		// Compare the input with the existing profiles.
		somethingToInsert := len(existingProfiles) != len(profileNames)
		// Only compare the order of the profiles if the input size is the same
		// as the existing profiles.
		if !somethingToInsert {
			for i, profile := range existingProfiles {
				if profileNames[i] != profile.Name {
					somethingToInsert = true
					fmt.Printf("breaking, profile: %v\n", profile)
					break
				}
			}
		}
		// No error to return and nothing to insert.
		if !somethingToInsert {
			fmt.Printf("existing profiles: %v\n", existingProfiles)
			return nil
		}

		// Make sure to clean up any existing profiles on the given machine.
		if err := tx.Query(ctx, removePreviousProfilesStmt, queryMachineUUID).Run(); err != nil {
			return internalerrors.Errorf("remove previous profiles for machine %q: %w", mUUID, err)
		}

		profilesToInsert := make([]lxdProfile, 0, len(profileNames))
		// Insert the profiles in the order they are provided.
		for index, profileName := range profileNames {
			profilesToInsert = append(profilesToInsert,
				lxdProfile{
					Name:        profileName,
					MachineUUID: mUUID,
					Index:       index,
				},
			)
		}
		if err := tx.Query(ctx, setLXDProfileStmt, profilesToInsert).Run(); err != nil {
			return internalerrors.Errorf("setting lxd profiles %v for machine %q: %w", profileNames, mUUID, err)
		}
		return nil
	})
}

// NamespaceForWatchMachineCloudInstance returns the namespace for watching
// machine cloud instance changes.
func (st *State) NamespaceForWatchMachineCloudInstance() string {
	return "machine_cloud_instance"
}

// NamespaceForWatchMachineLXDProfiles returns the namespace for watching
// machine LXD profile changes.
func (st *State) NamespaceForWatchMachineLXDProfiles() string {
	return "machine_lxd_profile"
}

// NamespaceForWatchMachineReboot returns the namespace string used for
// tracking machine reboot events in the model.
func (st *State) NamespaceForWatchMachineReboot() string {
	return "machine_requires_reboot"
}
