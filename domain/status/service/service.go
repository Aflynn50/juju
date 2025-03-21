// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/clock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/leadership"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/statushistory"
)

var (
	applicationNamespace  = statushistory.Namespace{Name: "application"}
	unitAgentNamespace    = statushistory.Namespace{Name: "unit-agent"}
	unitWorkloadNamespace = statushistory.Namespace{Name: "unit-workload"}
)

// State describes retrieval and persistence methods for the statuses of applications
// and units.
type State interface {
	// GetApplicationIDByName returns the application ID for the named application.
	// If no application is found, an error satisfying
	// [statuserrors.ApplicationNotFound] is returned.
	GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error)

	// GetApplicationIDAndNameByUnitName returns the application ID and name for
	// the named unit, returning an error satisfying
	// [statuserrors.UnitNotFound] if the unit doesn't exist.
	GetApplicationIDAndNameByUnitName(ctx context.Context, name coreunit.Name) (coreapplication.ID, string, error)

	// GetApplicationStatus looks up the status of the specified application,
	// returning an error satisfying [statuserrors.ApplicationNotFound] if the
	// application is not found.
	GetApplicationStatus(ctx context.Context, appID coreapplication.ID) (*status.StatusInfo[status.WorkloadStatusType], error)

	// SetApplicationStatus saves the given application status, overwriting any
	// current status data. If returns an error satisfying
	// [statuserrors.ApplicationNotFound] if the application doesn't exist.
	SetApplicationStatus(
		ctx context.Context,
		applicationID coreapplication.ID,
		status *status.StatusInfo[status.WorkloadStatusType],
	) error

	// GetUnitUUIDByName returns the UUID for the named unit, returning an
	// error satisfying [statuserrors.UnitNotFound] if the unit doesn't
	// exist.
	GetUnitUUIDByName(context.Context, coreunit.Name) (coreunit.UUID, error)

	// GetUnitWorkloadStatus returns the workload status of the specified unit,
	// returning:
	// - an error satisfying [statuserrors.UnitNotFound] if the unit
	//   doesn't exist or;
	// - an error satisfying [statuserrors.UnitStatusNotFound] if the
	//   status is not set.
	GetUnitWorkloadStatus(context.Context, coreunit.UUID) (*status.UnitStatusInfo[status.WorkloadStatusType], error)

	// SetUnitWorkloadStatus sets the workload status of the specified unit,
	// returning an error satisfying [statuserrors.UnitNotFound] if the
	// unit doesn't exist.
	SetUnitWorkloadStatus(context.Context, coreunit.UUID, *status.StatusInfo[status.WorkloadStatusType]) error

	// GetUnitCloudContainerStatus returns the cloud container status of the
	// specified unit. It returns;
	// - an error satisfying [statuserrors.UnitNotFound] if the unit
	//   doesn't exist or;
	// - an error satisfying [statuserrors.UnitStatusNotFound] if the
	//   status is not set.
	GetUnitCloudContainerStatus(context.Context, coreunit.UUID) (*status.StatusInfo[status.CloudContainerStatusType], error)

	// GetUnitWorkloadStatusesForApplication returns the workload statuses for
	// all units of the specified application, returning:
	//   - an error satisfying [statuserrors.ApplicationNotFound] if the
	//     application doesn't exist or;
	//   - error satisfying [statuserrors.ApplicationIsDead] if the
	//     application is dead.
	GetUnitWorkloadStatusesForApplication(context.Context, coreapplication.ID) (status.UnitWorkloadStatuses, error)

	// GetUnitWorkloadAndCloudContainerStatusesForApplication returns the workload statuses
	// and the cloud container statuses for all units of the specified application, returning:
	//   - an error satisfying [statuserrors.ApplicationNotFound] if the application
	//     doesn't exist or;
	//   - an error satisfying [statuserrors.ApplicationIsDead] if the application
	//     is dead.
	GetUnitWorkloadAndCloudContainerStatusesForApplication(
		context.Context, coreapplication.ID,
	) (status.UnitWorkloadStatuses, status.UnitCloudContainerStatuses, error)

	// GetUnitAgentStatus returns the workload status of the specified unit,
	// returning:
	// - an error satisfying [statuserrors.UnitNotFound] if the unit
	//   doesn't exist or;
	// - an error satisfying [statuserrors.UnitStatusNotFound] if the
	//   status is not set.
	GetUnitAgentStatus(context.Context, coreunit.UUID) (*status.UnitStatusInfo[status.UnitAgentStatusType], error)

	// SetUnitAgentStatus sets the workload status of the specified unit,
	// returning an error satisfying [statuserrors.UnitNotFound] if the
	// unit doesn't exist.
	SetUnitAgentStatus(context.Context, coreunit.UUID, *status.StatusInfo[status.UnitAgentStatusType]) error

	// GetAllFullUnitStatuses retrieves the presence, workload status, and agent status
	// of every unit in the model. Returns an error satisfying [statuserrors.UnitStatusNotFound]
	// if any units do not have statuses.
	GetAllFullUnitStatuses(context.Context) (status.FullUnitStatuses, error)

	// GetAllApplicationStatuses returns the statuses of all the applications in the model,
	// indexed by application name, if they have a status set.
	GetAllApplicationStatuses(context.Context) (map[string]status.StatusInfo[status.WorkloadStatusType], error)

	// SetUnitPresence marks the presence of the specified unit, returning an error
	// satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
	// The unit life is not considered when making this query.
	SetUnitPresence(ctx context.Context, name coreunit.Name) error

	// DeleteUnitPresence removes the presence of the specified unit. If the
	// unit isn't found it ignores the error.
	// The unit life is not considered when making this query.
	DeleteUnitPresence(ctx context.Context, name coreunit.Name) error
}

