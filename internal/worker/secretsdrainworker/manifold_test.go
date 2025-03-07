// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrainworker_test

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	dt "github.com/juju/worker/v3/dependency/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/internal/worker/secretsdrainworker"
	"github.com/juju/juju/internal/worker/secretsdrainworker/mocks"
	jujusecrets "github.com/juju/juju/secrets"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	config secretsdrainworker.ManifoldConfig
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig()
}

func (s *ManifoldSuite) validConfig() secretsdrainworker.ManifoldConfig {
	return secretsdrainworker.ManifoldConfig{
		APICallerName:         "api-caller",
		LeadershipTrackerName: "leadership-tracker",
		Logger:                loggo.GetLogger("test"),
		NewWorker: func(config secretsdrainworker.Config) (worker.Worker, error) {
			return nil, nil
		},
		NewSecretsDrainFacade: func(base.APICaller) secretsdrainworker.SecretsDrainFacade { return nil },
		NewBackendsClient: func(base.APICaller) (jujusecrets.BackendsClient, error) {
			return nil, nil
		},
	}
}

func (s *ManifoldSuite) TestValid(c *gc.C) {
	c.Check(s.config.Validate(), jc.ErrorIsNil)
}

func (s *ManifoldSuite) TestMissingAPICallerName(c *gc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *ManifoldSuite) TestMissingLogger(c *gc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *ManifoldSuite) TestMissingNewWorker(c *gc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *ManifoldSuite) TestMissingNewFacade(c *gc.C) {
	s.config.NewSecretsDrainFacade = nil
	s.checkNotValid(c, "nil NewSecretsDrainFacade not valid")
}

func (s *ManifoldSuite) TestMissingNewBackendsClient(c *gc.C) {
	s.config.NewBackendsClient = nil
	s.checkNotValid(c, "nil NewBackendsClient not valid")
}

func (s *ManifoldSuite) checkNotValid(c *gc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facade := mocks.NewMockSecretsDrainFacade(ctrl)
	s.config.NewSecretsDrainFacade = func(base.APICaller) secretsdrainworker.SecretsDrainFacade {
		return facade
	}

	backendClients := mocks.NewMockBackendsClient(ctrl)
	s.config.NewBackendsClient = func(base.APICaller) (jujusecrets.BackendsClient, error) {
		return backendClients, nil
	}

	called := false
	s.config.NewWorker = func(config secretsdrainworker.Config) (worker.Worker, error) {
		called = true
		mc := jc.NewMultiChecker()
		mc.AddExpr(`_.Facade`, gc.NotNil)
		mc.AddExpr(`_.Logger`, gc.NotNil)
		mc.AddExpr(`_.SecretsBackendGetter`, gc.NotNil)
		mc.AddExpr(`_.LeadershipTrackerFunc`, gc.NotNil)
		c.Check(config, mc, secretsdrainworker.Config{SecretsDrainFacade: facade})
		return nil, nil
	}
	manifold := secretsdrainworker.Manifold(s.config)
	w, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"api-caller":         struct{ base.APICaller }{&mockAPICaller{}},
		"leadership-tracker": struct{ leadership.TrackerWorker }{&mockLeadershipTracker{}},
	}))
	c.Assert(w, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *ManifoldSuite) TestStartNoLeadershipTracker(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facade := mocks.NewMockSecretsDrainFacade(ctrl)
	s.config.NewSecretsDrainFacade = func(base.APICaller) secretsdrainworker.SecretsDrainFacade {
		return facade
	}
	s.config.LeadershipTrackerName = ""

	backendClients := mocks.NewMockBackendsClient(ctrl)
	s.config.NewBackendsClient = func(base.APICaller) (jujusecrets.BackendsClient, error) {
		return backendClients, nil
	}

	called := false
	s.config.NewWorker = func(config secretsdrainworker.Config) (worker.Worker, error) {
		called = true
		mc := jc.NewMultiChecker()
		mc.AddExpr(`_.Facade`, gc.NotNil)
		mc.AddExpr(`_.Logger`, gc.NotNil)
		mc.AddExpr(`_.SecretsBackendGetter`, gc.NotNil)
		mc.AddExpr(`_.LeadershipTrackerFunc`, gc.NotNil)
		c.Check(config, mc, secretsdrainworker.Config{SecretsDrainFacade: facade})
		return nil, nil
	}
	manifold := secretsdrainworker.Manifold(s.config)
	w, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{&mockAPICaller{}},
	}))
	c.Assert(w, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

type mockAPICaller struct {
	base.APICaller
}

func (*mockAPICaller) BestFacadeVersion(facade string) int {
	return 1
}

type mockLeadershipTracker struct {
	leadership.TrackerWorker
}
