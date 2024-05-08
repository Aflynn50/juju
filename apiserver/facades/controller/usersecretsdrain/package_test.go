// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecretsdrain

import (
	"testing"

	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/service_mock.go github.com/juju/juju/apiserver/facades/controller/usersecretsdrain SecretService,SecretBackendService

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

var NewUserSecretsDrainAPI = newUserSecretsDrainAPI

func NewTestAPI(
	authorizer facade.Authorizer,
	secretService SecretService,
	secretBackendService SecretBackendService,
) (*SecretsDrainAPI, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return &SecretsDrainAPI{
		modelUUID:            coretesting.ModelTag.Id(),
		secretService:        secretService,
		secretBackendService: secretBackendService,
	}, nil
}