// Service provides the API for working with the statuses of applications and units.
type Service struct {
	st            State
	leaderEnsurer leadership.Ensurer
	logger        logger.Logger
	clock         clock.Clock
	statusHistory StatusHistory
}

// NewService returns a new service reference wrapping the input state.
func NewService(
	st State,
	leaderEnsurer leadership.Ensurer,
	clock clock.Clock,
	logger logger.Logger,
	statusHistory StatusHistory,
) *Service {
	return &Service{
		st:            st,
		leaderEnsurer: leaderEnsurer,
		logger:        logger,
		clock:         clock,
		statusHistory: statusHistory,
	}
}

// SetApplicationStatus saves the given application status, overwriting any
// current status data. If returns an error satisfying
// [statuserrors.ApplicationNotFound] if the application doesn't exist.
func (s *Service) SetApplicationStatus(
	ctx context.Context,
	applicationID coreapplication.ID,
	status *corestatus.StatusInfo,
) error {
	if err := applicationID.Validate(); err != nil {
		return errors.Errorf("application ID: %w", err)
	}

	if status == nil {
		return nil
	}

	// Ensure we have a valid timestamp. It's optional at the API server level.
	// but it is a requirement for the database.
	if status.Since == nil {
		status.Since = ptr(s.clock.Now())
	}

	// This will implicitly verify that the status is valid.
	encodedStatus, err := encodeWorkloadStatus(status)
	if err != nil {
		return errors.Errorf("encoding workload status: %w", err)
	}

	if err := s.st.SetApplicationStatus(ctx, applicationID, encodedStatus); err != nil {
		return errors.Capture(err)
	}

	if err := s.statusHistory.RecordStatus(ctx, applicationNamespace.WithID(applicationID.String()), *status); err != nil {
		s.logger.Infof(ctx, "failed recording setting application status history: %v", err)
	}

	return nil
}

