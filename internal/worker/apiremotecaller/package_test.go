// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"testing"
	time "time"

	jujutesting "github.com/juju/testing"
	"go.uber.org/goleak"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package apiremotecaller -destination package_mocks_test.go github.com/juju/juju/internal/worker/apiremotecaller RemoteServer
//go:generate go run go.uber.org/mock/mockgen -typed -package apiremotecaller -destination clock_mocks_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package apiremotecaller -destination connection_mocks_test.go github.com/juju/juju/api Connection

func TestPackage(t *testing.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	clock      *MockClock
	remote     *MockRemoteServer
	connection *MockConnection

	states chan string
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.remote = NewMockRemoteServer(ctrl)
	s.connection = NewMockConnection(ctrl)

	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	return ctrl
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().DoAndReturn(func() time.Time {
		return time.Now()
	}).AnyTimes()
}

func (s *baseSuite) ensureStartup(c *gc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, stateStarted)
	case <-time.After(jujutesting.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *baseSuite) ensureChanged(c *gc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, stateChanged)
	case <-time.After(jujutesting.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}
