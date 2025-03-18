// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/errors"
)

// State describes retrieval and persistence methods for entity removal.
type State interface {

	// RelationExists returns true if a relation exists with the input UUID.
	RelationExists(ctx context.Context, rUUID string) (bool, error)

	// RelationAdvanceLifeAndScheduleRemoval advances the life cycle of the
	// relation with the input UUID to dying if it is alive, and schedules a
	// removal job for the relation, qualified with the input force boolean.
	RelationAdvanceLifeAndScheduleRemoval(ctx context.Context, removalUUID, relUUID string, force bool) error
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new namespace watcher
	// for events based on the input change mask.
	NewNamespaceWatcher(string, changestream.ChangeType, eventsource.NamespaceQuery) (watcher.StringsWatcher, error)
}

// Service provides the API for working with entity removal.
type Service struct {
	st     State
	logger logger.Logger
}

// RemoveRelation checks if a relation with the input UUID exists.
// If it does, the relation is guaranteed after this call to be:
// - No longer alive.
// - Removed or scheduled to be removed with the input force qualification.
// [relationerrors.RelationNotFound] is returned if no such relation exists.
func (s *Service) RemoveRelation(ctx context.Context, relUUID corerelation.UUID, force bool) error {
	exists, err := s.st.RelationExists(ctx, relUUID.String())
	if err != nil {
		return errors.Errorf("checking if relation %q exists: %w", relUUID, err)
	}
	if !exists {
		return errors.Errorf("relation %q does not exist", relUUID).Add(relationerrors.RelationNotFound)
	}

	if err := s.st.RelationAdvanceLifeAndScheduleRemoval(ctx, "", relUUID.String(), force); err != nil {
		return errors.Errorf("removing relation %q: %w", relUUID, err)
	}
	return nil
}

// WatchableService provides the API for working with entity removal,
// including the ability to create watchers.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService creates a new WatchableService
// for working with entity removal.
func NewWatchableService(
	st State,
	watcherFactory WatcherFactory,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		Service: Service{
			st:     st,
			logger: logger,
		},
		watcherFactory: watcherFactory,
	}
}