// SetApplicationStatusForUnitLeader sets the application status using the
// leader unit of the application. If the specified unit is not the leader of
// it's application and error satisfying [statuserrors.UnitNotLeader] is
// returned. If the unit is not found, an error satisfying
// [statuserrors.UnitNotFound] is returned.
func (s *Service) SetApplicationStatusForUnitLeader(
	ctx context.Context,
	unitName coreunit.Name,
	status *corestatus.StatusInfo,
) error {
	if err := unitName.Validate(); err != nil {
		return errors.Errorf("unit name: %w", err)
	}

	if status == nil {
		return nil
	}

	// Ensure we have a valid timestamp. It's optional at the API server level.
	// but it is a requirement for the database.
	if status.Since == nil {
		status.Since = ptr(s.clock.Now())
	}

	// This will implicitly verify that the status is valid.
	encodedStatus, err := encodeWorkloadStatus(status)
	if err != nil {
		return errors.Errorf("encoding workload status: %w", err)
	}

	// This returns the UnitNotFound if we can't find the application. This
	// is because we're doing a reverse lookup from the unit to the application.
	// We can't return the application not found, as we're not looking up the
	// application directly.
	appID, appName, err := s.st.GetApplicationIDAndNameByUnitName(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
	}

	err = s.leaderEnsurer.WithLeader(ctx, appName, unitName.String(), func(ctx context.Context) error {
		return s.st.SetApplicationStatus(ctx, appID, encodedStatus)
	})
	if errors.Is(err, corelease.ErrNotHeld) {
		return statuserrors.UnitNotLeader
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// GetApplicationDisplayStatus returns the display status of the specified application.
// The display status is equal to the application status if it is set, otherwise it is
// derived from the unit display statuses.
// If no application is found, an error satisfying [statuserrors.ApplicationNotFound]
// is returned.
func (s *Service) GetApplicationDisplayStatus(ctx context.Context, appID coreapplication.ID) (*corestatus.StatusInfo, error) {
	if err := appID.Validate(); err != nil {
		return nil, errors.Errorf("application ID: %w", err)
	}

	applicationStatus, err := s.st.GetApplicationStatus(ctx, appID)
	if err != nil {
		return nil, errors.Capture(err)
	} else if applicationStatus == nil {
		return nil, errors.Errorf("application has no status")
	}
	if applicationStatus.Status != status.WorkloadStatusUnset {
		return decodeApplicationStatus(applicationStatus)
	}

	workloadStatuses, cloudContainerStatuses, err := s.st.GetUnitWorkloadAndCloudContainerStatusesForApplication(ctx, appID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	derivedApplicationStatus, err := applicationDisplayStatusFromUnits(workloadStatuses, cloudContainerStatuses)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return derivedApplicationStatus, nil
}

// GetApplicationAndUnitStatusesForUnitWithLeader returns the display status
// of the application the specified unit belongs to, and the workload statuses
// of all the units that belong to that application, indexed by unit name.
// If the specified unit is not the leader of it's application and error satisfying
// [statuserrors.UnitNotLeader] is returned. If no application is found for the
// unit name, an error satisfying [statuserrors.ApplicationNotFound] is returned.
func (s *Service) GetApplicationAndUnitStatusesForUnitWithLeader(
	ctx context.Context,
	unitName coreunit.Name,
) (
	applicationDisplayStatus *corestatus.StatusInfo,
	unitWorkloadStatuses map[coreunit.Name]corestatus.StatusInfo,
	err error,
) {
	if err := unitName.Validate(); err != nil {
		return nil, nil, errors.Errorf("unit name: %w", err)
	}

	appName := unitName.Application()
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return nil, nil, errors.Errorf("getting application id: %w", err)
	}

	err = s.leaderEnsurer.WithLeader(ctx, appName, unitName.String(), func(ctx context.Context) error {
		applicationStatus, err := s.st.GetApplicationStatus(ctx, appID)
		if err != nil {
			return errors.Errorf("getting application status: %w", err)
		}
		workloadStatuses, cloudContainerStatuses, err := s.st.GetUnitWorkloadAndCloudContainerStatusesForApplication(ctx, appID)
		if err != nil {
			return errors.Errorf("getting unit workload and container statuses")
		}

		unitWorkloadStatuses, err = decodeUnitWorkloadStatuses(workloadStatuses)
		if err != nil {
			return errors.Errorf("decoding unit workload statuses: %w", err)
		}

		if applicationStatus.Status != status.WorkloadStatusUnset {
			applicationDisplayStatus, err = decodeApplicationStatus(applicationStatus)
			if err != nil {
				return errors.Errorf("decoding application workload status: %w", err)
			}
			return nil
		}

		applicationDisplayStatus, err = applicationDisplayStatusFromUnits(workloadStatuses, cloudContainerStatuses)
		if err != nil {
			return errors.Capture(err)
		}
		return nil

	})
	if errors.Is(err, corelease.ErrNotHeld) {
		return nil, nil, statuserrors.UnitNotLeader
	} else if err != nil {
		return nil, nil, errors.Capture(err)
	}
	return applicationDisplayStatus, unitWorkloadStatuses, nil
}

// SetUnitWorkloadStatus sets the workload status of the specified unit,
// returning an error satisfying [statuserrors.UnitNotFound] if the unit
// doesn't exist.
func (s *Service) SetUnitWorkloadStatus(ctx context.Context, unitName coreunit.Name, status *corestatus.StatusInfo) error {
	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	if status == nil {
		return nil
	}

	// Ensure we have a valid timestamp. It's optional at the API server level.
	// but it is a requirement for the database.
	if status.Since == nil {
		status.Since = ptr(s.clock.Now())
	}

	workloadStatus, err := encodeWorkloadStatus(status)
	if err != nil {
		return errors.Errorf("encoding workload status: %w", err)
	}
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
	}
	if err := s.st.SetUnitWorkloadStatus(ctx, unitUUID, workloadStatus); err != nil {
		return errors.Errorf("setting workload status: %w", err)
	}

	if err := s.statusHistory.RecordStatus(ctx, unitWorkloadNamespace.WithID(unitName.String()), *status); err != nil {
		s.logger.Infof(ctx, "failed recording setting workload status for unit %q: %v", unitName, err)
	}
	return nil
}

// GetUnitWorkloadStatus returns the workload status of the specified unit,
// returning an error satisfying [statuserrors.UnitNotFound] if the unit
// doesn't exist.
func (s *Service) GetUnitWorkloadStatus(ctx context.Context, unitName coreunit.Name) (*corestatus.StatusInfo, error) {
	if err := unitName.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	workloadStatus, err := s.st.GetUnitWorkloadStatus(ctx, unitUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return decodeUnitWorkloadStatus(workloadStatus)
}

// SetUnitAgentStatus sets the agent status of the specified unit,
// returning an error satisfying [statuserrors.UnitNotFound] if the unit
// doesn't exist.
func (s *Service) SetUnitAgentStatus(ctx context.Context, unitName coreunit.Name, status *corestatus.StatusInfo) error {
	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	if status == nil {
		return nil
	}

	// Ensure we have a valid timestamp. It's optional at the API server level.
	// but it is a requirement for the database.
	if status.Since == nil {
		status.Since = ptr(s.clock.Now())
	}

	// Encoding the status will handle invalid status values.
	switch status.Status {
	case corestatus.Error:
		if status.Message == "" {
			return errors.Errorf("setting status %q without message", status.Status)
		}
	case corestatus.Lost, corestatus.Allocating:
		return errors.Errorf("setting status %q is not allowed", status.Status)
	}

	agentStatus, err := encodeUnitAgentStatus(status)
	if err != nil {
		return errors.Errorf("encoding agent status: %w", err)
	}
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
	}
	if err := s.st.SetUnitAgentStatus(ctx, unitUUID, agentStatus); err != nil {
		return errors.Errorf("setting agent status: %w", err)
	}

	if err := s.statusHistory.RecordStatus(ctx, unitAgentNamespace.WithID(unitName.String()), *status); err != nil {
		s.logger.Infof(ctx, "failed recording setting agent status for unit %q: %v", unitName, err)
	}
	return nil
}

// GetUnitAgentStatus returns the agent status of the specified unit,
// returning an error satisfying [statuserrors.UnitNotFound] if the unit
// doesn't exist.
func (s *Service) GetUnitAgentStatus(ctx context.Context, unitName coreunit.Name) (*corestatus.StatusInfo, error) {
	if err := unitName.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	agentStatus, err := s.st.GetUnitAgentStatus(ctx, unitUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return decodeUnitAgentStatus(agentStatus)
}

// GetUnitWorkloadStatusesForApplication returns the workload statuses of all
// units in the specified application, indexed by unit name, returning an error
// satisfying [statuserrors.ApplicationNotFound] if the application doesn't
// exist.
func (s *Service) GetUnitWorkloadStatusesForApplication(ctx context.Context, appID coreapplication.ID) (map[coreunit.Name]corestatus.StatusInfo, error) {
	if err := appID.Validate(); err != nil {
		return nil, errors.Errorf("application ID: %w", err)
	}

	statuses, err := s.st.GetUnitWorkloadStatusesForApplication(ctx, appID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	decoded, err := decodeUnitWorkloadStatuses(statuses)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return decoded, nil
}

// GetUnitDisplayStatus returns the display status of the specified unit. The display
// status a function of both the unit workload status and the cloud container status.
// It returns an error satisfying [statuserrors.UnitNotFound] if the unit doesn't
// exist.
func (s *Service) GetUnitDisplayStatus(ctx context.Context, unitName coreunit.Name) (*corestatus.StatusInfo, error) {
	if err := unitName.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	workloadStatus, err := s.st.GetUnitWorkloadStatus(ctx, unitUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	containerStatus, err := s.st.GetUnitCloudContainerStatus(ctx, unitUUID)
	if err != nil && !errors.Is(err, statuserrors.UnitStatusNotFound) {
		return nil, errors.Capture(err)
	}
	return unitDisplayStatus(workloadStatus, containerStatus)
}

// GetUnitAndAgentDisplayStatus returns the unit and agent display status of the
// specified unit. The display status a function of both the unit workload status
// and the cloud container status. It returns an error satisfying
// [statuserrors.UnitNotFound] if the unit doesn't exist.
func (s *Service) GetUnitAndAgentDisplayStatus(ctx context.Context, unitName coreunit.Name) (agent *corestatus.StatusInfo, workload *corestatus.StatusInfo, _ error) {
	if err := unitName.Validate(); err != nil {
		return nil, nil, errors.Capture(err)
	}

	// TODO (stickupkid) This should just be 1 or 2 calls to the state layer
	// to get the agent and workload status.

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}

	agentStatus, err := s.st.GetUnitAgentStatus(ctx, unitUUID)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}

	workloadStatus, err := s.st.GetUnitWorkloadStatus(ctx, unitUUID)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}

	containerStatus, err := s.st.GetUnitCloudContainerStatus(ctx, unitUUID)
	if err != nil && !errors.Is(err, statuserrors.UnitStatusNotFound) {
		return nil, nil, errors.Capture(err)
	}

	return decodeUnitAgentWorkloadStatus(agentStatus, workloadStatus, containerStatus)
}

// SetUnitPresence marks the presence of the unit in the model. It is called by
// the unit agent accesses the API server. If the unit is not found, an error
// satisfying [applicationerrors.UnitNotFound] is returned. The unit life is not
// considered when setting the presence.
func (s *Service) SetUnitPresence(ctx context.Context, unitName coreunit.Name) error {
	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}
	return s.st.SetUnitPresence(ctx, unitName)
}

