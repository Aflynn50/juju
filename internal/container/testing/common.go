// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"os"

	"github.com/juju/tc"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
	jujutesting "github.com/juju/juju/juju/testing"
)

func MockMachineConfig(machineId string) (*instancecfg.InstanceConfig, error) {

	apiInfo := jujutesting.FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(testing.ControllerTag, machineId, "fake-nonce",
		imagemetadata.ReleasedStream, corebase.MakeDefaultBase("ubuntu", "22.04"), apiInfo)
	if err != nil {
		return nil, err
	}
	err = instanceConfig.SetTools(tools.List{
		&tools.Tools{
			Version: semversion.MustParseBinary("2.5.2-ubuntu-amd64"),
			URL:     "http://tools.testing.invalid/2.5.2-ubuntu-amd64.tgz",
		},
	})
	if err != nil {
		return nil, err
	}

	return instanceConfig, nil
}

func CreateContainer(c *tc.C, manager container.Manager, machineId string) instances.Instance {
	instanceConfig, err := MockMachineConfig(machineId)
	c.Assert(err, tc.ErrorIsNil)
	return CreateContainerWithMachineConfig(c, manager, instanceConfig)
}

func CreateContainerWithMachineConfig(
	c *tc.C,
	manager container.Manager,
	instanceConfig *instancecfg.InstanceConfig,
) instances.Instance {

	networkConfig := container.BridgeNetworkConfig(0, nil)
	storageConfig := &container.StorageConfig{}
	return CreateContainerWithMachineAndNetworkAndStorageConfig(c, manager, instanceConfig, networkConfig, storageConfig)
}

func CreateContainerWithMachineAndNetworkAndStorageConfig(
	c *tc.C,
	manager container.Manager,
	instanceConfig *instancecfg.InstanceConfig,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
) instances.Instance {
	callback := func(ctx context.Context, settableStatus status.Status, info string, data map[string]interface{}) error {
		return nil
	}
	inst, hardware, err := manager.CreateContainer(
		context.Background(),
		instanceConfig, constraints.Value{}, corebase.MakeDefaultBase("ubuntu", "18.04"),
		networkConfig, storageConfig, callback)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hardware, tc.NotNil)
	c.Assert(hardware.String(), tc.Not(tc.Equals), "")
	return inst
}

func AssertCloudInit(c *tc.C, filename string) []byte {
	c.Assert(filename, tc.IsNonEmptyFile)
	data, err := os.ReadFile(filename)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.HasPrefix, "#cloud-config\n")
	return data
}
