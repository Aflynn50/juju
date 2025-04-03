// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreunit "github.com/juju/juju/core/unit"
	passworderrors "github.com/juju/juju/domain/password/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var logger = internallogger.GetLogger("juju.apiserver.common")

// PasswordService defines the methods required to set a password hash for a
// unit.
type PasswordService interface {
	// SetUnitPassword sets the password hash for the given unit. If the unit
	// does not exist, an error satisfying [applicationerrors.UnitNotFound] is
	// returned.
	SetUnitPassword(context.Context, coreunit.Name, string) error
}

// PasswordChanger implements a common SetPasswords method for use by
// various facades.
type PasswordChanger struct {
	passwordService PasswordService
	st              state.EntityFinder
	getCanChange    GetAuthFunc
}

// NewPasswordChanger returns a new PasswordChanger. The GetAuthFunc will be
// used on each invocation of SetPasswords to determine current permissions.
func NewPasswordChanger(passwordService PasswordService, st state.EntityFinder, getCanChange GetAuthFunc) *PasswordChanger {
	return &PasswordChanger{
		passwordService: passwordService,
		st:              st,
		getCanChange:    getCanChange,
	}
}

// SetPasswords sets the given password for each supplied entity, if possible.
func (pc *PasswordChanger) SetPasswords(ctx context.Context, args params.EntityPasswords) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}
	canChange, err := pc.getCanChange()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, param := range args.Changes {
		tag, err := names.ParseTag(param.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canChange(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if err := pc.setPassword(ctx, tag, param.Password); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}

func (pc *PasswordChanger) setPassword(ctx context.Context, tag names.Tag, password string) error {
	switch tag.Kind() {
	case names.UnitTagKind:
		unitTag := tag.(names.UnitTag)
		unitName := coreunit.Name(unitTag.Id())
		if err := pc.passwordService.SetUnitPassword(ctx, unitName, password); errors.Is(err, passworderrors.UnitNotFound) {
			return errors.NotFoundf("unit %q", tag.Id())
		} else if err != nil {
			return internalerrors.Errorf("setting password for %q: %w", tag, err)
		}
		return nil

	// TODO: Handle the following password setting:
	//  - machine
	//  - model

	default:
		return pc.legacySetPassword(tag, password)
	}
}

func (pc *PasswordChanger) legacySetPassword(tag names.Tag, password string) error {
	type isManager interface {
		IsManager() bool
	}
	var err error
	entity0, err := pc.st.FindEntity(tag)
	if err != nil {
		return err
	}
	entity, ok := entity0.(state.Authenticator)
	if !ok {
		return apiservererrors.NotSupportedError(tag, "authentication")
	}
	if entity, ok := entity0.(isManager); ok && entity.IsManager() {
		err = pc.setMongoPassword(entity0, password)
	}
	if err == nil {
		err = entity.SetPassword(password)
		logger.Infof(context.TODO(), "setting password for %q", tag)
	}
	return err
}

// setMongoPassword applies to controller machines.
func (pc *PasswordChanger) setMongoPassword(entity state.Entity, password string) error {
	type mongoPassworder interface {
		SetMongoPassword(password string) error
	}
	// We set the mongo password first on the grounds that
	// if it fails, the agent in question should still be able
	// to authenticate to another API server and ask it to change
	// its password.
	if entity0, ok := entity.(mongoPassworder); ok {
		if err := entity0.SetMongoPassword(password); err != nil {
			return err
		}
		logger.Infof(context.TODO(), "setting mongo password for %q", entity.Tag())
		return nil
	}
	// TODO(dfc) fix
	return apiservererrors.NotSupportedError(entity.Tag(), "mongo access")
}
