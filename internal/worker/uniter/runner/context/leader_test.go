// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
)

type LeaderSuite struct {
	testing.IsolationSuite
	testing.Stub
	tracker *StubTracker
	context context.LeadershipContext
}

var _ = tc.Suite(&LeaderSuite{})

func (s *LeaderSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.tracker = &StubTracker{
		Stub:            &s.Stub,
		applicationName: "led-application",
	}
	s.context = context.NewLeadershipContext(s.tracker)
}

func (s *LeaderSuite) CheckCalls(c *tc.C, stubCalls []testing.StubCall, f func()) {
	s.Stub = testing.Stub{}
	f()
	s.Stub.CheckCalls(c, stubCalls)
}

func (s *LeaderSuite) TestIsLeaderSuccess(c *tc.C) {
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The first call succeeds...
		s.tracker.results = []StubTicket{true}
		leader, err := s.context.IsLeader()
		c.Check(leader, jc.IsTrue)
		c.Check(err, jc.ErrorIsNil)
	})

	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// ...and so does the second.
		s.tracker.results = []StubTicket{true}
		leader, err := s.context.IsLeader()
		c.Check(leader, jc.IsTrue)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *LeaderSuite) TestIsLeaderFailure(c *tc.C) {
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The first call fails...
		s.tracker.results = []StubTicket{false}
		leader, err := s.context.IsLeader()
		c.Check(leader, jc.IsFalse)
		c.Check(err, jc.ErrorIsNil)
	})

	s.CheckCalls(c, nil, func() {
		// ...and the second doesn't even try.
		leader, err := s.context.IsLeader()
		c.Check(leader, jc.IsFalse)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *LeaderSuite) TestIsLeaderFailureAfterSuccess(c *tc.C) {
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The first call succeeds...
		s.tracker.results = []StubTicket{true}
		leader, err := s.context.IsLeader()
		c.Check(leader, jc.IsTrue)
		c.Check(err, jc.ErrorIsNil)
	})

	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The second fails...
		s.tracker.results = []StubTicket{false}
		leader, err := s.context.IsLeader()
		c.Check(leader, jc.IsFalse)
		c.Check(err, jc.ErrorIsNil)
	})

	s.CheckCalls(c, nil, func() {
		// The third doesn't even try.
		leader, err := s.context.IsLeader()
		c.Check(leader, jc.IsFalse)
		c.Check(err, jc.ErrorIsNil)
	})
}

type StubTracker struct {
	leadership.Tracker
	*testing.Stub
	applicationName string
	results         []StubTicket
}

func (stub *StubTracker) ApplicationName() string {
	stub.MethodCall(stub, "ApplicationName")
	return stub.applicationName
}

func (stub *StubTracker) ClaimLeader() (result leadership.Ticket) {
	stub.MethodCall(stub, "ClaimLeader")
	result, stub.results = stub.results[0], stub.results[1:]
	return result
}

type StubTicket bool

func (ticket StubTicket) Wait() bool {
	return bool(ticket)
}

func (ticket StubTicket) Ready() <-chan struct{} {
	return alwaysReady
}

var alwaysReady = make(chan struct{})

func init() {
	close(alwaysReady)
}
