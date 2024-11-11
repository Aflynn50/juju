// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/resource"
	"github.com/juju/juju/internal/errors"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory database.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

func (s *State) SetResource(ctx context.Context, resource resource.Resource) error {
	db, err := s.DB()
	if err != nil {
		return err
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := s.setResourceMetadata(ctx, tx, resource)
		if err != nil {
			return errors.Errorf("setting resource metadata: %w", err)
		}

		err = s.setResource(ctx, tx, resource)
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

func (s *State) setResourceMetadata(ctx context.Context, tx *sqlair.TX, resource resource.Resource) error {
	resourceType := dbResourceType{Type: resource.Type.String()}
	idStmt, err := s.Prepare(`
SELECT &dbResourceType.id 
FROM   resource_kind 
WHERE  type = $dbResourceType.name
`, resourceType)
	if err != nil {
		return errors.Errorf("preparing select resource type statement: %w")
	}

	err = tx.Query(ctx, idStmt, resourceType).Get(&resourceType)
	if err != nil {
		return errors.Errorf("getting id for resource type %q: %w", resource.Type, err)
	}

	resourceMeta := dbResourceMeta{
		ApplicationUUID: resource.ApplicationUUID.String(),
		Name:            resource.Name,
		TypeID:          resourceType.ID,
		Path:            resource.Path,
		Description:     resource.Description,
	}
	metaStmt, err := s.Prepare(`
INSERT INTO resource_meta (*) 
VALUES      ($dbResourceMeta.*)
`, resourceMeta)
	if err != nil {
		return errors.Errorf("preparing insert resource metadata statement: %w")
	}

	err = tx.Query(ctx, metaStmt, resourceMeta).Run()
	if err != nil {
		return errors.Errorf("inserting resource metadata: %w", err)
	}
	return nil
}

func (s *State) setResource(ctx context.Context, tx *sqlair.TX, resource resource.Resource) error {
	originType := dbResourceType{Type: resource.Type.String()}
	idStmt, err := s.Prepare(`
SELECT &dbResourceType.id 
FROM   resource_origin_type 
WHERE  type = $dbResourceType.name
`, originType)
	if err != nil {
		return errors.Errorf("preparing select resource origin type statement: %w")
	}

	err = tx.Query(ctx, idStmt, originType).Get(&originType)
	if err != nil {
		return errors.Errorf("getting id for resource type %q: %w", resource.Type, err)
	}

	res := dbResource{
		UUID:            resource.UUID.String(),
		ApplicationUUID: resource.ApplicationUUID.String(),
		Name:            resource.Name,
		OriginTypeID:    originType.ID,
		Size:            resource.Size,
		Hash:            resource.Fingerprint.String(),
		HashTypeID:      resource.Fingerprint.String(),
		CreatedAt:       resource.CreatedAt,
	}

	stmt, err := s.Prepare(`
INSERT INTO resource (*)
VALUES      ($dbResource.*)
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
