// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager_test

import (
	"context"
	"errors"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/diskmanager"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/blockdevice"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = gc.Suite(&DiskManagerSuite{})

type DiskManagerSuite struct {
	coretesting.BaseSuite
	resources          *common.Resources
	authorizer         *apiservertesting.FakeAuthorizer
	blockDeviceUpdater *mockBlockDeviceUpdater
	api                *diskmanager.DiskManagerAPI
}

func (s *DiskManagerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	tag := names.NewMachineTag("0")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	s.blockDeviceUpdater = &mockBlockDeviceUpdater{}
	s.api = diskmanager.NewDiskManagerAPIForTest(s.authorizer, s.blockDeviceUpdater)
}

func (s *DiskManagerSuite) TestSetMachineBlockDevices(c *gc.C) {
	devices := []params.BlockDevice{{DeviceName: "sda"}, {DeviceName: "sdb"}}
	results, err := s.api.SetMachineBlockDevices(context.Background(), params.SetMachineBlockDevices{
		MachineBlockDevices: []params.MachineBlockDevices{{
			Machine:      "machine-0",
			BlockDevices: devices,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesEmptyArgs(c *gc.C) {
	results, err := s.api.SetMachineBlockDevices(context.Background(), params.SetMachineBlockDevices{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *DiskManagerSuite) TestNewDiskManagerAPINonMachine(c *gc.C) {
	tag := names.NewUnitTag("mysql/0")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	_, err := diskmanager.NewDiskManagerAPI(facadetest.ModelContext{
		Auth_: s.authorizer,
	})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesInvalidTags(c *gc.C) {
	results, err := s.api.SetMachineBlockDevices(context.Background(), params.SetMachineBlockDevices{
		MachineBlockDevices: []params.MachineBlockDevices{{
			Machine: "machine-0",
		}, {
			Machine: "machine-1",
		}, {
			Machine: "unit-mysql-0",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: nil,
		}, {
			Error: &params.Error{Message: "permission denied", Code: "unauthorized access"},
		}, {
			Error: &params.Error{Message: "permission denied", Code: "unauthorized access"},
		}},
	})
	c.Assert(s.blockDeviceUpdater.calls, gc.Equals, 1)
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesStateError(c *gc.C) {
	s.blockDeviceUpdater.err = errors.New("boom")
	results, err := s.api.SetMachineBlockDevices(context.Background(), params.SetMachineBlockDevices{
		MachineBlockDevices: []params.MachineBlockDevices{{
			Machine: "machine-0",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{Message: "boom", Code: ""},
		}},
	})
}

type mockBlockDeviceUpdater struct {
	calls   int
	devices map[string][]blockdevice.BlockDevice
	err     error
}

func (st *mockBlockDeviceUpdater) UpdateBlockDevices(_ context.Context, machineId string, devices ...blockdevice.BlockDevice) error {
	st.calls++
	if st.devices == nil {
		st.devices = make(map[string][]blockdevice.BlockDevice)
	}
	st.devices[machineId] = devices
	return st.err
}
