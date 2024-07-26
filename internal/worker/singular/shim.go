// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/singular"
)

// NewFacade creates a Facade from an APICaller and an entity for which
// administrative control will be claimed. It's a suitable default value
// for ManifoldConfig.NewFacade.
func NewFacade(apiCaller base.APICaller, claimant names.Tag, entity names.Tag) (Facade, error) {
	facade, err := singular.NewAPI(apiCaller, claimant, entity)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return facade, nil
}

// NewWorker calls NewFlagWorker but returns a more convenient type. It's
// a suitable default value for ManifoldConfig.NewWorker.
func NewWorker(ctx context.Context, config FlagConfig) (worker.Worker, error) {
	worker, err := NewFlagWorker(ctx, config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
