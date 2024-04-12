// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/secretbackend"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

func ptr[T any](v T) *T {
	return &v
}

type SecretsSuite struct {
	testing.IsolationSuite

	authorizer         *facademocks.MockAuthorizer
	mockBackendService *MockSecretBackendService
}

var _ = gc.Suite(&SecretsSuite{})

func (s *SecretsSuite) setup(c *gc.C) (*SecretBackendsAPI, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.authorizer.EXPECT().AuthClient().Return(true)
	s.mockBackendService = NewMockSecretBackendService(ctrl)
	api, err := NewTestAPI(s.authorizer, s.mockBackendService)
	c.Assert(err, jc.ErrorIsNil)
	return api, ctrl
}

func (s *SecretsSuite) TestAddSecretBackends(c *gc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(nil)
	addedConfig := map[string]interface{}{
		"endpoint": "http://vault",
	}
	s.mockBackendService.EXPECT().CreateSecretBackend(gomock.Any(), secrets.SecretBackend{
		ID:                  "backend-id",
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(200 * time.Minute),
		Config:              addedConfig,
	}).Return(nil)
	s.mockBackendService.EXPECT().CreateSecretBackend(gomock.Any(), secrets.SecretBackend{
		ID:          "existing-id",
		Name:        "myvault2",
		BackendType: "vault",
		Config:      addedConfig,
	}).Return(secretbackenderrors.AlreadyExists)

	results, err := facade.AddSecretBackends(context.Background(), params.AddSecretBackendArgs{
		Args: []params.AddSecretBackendArg{{
			ID: "backend-id",
			SecretBackend: params.SecretBackend{
				Name:                "myvault",
				BackendType:         "vault",
				TokenRotateInterval: ptr(200 * time.Minute),
				Config:              map[string]interface{}{"endpoint": "http://vault"},
			},
		}, {
			ID: "existing-id",
			SecretBackend: params.SecretBackend{
				Name:        "myvault2",
				BackendType: "vault",
				Config:      map[string]interface{}{"endpoint": "http://vault"},
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{
		{},
		{Error: &params.Error{
			Code:    "secret backend already exists",
			Message: `secret backend already exists`}},
	})
}

func (s *SecretsSuite) TestAddSecretBackendsPermissionDenied(c *gc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	_, err := facade.AddSecretBackends(context.Background(), params.AddSecretBackendArgs{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestListSecretBackends(c *gc.C) {
	s.assertListSecretBackends(c, false)
}

func (s *SecretsSuite) TestListSecretBackendsReveal(c *gc.C) {
	s.assertListSecretBackends(c, true)
}

func (s *SecretsSuite) assertListSecretBackends(c *gc.C, reveal bool) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	if reveal {
		s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(nil)
	}
	s.mockBackendService.EXPECT().BackendSummaryInfo(gomock.Any(), reveal, true, "myvault").
		Return([]*secretbackendservice.SecretBackendInfo{
			{
				SecretBackend: secrets.SecretBackend{
					ID:                  "backend-id",
					Name:                "myvault",
					BackendType:         "vault",
					TokenRotateInterval: ptr(666 * time.Minute),
					Config: map[string]any{
						"endpoint": "http://vault",
						"token":    "s.ajehjdee",
					},
				},
				NumSecrets: 3,
				Status:     "error",
				Message:    "ping error",
			},
			{
				SecretBackend: secrets.SecretBackend{
					ID:          coretesting.ControllerTag.Id(),
					Name:        "internal",
					BackendType: "controller",
					Config:      map[string]any{},
				},
				NumSecrets: 1,
				Status:     "active",
			},
		}, nil)

	results, err := facade.ListSecretBackends(context.Background(),
		params.ListSecretBackendsArgs{
			Names: []string{"myvault"}, Reveal: reveal,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ListSecretBackendsResults{
		Results: []params.SecretBackendResult{
			{
				Result: params.SecretBackend{
					Name:                "myvault",
					BackendType:         "vault",
					TokenRotateInterval: ptr(666 * time.Minute),
					Config: map[string]any{
						"endpoint": "http://vault",
						"token":    "s.ajehjdee",
					},
				},
				ID:         "backend-id",
				NumSecrets: 3,
				Status:     "error",
				Message:    "ping error",
			},
			{
				Result: params.SecretBackend{
					Name:        "internal",
					BackendType: "controller",
					Config:      map[string]interface{}{},
				},
				ID:         coretesting.ControllerTag.Id(),
				Status:     "active",
				NumSecrets: 1,
			},
		},
	})
}

func (s *SecretsSuite) TestListSecretBackendsPermissionDeniedReveal(c *gc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	_, err := facade.ListSecretBackends(context.Background(), params.ListSecretBackendsArgs{Reveal: true})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestUpdateSecretBackends(c *gc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(nil)

	s.mockBackendService.EXPECT().UpdateSecretBackend(gomock.Any(),
		secretbackendservice.UpdateSecretBackendParams{
			UpdateSecretBackendParams: secretbackend.UpdateSecretBackendParams{
				BackendIdentifier:   secretbackend.BackendIdentifier{Name: "myvault"},
				NewName:             ptr("new-name"),
				TokenRotateInterval: ptr(200 * time.Minute),
				Config: map[string]string{
					"endpoint":        "http://vault",
					"namespace":       "foo",
					"tls-server-name": "server-name",
				},
			},
			Reset:    []string{"namespace"},
			SkipPing: true,
		},
	).Return(nil)
	s.mockBackendService.EXPECT().UpdateSecretBackend(gomock.Any(),
		secretbackendservice.UpdateSecretBackendParams{
			UpdateSecretBackendParams: secretbackend.UpdateSecretBackendParams{
				BackendIdentifier: secretbackend.BackendIdentifier{Name: "not-existing-name"},
			},
		},
	).Return(secretbackenderrors.NotFound)

	results, err := facade.UpdateSecretBackends(context.Background(), params.UpdateSecretBackendArgs{
		Args: []params.UpdateSecretBackendArg{{
			Name:                "myvault",
			NameChange:          ptr("new-name"),
			TokenRotateInterval: ptr(200 * time.Minute),
			Config: map[string]interface{}{
				"endpoint":        "http://vault",
				"namespace":       "foo",
				"tls-server-name": "server-name",
			},
			Reset: []string{"namespace"},
			Force: true,
		}, {
			Name: "not-existing-name",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{
		{},
		{Error: &params.Error{
			Code:    "secret backend not found",
			Message: `secret backend not found`}},
	})
}

func (s *SecretsSuite) TestUpdateSecretBackendsPermissionDenied(c *gc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	_, err := facade.UpdateSecretBackends(context.Background(), params.UpdateSecretBackendArgs{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestRemoveSecretBackends(c *gc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(nil)

	gomock.InOrder(
		s.mockBackendService.EXPECT().DeleteSecretBackend(gomock.Any(),
			secretbackendservice.DeleteSecretBackendParams{
				BackendIdentifier: secretbackend.BackendIdentifier{Name: "myvault"},
				DeleteInUse:       true,
			}).Return(nil),
		s.mockBackendService.EXPECT().DeleteSecretBackend(gomock.Any(),
			secretbackendservice.DeleteSecretBackendParams{
				BackendIdentifier: secretbackend.BackendIdentifier{Name: "myvault2"},
				DeleteInUse:       false,
			}).Return(errors.NotSupportedf("remove with revisions")),
	)

	results, err := facade.RemoveSecretBackends(context.Background(), params.RemoveSecretBackendArgs{
		Args: []params.RemoveSecretBackendArg{{
			Name:  "myvault",
			Force: true,
		}, {
			Name: "myvault2",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{
		{},
		{Error: &params.Error{
			Code:    "not supported",
			Message: `remove with revisions not supported`}},
	})
}

func (s *SecretsSuite) TestRemoveSecretBackendsPermissionDenied(c *gc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	_, err := facade.RemoveSecretBackends(context.Background(), params.RemoveSecretBackendArgs{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}
