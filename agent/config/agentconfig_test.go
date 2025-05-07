// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/agent"
	coretesting "github.com/juju/juju/internal/testing"
)

type agentConfSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(&agentConfSuite{})

func (s *agentConfSuite) TestChangeConfigSuccess(c *tc.C) {
	mcsw := &mockConfigSetterWriter{}
	conf := NewAgentConfigWithConfigSetterWriter(c.MkDir(), mcsw)
	err := conf.ChangeConfig(func(agent.ConfigSetter) error {
		return nil
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcsw.WriteCalled, jc.IsTrue)
}

func (s *agentConfSuite) TestChangeConfigMutateFailure(c *tc.C) {
	mcsw := &mockConfigSetterWriter{}
	conf := NewAgentConfigWithConfigSetterWriter(c.MkDir(), mcsw)

	err := conf.ChangeConfig(func(agent.ConfigSetter) error {
		return errors.New("blam")
	})

	c.Assert(err, tc.ErrorMatches, "blam")
	c.Assert(mcsw.WriteCalled, jc.IsFalse)
}

func (s *agentConfSuite) TestChangeConfigWriteFailure(c *tc.C) {
	mcsw := &mockConfigSetterWriter{
		WriteError: errors.New("boom"),
	}
	conf := NewAgentConfigWithConfigSetterWriter(c.MkDir(), mcsw)
	err := conf.ChangeConfig(func(agent.ConfigSetter) error {
		return nil
	})

	c.Assert(err, tc.ErrorMatches, "cannot write agent configuration: boom")
	c.Assert(mcsw.WriteCalled, jc.IsTrue)
}

type readAgentConfigSuite struct {
	coretesting.BaseSuite

	agentConfigReader *MockAgentConfigReader
}

var _ = tc.Suite(&readAgentConfigSuite{})

func (s *readAgentConfigSuite) TestReadAgentConfigMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentConfigReader.EXPECT().ReadConfig("machine-0").Return(nil)

	err := ReadAgentConfig(s.agentConfigReader, "0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *readAgentConfigSuite) TestReadAgentConfigController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentConfigReader.EXPECT().ReadConfig("machine-0").Return(fmt.Errorf("boom"))
	s.agentConfigReader.EXPECT().ReadConfig("controller-0").Return(nil)

	err := ReadAgentConfig(s.agentConfigReader, "0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *readAgentConfigSuite) TestReadAgentConfigFallback(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentConfigReader.EXPECT().ReadConfig("machine-0").Return(fmt.Errorf("boom 1"))
	s.agentConfigReader.EXPECT().ReadConfig("controller-0").Return(fmt.Errorf("boom 2"))

	err := ReadAgentConfig(s.agentConfigReader, "0")
	c.Assert(err, tc.ErrorMatches, `reading agent config for "0": boom 1, boom 2`)
}

func (s *readAgentConfigSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agentConfigReader = NewMockAgentConfigReader(ctrl)

	return ctrl
}

type mockConfigSetterWriter struct {
	agent.ConfigSetterWriter
	WriteError  error
	WriteCalled bool
}

func (c *mockConfigSetterWriter) Write() error {
	c.WriteCalled = true
	return c.WriteError
}
