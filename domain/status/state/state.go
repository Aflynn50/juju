// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	domainlife "github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// State represents the persistence layer for the statuses of applications and units.
type State struct {
	*domain.StateBase
	clock  clock.Clock
	logger logger.Logger
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory, clock clock.Clock, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		clock:     clock,
		logger:    logger,
	}
}

// GetAllRelationStatuses returns all the relation statuses of the given model.
func (st *State) GetAllRelationStatuses(ctx context.Context) (map[corerelation.UUID]status.StatusInfo[status.RelationStatusType],
	error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &relationStatus.*
FROM   relation_status`, relationStatus{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var statuses []relationStatus
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt).GetAll(&statuses)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("getting all relations statuses: %w", err)
	}
	relationsStatuses := make(map[corerelation.UUID]status.StatusInfo[status.RelationStatusType], len(statuses))
	for _, relStatus := range statuses {
		statusType, err := status.DecodeRelationStatus(relStatus.StatusID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		relationsStatuses[relStatus.RelationUUID] = status.StatusInfo[status.RelationStatusType]{
			Status:  statusType,
			Message: relStatus.Reason,
			Since:   relStatus.Since,
		}
	}

	return relationsStatuses, errors.Capture(err)
}

// GetApplicationIDByName returns the application ID for the named application.
// If no application is found, an error satisfying
// [statuserrors.ApplicationNotFound] is returned.
func (st *State) GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var id coreapplication.ID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		id, err = st.lookupApplication(ctx, tx, name)
		return err
	}); err != nil {
		return "", errors.Capture(err)
	}
	return id, nil
}

// GetApplicationIDAndNameByUnitName returns the application ID and name for the
// named unit.
//
// Returns an error satisfying [statuserrors.UnitNotFound] if the unit
// doesn't exist.
func (st *State) GetApplicationIDAndNameByUnitName(
	ctx context.Context,
	name coreunit.Name,
) (coreapplication.ID, string, error) {
	db, err := st.DB()
	if err != nil {
		return "", "", errors.Capture(err)
	}

	unit := unitName{Name: name}
	queryUnit := `
SELECT a.uuid AS &applicationIDAndName.uuid,
       a.name AS &applicationIDAndName.name
FROM unit u
JOIN application a ON a.uuid = u.application_uuid
WHERE u.name = $unitName.name;
`
	query, err := st.Prepare(queryUnit, applicationIDAndName{}, unit)
	if err != nil {
		return "", "", errors.Errorf("preparing query for unit %q: %w", name, err)
	}

	var app applicationIDAndName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, unit).Get(&app)
		if errors.Is(err, sqlair.ErrNoRows) {
			return statuserrors.UnitNotFound
		}
		return err
	})
	if err != nil {
		return "", "", errors.Errorf("querying unit %q application id: %w", name, err)
	}
	return app.ID, app.Name, nil
}

// GetApplicationStatus looks up the status of the specified application,
// returning an error satisfying [statuserrors.ApplicationNotFound] if the
// application is not found.
func (st *State) GetApplicationStatus(ctx context.Context, appID coreapplication.ID) (status.StatusInfo[status.WorkloadStatusType], error) {
	db, err := st.DB()
	if err != nil {
		return status.StatusInfo[status.WorkloadStatusType]{}, errors.Capture(err)
	}

	identID := applicationID{ID: appID}
	query, err := st.Prepare(`
SELECT &statusInfo.*
FROM application_status
WHERE application_uuid = $applicationID.uuid;
`, identID, statusInfo{})
	if err != nil {
		return status.StatusInfo[status.WorkloadStatusType]{}, errors.Capture(err)
	}
	var sts statusInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkApplicationNotDead(ctx, tx, identID); err != nil {
			return errors.Capture(err)
		}
		if err := tx.Query(ctx, query, identID).Get(&sts); errors.Is(err, sqlair.ErrNoRows) {
			// If the application status is not set, we should return a default
			// unset status. It's then it's up to the caller to either return that
			// information or use derive the status from the units.
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return status.StatusInfo[status.WorkloadStatusType]{}, errors.Capture(err)
	}

	statusType, err := status.DecodeWorkloadStatus(sts.StatusID)
	if err != nil {
		return status.StatusInfo[status.WorkloadStatusType]{}, errors.Capture(err)
	}

	return status.StatusInfo[status.WorkloadStatusType]{
		Status:  statusType,
		Message: sts.Message,
		Data:    sts.Data,
		Since:   sts.UpdatedAt,
	}, nil
}

// SetApplicationStatus saves the given application status, overwriting any
// current status data. If returns an error satisfying
// [statuserrors.ApplicationNotFound] if the application doesn't exist.
func (st *State) SetApplicationStatus(
	ctx context.Context,
	applicationID coreapplication.ID,
	sts status.StatusInfo[status.WorkloadStatusType],
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	statusID, err := status.EncodeWorkloadStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := applicationStatusInfo{
		ApplicationID: applicationID,
		StatusID:      statusID,
		Message:       sts.Message,
		Data:          sts.Data,
		UpdatedAt:     sts.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO application_status (*) VALUES ($applicationStatusInfo.*)
ON CONFLICT(application_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at,
    data = excluded.data;
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, statusInfo).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
			return errors.Errorf("%w: %q", statuserrors.ApplicationNotFound, applicationID)
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("updating application status for %q: %w", applicationID, err)
	}
	return nil
}

// GetRelationStatus gets the status of the given relation. It returns an error
// satisfying [statuserrors.RelationNotFound] if the relation doesn't exist.
func (st *State) GetRelationStatus(
	ctx context.Context,
	uuid corerelation.UUID,
) (status.StatusInfo[status.RelationStatusType], error) {
	empty := status.StatusInfo[status.RelationStatusType]{}
	db, err := st.DB()
	if err != nil {
		return empty, errors.Capture(err)
	}

	id := relationUUID{
		RelationUUID: uuid,
	}

	stmt, err := st.Prepare(`
SELECT &relationStatus.*
FROM   relation_status
WHERE  relation_uuid = $relationUUID.relation_uuid
`, id, relationStatus{})
	if err != nil {
		return empty, errors.Capture(err)
	}

	var sts relationStatus
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, id).Get(&sts)
		if errors.Is(err, sqlair.ErrNoRows) {
			return statuserrors.RelationNotFound
		} else if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return empty, errors.Errorf("getting all relations statuses: %w", err)
	}
	statusType, err := status.DecodeRelationStatus(sts.StatusID)
	if err != nil {
		return empty, errors.Capture(err)
	}
	return status.StatusInfo[status.RelationStatusType]{
		Status:  statusType,
		Message: sts.Reason,
		Since:   sts.Since,
	}, nil
}

// SetRelationStatus saves the given relation status, overwriting any current
// status data. It returns an error satisfying [statuserrors.RelationNotFound]
// if the relation doesn't exist.
func (st *State) SetRelationStatus(
	ctx context.Context,
	relationUUID corerelation.UUID,
	sts status.StatusInfo[status.RelationStatusType],
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	statusID, err := status.EncodeRelationStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := relationStatus{
		RelationUUID: relationUUID,
		StatusID:     statusID,
		Reason:       sts.Message,
		Since:        sts.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO relation_status (*) 
VALUES ($relationStatus.*)
ON CONFLICT(relation_uuid) DO UPDATE SET
    relation_status_type_id = excluded.relation_status_type_id,
    suspended_reason = excluded.suspended_reason,
    updated_at = excluded.updated_at;
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, statusInfo).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
			return errors.Errorf("%w: %q", statuserrors.RelationNotFound, relationUUID)
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("updating relation status for %q: %w", relationUUID, err)
	}
	return nil
}

// GetUnitUUIDByName returns the UUID for the named unit, returning an error
// satisfying [statuserrors.UnitNotFound] if the unit doesn't exist.
func (st *State) GetUnitUUIDByName(ctx context.Context, name coreunit.Name) (coreunit.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}
	unitName := unitName{Name: name}

	query, err := st.Prepare(`
SELECT &unitUUID.*
FROM unit
WHERE name = $unitName.name
`, unitUUID{}, unitName)
	if err != nil {
		return "", errors.Errorf("preparing query: %w", err)
	}

	unitUUID := unitUUID{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, query, unitName).Get(&unitUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unit %q not found", name).Add(statuserrors.UnitNotFound)
		}
		return err
	})
	if err != nil {
		return "", errors.Errorf("querying unit name: %w", err)
	}

	return unitUUID.UnitUUID, nil
}

// GetUnitAgentStatus returns the agent status of the specified unit, returning:
// - an error satisfying [statuserrors.UnitNotFound] if the unit doesn't exist or;
// - an error satisfying [statuserrors.UnitIsDead] if the unit is dead or;
// - an error satisfying [statuserrors.UnitStatusNotFound] if the status is not set.
func (st *State) GetUnitAgentStatus(ctx context.Context, uuid coreunit.UUID) (status.UnitStatusInfo[status.UnitAgentStatusType], error) {
	db, err := st.DB()
	if err != nil {
		return status.UnitStatusInfo[status.UnitAgentStatusType]{}, errors.Capture(err)
	}

	unitUUID := unitUUID{UnitUUID: uuid}
	getUnitStatusStmt, err := st.Prepare(`
SELECT &unitPresentStatusInfo.* FROM v_unit_agent_status WHERE unit_uuid = $unitUUID.uuid
`, unitPresentStatusInfo{}, unitUUID)
	if err != nil {
		return status.UnitStatusInfo[status.UnitAgentStatusType]{}, errors.Capture(err)
	}

	var unitStatusInfo unitPresentStatusInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkUnitNotDead(ctx, tx, unitUUID)
		if err != nil {
			return errors.Errorf("checking unit %q exists: %w", uuid, err)
		}

		err = tx.Query(ctx, getUnitStatusStmt, unitUUID).Get(&unitStatusInfo)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("agent status for unit %q not found", unitUUID).Add(statuserrors.UnitStatusNotFound)
		}
		return err
	})
	if err != nil {
		return status.UnitStatusInfo[status.UnitAgentStatusType]{}, errors.Errorf("getting agent status for unit %q: %w", unitUUID, err)
	}

	statusID, err := status.DecodeAgentStatus(unitStatusInfo.StatusID)
	if err != nil {
		return status.UnitStatusInfo[status.UnitAgentStatusType]{}, errors.Errorf("decoding agent status ID for unit %q: %w", unitUUID, err)
	}

	return status.UnitStatusInfo[status.UnitAgentStatusType]{
		StatusInfo: status.StatusInfo[status.UnitAgentStatusType]{
			Status:  statusID,
			Message: unitStatusInfo.Message,
			Data:    unitStatusInfo.Data,
			Since:   unitStatusInfo.UpdatedAt,
		},
		Present: unitStatusInfo.Present,
	}, nil
}

// SetUnitAgentStatus updates the agent status of the specified unit,
// returning an error satisfying [statuserrors.UnitNotFound] if the unit
// doesn't exist.
func (st *State) SetUnitAgentStatus(ctx context.Context, unitUUID coreunit.UUID, status status.StatusInfo[status.UnitAgentStatusType]) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setUnitAgentStatus(ctx, tx, unitUUID, status)
	})
	if err != nil {
		return errors.Errorf("setting agent status for unit %q: %w", unitUUID, err)
	}
	return nil
}

// GetUnitWorkloadStatus returns the workload status of the specified unit, returning:
// - an error satisfying [statuserrors.UnitNotFound] if the unit doesn't exist or;
// - an error satisfying [statuserrors.UnitIsDead] if the unit is dead or;
// - an error satisfying [statuserrors.UnitStatusNotFound] if the status is not set.
func (st *State) GetUnitWorkloadStatus(ctx context.Context, uuid coreunit.UUID) (status.UnitStatusInfo[status.WorkloadStatusType], error) {
	db, err := st.DB()
	if err != nil {
		return status.UnitStatusInfo[status.WorkloadStatusType]{}, errors.Capture(err)
	}

	unitUUID := unitUUID{UnitUUID: uuid}
	getUnitStatusStmt, err := st.Prepare(`
SELECT &unitPresentStatusInfo.* FROM v_unit_workload_status WHERE unit_uuid = $unitUUID.uuid
`, unitPresentStatusInfo{}, unitUUID)
	if err != nil {
		return status.UnitStatusInfo[status.WorkloadStatusType]{}, errors.Capture(err)
	}

	var unitStatusInfo unitPresentStatusInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkUnitNotDead(ctx, tx, unitUUID)
		if err != nil {
			return errors.Errorf("checking unit %q exists: %w", uuid, err)
		}

		err = tx.Query(ctx, getUnitStatusStmt, unitUUID).Get(&unitStatusInfo)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("workload status for unit %q not found", unitUUID).Add(statuserrors.UnitStatusNotFound)
		}
		return err
	})
	if err != nil {
		return status.UnitStatusInfo[status.WorkloadStatusType]{}, errors.Errorf("getting workload status for unit %q: %w", unitUUID, err)
	}

	statusID, err := status.DecodeWorkloadStatus(unitStatusInfo.StatusID)
	if err != nil {
		return status.UnitStatusInfo[status.WorkloadStatusType]{}, errors.Errorf("decoding workload status ID for unit %q: %w", unitUUID, err)
	}

	return status.UnitStatusInfo[status.WorkloadStatusType]{
		StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
			Status:  statusID,
			Message: unitStatusInfo.Message,
			Data:    unitStatusInfo.Data,
			Since:   unitStatusInfo.UpdatedAt,
		},
		Present: unitStatusInfo.Present,
	}, nil
}

// SetUnitWorkloadStatus updates the workload status of the specified unit,
// returning an error satisfying [statuserrors.UnitNotFound] if the unit
// doesn't exist.
func (st *State) SetUnitWorkloadStatus(ctx context.Context, unitUUID coreunit.UUID, status status.StatusInfo[status.WorkloadStatusType]) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setUnitWorkloadStatus(ctx, tx, unitUUID, status)
	})
	if err != nil {
		return errors.Errorf("setting workload status for unit %q: %w", unitUUID, err)
	}
	return nil
}

// GetUnitCloudContainerStatus returns the cloud container status of the specified
// unit. It returns;
// - an error satisfying [statuserrors.UnitNotFound] if the unit doesn't exist or;
// - an error satisfying [statuserrors.UnitIsDead] if the unit is dead or;
func (st *State) GetUnitCloudContainerStatus(ctx context.Context, uuid coreunit.UUID) (status.StatusInfo[status.CloudContainerStatusType], error) {
	db, err := st.DB()
	if err != nil {
		return status.StatusInfo[status.CloudContainerStatusType]{}, errors.Capture(err)
	}

	unitUUID := unitUUID{UnitUUID: uuid}
	getUnitStatusStmt, err := st.Prepare(`
SELECT &statusInfo.*
FROM   k8s_pod_status
WHERE  unit_uuid = $unitUUID.uuid
	`, statusInfo{}, unitUUID)
	if err != nil {
		return status.StatusInfo[status.CloudContainerStatusType]{}, errors.Capture(err)
	}

	var containerStatusInfo statusInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkUnitNotDead(ctx, tx, unitUUID)
		if err != nil {
			return errors.Errorf("checking unit %q exists: %w", uuid, err)
		}

		if err := tx.Query(ctx, getUnitStatusStmt, unitUUID).Get(&containerStatusInfo); errors.Is(err, sql.ErrNoRows) {
			// If the unit has not container status, this is fine. Container status
			// is optional since non-CAAS models do no have containers. In this
			// case, return a default unset status
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return status.StatusInfo[status.CloudContainerStatusType]{}, errors.Errorf("getting cloud container status for unit %q: %w", unitUUID, err)
	}

	statusID, err := status.DecodeCloudContainerStatus(containerStatusInfo.StatusID)
	if err != nil {
		return status.StatusInfo[status.CloudContainerStatusType]{}, errors.Errorf("decoding cloud container status ID for unit %q: %w", uuid, err)
	}
	return status.StatusInfo[status.CloudContainerStatusType]{
		Status:  statusID,
		Message: containerStatusInfo.Message,
		Data:    containerStatusInfo.Data,
		Since:   containerStatusInfo.UpdatedAt,
	}, nil
}

// GetUnitWorkloadStatusesForApplication returns the workload statuses for all units
// of the specified application, returning:
//   - an error satisfying [statuserrors.ApplicationNotFound] if the application
//     doesn't exist or;
//   - error satisfying [statuserrors.ApplicationIsDead] if the application
//     is dead.
func (st *State) GetUnitWorkloadStatusesForApplication(
	ctx context.Context, appID coreapplication.ID,
) (status.UnitWorkloadStatuses, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}
	ident := applicationID{ID: appID}

	var unitStatuses status.UnitWorkloadStatuses
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		unitStatuses, err = st.getUnitWorkloadStatusesForApplication(ctx, tx, ident)
		return err
	})
	if err != nil {
		return nil, errors.Errorf("getting workload statuses for application %q: %w", appID, err)
	}
	return unitStatuses, nil
}

// GetAllFullUnitStatusesForApplication returns the workload, agent and container
// statuses for all units of the specified application, returning:
//   - an error satisfying [statuserrors.ApplicationNotFound] if the application
//     doesn't exist or;
//   - an error satisfying [statuserrors.ApplicationIsDead] if the application
//     is dead.
func (st *State) GetAllFullUnitStatusesForApplication(
	ctx context.Context, appID coreapplication.ID,
) (
	status.FullUnitStatuses, error,
) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}
	ident := applicationID{ID: appID}

	stmt, err := st.Prepare(`
SELECT &fullUnitStatus.*
FROM v_full_unit_status
WHERE application_uuid = $applicationID.uuid
`, fullUnitStatus{}, ident)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var fullUnitStatuses []fullUnitStatus
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkApplicationNotDead(ctx, tx, ident)
		if err != nil {
			return errors.Errorf("checking application not dead: %w", err)
		}
		err = tx.Query(ctx, stmt, ident).GetAll(&fullUnitStatuses)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("getting unit statuses for application %q: %w", appID, err)
	}
	ret := make(status.FullUnitStatuses, len(fullUnitStatuses))
	for _, s := range fullUnitStatuses {
		if s.WorkloadStatusID == nil {
			return nil, errors.Errorf("workload status for unit %q not found", s.UnitName).Add(statuserrors.UnitStatusNotFound)
		}
		if s.AgentStatusID == nil {
			return nil, errors.Errorf("agent status for unit %q not found", s.UnitName).Add(statuserrors.UnitStatusNotFound)
		}
		workloadStatusID, err := status.DecodeWorkloadStatus(*s.WorkloadStatusID)
		if err != nil {
			return nil, errors.Errorf("decoding workload status ID for unit %q: %w", s.UnitName, err)
		}
		agentStatusID, err := status.DecodeAgentStatus(*s.AgentStatusID)
		if err != nil {
			return nil, errors.Errorf("decoding agent status ID for unit %q: %w", s.UnitName, err)
		}

		// Container status is optional.
		containerStatus := status.StatusInfo[status.CloudContainerStatusType]{
			Status: status.CloudContainerStatusUnset,
		}
		if s.ContainerStatusID != nil {
			containerStatusID, err := status.DecodeCloudContainerStatus(*s.ContainerStatusID)
			if err != nil {
				return nil, errors.Errorf("decoding cloud container status ID for unit %q: %w", s.UnitName, err)
			}
			containerStatus = status.StatusInfo[status.CloudContainerStatusType]{
				Status:  containerStatusID,
				Message: s.ContainerMessage,
				Data:    s.ContainerData,
				Since:   s.ContainerUpdatedAt,
			}
		}

		ret[s.UnitName] = status.FullUnitStatus{
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  workloadStatusID,
				Message: s.WorkloadMessage,
				Data:    s.WorkloadData,
				Since:   s.WorkloadUpdatedAt,
			},
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  agentStatusID,
				Message: s.AgentMessage,
				Data:    s.AgentData,
				Since:   s.AgentUpdatedAt,
			},
			ContainerStatus: containerStatus,
			Present:         s.Present,
		}
	}
	return ret, nil
}

// GetAllUnitWorkloadAgentStatuses retrieves the presence, workload status, and agent status
// of every unit in the model. Returns an error satisfying [statuserrors.UnitStatusNotFound]
// if any units do not have statuses.
func (st *State) GetAllUnitWorkloadAgentStatuses(ctx context.Context) (status.UnitWorkloadAgentStatuses, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	query, err := st.Prepare(`SELECT &workloadAgentStatus.* FROM v_unit_workload_agent_status`, workloadAgentStatus{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var statuses []workloadAgentStatus
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query).GetAll(&statuses)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	ret := make(status.UnitWorkloadAgentStatuses, len(statuses))
	for _, s := range statuses {
		if s.WorkloadStatusID == nil {
			return nil, errors.Errorf("workload status for unit %q not found", s.UnitName).Add(statuserrors.UnitStatusNotFound)
		}
		if s.AgentStatusID == nil {
			return nil, errors.Errorf("agent status for unit %q not found", s.UnitName).Add(statuserrors.UnitStatusNotFound)
		}
		workloadStatusID, err := status.DecodeWorkloadStatus(*s.WorkloadStatusID)
		if err != nil {
			return nil, errors.Errorf("decoding workload status for unit %q: %w", s.UnitName, err)
		}
		agentStatusID, err := status.DecodeAgentStatus(*s.AgentStatusID)
		if err != nil {
			return nil, errors.Errorf("decoding workload status for unit %q: %w", s.UnitName, err)
		}

		ret[s.UnitName] = status.UnitWorkloadAgentStatus{
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  workloadStatusID,
				Message: s.WorkloadMessage,
				Data:    s.WorkloadData,
				Since:   s.WorkloadUpdatedAt,
			},
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  agentStatusID,
				Message: s.AgentMessage,
				Data:    s.AgentData,
				Since:   s.AgentUpdatedAt,
			},
			Present: s.Present,
		}
	}
	return ret, nil
}

// GetAllApplicationStatuses returns the statuses of all the applications in the model,
// indexed by application name, if they have a status set.
func (st *State) GetAllApplicationStatuses(ctx context.Context) (map[string]status.StatusInfo[status.WorkloadStatusType], error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	query, err := st.Prepare(`
SELECT &applicationNameStatusInfo.*
FROM application_status
JOIN application ON application.uuid = application_status.application_uuid
`, applicationNameStatusInfo{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var statuses []applicationNameStatusInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query).GetAll(&statuses)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	ret := make(map[string]status.StatusInfo[status.WorkloadStatusType], len(statuses))
	for _, s := range statuses {
		statusType, err := status.DecodeWorkloadStatus(s.StatusID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		ret[s.ApplicationName] = status.StatusInfo[status.WorkloadStatusType]{
			Status:  statusType,
			Message: s.Message,
			Data:    s.Data,
			Since:   s.UpdatedAt,
		}
	}
	return ret, nil
}

// SetUnitPresence marks the presence of the specified unit, returning an error
// satisfying [statuserrors.UnitNotFound] if the unit doesn't exist.
// The unit life is not considered when making this query.
func (st *State) SetUnitPresence(ctx context.Context, name coreunit.Name) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	unit := unitName{Name: name}
	var uuid unitUUID

	queryUnit := `SELECT &unitUUID.uuid FROM unit WHERE name = $unitName.name;`
	queryUnitStmt, err := st.Prepare(queryUnit, unit, uuid)
	if err != nil {
		return errors.Capture(err)
	}

	recordUnit := `
INSERT INTO unit_agent_presence (*) VALUES ($unitPresence.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
	last_seen = excluded.last_seen;
`
	var presence unitPresence
	recordUnitStmt, err := st.Prepare(recordUnit, presence)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, queryUnitStmt, unit).Get(&uuid); errors.Is(err, sql.ErrNoRows) {
			return statuserrors.UnitNotFound
		} else if err != nil {
			return errors.Capture(err)
		}

		presence := unitPresence{
			UnitUUID: uuid.UnitUUID,
			LastSeen: st.clock.Now(),
		}

		if err := tx.Query(ctx, recordUnitStmt, presence).Run(); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// DeleteUnitPresence removes the presence of the specified unit. If the
// unit isn't found it ignores the error.
// The unit life is not considered when making this query.
func (st *State) DeleteUnitPresence(ctx context.Context, name coreunit.Name) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	unit := unitName{Name: name}

	deleteStmt, err := st.Prepare(`
DELETE FROM unit_agent_presence
WHERE unit_uuid = (
	SELECT uuid FROM unit
	WHERE name = $unitName.name
);
`, unit)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, deleteStmt, unit).Run(); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})

	return errors.Capture(err)
}

// lookupApplication looks up the application by name and returns the
// application.ID.
// If no application is found, an error satisfying
// [statuserrors.ApplicationNotFound] is returned.
func (st *State) lookupApplication(ctx context.Context, tx *sqlair.TX, name string) (coreapplication.ID, error) {
	app := applicationIDAndName{Name: name}
	queryApplicationStmt, err := st.Prepare(`
SELECT uuid AS &applicationIDAndName.uuid
FROM application
WHERE name = $applicationIDAndName.name
`, app)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = tx.Query(ctx, queryApplicationStmt, app).Get(&app)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("%w: %s", statuserrors.ApplicationNotFound, name)
	} else if err != nil {
		return "", errors.Errorf("looking up UUID for application %q: %w", name, err)
	}
	return app.ID, nil
}

// checkApplicationNotDead checks if the application exists and is not dead. It's
// possible to access alive and dying applications, but not dead ones.
//   - If the application is dead, [statuserrors.ApplicationIsDead] is returned.
//   - If the application is not found, [statuserrors.ApplicationNotFound]
//     is returned.
func (st *State) checkApplicationNotDead(ctx context.Context, tx *sqlair.TX, ident applicationID) error {
	type life struct {
		LifeID domainlife.Life `db:"life_id"`
	}

	query := `
SELECT &life.*
FROM application
WHERE uuid = $applicationID.uuid;
`
	stmt, err := st.Prepare(query, ident, life{})
	if err != nil {
		return errors.Errorf("preparing query for application %q: %w", ident.ID, err)
	}

	var result life
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return statuserrors.ApplicationNotFound
	} else if err != nil {
		return errors.Errorf("checking application %q exists: %w", ident.ID, err)
	}

	switch result.LifeID {
	case domainlife.Dead:
		return statuserrors.ApplicationIsDead
	default:
		return nil
	}
}

// checkUnitNotDead checks if the unit exists and is not dead. It's possible to
// access alive and dying units, but not dead ones:
// - If the unit is not found, [statuserrors.UnitNotFound] is returned.
// - If the unit is dead, [statuserrors.UnitIsDead] is returned.
func (st *State) checkUnitNotDead(ctx context.Context, tx *sqlair.TX, ident unitUUID) error {
	type life struct {
		LifeID domainlife.Life `db:"life_id"`
	}

	query := `
SELECT &life.*
FROM unit
WHERE uuid = $unitUUID.uuid;
`
	stmt, err := st.Prepare(query, ident, life{})
	if err != nil {
		return errors.Errorf("preparing query for unit %q: %w", ident.UnitUUID, err)
	}

	var result life
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return statuserrors.UnitNotFound
	} else if err != nil {
		return errors.Errorf("checking unit %q exists: %w", ident.UnitUUID, err)
	}

	switch result.LifeID {
	case domainlife.Dead:
		return statuserrors.UnitIsDead
	default:
		return nil
	}
}

func (st *State) getUnitWorkloadStatusesForApplication(
	ctx context.Context, tx *sqlair.TX, ident applicationID,
) (
	status.UnitWorkloadStatuses, error,
) {
	getUnitStatusesStmt, err := st.Prepare(`
SELECT &statusInfoAndUnitNameAndPresence.*
FROM   v_unit_workload_status
WHERE  application_uuid = $applicationID.uuid
`, statusInfoAndUnitNameAndPresence{}, ident)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var unitStatuses []statusInfoAndUnitNameAndPresence
	err = st.checkApplicationNotDead(ctx, tx, ident)
	if err != nil {
		return nil, errors.Capture(err)
	}
	err = tx.Query(ctx, getUnitStatusesStmt, ident).GetAll(&unitStatuses)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	statuses := make(status.UnitWorkloadStatuses, len(unitStatuses))
	for _, unitStatus := range unitStatuses {
		statusID, err := status.DecodeWorkloadStatus(unitStatus.StatusID)
		if err != nil {
			return nil, errors.Errorf("decoding workload status ID for unit %q: %w", unitStatus.UnitName, err)
		}
		statuses[unitStatus.UnitName] = status.UnitStatusInfo[status.WorkloadStatusType]{
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status:  statusID,
				Message: unitStatus.Message,
				Data:    unitStatus.Data,
				Since:   unitStatus.UpdatedAt,
			},
			Present: unitStatus.Present,
		}
	}

	return statuses, nil
}

// setUnitAgentStatus saves the given unit agent status, overwriting any
// current status data. If returns an error satisfying
// [statuserrors.UnitNotFound] if the unit doesn't exist.
func (st *State) setUnitAgentStatus(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	sts status.StatusInfo[status.UnitAgentStatusType],
) error {
	statusID, err := status.EncodeAgentStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID,
		StatusID:  statusID,
		Message:   sts.Message,
		Data:      sts.Data,
		UpdatedAt: sts.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO unit_agent_status (*) VALUES ($unitStatusInfo.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at,
    data = excluded.data;
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, statusInfo).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
		return errors.Errorf("%w: %q", statuserrors.UnitNotFound, unitUUID)
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// setUnitWorkloadStatus saves the given unit workload status, overwriting any
// current status data. If returns an error satisfying
// [statuserrors.UnitNotFound] if the unit doesn't exist.
func (st *State) setUnitWorkloadStatus(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	sts status.StatusInfo[status.WorkloadStatusType],
) error {
	statusID, err := status.EncodeWorkloadStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID,
		StatusID:  statusID,
		Message:   sts.Message,
		Data:      sts.Data,
		UpdatedAt: sts.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO unit_workload_status (*) VALUES ($unitStatusInfo.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at,
    data = excluded.data;
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, statusInfo).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
		return errors.Errorf("%w: %q", statuserrors.UnitNotFound, unitUUID)
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// setCloudContainerStatus saves the given cloud container status, overwriting
// any current status data. If returns an error satisfying
// [statuserrors.UnitNotFound] if the unit doesn't exist.
func (st *State) setCloudContainerStatus(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	sts status.StatusInfo[status.CloudContainerStatusType],
) error {
	statusID, err := status.EncodeCloudContainerStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID,
		StatusID:  statusID,
		Message:   sts.Message,
		Data:      sts.Data,
		UpdatedAt: sts.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO k8s_pod_status (*) VALUES ($unitStatusInfo.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at,
    data = excluded.data;
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, statusInfo).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
		return errors.Errorf("%w: %q", statuserrors.UnitNotFound, unitUUID)
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}
