// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"strconv"
	"strings"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type AddMachineSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fakeAddMachine *fakeAddMachineAPI
}

var _ = gc.Suite(&AddMachineSuite{})

func (s *AddMachineSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fakeAddMachine = &fakeAddMachineAPI{}
}

func (s *AddMachineSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		base        string
		constraints string
		placement   string
		count       int
		errorString string
	}{
		{
			count: 1,
		}, {
			args:  []string{"--base", "some-series"},
			count: 1,
			base:  "some-series",
		}, {
			args:  []string{"-n", "2"},
			count: 2,
		}, {
			args:      []string{"lxd"},
			count:     1,
			placement: "lxd:",
		}, {
			args:      []string{"lxd", "-n", "2"},
			count:     2,
			placement: "lxd:",
		}, {
			args:      []string{"lxd:4"},
			count:     1,
			placement: "lxd:4",
		}, {
			args:      []string{"ssh:user@10.10.0.3"},
			count:     1,
			placement: "ssh:user@10.10.0.3",
		}, {
			args:      []string{"winrm:user@10.10.0.3"},
			count:     1,
			placement: "winrm:user@10.10.0.3",
		}, {
			args:      []string{"ssh:user@10.10.0.3", "--private-key", "pv"},
			count:     1,
			placement: "ssh:user@10.10.0.3",
		}, {
			args:      []string{"ssh:user@10.10.0.3", "--public-key", "pb"},
			count:     1,
			placement: "ssh:user@10.10.0.3",
		}, {
			args:      []string{"ssh:user@10.10.0.3", "--private-key", "pv", "--public-key", "pb"},
			count:     1,
			placement: "ssh:user@10.10.0.3",
		}, {
			args:      []string{"zone=us-east-1a"},
			count:     1,
			placement: "model-uuid:zone=us-east-1a",
		}, {
			args:      []string{"anything-here"},
			count:     1,
			placement: "model-uuid:anything-here",
		}, {
			args:        []string{"anything", "else"},
			errorString: `unrecognized args: \["else"\]`,
		}, {
			args:      []string{"something:special"},
			count:     1,
			placement: "something:special",
		},
	} {
		c.Logf("test %d", i)
		wrappedCommand, addCmd := machine.NewAddCommandForTest(s.fakeAddMachine, s.fakeAddMachine)
		err := cmdtesting.InitCommand(wrappedCommand, test.args)
		if test.errorString == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(addCmd.Base, gc.Equals, test.base)
			c.Check(addCmd.Constraints.String(), gc.Equals, test.constraints)
			if addCmd.Placement != nil {
				c.Check(addCmd.Placement.String(), gc.Equals, test.placement)
			} else {
				c.Check("", gc.Equals, test.placement)
			}
			c.Check(addCmd.NumMachines, gc.Equals, test.count)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *AddMachineSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	add, _ := machine.NewAddCommandForTest(s.fakeAddMachine, s.fakeAddMachine)
	return cmdtesting.RunCommand(c, add, args...)
}

func (s *AddMachineSuite) TestSeriesAndBaseError(c *gc.C) {
	_, err := s.run(c, "--series=jammy", "--base=ubuntu@22.04")
	c.Assert(err, gc.ErrorMatches, "--series and --base cannot be specified together")
}

func (s *AddMachineSuite) TestAddMachine(c *gc.C) {
	context, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "created machine 0\n")

	c.Assert(s.fakeAddMachine.args, gc.HasLen, 1)
	c.Assert(s.fakeAddMachine.args[0].Jobs, jc.DeepEquals, []model.MachineJob{model.JobHostUnits})
}

func (s *AddMachineSuite) TestAddMachineUnauthorizedMentionsJujuGrant(c *gc.C) {
	s.fakeAddMachine.addModelGetError = &params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	}
	ctx, _ := s.run(c)
	errString := strings.Replace(cmdtesting.Stderr(ctx), "\n", " ", -1)
	c.Assert(errString, gc.Matches, `.*juju grant.*`)
}

func (s *AddMachineSuite) TestSSHPlacement(c *gc.C) {
	s.PatchValue(machine.SSHProvisioner, func(args manual.ProvisionMachineArgs) (string, error) {
		return "42", nil
	})
	context, err := s.run(c, "ssh:10.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "created machine 42\n")
}

func (s *AddMachineSuite) TestSSHPlacementError(c *gc.C) {
	s.PatchValue(machine.SSHProvisioner, func(args manual.ProvisionMachineArgs) (string, error) {
		return "", errors.New("failed to initialize warp core")
	})
	context, err := s.run(c, "ssh:10.1.2.3")
	c.Assert(err, gc.ErrorMatches, "failed to initialize warp core")
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "")
}

