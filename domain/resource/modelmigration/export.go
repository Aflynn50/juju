// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v8"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	coreresource "github.com/juju/juju/core/resource"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/domain/resource/service"
	"github.com/juju/juju/domain/resource/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(
	coordinator Coordinator,
	registry corestorage.ModelStorageRegistryGetter,
	clock clock.Clock,
	logger logger.Logger,
) {
	coordinator.Add(&exportOperation{
		registry: registry,
		clock:    clock,
		logger:   logger,
	})
}

type ExportService interface {
	// GetResourcesByApplicationName retrieves resources associated with a
	// specific application name. If the application doesn't have any resources,
	// no error are returned, the result just contains an empty list.
	GetResourcesByApplicationName(ctx context.Context, name string) ([]coreresource.Resource, error)
}

// exportOperation describes a way to execute a migration for
// exporting applications.
type exportOperation struct {
	modelmigration.BaseOperation

	service ExportService

	registry corestorage.ModelStorageRegistryGetter
	clock    clock.Clock
	logger   logger.Logger
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export resources"
}

// Setup the export operation.
// This will create a new service instance.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.service = service.NewService(
		state.NewState(scope.ModelDB(), e.clock, e.logger),
		nil,
		e.logger,
	)
	return nil
}

// Execute the export, adding the application to the model.
// The export also includes all the charm metadata, manifest, config and
// actions. Along with units and resources.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	for _, app := range model.Applications() {
		resources, err := e.service.GetResourcesByApplicationName(ctx, app.Name())
		if err != nil {
			return errors.Errorf("getting resource of application %s: %w", app.Name(), err)
		}
		for _, res := range resources {
			app.AddResource(description.ResourceArgs{
				Name:      res.Name,
				Type:      res.Type.String(),
				Origin:    res.Origin.String(),
				Timestamp: res.Timestamp,
				Revision:  res.Revision,
			})
		}
	}
	return nil
}
