// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package manager_test

import (
	"os"
	"os/exec"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/packaging/commands"
	"github.com/juju/juju/internal/packaging/manager"
)

var _ = gc.Suite(&ManagerSuite{})

type ManagerSuite struct {
	apt, snap manager.PackageManager
	testing.IsolationSuite
	calledCommand string
}

func (s *ManagerSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
	s.apt = manager.NewAptPackageManager()
	s.snap = manager.NewSnapPackageManager()
}

func (s *ManagerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *ManagerSuite) TearDownTest(c *gc.C) {
	s.IsolationSuite.TearDownTest(c)
}

func (s *ManagerSuite) TearDownSuite(c *gc.C) {
	s.IsolationSuite.TearDownSuite(c)
}

var (
	// aptCmder is the commands.PackageCommander for apt-based
	// systems whose commands will be checked against.
	aptCmder = commands.NewAptPackageCommander()

	// snapCmder is the commands.PackageCommander for snap-based
	// systems whose commands will be checked against.
	snapCmder = commands.NewSnapPackageCommander()

	// testedPackageName is the package name used in all
	// single-package testing scenarios.
	testedPackageName = "test-package"

	// testedPackageNames is a list of package names used in all
	// multiple-package testing scenario's.
	testedPackageNames = []string{
		"first-test-package",
		"second-test-package",
		"third-test-package",
	}
)

// getMockRunCommandWithRetry returns a function with the same signature as
// RunCommandWithRetry which saves the command it receives in the provided
// string whilst always returning no output, 0 error code and nil error.
func getMockRunCommandWithRetry(stor *string) func(string, manager.Retryable, manager.RetryPolicy) (string, int, error) {
	return func(cmd string, _ manager.Retryable, _ manager.RetryPolicy) (string, int, error) {
		*stor = cmd
		return "", 0, nil
	}
}

// getMockRunCommand returns a function with the same signature as RunCommand
// which saves the command it receives in the provided string whilst always
// returning empty output and no error.
func getMockRunCommand(stor *string) func(string, ...string) (string, error) {
	return func(cmd string, args ...string) (string, error) {
		*stor = strings.Join(append([]string{cmd}, args...), " ")
		return "", nil
	}
}

// simpleTestCase is a struct containing all the information required for
// running a simple error/no error test case.
type simpleTestCase struct {
	// description of the test:
	desc string

	// the expected apt command which will get executed:
	expectedAptCmd string

	// the expected result of the given apt operation:
	expectedAptResult interface{}

	// the expected snap command which will get executed:
	expectedSnapCmd string

	// the expected result of the given snap operation:
	expectedSnapResult interface{}

	// the function to be applied on the package manager.
	// returns the result of the operation and the error.
	operation func(manager.PackageManager) (interface{}, error)
}

var simpleTestCases = []*simpleTestCase{
	{
		"Test install packages.",
		aptCmder.InstallCmd(testedPackageNames...),
		nil,
		snapCmder.InstallCmd(testedPackageNames...),
		nil,
		func(pacman manager.PackageManager) (interface{}, error) {
			return nil, pacman.Install(testedPackageNames...)
		},
	},
}

func (s *ManagerSuite) TestSimpleCases(c *gc.C) {
	s.PatchValue(&manager.RunCommand, getMockRunCommand(&s.calledCommand))
	s.PatchValue(&manager.RunCommandWithRetry, getMockRunCommandWithRetry(&s.calledCommand))

	for i, testCase := range simpleTestCases {
		c.Logf("Simple test %d: %s", i+1, testCase.desc)

		// run for the apt PackageManager implementation:
		res, err := testCase.operation(s.apt)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(s.calledCommand, gc.Equals, testCase.expectedAptCmd)
		c.Assert(res, jc.DeepEquals, testCase.expectedAptResult)

		// run for the snap PackageManager implementation.
		res, err = testCase.operation(s.snap)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(s.calledCommand, gc.Equals, testCase.expectedSnapCmd)
		c.Assert(res, jc.DeepEquals, testCase.expectedSnapResult)
	}
}

func (s *ManagerSuite) TestSimpleErrorCases(c *gc.C) {
	const (
		expectedErr    = "packaging command failed: exit status 0"
		expectedErrMsg = `E: I done failed :(`
	)
	state := os.ProcessState{}
	cmdError := &exec.ExitError{ProcessState: &state}

	cmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte(expectedErrMsg), error(cmdError))

	for i, testCase := range simpleTestCases {
		c.Logf("Error'd test %d: %s", i+1, testCase.desc)
		s.PatchValue(&manager.ProcessStateSys, func(*os.ProcessState) interface{} {
			return mockExitStatuser(i + 1)
		})

		// run for the apt PackageManager implementation:
		_, err := testCase.operation(s.apt)
		c.Assert(err, gc.ErrorMatches, expectedErr)

		cmd := <-cmdChan
		c.Assert(strings.Join(cmd.Args, " "), gc.DeepEquals, testCase.expectedAptCmd)

		// run for the snap PackageManager implementation:
		_, err = testCase.operation(s.snap)
		c.Assert(err, gc.ErrorMatches, expectedErr)

		cmd = <-cmdChan
		c.Assert(strings.Join(cmd.Args, " "), gc.DeepEquals, testCase.expectedSnapCmd)
	}
}