func (s *AddMachineSuite) TestParamsPassedOn(c *gc.C) {
	_, err := s.run(c, "--constraints", "mem=8G", "--base=ubuntu@22.04", "zone=nz")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeAddMachine.args, gc.HasLen, 1)

	param := s.fakeAddMachine.args[0]

	c.Assert(param.Placement.String(), gc.Equals, "fake-uuid:zone=nz")
	c.Assert(param.Base, jc.DeepEquals, &params.Base{Name: "ubuntu", Channel: "22.04/stable"})
	c.Assert(param.Constraints.String(), gc.Equals, "mem=8192M")
}

func (s *AddMachineSuite) TestParamsPassedOnNTimes(c *gc.C) {
	_, err := s.run(c, "-n", "3", "--constraints", "mem=8G", "--base=ubuntu@22.04")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeAddMachine.args, gc.HasLen, 3)

	param := s.fakeAddMachine.args[0]
	c.Assert(param.Base, jc.DeepEquals, &params.Base{Name: "ubuntu", Channel: "22.04/stable"})

	c.Assert(param.Constraints.String(), gc.Equals, "mem=8192M")
	c.Assert(param, jc.DeepEquals, s.fakeAddMachine.args[1])
	c.Assert(param, jc.DeepEquals, s.fakeAddMachine.args[2])
}

func (s *AddMachineSuite) TestAddThreeMachinesWithTwoFailures(c *gc.C) {
	s.fakeAddMachine.successOrder = []bool{true, false, false}
	expectedOutput := `created machine 0
failed to create 2 machines
`
	context, err := s.run(c, "-n", "3")
	c.Assert(err, gc.ErrorMatches, "something went wrong, something went wrong")
	c.Assert(cmdtesting.Stderr(context), gc.Equals, expectedOutput)
}

func (s *AddMachineSuite) TestBlockedError(c *gc.C) {
	s.fakeAddMachine.addError = apiservererrors.OperationBlockedError("TestBlockedError")
	_, err := s.run(c)
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockedError.*")
}

func (s *AddMachineSuite) TestAddMachineWithDisks(c *gc.C) {
	_, err := s.run(c, "--disks", "2,1G", "--disks", "2G")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeAddMachine.args, gc.HasLen, 1)
	param := s.fakeAddMachine.args[0]
	c.Assert(param.Disks, gc.DeepEquals, []storage.Constraints{
		{Size: 1024, Count: 2},
		{Size: 2048, Count: 1},
	})
}

type fakeAddMachineAPI struct {
	successOrder     []bool
	currentOp        int
	args             []params.AddMachineParams
	addError         error
	addModelGetError error
	providerType     string
}

func (f *fakeAddMachineAPI) Close() error {
	return nil
}

func (f *fakeAddMachineAPI) ModelUUID() (string, bool) {
	return "fake-uuid", true
}

func (f *fakeAddMachineAPI) AddMachines(args []params.AddMachineParams) ([]params.AddMachinesResult, error) {
	if f.addError != nil {
		return nil, f.addError
	}
	results := []params.AddMachinesResult{}
	for i := range args {
		f.args = append(f.args, args[i])
		if i >= len(f.successOrder) || f.successOrder[i] {
			results = append(results, params.AddMachinesResult{
				Machine: strconv.Itoa(i),
				Error:   nil,
			})
		} else {
			results = append(results, params.AddMachinesResult{
				Machine: string(rune(i)),
				Error:   &params.Error{Message: "something went wrong", Code: "1"},
			})
		}
		f.currentOp++
	}
	return results, nil
}

func (f *fakeAddMachineAPI) DestroyMachinesWithParams(force, keep, dryRun bool, maxWait *time.Duration, machines ...string) ([]params.DestroyMachineResult, error) {
	return nil, errors.NotImplementedf("ForceDestroyMachinesWithParams")
}

func (f *fakeAddMachineAPI) ProvisioningScript(params.ProvisioningScriptParams) (script string, err error) {
	return "", errors.NotImplementedf("ProvisioningScript")
}

func (f *fakeAddMachineAPI) ModelGet() (map[string]interface{}, error) {
	if f.addModelGetError != nil {
		return nil, f.addModelGetError
	}
	providerType := "dummy"
	if f.providerType != "" {
		providerType = f.providerType
	}
	return dummy.SampleConfig().Merge(map[string]interface{}{
		"type": providerType,
	}), nil
}
