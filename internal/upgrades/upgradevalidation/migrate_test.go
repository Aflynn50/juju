// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	"github.com/juju/collections/transform"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
	"github.com/juju/juju/internal/upgrades/upgradevalidation/mocks"
	"github.com/juju/juju/state"
)

func makeBases(os string, vers []string) []state.Base {
	bases := make([]state.Base, len(vers))
	for i, vers := range vers {
		bases[i] = state.Base{OS: os, Channel: vers}
	}
	return bases
}

var _ = gc.Suite(&migrateSuite{})

type migrateSuite struct {
	jujutesting.IsolationSuite

	st           *mocks.MockState
	agentService *mocks.MockModelAgentService
}

func (s *migrateSuite) TestValidatorsForModelMigrationSourceJuju3(c *gc.C) {
	defer s.setupMocks(c).Finish()

	validators := upgradevalidation.ValidatorsForModelMigrationSource()

	checker := upgradevalidation.NewModelUpgradeCheck(s.st, "test-model", s.agentService, validators...)
	blockers, err := checker.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers, gc.IsNil)
}

func (s *migrateSuite) TestValidatorsForModelMigrationSourceJuju31(c *gc.C) {
	defer s.setupMocks(c).Finish()

	validators := upgradevalidation.ValidatorsForModelMigrationSource()

	checker := upgradevalidation.NewModelUpgradeCheck(s.st, "test-model", s.agentService, validators...)
	blockers, err := checker.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers, gc.IsNil)
}

func (s *migrateSuite) initializeMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.st = mocks.NewMockState(ctrl)
	s.agentService = mocks.NewMockModelAgentService(ctrl)
	return ctrl
}

func (s *migrateSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.initializeMocks(c)

	s.PatchValue(&upgradevalidation.SupportedJujuBases, func() []base.Base {
		return transform.Slice([]string{"ubuntu@24.04", "ubuntu@22.04", "ubuntu@20.04"}, base.MustParseBaseFromString)
	})

	// - check if the model has win machines;
	s.st.EXPECT().MachineCountForBase(makeBases("ubuntu", []string{"24.04/stable", "22.04/stable", "20.04/stable"})).Return(nil, nil)
	s.st.EXPECT().AllMachinesCount().Return(0, nil)

	return ctrl
}
