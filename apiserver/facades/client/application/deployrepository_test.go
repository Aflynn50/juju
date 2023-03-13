// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v10"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type validatorSuite struct {
	machine     *MockMachine
	model       *MockModel
	repo        *MockRepository
	repoFactory *MockRepositoryFactory
	state       *MockDeployFromRepositoryState
}

var _ = gc.Suite(&validatorSuite{})

func (s *validatorSuite) TestValidateSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// createOrigin
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig()))
	// resolveCharm
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04/stable"},
		Revision: intptr(4),
	}
	supportedSeries := []string{"jammy", "focal"}
	s.repo.EXPECT().ResolveWithPreferredChannel(curl, origin).Return(resultURL, resolvedOrigin, supportedSeries, nil)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig()))
	// getCharm
	expMeta := &charm.Meta{
		Name: "test-charm",
	}
	expManifest := new(charm.Manifest)
	expConfig := new(charm.Config)
	essMeta := corecharm.EssentialMetadata{
		Meta:           expMeta,
		Manifest:       expManifest,
		Config:         expConfig,
		ResolvedOrigin: resolvedOrigin,
	}
	s.repo.EXPECT().GetEssentialMetadata(corecharm.MetadataRequest{
		CharmURL: resultURL,
		Origin:   resolvedOrigin,
	}).Return([]corecharm.EssentialMetadata{essMeta}, nil)

	arg := params.DeployFromRepositoryArg{
		CharmName: "testcharm",
	}
	dt, errs := s.getValidator().validate(arg)
	c.Assert(errs, gc.HasLen, 0)
	c.Assert(dt, gc.DeepEquals, deployTemplate{
		applicationName: "testcharm",
		charm:           corecharm.NewCharmInfoAdapter(essMeta),
		charmURL:        resultURL,
		numUnits:        1,
		origin:          resolvedOrigin,
		storage:         map[string]state.StorageConstraints{},
	})
}

func (s *validatorSuite) TestResolveCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04/stable"},
		Revision: intptr(4),
	}
	supportedSeries := []string{"jammy", "focal"}
	s.repo.EXPECT().ResolveWithPreferredChannel(curl, origin).Return(resultURL, resolvedOrigin, supportedSeries, nil)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig()))

	obtainedCurl, obtainedOrigin, err := s.getValidator().resolveCharm(curl, origin, false, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedCurl, gc.DeepEquals, resultURL)
	c.Assert(obtainedOrigin, gc.DeepEquals, resolvedOrigin)
}

func (s *validatorSuite) TestResolveCharmArchAll(c *gc.C) {
	defer s.setupMocks(c).Finish()
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "all", OS: "ubuntu", Channel: "22.04/stable"},
		Revision: intptr(4),
	}
	supportedSeries := []string{"jammy", "focal"}
	s.repo.EXPECT().ResolveWithPreferredChannel(curl, origin).Return(resultURL, resolvedOrigin, supportedSeries, nil)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig()))
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{Arch: strptr("arm64")}, nil)

	obtainedCurl, obtainedOrigin, err := s.getValidator().resolveCharm(curl, origin, false, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedCurl, gc.DeepEquals, resultURL)
	expectedOrigin := resolvedOrigin
	expectedOrigin.Platform.Architecture = "arm64"
	c.Assert(obtainedOrigin, gc.DeepEquals, expectedOrigin)
}

func (s *validatorSuite) TestResolveCharmUnsupportedSeriesErrorForce(c *gc.C) {
	defer s.setupMocks(c).Finish()
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04/stable"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04/stable"},
		Revision: intptr(4),
	}
	supportedSeries := []string{"focal"}
	newErr := charm.NewUnsupportedSeriesError("jammy", supportedSeries)
	s.repo.EXPECT().ResolveWithPreferredChannel(curl, origin).Return(resultURL, resolvedOrigin, supportedSeries, newErr)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig()))

	obtainedCurl, obtainedOrigin, err := s.getValidator().resolveCharm(curl, origin, true, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedCurl, gc.DeepEquals, resultURL)
	c.Assert(obtainedOrigin, gc.DeepEquals, resolvedOrigin)
}

