// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v9"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/status/service"
	"github.com/juju/juju/domain/status/state"
	"github.com/juju/juju/internal/errors"
)

func RegisterExport(
	coordinator Coordinator,
	clock clock.Clock,
	logger logger.Logger,
) {
	coordinator.Add(&exportOperation{
		clock:  clock,
		logger: logger,
	})
}

type ExportService interface {
	// ExportUnitStatuses returns the workload and agent statuses of all the units in
	// in the model, indexed by unit name.
	ExportUnitStatuses(ctx context.Context) (map[coreunit.Name]corestatus.StatusInfo, map[coreunit.Name]corestatus.StatusInfo, error)

	// ExportApplicationStatuses returns the statuses of all applications in the model,
	// indexed by application name, if they have a status set.
	ExportApplicationStatuses(ctx context.Context) (map[string]corestatus.StatusInfo, error)
}

type exportOperation struct {
	modelmigration.BaseOperation

	service ExportService

	clock  clock.Clock
	logger logger.Logger
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export status"
}

// Setup the export operation.
// This will create a new service instance.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.service = service.NewService(
		state.NewState(scope.ModelDB(), e.clock, e.logger),
		nil,
		e.clock,
		e.logger,
		nil,
	)
	return nil
}

// Execute the export operation, loading the statuses of the various entities in
// the model onto their description representation.
func (e *exportOperation) Execute(ctx context.Context, m description.Model) error {
	appStatuses, err := e.service.ExportApplicationStatuses(ctx)
	if err != nil {
		return errors.Errorf("retrieving application statuses: %w", err)
	}

	unitWorkloadStatuses, unitAgentStatuses, err := e.service.ExportUnitStatuses(ctx)
	if err != nil {
		return errors.Errorf("retrieving unit statuses: %w", err)
	}

	for _, app := range m.Applications() {
		appName := app.Name()

		// Application statuses are optional
		if appStatus, ok := appStatuses[appName]; ok {
			app.SetStatus(e.exportStatus(appStatus))
		}

		for _, unit := range app.Units() {
			unitName := coreunit.Name(unit.Name())
			agentStatus, ok := unitAgentStatuses[unitName]
			if !ok {
				return errors.Errorf("unit %q has no agent status", unitName)
			}
			unit.SetAgentStatus(e.exportStatus(agentStatus))

			workloadStatus, ok := unitWorkloadStatuses[unitName]
			if !ok {
				return errors.Errorf("unit %q has no workload status", unitName)
			}
			unit.SetWorkloadStatus(e.exportStatus(workloadStatus))
		}
	}
	return nil
}

func (e *exportOperation) exportStatus(status corestatus.StatusInfo) description.StatusArgs {
	now := e.clock.Now().UTC()
	if status.Since != nil {
		now = *status.Since
	}

	return description.StatusArgs{
		Value:   status.Status.String(),
		Message: status.Message,
		Data:    status.Data,
		Updated: now,
	}
}
