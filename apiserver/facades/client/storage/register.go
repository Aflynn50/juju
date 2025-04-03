// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Storage", 6, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStorageAPI(stdCtx, ctx) // modify Remove to support force and maxWait; add DetachStorage to support force and maxWait.
	}, reflect.TypeOf((*StorageAPI)(nil)))
}

// newStorageAPI returns a new storage API facade.
func newStorageAPI(stdCtx context.Context, ctx facade.ModelContext) (*StorageAPI, error) {
	st := ctx.State()

	domainServices := ctx.DomainServices()
	storageAccessor, err := getStorageAccessor(st)
	if err != nil {
		return nil, errors.Annotate(err, "getting backend")
	}

	modelInfo, err := domainServices.ModelInfo().GetModelInfo(stdCtx)
	if err != nil {
		return nil, errors.Annotate(err, "getting model info")
	}

	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	storageService := domainServices.Storage()
	return NewStorageAPI(
		ctx.ControllerUUID(),
		ctx.ModelUUID(),
		modelInfo.Type,
		stateShim{st},
		storageAccessor, domainServices.BlockDevice(), storageService,
		storageService.GetStorageRegistry, authorizer,
		domainServices.BlockCommand()), nil
}
