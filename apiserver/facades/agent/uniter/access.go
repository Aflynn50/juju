// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/state"
)

func applicationAccessor(authorizer facade.Authorizer, st *state.State) common.GetAuthFunc {
	return func(context.Context) (common.AuthFunc, error) {
		switch tag := authorizer.GetAuthTag().(type) {
		case names.ApplicationTag:
			return func(applicationTag names.Tag) bool {
				return tag == applicationTag
			}, nil
		case names.UnitTag:
			entity, err := st.Unit(tag.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
			applicationName := entity.ApplicationName()
			applicationTag := names.NewApplicationTag(applicationName)
			return func(tag names.Tag) bool {
				return tag == applicationTag
			}, nil
		default:
			return nil, errors.Errorf("expected names.UnitTag or names.ApplicationTag, got %T", tag)
		}
	}
}

func machineAccessor(authorizer facade.Authorizer, st *state.State) common.GetAuthFunc {
	return func(context.Context) (common.AuthFunc, error) {
		switch tag := authorizer.GetAuthTag().(type) {
		// Application agents can't access machines.
		case names.ApplicationTag:
			return func(tag names.Tag) bool {
				return false
			}, nil
		case names.UnitTag:
			entity, err := st.Unit(tag.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
			machineId, err := entity.AssignedMachineId()
			if err != nil {
				return nil, errors.Trace(err)
			}
			machineTag := names.NewMachineTag(machineId)
			return func(tag names.Tag) bool {
				return tag == machineTag
			}, nil
		default:
			return nil, errors.Errorf("expected names.UnitTag or names.ApplicationTag, got %T", tag)
		}
	}
}

func cloudSpecAccessor(authorizer facade.Authorizer, appService ApplicationService) func(ctx context.Context) (func() bool, error) {
	return func(ctx context.Context) (func() bool, error) {
		var appName string
		switch tag := authorizer.GetAuthTag().(type) {
		case names.UnitTag:
			unitName, err := unit.NewName(tag.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
			appName = unitName.Application()
		default:
			return nil, errors.Errorf("expected names.UnitTag, got %T", tag)
		}

		appUUID, err := appService.GetApplicationIDByName(ctx, appName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		config, err := appService.GetApplicationConfig(ctx, appUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return func() bool {
			return config.GetBool(coreapplication.TrustConfigOptionName, false)
		}, nil
	}
}
