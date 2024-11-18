// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
    "context"

    "github.com/canonical/sqlair"

    "github.com/juju/juju/core/application"
    "github.com/juju/juju/core/database"
    "github.com/juju/juju/core/logger"
    "github.com/juju/juju/core/resources"
    coreunit "github.com/juju/juju/core/unit"
    "github.com/juju/juju/domain"
    "github.com/juju/juju/domain/application/resource"
    "github.com/juju/juju/internal/errors"
)

// ResourceState is used to access the database.
type ResourceState struct {
    *commonStateBase
    logger logger.Logger
}

// NewResourceState creates a state to access the database.
func NewResourceState(factory database.TxnRunnerFactory, logger logger.Logger) *ResourceState {
    return &ResourceState{
        commonStateBase: &commonStateBase{
            StateBase: domain.NewStateBase(factory),
        },
        logger: logger,
    }
}

// GetApplicationResourceID returns the UUID of the application resource
// specified by natural key of application and resource name.
func (st *ResourceState) GetApplicationResourceID(
    ctx context.Context,
    args resource.GetApplicationResourceIDArgs,
) (resources.UUID, error) {
    return "", nil
}

// ListResources returns the list of resources for the given application.
func (st *ResourceState) ListResources(
    ctx context.Context,
    applicationID application.ID,
) (resource.ApplicationResources, error) {
    return resource.ApplicationResources{}, nil
}

// GetResource returns the identified resource.
func (st *ResourceState) GetResource(ctx context.Context, resourceID resources.UUID) (resource.Resource, error) {
    return resource.Resource{}, nil
}

// SetResource updates the resource metadata when the resource is added to blob storage.
func (st *ResourceState) SetResource(
    ctx context.Context,
    resource resource.Resource,
    increment resource.IncrementCharmModifiedVersionType,
) error {
    db, err := st.DB()
    if err != nil {
        return err
    }

    err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
        err := st.setResourceMetadata(ctx, tx, resource)
        if err != nil {
            return errors.Errorf("setting resource metadata: %w", err)
        }

        err = st.setResource(ctx, tx, resource)
        if err != nil {
            return errors.Errorf("setting resource: %w", err)
        }
        return nil
    })
    if err != nil {
        return errors.Errorf("setting resource %q of application %q: %w", resource.Name, resource.ApplicationUUID, err)
    }
    return nil
}

// SetUnitResource sets the resource metadata for a specific unit.
func (st *ResourceState) SetUnitResource(
    ctx context.Context,
    config resource.SetUnitResourceArgs,
) (resource.SetUnitResourceResult, error) {
    return resource.SetUnitResourceResult{}, nil
}

// OpenApplicationResource returns the metadata for a resource.
func (st *ResourceState) OpenApplicationResource(
    ctx context.Context,
    resourceID resources.UUID,
) (resource.Resource, error) {
    return resource.Resource{}, nil
}

// OpenUnitResource returns the metadata for a resource. A unit
// resource is created to track the given unit and which resource
// its using.
func (st *ResourceState) OpenUnitResource(
    ctx context.Context,
    resourceID resources.UUID,
    unitID coreunit.UUID,
) (resource.Resource, error) {
    return resource.Resource{}, nil
}

// SetRepositoryResources sets the "polled" resource
// s for the
// application to the provided values. The current data for this
// application/resource combination will be overwritten.
func (st *ResourceState) SetRepositoryResources(
    ctx context.Context,
    config resource.SetRepositoryResourcesArgs,
) error {
    return nil
}

func (s *ResourceState) setResourceMetadata(ctx context.Context, tx *sqlair.TX, resource resource.Resource) error {
    resourceType := setResourceType{Type: resource.Type.String()}
    idStmt, err := s.Prepare(`
SELECT &setResourceType.id 
FROM   resource_kind 
WHERE  type = $setResourceType.name
`, resourceType)
    if err != nil {
        return errors.Errorf("preparing select resource type statement: %w")
    }

    err = tx.Query(ctx, idStmt, resourceType).Get(&resourceType)
    if err != nil {
        return errors.Errorf("getting id for resource type %q: %w", resource.Type, err)
    }

    resourceMeta := setResourceMeta{
        ApplicationUUID: resource.ApplicationUUID.String(),
        Name:            resource.Name,
        TypeID:          resourceType.ID,
        Path:            resource.Path,
        Description:     resource.Description,
    }
    metaStmt, err := s.Prepare(`
INSERT INTO resource_meta (*) 
VALUES      ($setResourceMeta.*)
`, resourceMeta)
    if err != nil {
        return errors.Errorf("preparing insert resource metadata statement: %w")
    }

    err = tx.Query(ctx, metaStmt, resourceMeta).Run()
    // TODO: If it already exists, then that's OK
    if err != nil {
        return errors.Errorf("inserting resource metadata: %w", err)
    }
    return nil
}

func (s *ResourceState) setResource(ctx context.Context, tx *sqlair.TX, resource resource.Resource) error {
    originType := setResourceType{Type: resource.Type.String()}
    idStmt, err := s.Prepare(`
SELECT &setResourceType.id 
FROM   resource_origin_type 
WHERE  type = $setResourceType.name
`, originType)
    if err != nil {
        return errors.Errorf("preparing select resource origin type statement: %w")
    }

    err = tx.Query(ctx, idStmt, originType).Get(&originType)
    if err != nil {
        return errors.Errorf("getting id for resource type %q: %w", resource.Type, err)
    }

    res := setResource{
        UUID:            resource.UUID.String(),
        ApplicationUUID: resource.ApplicationUUID.String(),
        Name:            resource.Name,
        OriginTypeID:    originType.ID,
        Size:            resource.Size,
        Hash:            resource.Fingerprint.String(),
        HashTypeID:      resource.Fingerprint.String(),
        CreatedAt:       resource.Timestamp,
    }

    stmt, err := s.Prepare(`
INSERT INTO resource (*)
VALUES      ($setResource.*)
`, res)
    if err != nil {
        return errors.Errorf("preparing insert resource statement: %w")
    }

    err = tx.Query(ctx, stmt, res).Run()
    if err != nil {
        return errors.Errorf("inserting resource: %w", err)
    }
    return nil
}
