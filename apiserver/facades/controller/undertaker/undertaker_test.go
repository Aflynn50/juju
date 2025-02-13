// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"
	"time"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/secrets/provider"
	_ "github.com/juju/juju/internal/secrets/provider/all"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

type undertakerSuite struct {
	coretesting.BaseSuite
	secrets                  *mockSecrets
	mockSecretBackendService *MockSecretBackendService
	modelConfigService       *MockModelConfigService
}

var _ = gc.Suite(&undertakerSuite{})

func (s *undertakerSuite) setupStateAndAPI(c *gc.C, isSystem bool, modelName string) (*mockState, *UndertakerAPI, *gomock.Controller) {
	ctrl := gomock.NewController(c)
	s.mockSecretBackendService = NewMockSecretBackendService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)

	machineNo := "1"
	if isSystem {
		machineNo = "0"
	}

	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag(machineNo),
		Controller: true,
	}

	st := newMockState(names.NewUserTag("admin"), modelName, isSystem)
	s.secrets = &mockSecrets{}
	s.PatchValue(&GetProvider, func(string) (provider.SecretBackendProvider, error) { return s.secrets, nil })

	api, err := newUndertakerAPI(
		st,
		nil,
		authorizer,
		nil,
		s.mockSecretBackendService,
		s.modelConfigService,
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	return st, api, ctrl
}

func (s *undertakerSuite) TestNoPerms(c *gc.C) {
	for _, authorizer := range []apiservertesting.FakeAuthorizer{{
		Tag: names.NewMachineTag("0"),
	}, {
		Tag: names.NewUserTag("bob"),
	}} {
		st := newMockState(names.NewUserTag("admin"), "admin", true)
		_, err := newUndertakerAPI(
			st,
			nil,
			authorizer,
			nil,
			nil,
			nil,
			nil,
		)
		c.Assert(err, gc.ErrorMatches, "permission denied")
	}
}

func (s *undertakerSuite) TestModelInfo(c *gc.C) {
	otherSt, hostedAPI, _ := s.setupStateAndAPI(c, false, "hostedmodel")
	st, api, _ := s.setupStateAndAPI(c, true, "admin")
	for _, test := range []struct {
		st        *mockState
		api       *UndertakerAPI
		isSystem  bool
		modelName string
	}{
		{otherSt, hostedAPI, false, "hostedmodel"},
		{st, api, true, "admin"},
	} {
		test.st.model.life = state.Dying
		test.st.model.forced = true
		minute := time.Minute
		test.st.model.timeout = &minute

		result, err := test.api.ModelInfo(context.Background())
		c.Assert(err, jc.ErrorIsNil)

		info := result.Result
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.Error, gc.IsNil)

		c.Assert(info.UUID, gc.Equals, test.st.model.UUID())
		c.Assert(info.GlobalName, gc.Equals, "user-admin/"+test.modelName)
		c.Assert(info.Name, gc.Equals, test.modelName)
		c.Assert(info.IsSystem, gc.Equals, test.isSystem)
		c.Assert(info.Life, gc.Equals, life.Dying)
		c.Assert(info.ForceDestroyed, gc.Equals, true)
		c.Assert(info.DestroyTimeout, gc.NotNil)
		c.Assert(*info.DestroyTimeout, gc.Equals, time.Minute)
	}
}

func (s *undertakerSuite) TestProcessDyingModel(c *gc.C) {
	otherSt, hostedAPI, _ := s.setupStateAndAPI(c, false, "hostedmodel")
	model, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = hostedAPI.ProcessDyingModel(context.Background())
	c.Assert(err, gc.ErrorMatches, "model is not dying")
	c.Assert(model.Life(), gc.Equals, state.Alive)

	otherSt.model.life = state.Dying
	err = hostedAPI.ProcessDyingModel(context.Background())
	c.Assert(err, gc.IsNil)
	c.Assert(model.Life(), gc.Equals, state.Dead)
}

func (s *undertakerSuite) TestRemoveAliveModel(c *gc.C) {
	otherSt, hostedAPI, ctrl := s.setupStateAndAPI(c, false, "hostedmodel")
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().GetSecretBackendConfigForAdmin(gomock.Any(), coremodel.UUID(otherSt.model.uuid)).Return(&provider.ModelBackendConfigInfo{}, nil)
	_, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = hostedAPI.RemoveModel(context.Background())
	c.Assert(err, gc.ErrorMatches, "model not dying or dead")
}

func (s *undertakerSuite) TestRemoveDyingModel(c *gc.C) {
	otherSt, hostedAPI, ctrl := s.setupStateAndAPI(c, false, "hostedmodel")
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().GetSecretBackendConfigForAdmin(gomock.Any(), coremodel.UUID(otherSt.model.uuid)).Return(&provider.ModelBackendConfigInfo{}, nil)
	// Set model to dying
	otherSt.model.life = state.Dying

	c.Assert(hostedAPI.RemoveModel(context.Background()), jc.ErrorIsNil)
}

func (s *undertakerSuite) TestDeadRemoveModel(c *gc.C) {
	otherSt, hostedAPI, ctrl := s.setupStateAndAPI(c, false, "hostedmodel")
	defer ctrl.Finish()

	s.mockSecretBackendService.EXPECT().GetSecretBackendConfigForAdmin(gomock.Any(), coremodel.UUID(otherSt.model.uuid)).Return(&provider.ModelBackendConfigInfo{
		ActiveID: "backend-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ModelUUID: "9d3d3b19-2b0c-4a3f-acde-0b1645586a72",
				BackendConfig: provider.BackendConfig{
					BackendType: "some-backend",
				},
			},
		},
	}, nil)

	// Set model to dead
	otherSt.model.life = state.Dying
	err := hostedAPI.ProcessDyingModel(context.Background())
	c.Assert(err, gc.IsNil)

	err = hostedAPI.RemoveModel(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(otherSt.removed, jc.IsTrue)
	c.Assert(s.secrets.cleanedUUID, gc.Equals, otherSt.model.uuid)
}

func (s *undertakerSuite) TestDeadRemoveModelSecretsConfigNotFound(c *gc.C) {
	otherSt, hostedAPI, ctrl := s.setupStateAndAPI(c, false, "hostedmodel")
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().GetSecretBackendConfigForAdmin(gomock.Any(), coremodel.UUID(otherSt.model.uuid)).Return(nil, secretbackenderrors.NotFound)
	// Set model to dead
	otherSt.model.life = state.Dying
	err := hostedAPI.ProcessDyingModel(context.Background())
	c.Assert(err, gc.IsNil)

	err = hostedAPI.RemoveModel(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(otherSt.removed, jc.IsTrue)
	c.Assert(s.secrets.cleanedUUID, gc.Equals, "")
}

func (s *undertakerSuite) TestModelConfig(c *gc.C) {
	_, hostedAPI, _ := s.setupStateAndAPI(c, false, "hostedmodel")

	expectedCfg, err := config.New(false, coretesting.FakeConfig())
	c.Assert(err, jc.ErrorIsNil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(expectedCfg, nil)

	cfg, err := hostedAPI.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, gc.NotNil)
}