func (s *validatorSuite) TestResolveCharmUnsupportedSeriesError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04/stable"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04/stable"},
		Revision: intptr(4),
	}
	supportedSeries := []string{"focal"}
	newErr := charm.NewUnsupportedSeriesError("jammy", supportedSeries)
	s.repo.EXPECT().ResolveWithPreferredChannel(curl, origin).Return(resultURL, resolvedOrigin, supportedSeries, newErr)

	_, _, err := s.getValidator().resolveCharm(curl, origin, false, false)
	c.Assert(err, gc.ErrorMatches, `series "jammy" not supported by charm, supported series are: focal. Use --force to deploy the charm anyway.`)
}

func (s *validatorSuite) TestCreateOrigin(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig()))

	arg := params.DeployFromRepositoryArg{
		CharmName: "testcharm",
		Revision:  intptr(7),
	}
	curl, origin, defaultBase, err := s.getValidator().createOrigin(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, gc.DeepEquals, charm.MustParseURL("ch:testcharm-7"))
	c.Assert(origin, gc.DeepEquals, corecharm.Origin{
		Source:   "charm-hub",
		Revision: intptr(7),
		Channel:  &corecharm.DefaultChannel,
		Platform: corecharm.Platform{Architecture: "amd64"},
	})
	c.Assert(defaultBase, jc.IsFalse)
}

func (s *validatorSuite) TestCreateOriginChannel(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig()))

	arg := params.DeployFromRepositoryArg{
		CharmName: "testcharm",
		Revision:  intptr(7),
		Channel:   strptr("yoga/candidate"),
	}
	curl, origin, defaultBase, err := s.getValidator().createOrigin(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, gc.DeepEquals, charm.MustParseURL("ch:testcharm-7"))
	expectedChannel := corecharm.MustParseChannel("yoga/candidate")
	c.Assert(origin, gc.DeepEquals, corecharm.Origin{
		Source:   "charm-hub",
		Revision: intptr(7),
		Channel:  &expectedChannel,
		Platform: corecharm.Platform{Architecture: "amd64"},
	})
	c.Assert(defaultBase, jc.IsFalse)
}

func (s *validatorSuite) TestGetCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()
	curl := charm.MustParseURL("ch:amd64/jammy/testcharm-1")
	origin := corecharm.Origin{
		Source: "charm-hub",
	}
	expMeta := &charm.Meta{
		Name: "test-charm",
	}
	expManifest := new(charm.Manifest)
	expConfig := new(charm.Config)
	essMeta := corecharm.EssentialMetadata{
		Meta:           expMeta,
		Manifest:       expManifest,
		Config:         expConfig,
		ResolvedOrigin: origin,
	}
	s.repo.EXPECT().GetEssentialMetadata(corecharm.MetadataRequest{
		CharmURL: curl,
		Origin:   origin,
	}).Return([]corecharm.EssentialMetadata{essMeta}, nil)
	obtainedOrigin, obtainedCharm, err := s.getValidator().getCharm(curl, origin)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedOrigin, gc.DeepEquals, origin)
	c.Assert(obtainedCharm, gc.DeepEquals, corecharm.NewCharmInfoAdapter(essMeta))
}

func (s *validatorSuite) TestDeducePlatformSimple(c *gc.C) {
	defer s.setupMocks(c).Finish()
	//model constraint default
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{Arch: strptr("amd64")}, nil)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig()))

	arg := params.DeployFromRepositoryArg{CharmName: "testme"}
	plat, usedModelDefaultBase, err := s.getValidator().deducePlatform(arg)
	c.Assert(err, gc.IsNil)
	c.Assert(usedModelDefaultBase, jc.IsFalse)
	c.Assert(plat, gc.DeepEquals, corecharm.Platform{Architecture: "amd64"})
}

func (s *validatorSuite) TestDeducePlatformArgArchBase(c *gc.C) {
	defer s.setupMocks(c).Finish()

	arg := params.DeployFromRepositoryArg{
		CharmName: "testme",
		Cons:      constraints.Value{Arch: strptr("arm64")},
		Base: &params.Base{
			Name:    "ubuntu",
			Channel: "22.10",
		},
	}
	plat, usedModelDefaultBase, err := s.getValidator().deducePlatform(arg)
	c.Assert(err, gc.IsNil)
	c.Assert(usedModelDefaultBase, jc.IsFalse)
	c.Assert(plat, gc.DeepEquals, corecharm.Platform{
		Architecture: "arm64",
		OS:           "ubuntu",
		Channel:      "22.10",
	})
}

