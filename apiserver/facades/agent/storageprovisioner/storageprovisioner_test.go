// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/storageprovisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type provisionerSuite struct {
	jujutesting.ApiServerSuite

	storageSetUp

	st             *state.State
	resources      *common.Resources
	authorizer     *apiservertesting.FakeAuthorizer
	api            *storageprovisioner.StorageProvisionerAPIv4
	storageBackend storageprovisioner.StorageBackend
}

func (s *provisionerSuite) SetUpTest(c *tc.C) {
	s.ApiServerSuite.SetUpTest(c)
}

func (s *provisionerSuite) TestNewStorageProvisionerAPINonMachine(c *tc.C) {
	tag := names.NewUnitTag("mysql/0")
	authorizer := &apiservertesting.FakeAuthorizer{Tag: tag}
	backend, storageBackend, err := storageprovisioner.NewStateBackends(s.st)
	c.Assert(err, jc.ErrorIsNil)

	modelInfo, err := s.ControllerDomainServices(c).ModelInfo().GetModelInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	_, err = storageprovisioner.NewStorageProvisionerAPIv4(
		context.Background(),
		nil,
		clock.WallClock,
		backend,
		storageBackend,
		s.DefaultModelDomainServices(c).BlockDevice(),
		s.ControllerDomainServices(c).Config(),
		s.DefaultModelDomainServices(c).Machine(),
		common.NewResources(),
		authorizer,
		nil, nil,
		loggertesting.WrapCheckLog(c),
		modelInfo.UUID,
		testing.ControllerTag.Id(),
	)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *provisionerSuite) TestVolumesEmptyArgs(c *tc.C) {
	results, err := s.api.Volumes(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 0)
}

func (s *provisionerSuite) TestVolumeParamsEmptyArgs(c *tc.C) {
	results, err := s.api.VolumeParams(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 0)
}
