// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/charm"
)

var (
	defaultBase = base.MustParseBaseFromString("22.04@ubuntu")
)

type ControllerSuite struct {
	baseSuite
}

var _ = gc.Suite(&ControllerSuite{})

func (s *ControllerSuite) TestPopulateControllerCharmLocalCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	origin := corecharm.Origin{
		Source: corecharm.Local,
		ID:     "deadbeef",
	}

	s.expectControllerAddress()
	s.expectCharmInfo()
	s.expectLocalDeployment(origin)
	s.expectAddApplication(origin)
	s.expectCompletion()

	err := PopulateControllerCharm(context.Background(), s.deployer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerSuite) TestPopulateControllerCharmLocalCharmFails(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectControllerAddress()
	s.expectCharmInfo()
	s.expectLocalCharmError()

	err := PopulateControllerCharm(context.Background(), s.deployer)
	c.Assert(err, gc.ErrorMatches, `.*boom`)
}

func (s *ControllerSuite) TestPopulateControllerCharmCharmhubCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	origin := corecharm.Origin{
		Source: corecharm.CharmHub,
		ID:     "deadbeef",
	}

	s.expectControllerAddress()
	s.expectCharmInfo()
	s.expectLocalCharmNotFound()
	s.expectCharmhubDeployment(origin)
	s.expectAddApplication(origin)
	s.expectCompletion()

	err := PopulateControllerCharm(context.Background(), s.deployer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerSuite) TestPopulateControllerAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()

	origin := corecharm.Origin{
		Source: corecharm.CharmHub,
		ID:     "deadbeef",
	}

	s.expectControllerAddress()
	s.expectCharmInfo()
	s.expectLocalCharmNotFound()
	s.expectCharmhubDeployment(origin)
	s.deployer.EXPECT().AddControllerApplication(gomock.Any(), DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}, "10.0.0.1").Return(coreunit.Name("controller/0"), applicationerrors.ApplicationAlreadyExists)
	s.expectCompletion()

	err := PopulateControllerCharm(context.Background(), s.deployer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerSuite) expectControllerAddress() {
	s.deployer.EXPECT().ControllerAddress(gomock.Any()).Return("10.0.0.1", nil)
}

func (s *ControllerSuite) expectCharmInfo() {
	s.deployer.EXPECT().ControllerCharmArch().Return(arch.DefaultArchitecture)
	s.deployer.EXPECT().ControllerCharmBase().Return(defaultBase, nil)
}

func (s *ControllerSuite) expectLocalDeployment(origin corecharm.Origin) {
	s.deployer.EXPECT().DeployLocalCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return(DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}, nil)
}

func (s *ControllerSuite) expectLocalCharmNotFound() {
	s.deployer.EXPECT().DeployLocalCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return(DeployCharmInfo{}, errors.NotFoundf("not found"))
}

func (s *ControllerSuite) expectLocalCharmError() {
	s.deployer.EXPECT().DeployLocalCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return(DeployCharmInfo{}, errors.Errorf("boom"))
}

func (s *ControllerSuite) expectCharmhubDeployment(origin corecharm.Origin) {
	s.deployer.EXPECT().DeployCharmhubCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return(DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}, nil)
}

func (s *ControllerSuite) expectAddApplication(origin corecharm.Origin) {
	s.deployer.EXPECT().AddControllerApplication(gomock.Any(), DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}, "10.0.0.1").Return(coreunit.Name("controller/0"), nil)
}

func (s *ControllerSuite) expectCompletion() {
	s.deployer.EXPECT().CompleteProcess(gomock.Any(), coreunit.Name("controller/0")).Return(nil)
}
