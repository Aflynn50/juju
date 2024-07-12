// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiservercertwatcher_test

import (
	"context"
	"sync"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/pki"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/apiservercertwatcher"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	manifold dependency.Manifold
	getter   dependency.Getter
	agent    *mockAgent
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.agent = &mockAgent{
		conf: mockConfig{
			caCert: coretesting.OtherCACert,
			info: &controller.StateServingInfo{
				CAPrivateKey: coretesting.OtherCAKey,
				Cert:         coretesting.ServerCert,
				PrivateKey:   coretesting.ServerKey,
			},
		},
	}
	s.getter = dt.StubGetter(map[string]interface{}{
		"agent": s.agent,
	})
	s.manifold = apiservercertwatcher.Manifold(apiservercertwatcher.ManifoldConfig{
		AgentName: "agent",
	})
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, []string{"agent"})
}

func (s *ManifoldSuite) TestNoAgent(c *gc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent": dependency.ErrMissing,
	})
	_, err := s.manifold.Start(context.Background(), getter)
	c.Assert(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestNoStateServingInfo(c *gc.C) {
	s.agent.conf.info = nil
	_, err := s.manifold.Start(context.Background(), s.getter)
	c.Assert(err, gc.ErrorMatches, "setting up initial ca authority: no state serving info in agent config")
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)
}

func (s *ManifoldSuite) TestOutput(c *gc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	var authority pki.Authority
	err := s.manifold.Output(w, &authority)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(context.Background(), s.getter)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}

type mockAgent struct {
	agent.Agent
	conf mockConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

type mockConfig struct {
	agent.Config

	mu     sync.Mutex
	info   *controller.StateServingInfo
	caCert string
}

func (mc *mockConfig) CACert() string {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.caCert
}

func (mc *mockConfig) StateServingInfo() (controller.StateServingInfo, bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if mc.info != nil {
		return *mc.info, true
	}
	return controller.StateServingInfo{}, false
}
