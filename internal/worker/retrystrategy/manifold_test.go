// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/retrystrategy"
	"github.com/juju/juju/rpc/params"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	getter     dependency.Getter
	fakeAgent  agent.Agent
	fakeCaller base.APICaller
	fakeFacade retrystrategy.Facade
	fakeWorker worker.Worker
	newFacade  func(retrystrategy.Facade) func(base.APICaller) retrystrategy.Facade
	newWorker  func(worker.Worker, error) func(retrystrategy.WorkerConfig) (worker.Worker, error)
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
	s.fakeAgent = &fakeAgent{}
	s.fakeCaller = &fakeCaller{}
	s.getter = dt.StubGetter(map[string]interface{}{
		"agent":      s.fakeAgent,
		"api-caller": s.fakeCaller,
	})
	s.newFacade = func(facade retrystrategy.Facade) func(base.APICaller) retrystrategy.Facade {
		s.fakeFacade = facade
		return func(apiCaller base.APICaller) retrystrategy.Facade {
			c.Assert(apiCaller, gc.Equals, s.fakeCaller)
			return facade
		}
	}
	s.newWorker = func(w worker.Worker, err error) func(retrystrategy.WorkerConfig) (worker.Worker, error) {
		s.fakeWorker = w
		return func(wc retrystrategy.WorkerConfig) (worker.Worker, error) {
			c.Assert(wc.Facade, gc.Equals, s.fakeFacade)
			c.Assert(wc.AgentTag, gc.Equals, fakeTag)
			c.Assert(wc.RetryStrategy, gc.Equals, fakeStrategy)
			return w, err
		}
	}
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentName:     "wut",
		APICallerName: "exactly",
	})
	c.Check(manifold.Inputs, jc.DeepEquals, []string{"wut", "exactly"})
}

func (s *ManifoldSuite) TestStartMissingAgent(c *gc.C) {
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
	})
	getter := dt.StubGetter(map[string]interface{}{
		"agent":      dependency.ErrMissing,
		"api-caller": s.fakeCaller,
	})

	w, err := manifold.Start(context.Background(), getter)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Assert(w, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingAPI(c *gc.C) {
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
	})
	getter := dt.StubGetter(map[string]interface{}{
		"agent":      s.fakeAgent,
		"api-caller": dependency.ErrMissing,
	})

	w, err := manifold.Start(context.Background(), getter)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Assert(w, gc.IsNil)
}

func (s *ManifoldSuite) TestStartFacadeValueError(c *gc.C) {
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
		NewFacade:     s.newFacade(&fakeFacadeErr{err: errors.New("blop")}),
	})

	w, err := manifold.Start(context.Background(), s.getter)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "blop")
	c.Assert(w, gc.IsNil)
}

func (s *ManifoldSuite) TestStartWorkerError(c *gc.C) {
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
		NewFacade:     s.newFacade(&fakeFacade{}),
		NewWorker:     s.newWorker(nil, errors.New("blam")),
	})

	w, err := manifold.Start(context.Background(), s.getter)
	c.Assert(err, gc.ErrorMatches, "blam")
	c.Assert(w, gc.IsNil)
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	fakeWorker := &fakeWorker{}
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
		NewFacade:     s.newFacade(&fakeFacade{}),
		NewWorker:     s.newWorker(fakeWorker, nil),
	})

	w, err := manifold.Start(context.Background(), s.getter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.Equals, fakeWorker)
}

func (s *ManifoldSuite) TestOutputSuccess(c *gc.C) {
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
		NewFacade:     s.newFacade(&fakeFacade{}),
		NewWorker:     retrystrategy.NewRetryStrategyWorker,
	})

	w, err := manifold.Start(context.Background(), s.getter)
	s.AddCleanup(func(c *gc.C) { w.Kill() })
	c.Assert(err, jc.ErrorIsNil)

	var out params.RetryStrategy
	err = manifold.Output(w, &out)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Equals, fakeStrategy)
}

func (s *ManifoldSuite) TestOutputBadInput(c *gc.C) {
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
		NewFacade:     s.newFacade(&fakeFacade{}),
		NewWorker:     s.newWorker(&fakeWorker{}, nil),
	})

	w, err := manifold.Start(context.Background(), s.getter)
	c.Assert(err, jc.ErrorIsNil)

	var out params.RetryStrategy
	err = manifold.Output(w, &out)
	c.Assert(out, gc.Equals, params.RetryStrategy{})
	c.Assert(err.Error(), gc.Equals, "in should be a *retryStrategyWorker; is *retrystrategy_test.fakeWorker")
}

func (s *ManifoldSuite) TestOutputBadTarget(c *gc.C) {
	manifold := retrystrategy.Manifold(retrystrategy.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
		NewFacade:     s.newFacade(&fakeFacade{}),
		NewWorker:     retrystrategy.NewRetryStrategyWorker,
	})

	w, err := manifold.Start(context.Background(), s.getter)
	s.AddCleanup(func(c *gc.C) { w.Kill() })
	c.Assert(err, jc.ErrorIsNil)

	var out interface{}
	err = manifold.Output(w, &out)
	c.Assert(err.Error(), gc.Equals, "out should be a *params.RetryStrategy; is *interface {}")
}

var fakeTag = names.NewUnitTag("whatatag/0")

var fakeStrategy = params.RetryStrategy{
	ShouldRetry:  false,
	MinRetryTime: 2 * time.Second,
}

type fakeAgent struct {
	agent.Agent
}

func (mock *fakeAgent) CurrentConfig() agent.Config {
	return &fakeConfig{}
}

type fakeConfig struct {
	agent.Config
}

func (mock *fakeConfig) Tag() names.Tag {
	return fakeTag
}

type fakeCaller struct {
	base.APICaller
}

type fakeFacade struct {
	retrystrategy.Facade
}

func (mock *fakeFacade) RetryStrategy(ctx context.Context, agentTag names.Tag) (params.RetryStrategy, error) {
	return fakeStrategy, nil
}

func (mock *fakeFacade) WatchRetryStrategy(ctx context.Context, agentTag names.Tag) (watcher.NotifyWatcher, error) {
	return newStubWatcher(), nil
}

type fakeFacadeErr struct {
	retrystrategy.Facade
	err error
}

func (mock *fakeFacadeErr) RetryStrategy(ctx context.Context, agentTag names.Tag) (params.RetryStrategy, error) {
	return params.RetryStrategy{}, mock.err
}

type fakeWorker struct {
	worker.Worker
}
