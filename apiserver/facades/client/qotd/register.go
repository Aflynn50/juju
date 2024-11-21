// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package qotd

import (
	"context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("QOTD", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacade(stdCtx, ctx)
	}, reflect.TypeOf((*QOTDAPI)(nil)))
}

func newFacade(stdCtx context.Context, ctx facade.ModelContext) (*QOTDAPI, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &QOTDAPI{
		authorizer:     authorizer,
		logger:         ctx.Logger().Child("qotd"),
		controllerUUID: ctx.ControllerUUID(),
	}, nil
}