func (s *validatorSuite) TestDeducePlatformModelDefaultBase(c *gc.C) {
	defer s.setupMocks(c).Finish()
	//model constraint default
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	sConfig := coretesting.FakeConfig()
	sConfig = sConfig.Merge(coretesting.Attrs{
		"default-base": "ubuntu@22.04",
	})
	cfg, err := config.New(config.NoDefaults, sConfig)
	c.Assert(err, jc.ErrorIsNil)
	s.model.EXPECT().Config().Return(cfg, nil)

	arg := params.DeployFromRepositoryArg{
		CharmName: "testme",
	}
	plat, usedModelDefaultBase, err := s.getValidator().deducePlatform(arg)
	c.Assert(err, gc.IsNil)
	c.Assert(usedModelDefaultBase, jc.IsTrue)
	c.Assert(plat, gc.DeepEquals, corecharm.Platform{
		Architecture: "amd64",
		OS:           "ubuntu",
		Channel:      "22.04/stable",
	})
}

func (s *validatorSuite) TestDeducePlatformPlacementSimpleFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	s.state.EXPECT().Machine("0").Return(s.machine, nil)
	s.machine.EXPECT().Base().Return(state.Base{
		OS:      "ubuntu",
		Channel: "18.04",
	})
	hwc := &instance.HardwareCharacteristics{Arch: strptr("arm64")}
	s.machine.EXPECT().HardwareCharacteristics().Return(hwc, nil)

	arg := params.DeployFromRepositoryArg{
		CharmName: "testme",
		Placement: []*instance.Placement{{
			Directive: "0",
		}},
	}
	plat, usedModelDefaultBase, err := s.getValidator().deducePlatform(arg)
	c.Assert(err, gc.IsNil)
	c.Assert(usedModelDefaultBase, jc.IsFalse)
	c.Assert(plat, gc.DeepEquals, corecharm.Platform{
		Architecture: "arm64",
		OS:           "ubuntu",
		Channel:      "18.04",
	})
}

func (s *validatorSuite) TestDeducePlatformPlacementSimpleNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	//model constraint default
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{Arch: strptr("amd64")}, nil)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig()))
	s.state.EXPECT().Machine("0/lxd/0").Return(nil, errors.NotFoundf("machine 0/lxd/0 not found"))

	arg := params.DeployFromRepositoryArg{
		CharmName: "testme",
		Placement: []*instance.Placement{{
			Directive: "0/lxd/0",
		}},
	}
	plat, usedModelDefaultBase, err := s.getValidator().deducePlatform(arg)
	c.Assert(err, gc.IsNil)
	c.Assert(usedModelDefaultBase, jc.IsFalse)
	c.Assert(plat, gc.DeepEquals, corecharm.Platform{Architecture: "amd64"})
}

func (s *validatorSuite) TestDeducePlatformPlacementMutipleMatch(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	s.state.EXPECT().Machine(gomock.Any()).Return(s.machine, nil).Times(3)
	s.machine.EXPECT().Base().Return(state.Base{
		OS:      "ubuntu",
		Channel: "18.04",
	}).Times(3)
	hwc := &instance.HardwareCharacteristics{Arch: strptr("arm64")}
	s.machine.EXPECT().HardwareCharacteristics().Return(hwc, nil).Times(3)

	arg := params.DeployFromRepositoryArg{
		CharmName: "testme",
		Placement: []*instance.Placement{
			{Directive: "0"},
			{Directive: "1"},
			{Directive: "3"},
		},
	}
	plat, usedModelDefaultBase, err := s.getValidator().deducePlatform(arg)
	c.Assert(err, gc.IsNil)
	c.Assert(usedModelDefaultBase, jc.IsFalse)
	c.Assert(plat, gc.DeepEquals, corecharm.Platform{
		Architecture: "arm64",
		OS:           "ubuntu",
		Channel:      "18.04",
	})
}

