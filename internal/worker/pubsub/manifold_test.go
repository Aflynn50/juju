// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pubsub_test

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	psworker "github.com/juju/juju/internal/worker/pubsub"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	config psworker.ManifoldConfig
}

var _ = tc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = psworker.ManifoldConfig{
		AgentName:      "agent",
		CentralHubName: "central-hub",
		Clock:          testclock.NewClock(time.Now()),
	}
}

func (s *ManifoldSuite) manifold() dependency.Manifold {
	return psworker.Manifold(s.config)
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.manifold().Inputs, tc.DeepEquals, []string{"agent", "central-hub"})
}

func (s *ManifoldSuite) TestAgentMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestCentralHubMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent":       &fakeAgent{},
		"central-hub": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestAgentAPIInfoNotReady(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent":       &fakeAgent{missingAPIinfo: true},
		"central-hub": pubsub.NewStructuredHub(nil),
	})

	worker, err := s.manifold().Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestNewWorkerArgs(c *tc.C) {
	clock := s.config.Clock
	hub := pubsub.NewStructuredHub(nil)
	var config psworker.WorkerConfig
	s.config.NewWorker = func(c psworker.WorkerConfig) (worker.Worker, error) {
		config = c
		return &fakeWorker{}, nil
	}

	getter := dt.StubGetter(map[string]interface{}{
		"agent":       &fakeAgent{tag: names.NewMachineTag("42")},
		"central-hub": hub,
	})

	worker, err := s.manifold().Start(context.Background(), getter)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker, tc.NotNil)

	c.Check(config.Origin, tc.Equals, "machine-42")
	c.Check(config.Clock, tc.Equals, clock)
	c.Check(config.Hub, tc.Equals, hub)
	c.Check(config.APIInfo.CACert, tc.Equals, "fake as")
	c.Check(config.NewWriter, tc.NotNil)
}

type fakeWorker struct {
	worker.Worker
}

type fakeAgent struct {
	agent.Agent

	tag            names.Tag
	missingAPIinfo bool
}

type fakeConfig struct {
	agent.Config

	tag            names.Tag
	missingAPIinfo bool
}

func (f *fakeAgent) CurrentConfig() agent.Config {
	return &fakeConfig{tag: f.tag, missingAPIinfo: f.missingAPIinfo}
}

func (f *fakeConfig) APIInfo() (*api.Info, bool) {
	if f.missingAPIinfo {
		return nil, false
	}
	return &api.Info{
		CACert: "fake as",
		Tag:    f.tag,
	}, true
}

func (f *fakeConfig) Tag() names.Tag {
	return f.tag
}
