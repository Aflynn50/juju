// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"encoding/json"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/internal/errors"
)

// State provides persistence and retrieval associated with entity removal.
type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// GetAllJobs returns all scheduled removal jobs.
func (st *State) GetAllJobs(ctx context.Context) ([]removal.Job, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare("SELECT &removalJob.* FROM removal", removalJob{})
	if err != nil {
		return nil, errors.Errorf("preparing select jobs query: %w", err)
	}

	var dbJobs []removalJob
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt).GetAll(&dbJobs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("running select jobs query: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	if len(dbJobs) == 0 {
		return nil, nil
	}

	jobs := make([]removal.Job, len(dbJobs))
	for i, job := range dbJobs {
		var arg map[string]any
		if job.Arg.Valid && job.Arg.String != "" {
			if err := json.Unmarshal([]byte(job.Arg.String), &arg); err != nil {
				return nil, errors.Errorf("decoding job arg: %w", err)
			}
		}

		jobs[i] = removal.Job{
			UUID:         removal.UUID(job.UUID),
			RemovalType:  removal.JobType(job.RemovalTypeID),
			EntityUUID:   job.EntityUUID,
			Force:        job.Force,
			ScheduledFor: job.ScheduledFor,
			Arg:          arg,
		}
	}

	return jobs, err
}
