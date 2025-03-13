// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"context"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type RemoteSuite struct {
	baseSuite

	apiConnect        chan struct{}
	apiConnectHandler func(context.Context) error

	apiConnection *MockConnection
}

var _ = gc.Suite(&RemoteSuite{})

func (s *RemoteSuite) TestNotConnectedConnection(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	ctx, cancel := context.WithTimeout(context.Background(), jujutesting.ShortWait)
	defer cancel()

	var called bool
	err := w.Connection(ctx, func(ctx context.Context, c api.Connection) error {
		called = true
		return nil
	})
	c.Assert(err, jc.ErrorIs, context.DeadlineExceeded)
	c.Check(called, jc.IsFalse)

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestConnect(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	addr := &url.URL{Scheme: "wss", Host: "10.0.0.1"}

	s.apiConnection.EXPECT().Broken().Return(make(<-chan struct{}))
	s.apiConnection.EXPECT().Close().Return(nil)
	s.apiConnection.EXPECT().Addr().Return(addr)

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.UpdateAddresses([]string{addr.String()})

	select {
	case <-s.apiConnect:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for API connect")
	}

	s.ensureChanged(c)

	var conn api.Connection
	err := w.Connection(context.Background(), func(ctx context.Context, c api.Connection) error {
		conn = c
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(conn, gc.NotNil)
	c.Check(conn.Addr().String(), jc.DeepEquals, addr.String())

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestConnectWhenAlreadyContextCancelled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var called bool
	err := w.Connection(ctx, func(ctx context.Context, c api.Connection) error {
		called = true
		return nil
	})
	c.Assert(err, jc.ErrorIs, context.Canceled)
	c.Check(called, jc.IsFalse)

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestConnectWhenAlreadyKilled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	workertest.CleanKill(c, w)

	var called bool
	err := w.Connection(context.Background(), func(ctx context.Context, c api.Connection) error {
		called = true
		return nil
	})
	c.Assert(err, jc.ErrorIs, tomb.ErrDying)
	c.Check(called, jc.IsFalse)
}

func (s *RemoteSuite) TestConnectMultipleWithFirstCancelled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// This test ensures that when the first connection is cancelled, the second
	// connection is not stalled.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.apiConnectHandler = func(ctx context.Context) error {
		cancel()
		close(s.apiConnect)
		return nil
	}

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	addr := &url.URL{Scheme: "wss", Host: "10.0.0.1"}

	s.apiConnection.EXPECT().Broken().Return(make(<-chan struct{}))
	s.apiConnection.EXPECT().Close().Return(nil)

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	var wg sync.WaitGroup
	wg.Add(2)

	res := make(chan error)
	seq := make(chan struct{})
	go func() {
		// Force the first connection to be enqueued, so that second connection
		// will be stalled.
		go func() {
			select {
			case <-time.After(time.Millisecond * 100):
				close(seq)
			}
		}()

		wg.Done()

		var called bool
		err := w.Connection(ctx, func(ctx context.Context, c api.Connection) error {
			called = true
			return nil
		})
		c.Assert(err, jc.ErrorIs, context.Canceled)
		c.Check(called, jc.IsFalse)
	}()
	go func() {
		// Wait for the first connection to be enqueued.
		select {
		case <-seq:
		case <-time.After(jujutesting.LongWait):
			c.Fatalf("timed out waiting for first connection to be cancelled")
		}

		wg.Done()

		err := w.Connection(context.Background(), func(ctx context.Context, c api.Connection) error {
			return nil
		})
		select {
		case res <- err:
		case <-time.After(jujutesting.LongWait):
			c.Fatalf("timed out sending result")
		}
	}()

	// Ensure both goroutines have started, before we start the test.
	sync := make(chan struct{})
	go func() {
		wg.Wait()
		close(sync)
	}()
	select {
	case <-sync:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for connections to finish")
	}

	select {
	case <-seq:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for first connection to be cancelled")
	}

	w.UpdateAddresses([]string{addr.String()})

	// This is our sequence point to ensure that we connect.
	select {
	case <-s.apiConnect:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for API connect")
	}

	s.ensureChanged(c)

	select {
	case err := <-res:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for connection")
	}

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestConnectWhilstConnecting(c *gc.C) {
	defer s.setupMocks(c).Finish()

	var counter atomic.Int64
	s.apiConnectHandler = func(ctx context.Context) error {
		v := counter.Add(1)
		if v == 1 {
			// This should block the apiConnection.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(jujutesting.LongWait):
				c.Fatalf("timed out waiting for context to be done")
			}
		}
		close(s.apiConnect)
		return nil
	}

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	addr0 := &url.URL{Scheme: "wss", Host: "10.0.0.1"}
	addr1 := &url.URL{Scheme: "wss", Host: "10.0.0.2"}

	s.apiConnection.EXPECT().Broken().Return(make(<-chan struct{}))
	s.apiConnection.EXPECT().Close().Return(nil)
	s.apiConnection.EXPECT().Addr().Return(addr1)

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	// UpdateAddresses will block the first connection, so we can trigger a
	// connection failure. The second UpdateAddresses should then cancel the
	// current connection and start a new one, that one should then succeed.
	w.UpdateAddresses([]string{addr0.String()})
	w.UpdateAddresses([]string{addr1.String()})

	select {
	case <-s.apiConnect:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for API connect")
	}

	s.ensureChanged(c)

	var conn api.Connection
	err := w.Connection(context.Background(), func(ctx context.Context, c api.Connection) error {
		conn = c
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(conn, gc.NotNil)
	c.Check(conn.Addr().String(), jc.DeepEquals, addr1.String())

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestConnectBlocks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.apiConnectHandler = func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(jujutesting.LongWait):
			c.Fatalf("timed out waiting for context to be done")
		}
		return nil
	}

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	addr := &url.URL{Scheme: "wss", Host: "10.0.0.1"}

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.UpdateAddresses([]string{addr.String()})

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) TestConnectWithSameAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	var counter atomic.Int64
	s.apiConnectHandler = func(ctx context.Context) error {
		counter.Add(1)

		select {
		case s.apiConnect <- struct{}{}:
		case <-time.After(time.Second):
			c.Fatalf("timed out waiting for API connect")
		}
		return nil
	}

	s.expectClock()
	s.expectClockAfter(make(<-chan time.Time))

	addr := &url.URL{Scheme: "wss", Host: "10.0.0.1"}

	s.apiConnection.EXPECT().Broken().Return(make(<-chan struct{}))
	s.apiConnection.EXPECT().Close().Return(nil)

	w := s.newRemoteServer(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.UpdateAddresses([]string{addr.String()})

	select {
	case <-s.apiConnect:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for API connect")
	}

	w.UpdateAddresses([]string{addr.String()})

	select {
	case <-s.apiConnect:
		c.Fatalf("the connection should not be called")
	case <-time.After(time.Second):
	}

	c.Assert(counter.Load(), gc.Equals, int64(1))

	workertest.CleanKill(c, w)
}

func (s *RemoteSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.apiConnection = NewMockConnection(ctrl)

	s.apiConnect = make(chan struct{})
	s.apiConnectHandler = func(ctx context.Context) error {
		close(s.apiConnect)
		return nil
	}

	return ctrl
}

func (s *RemoteSuite) newRemoteServer(c *gc.C) RemoteServer {
	return newRemoteServer(s.newConfig(c), s.states)
}

func (s *RemoteSuite) newConfig(c *gc.C) RemoteServerConfig {
	return RemoteServerConfig{
		Clock:   s.clock,
		Logger:  loggertesting.WrapCheckLog(c),
		APIInfo: &api.Info{},
		APIOpener: func(ctx context.Context, i *api.Info, do api.DialOpts) (api.Connection, error) {
			err := s.apiConnectHandler(ctx)
			if err != nil {
				return nil, err
			}
			return s.apiConnection, nil
		},
	}
}

func (s *RemoteSuite) expectClockAfter(ch <-chan time.Time) {
	s.clock.EXPECT().After(gomock.Any()).DoAndReturn(func(d time.Duration) <-chan time.Time {
		return ch
	}).AnyTimes()
}
