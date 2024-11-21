// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package qotd

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
)

// QOTDAPI implements the client API for the quote of the day feature.
type QOTDAPI struct {
	authorizer     facade.Authorizer
	logger         logger.Logger
	controllerUUID string
}

func (q *QOTDAPI) checkAdmin(ctx context.Context) error {
	return q.authorizer.HasPermission(ctx, permission.AdminAccess, names.NewControllerTag(q.controllerUUID))
}

// SetQOTDAuthor sets the author of the quote of the day.
func (q *QOTDAPI) SetQOTDAuthor(ctx context.Context, arg params.SetQOTDAuthorArgs) (params.SetQOTDAuthorResult, error) {
	if err := q.checkAdmin(ctx); err != nil {
		return params.SetQOTDAuthorResult{}, errors.Trace(err)
	}

	response := params.SetQOTDAuthorResult{}
	_, err := names.ParseTag(arg.Entity.Tag)
	if err != nil {
		return params.SetQOTDAuthorResult{
			Error: apiservererrors.ServerError(apiservererrors.ErrBadId),
		}, nil
	}
	q.logger.Criticalf("Setting QOTD author to %s", arg.Author)
	return response, nil
}