// DeleteUnitPresence removes the presence of the unit in the model. If the unit
// is not found, it ignores the error. The unit life is not considered when
// deleting the presence.
func (s *Service) DeleteUnitPresence(ctx context.Context, unitName coreunit.Name) error {
	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}
	return s.st.DeleteUnitPresence(ctx, unitName)
}

// CheckUnitStatusesReadyForMigration returns an error if the statuses of any units
// in the model indicate they cannot be migrated.
func (s *Service) CheckUnitStatusesReadyForMigration(ctx context.Context) error {
	fullStatuses, err := s.st.GetAllFullUnitStatuses(ctx)
	if err != nil {
		return errors.Errorf("getting unit statuses: %w", err)
	}

	failedChecks := []string{}
	for unitName, fullStatus := range fullStatuses {
		present, agentStatus, workloadStatus, err := decodeFullUnitStatus(fullStatus)
		if err != nil {
			return errors.Errorf("decoding full unit status for unit %q: %w", unitName, err)
		}
		if !present {
			failedChecks = append(failedChecks, fmt.Sprintf("- unit %q is not logged into the controller", unitName))
		}
		if !corestatus.IsAgentPresent(agentStatus) {
			failedChecks = append(failedChecks, fmt.Sprintf("- unit %q agent not idle or executing", unitName))
		}
		if !corestatus.IsUnitWorkloadPresent(workloadStatus) {
			failedChecks = append(failedChecks, fmt.Sprintf("- unit %q workload not active or viable", unitName))
		}
	}

	if len(failedChecks) > 0 {
		return errors.Errorf(
			"model unit(s) are not ready for migration:\n%s", strings.Join(failedChecks, "\n"))
	}
	return nil
}