func (s *validatorSuite) TestDeducePlatformPlacementMutipleMatchFail(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	s.state.EXPECT().Machine(gomock.Any()).Return(s.machine, nil).AnyTimes()
	s.machine.EXPECT().Base().Return(
		state.Base{
			OS:      "ubuntu",
			Channel: "18.04",
		}).AnyTimes()
	gomock.InOrder(
		s.machine.EXPECT().HardwareCharacteristics().Return(
			&instance.HardwareCharacteristics{Arch: strptr("arm64")},
			nil),
		s.machine.EXPECT().HardwareCharacteristics().Return(
			&instance.HardwareCharacteristics{Arch: strptr("amd64")},
			nil),
	)

	arg := params.DeployFromRepositoryArg{
		CharmName: "testme",
		Placement: []*instance.Placement{
			{Directive: "0"},
			{Directive: "1"},
		},
	}
	_, _, err := s.getValidator().deducePlatform(arg)
	c.Assert(errors.Is(err, errors.BadRequest), jc.IsTrue, gc.Commentf("%+v", err))
}

func (s *validatorSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.machine = NewMockMachine(ctrl)
	s.model = NewMockModel(ctrl)
	s.repo = NewMockRepository(ctrl)
	s.repoFactory = NewMockRepositoryFactory(ctrl)
	s.state = NewMockDeployFromRepositoryState(ctrl)
	return ctrl
}

func (s *validatorSuite) getValidator() *deployFromRepositoryValidator {
	s.repoFactory.EXPECT().GetCharmRepository(gomock.Any()).Return(s.repo, nil).AnyTimes()
	return &deployFromRepositoryValidator{
		model:       s.model,
		state:       s.state,
		repoFactory: s.repoFactory,
	}
}

func strptr(s string) *string {
	return &s
}

func intptr(i int) *int {
	return &i
}

type deployRepositorySuite struct {
	application *MockApplication
	state       *MockDeployFromRepositoryState
	validator   *MockDeployFromRepositoryValidator
}

var _ = gc.Suite(&deployRepositorySuite{})

func (s *deployRepositorySuite) TestDeployFromRepositoryAPI(c *gc.C) {
	defer s.setupMocks(c).Finish()
	arg := params.DeployFromRepositoryArg{
		CharmName: "testme",
	}
	template := deployTemplate{
		applicationName: "testme",
		charm:           corecharm.NewCharmInfoAdapter(corecharm.EssentialMetadata{}),
		charmURL:        charm.MustParseURL("ch:amd64/jammy/testme-5"),
		numUnits:        1,
		origin: corecharm.Origin{
			Source:   "charm-hub",
			Revision: intptr(5),
			Channel:  &charm.Channel{Risk: "stable"},
			Platform: corecharm.MustParsePlatform("amd64/ubuntu/22.04"),
		},
	}
	s.validator.EXPECT().ValidateArg(arg).Return(template, nil)
	info := state.CharmInfo{
		Charm: template.charm,
		ID:    charm.MustParseURL("ch:amd64/jammy/testme-5"),
	}
	s.state.EXPECT().AddCharmMetadata(info).Return(&state.Charm{}, nil)
	addAppArgs := state.AddApplicationArgs{
		Name:  "testme",
		Charm: &state.Charm{},
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-hub",
			Revision: intptr(5),
			Channel: &state.Channel{
				Risk: "stable",
			},
			Platform: &state.Platform{
				Architecture: "amd64",
				OS:           "ubuntu",
				Channel:      "22.04",
			},
		},
		NumUnits: 1,
	}
	s.state.EXPECT().AddApplication(addAppArgs).Return(s.application, nil)

	obtainedInfo, resources, errs := s.getDeployFromRepositoryAPI().DeployFromRepository(arg)
	c.Assert(errs, gc.HasLen, 0)
	c.Assert(resources, gc.HasLen, 0)
	c.Assert(obtainedInfo, gc.DeepEquals, params.DeployFromRepositoryInfo{
		CharmURL:         "ch:amd64/jammy/testme-5",
		Channel:          "stable",
		Architecture:     "amd64",
		Base:             params.Base{Name: "ubuntu", Channel: "22.04"},
		EffectiveChannel: nil,
	})
}

func (s *deployRepositorySuite) getDeployFromRepositoryAPI() *DeployFromRepositoryAPI {
	return &DeployFromRepositoryAPI{
		state:      s.state,
		validator:  s.validator,
		stateCharm: func(Charm) *state.Charm { return &state.Charm{} },
	}
}

func (s *deployRepositorySuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockDeployFromRepositoryState(ctrl)
	s.validator = NewMockDeployFromRepositoryValidator(ctrl)
	return ctrl
}
