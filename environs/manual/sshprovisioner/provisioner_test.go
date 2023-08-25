// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package sshprovisioner_test

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3/shell"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type provisionerSuite struct {
	jujutesting.LoggingCleanupSuite
}

var _ = gc.Suite(&provisionerSuite{})

type mockMachineManager struct {
	manual.ProvisioningClientAPI
}

func (m *mockMachineManager) ProvisioningScript(params.ProvisioningScriptParams) (script string, err error) {
	return "echo hello", nil
}

func (m *mockMachineManager) AddMachines(args []params.AddMachineParams) ([]params.AddMachinesResult, error) {
	if len(args) != 1 {
		return nil, errors.Errorf("unexpected args: %+v", args)
	}
	a := args[0]
	b := jujuversion.DefaultSupportedLTSBase()
	if a.Base == nil || a.Base.Name != b.OS || a.Base.Channel != b.Channel.String() {
		return nil, errors.Errorf("unexpected base: %v", a.Base)
	}
	if a.Nonce == "" {
		return nil, errors.Errorf("unexpected empty nonce")
	}
	if len(a.Jobs) != 1 && a.Jobs[0] != model.JobHostUnits {
		return nil, errors.Errorf("unexpected jobs: %v", a.Jobs)
	}
	return []params.AddMachinesResult{{
		Machine: "2",
	}}, nil
}

func (m *mockMachineManager) DestroyMachinesWithParams(force, keep, dryRun bool, maxWait *time.Duration, machines ...string) ([]params.DestroyMachineResult, error) {
	if len(machines) == 0 || machines[0] != "2" {
		return nil, errors.Errorf("unexpected machines: %v", machines)
	}
	return []params.DestroyMachineResult{{
		Info: &params.DestroyMachineInfo{MachineId: machines[0]},
	}}, nil
}

func (s *provisionerSuite) getArgs(c *gc.C) manual.ProvisionMachineArgs {
	hostname, err := os.Hostname()
	c.Assert(err, jc.ErrorIsNil)
	client := &mockMachineManager{}
	return manual.ProvisionMachineArgs{
		Host:           hostname,
		Client:         client,
		UpdateBehavior: &params.UpdateBehavior{true, true},
	}
}

func (s *provisionerSuite) TestProvisionMachine(c *gc.C) {
	var series = jujuversion.DefaultSupportedLTS()

	args := s.getArgs(c)
	hostname := args.Host
	args.Host = hostname
	args.User = "ubuntu"

	defer fakeSSH{
		Series:             series,
		Arch:               arch.AMD64,
		InitUbuntuUser:     true,
		SkipProvisionAgent: true,
	}.install(c).Restore()

	for i, errorCode := range []int{255, 0} {
		c.Logf("test %d: code %d", i, errorCode)
		defer fakeSSH{
			Series:                 series,
			Arch:                   arch.AMD64,
			InitUbuntuUser:         true,
			ProvisionAgentExitCode: errorCode,
		}.install(c).Restore()
		machineId, err := sshprovisioner.ProvisionMachine(args)
		if errorCode != 0 {
			c.Assert(err, gc.ErrorMatches, fmt.Sprintf("subprocess encountered error code %d", errorCode))
			c.Assert(machineId, gc.Equals, "")
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(machineId, gc.Not(gc.Equals), "")
			// machine ID will be incremented. Even though we failed and the
			// machine is removed, the ID is not reused.
			c.Assert(machineId, gc.Equals, fmt.Sprint(i+1))
		}
	}

	// Attempting to provision a machine twice should fail. We effect
	// this by checking for existing juju systemd configurations.
	defer fakeSSH{
		Provisioned:        true,
		InitUbuntuUser:     true,
		SkipDetection:      true,
		SkipProvisionAgent: true,
	}.install(c).Restore()
	_, err := sshprovisioner.ProvisionMachine(args)
	c.Assert(err, gc.Equals, manual.ErrProvisioned)
	defer fakeSSH{
		Provisioned:              true,
		CheckProvisionedExitCode: 255,
		InitUbuntuUser:           true,
		SkipDetection:            true,
		SkipProvisionAgent:       true,
	}.install(c).Restore()
	_, err = sshprovisioner.ProvisionMachine(args)
	c.Assert(err, gc.ErrorMatches, "error checking if provisioned: subprocess encountered error code 255")
}

func (s *provisionerSuite) TestProvisioningScript(c *gc.C) {
	defer fakeSSH{
		Series:         jujuversion.DefaultSupportedLTS(),
		Arch:           arch.AMD64,
		InitUbuntuUser: true,
	}.install(c).Restore()

	logDir := "/var/log"
	icfg := &instancecfg.InstanceConfig{
		ControllerTag: testing.ControllerTag,
		MachineId:     "10",
		MachineNonce:  "5432",
		Base:          base.MustParseBaseFromString("ubuntu@22.04"),
		APIInfo: &api.Info{
			Addrs:    []string{"127.0.0.1:1234"},
			Password: "pw2",
			CACert:   "CA CERT\n" + testing.CACert,
			Tag:      names.NewMachineTag("10"),
			ModelTag: testing.ModelTag,
		},
		DataDir:                 c.MkDir(),
		LogDir:                  path.Join(logDir, "juju"),
		MetricsSpoolDir:         c.MkDir(),
		Jobs:                    []model.MachineJob{model.JobManageModel, model.JobHostUnits},
		CloudInitOutputLog:      path.Join(logDir, "cloud-init-output.log"),
		AgentEnvironment:        map[string]string{agent.ProviderType: "dummy"},
		AuthorizedKeys:          "wheredidileavemykeys",
		MachineAgentServiceName: "jujud-machine-10",
	}
	tools := []*coretools.Tools{{
		Version: version.MustParseBinary("6.6.6-ubuntu-amd64"),
		URL:     "https://example.org",
	}}
	err := icfg.SetTools(tools)
	c.Assert(err, jc.ErrorIsNil)

	script, err := sshprovisioner.ProvisioningScript(icfg)
	c.Assert(err, jc.ErrorIsNil)

	cloudcfg, err := cloudinit.New("ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	udata, err := cloudconfig.NewUserdataConfig(icfg, cloudcfg)
	c.Assert(err, jc.ErrorIsNil)
	err = udata.ConfigureJuju()
	c.Assert(err, jc.ErrorIsNil)
	cloudcfg.SetSystemUpgrade(false)
	provisioningScript, err := cloudcfg.RenderScript()
	c.Assert(err, jc.ErrorIsNil)

	removeLogFile := "rm -f '/var/log/cloud-init-output.log'\n"
	expectedScript := removeLogFile + shell.DumpFileOnErrorScript("/var/log/cloud-init-output.log") + provisioningScript
	c.Assert(script, gc.Equals, expectedScript)
}