// ExportUnitStatuses returns the workload and agent statuses of all the units in
// in the model, indexed by unit name.
func (s *Service) ExportUnitStatuses(ctx context.Context) (map[coreunit.Name]corestatus.StatusInfo, map[coreunit.Name]corestatus.StatusInfo, error) {
	fullStatuses, err := s.st.GetAllFullUnitStatuses(ctx)
	if err != nil {
		return nil, nil, errors.Errorf("getting unit statuses: %w", err)
	}

	workloadStatuses := make(map[coreunit.Name]corestatus.StatusInfo, len(fullStatuses))
	agentStatuses := make(map[coreunit.Name]corestatus.StatusInfo, len(fullStatuses))
	for unitName, fullStatus := range fullStatuses {
		_, agentStatus, workloadStatus, err := decodeFullUnitStatus(fullStatus)
		if err != nil {
			return nil, nil, errors.Errorf("decoding full unit status for unit %q: %w", unitName, err)
		}
		workloadStatuses[unitName] = workloadStatus
		agentStatuses[unitName] = agentStatus
	}
	return workloadStatuses, agentStatuses, nil
}

// ExportApplicationStatuses returns the statuses of all applications in the model,
// indexed by application name, if they have a status set.
func (s *Service) ExportApplicationStatuses(ctx context.Context) (map[string]corestatus.StatusInfo, error) {
	appStatuses, err := s.st.GetAllApplicationStatuses(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ret := make(map[string]corestatus.StatusInfo, len(appStatuses))
	for name, status := range appStatuses {
		decoded, err := decodeApplicationStatus(&status)
		if err != nil {
			return nil, errors.Errorf("decoding application status for %q: %w", name, err)
		}
		ret[name] = *decoded
	}

	return ret, nil
}

func ptr[T any](v T) *T {
	return &v
}
