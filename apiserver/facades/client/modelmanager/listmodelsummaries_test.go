// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	stdcontext "context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/modelmanager"
	"github.com/juju/juju/apiserver/facades/client/modelmanager/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/environs/config"
	_ "github.com/juju/juju/internal/provider/azure"
	_ "github.com/juju/juju/internal/provider/ec2"
	_ "github.com/juju/juju/internal/provider/maas"
	_ "github.com/juju/juju/internal/provider/openstack"
	"github.com/juju/juju/internal/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	jtesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	jujuversion "github.com/juju/juju/version"
)

type ListModelsWithInfoSuite struct {
	jujutesting.IsolationSuite

	st   *mockState
	cred cloud.Credential

	authoriser        apiservertesting.FakeAuthorizer
	adminUser         names.UserTag
	mockAccessService *mocks.MockAccessService

	api *modelmanager.ModelManagerAPI

	controllerUUID uuid.UUID
}

var _ = gc.Suite(&ListModelsWithInfoSuite{})

func (s *ListModelsWithInfoSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	var err error
	s.controllerUUID, err = uuid.UUIDFromString(coretesting.ControllerTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	adminUser := "admin"
	s.adminUser = names.NewUserTag(adminUser)

	s.st = &mockState{
		model: s.createModel(c, s.adminUser),
	}
	s.st.modelDetailsForUser = func() ([]state.ModelSummary, error) {
		return []state.ModelSummary{s.st.model.getModelDetails()}, s.st.NextErr()
	}

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.adminUser,
	}

	s.cred = cloud.NewEmptyCredential()
	api, err := modelmanager.NewModelManagerAPI(
		s.st, nil, &mockState{},
		s.controllerUUID,
		modelmanager.Services{
			ServiceFactoryGetter: nil,
			CloudService: &mockCloudService{
				clouds: map[string]cloud.Cloud{"dummy": jtesting.DefaultCloud},
			},
			CredentialService:    apiservertesting.ConstCredentialGetter(&s.cred),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.mockAccessService,
			ObjectStore:          &mockObjectStore{},
		},
		state.NoopConfigSchemaSource,
		nil, nil,
		common.NewBlockChecker(s.st), s.authoriser, s.st.model,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *ListModelsWithInfoSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockAccessService = mocks.NewMockAccessService(ctrl)
	api, err := modelmanager.NewModelManagerAPI(
		s.st, nil, &mockState{},
		s.controllerUUID,
		modelmanager.Services{
			ServiceFactoryGetter: nil,
			CloudService: &mockCloudService{
				clouds: map[string]cloud.Cloud{"dummy": jtesting.DefaultCloud},
			},
			CredentialService:    apiservertesting.ConstCredentialGetter(&s.cred),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.mockAccessService,
			ObjectStore:          &mockObjectStore{},
		},
		state.NoopConfigSchemaSource,
		nil, nil,
		common.NewBlockChecker(s.st), s.authoriser, s.st.model,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
	return ctrl
}

func (s *ListModelsWithInfoSuite) createModel(c *gc.C, user names.UserTag) *mockModel {
	attrs := testing.FakeConfig()
	attrs["agent-version"] = jujuversion.Current.String()
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return &mockModel{
		owner:               user,
		cfg:                 cfg,
		setCloudCredentialF: func(tag names.CloudCredentialTag) (bool, error) { return false, nil },
	}
}

func (s *ListModelsWithInfoSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authoriser.Tag = user
	modelmanager, err := modelmanager.NewModelManagerAPI(
		s.st, nil, &mockState{},
		s.controllerUUID,
		modelmanager.Services{
			ServiceFactoryGetter: nil,
			CloudService: &mockCloudService{
				clouds: map[string]cloud.Cloud{"dummy": jtesting.DefaultCloud},
			},
			CredentialService:    apiservertesting.ConstCredentialGetter(&s.cred),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.mockAccessService,
			ObjectStore:          &mockObjectStore{},
		},
		state.NoopConfigSchemaSource,
		nil, nil,
		common.NewBlockChecker(s.st), s.authoriser, s.st.model,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = modelmanager
}

func (s *ListModelsWithInfoSuite) TestListModelSummaries(c *gc.C) {
	defer s.setupMocks(c).Finish()
	lastLoginTime := time.Now()
	s.mockAccessService.EXPECT().LastModelLogin(
		gomock.Any(),
		"admin",
		coremodel.UUID(s.st.ModelUUID()),
	).Return(lastLoginTime, nil)

	result, err := s.api.ListModelSummaries(stdcontext.Background(), params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ModelSummaryResults{
		Results: []params.ModelSummaryResult{
			{
				Result: &params.ModelSummary{
					Name:               "testmodel",
					OwnerTag:           s.adminUser.String(),
					UUID:               s.st.ModelUUID(),
					Type:               string(state.ModelTypeIAAS),
					CloudTag:           "dummy",
					CloudRegion:        "dummy-region",
					CloudCredentialTag: "cloudcred-dummy_bob_some-credential",
					Life:               "alive",
					Status:             params.EntityStatus{},
					Counts:             []params.ModelEntityCount{},
					UserLastConnection: &lastLoginTime,
				},
			},
		},
	})
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesWithUserAccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.mockAccessService.EXPECT().LastModelLogin(
		gomock.Any(),
		"admin",
		coremodel.UUID(s.st.ModelUUID()),
	).Return(time.Time{}, accesserrors.UserNeverAccessedModel)
	s.st.modelDetailsForUser = func() ([]state.ModelSummary, error) {
		summary := s.st.model.getModelDetails()
		summary.Access = permission.AdminAccess
		return []state.ModelSummary{summary}, nil
	}
	result, err := s.api.ListModelSummaries(stdcontext.Background(), params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Result.UserAccess, jc.DeepEquals, params.ModelAdminAccess)
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesWithLastConnected(c *gc.C) {
	now := time.Now()
	s.st.modelDetailsForUser = func() ([]state.ModelSummary, error) {
		summary := s.st.model.getModelDetails()
		summary.UserLastConnection = &now
		return []state.ModelSummary{summary}, nil
	}
	result, err := s.api.ListModelSummaries(stdcontext.Background(), params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Result.UserLastConnection, jc.DeepEquals, &now)
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesWithMachineCount(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.mockAccessService.EXPECT().LastModelLogin(
		gomock.Any(),
		"admin",
		coremodel.UUID(s.st.ModelUUID()),
	).Return(time.Time{}, accesserrors.UserNeverAccessedModel)
	s.st.modelDetailsForUser = func() ([]state.ModelSummary, error) {
		summary := s.st.model.getModelDetails()
		summary.MachineCount = int64(64)
		return []state.ModelSummary{summary}, nil
	}
	result, err := s.api.ListModelSummaries(stdcontext.Background(), params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Result.Counts[0], jc.DeepEquals, params.ModelEntityCount{params.Machines, 64})
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesWithCoreCount(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.mockAccessService.EXPECT().LastModelLogin(
		gomock.Any(),
		"admin",
		coremodel.UUID(s.st.ModelUUID()),
	).Return(time.Time{}, accesserrors.UserNeverAccessedModel)
	s.st.modelDetailsForUser = func() ([]state.ModelSummary, error) {
		summary := s.st.model.getModelDetails()
		summary.CoreCount = int64(43)
		return []state.ModelSummary{summary}, nil
	}
	result, err := s.api.ListModelSummaries(stdcontext.Background(), params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Result.Counts[0], jc.DeepEquals, params.ModelEntityCount{params.Cores, 43})
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesWithMachineAndUserDetails(c *gc.C) {
	now := time.Now()
	defer s.setupMocks(c).Finish()
	s.mockAccessService.EXPECT().LastModelLogin(
		gomock.Any(),
		"admin",
		coremodel.UUID(s.st.ModelUUID()),
	).Return(now, nil)
	s.st.modelDetailsForUser = func() ([]state.ModelSummary, error) {
		summary := s.st.model.getModelDetails()
		summary.Access = permission.AdminAccess
		summary.MachineCount = int64(10)
		summary.CoreCount = int64(42)
		return []state.ModelSummary{summary}, nil
	}
	result, err := s.api.ListModelSummaries(stdcontext.Background(), params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ModelSummaryResults{
		Results: []params.ModelSummaryResult{
			{
				Result: &params.ModelSummary{
					Name:               "testmodel",
					OwnerTag:           s.adminUser.String(),
					UUID:               s.st.ModelUUID(),
					Type:               string(state.ModelTypeIAAS),
					CloudTag:           "dummy",
					CloudRegion:        "dummy-region",
					CloudCredentialTag: "cloudcred-dummy_bob_some-credential",
					Life:               "alive",
					Status:             params.EntityStatus{},
					UserAccess:         params.ModelAdminAccess,
					UserLastConnection: &now,
					Counts: []params.ModelEntityCount{
						{params.Machines, 10},
						{params.Cores, 42},
					},
				},
			},
		},
	})
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesDenied(c *gc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	other := names.NewUserTag("other@remote")
	_, err := s.api.ListModelSummaries(stdcontext.Background(), params.ModelSummariesRequest{UserTag: other.String()})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesInvalidUser(c *gc.C) {
	_, err := s.api.ListModelSummaries(stdcontext.Background(), params.ModelSummariesRequest{UserTag: "invalid"})
	c.Assert(err, gc.ErrorMatches, `"invalid" is not a valid tag`)
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesStateError(c *gc.C) {
	errMsg := "captain error for ModelSummariesForUser"
	s.st.Stub.SetErrors(errors.New(errMsg))
	_, err := s.api.ListModelSummaries(stdcontext.Background(), params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, gc.ErrorMatches, errMsg)
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesNoModelsForUser(c *gc.C) {
	s.st.modelDetailsForUser = func() ([]state.ModelSummary, error) {
		return []state.ModelSummary{}, nil
	}
	results, err := s.api.ListModelSummaries(stdcontext.Background(), params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}
