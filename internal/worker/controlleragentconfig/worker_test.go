// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlleragentconfig

import (
	"context"
	"net"
	"net/http"
	"path"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/juju/clock"
	coretesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type workerSuite struct {
	baseSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestStartup(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, _, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestReloadRequest(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, socket, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	s.requestReload(c, socket)
	s.ensureReload(c, states)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestIncorrectEndpoint(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, socket, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	s.ensureEndpointNotFound(c, socket, "/relod")

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestReloadRequestMultipleTimes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, socket, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	for i := 0; i < 10; i++ {
		s.requestReload(c, socket)
		s.ensureReload(c, states)
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestReloadRequestAfterDeath(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, socket, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	workertest.CleanKill(c, w)

	// We should not receive a reload signal after the worker has died.
	s.ensureReloadRequestRefused(c, socket)

	select {
	case state := <-states:
		c.Fatalf("should not have received state %q", state)
	case <-time.After(coretesting.ShortWait * 10):
	}
}

func (s *workerSuite) TestWatchWithNoChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, _, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	watcher, err := w.Watcher()
	c.Assert(err, jc.ErrorIsNil)
	defer watcher.Unsubscribe()

	changes := watcher.Changes()
	select {
	case <-changes:
		c.Fatal("should not have received a change")
	case <-time.After(coretesting.ShortWait * 10):
	}
}

func (s *workerSuite) TestWatchWithSubscribe(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, socket, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	watcher, err := w.Watcher()
	c.Assert(err, jc.ErrorIsNil)
	defer watcher.Unsubscribe()

	s.requestReload(c, socket)
	s.ensureReload(c, states)

	changes := watcher.Changes()

	var count int
	select {
	case <-changes:
		count++
	case <-time.After(coretesting.ShortWait):
		c.Fatal("should have received a change")
	}

	c.Assert(count, gc.Equals, 1)

	select {
	case <-watcher.Done():
		c.Fatalf("should not have received a done signal")
	case <-time.After(coretesting.ShortWait):
	}

	workertest.CleanKill(c, w)

	ensureDone(c, watcher)
}

func (s *workerSuite) TestWatchAfterUnsubscribe(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, socket, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	watcher, err := w.Watcher()
	c.Assert(err, jc.ErrorIsNil)
	defer watcher.Unsubscribe()

	s.requestReload(c, socket)
	s.ensureReload(c, states)

	watcher.Unsubscribe()

	changes := watcher.Changes()

	// The channel should be closed.
	select {
	case _, ok := <-changes:
		c.Assert(ok, jc.IsFalse)
	case <-time.After(coretesting.ShortWait * 10):
	}

	ensureDone(c, watcher)
}

func (s *workerSuite) TestWatchWithKilledWorker(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, _, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	watcher, err := w.Watcher()
	c.Assert(err, jc.ErrorIsNil)
	defer watcher.Unsubscribe()

	workertest.CleanKill(c, w)

	changes := watcher.Changes()

	select {
	case _, ok := <-changes:
		c.Assert(ok, jc.IsFalse)
	case <-time.After(coretesting.ShortWait * 10):
	}

	ensureDone(c, watcher)
}

func (s *workerSuite) TestWatchMultiple(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, socket, states := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c, states)

	watchers := make([]ConfigWatcher, 10)
	for i := range watchers {
		watcher, err := w.Watcher()
		c.Assert(err, jc.ErrorIsNil)
		defer watcher.Unsubscribe()
		watchers[i] = watcher
	}

	s.requestReload(c, socket)
	s.ensureReload(c, states)

	var wg sync.WaitGroup
	wg.Add(len(watchers))

	var count int64
	for i := 0; i < len(watchers); i++ {
		go func(w ConfigWatcher) {
			defer wg.Done()

			changes := w.Changes()
			select {
			case _, ok := <-changes:
				atomic.AddInt64(&count, 1)
				c.Assert(ok, jc.IsTrue)
			case <-time.After(coretesting.ShortWait * 10):
				c.Fatal("should have received a change")
			}
		}(watchers[i])
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for changes to finish")
	}

	c.Assert(atomic.LoadInt64(&count), gc.Equals, int64(len(watchers)))
}

func (s *workerSuite) TestWatchMultipleWithUnsubscribe(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, socket, states := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c, states)

	watchers := make([]ConfigWatcher, 10)
	for i := range watchers {
		watcher, err := w.Watcher()
		c.Assert(err, jc.ErrorIsNil)
		watchers[i] = watcher
	}

	s.requestReload(c, socket)
	s.ensureReload(c, states)

	var wg sync.WaitGroup
	wg.Add(len(watchers))

	var count int64
	for i := 0; i < len(watchers); i++ {
		go func(i int, w ConfigWatcher) {
			defer wg.Done()

			changes := w.Changes()

			// Test to ensure that a unsubscribe doesn't block another watcher.
			if (i % 2) == 0 {
				w.Unsubscribe()
				// Notice that we don't wait for the unsubscribe to complete.
				// Which means that the worker should not block sending
				// messages.
				return
			}

			select {
			case _, ok := <-changes:
				atomic.AddInt64(&count, 1)
				c.Assert(ok, jc.IsTrue)
			case <-time.After(coretesting.ShortWait * 10):
				c.Fatal("should have received a change")
			}
		}(i, watchers[i])
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for changes to finish")
	}

	c.Assert(atomic.LoadInt64(&count), gc.Equals, int64(len(watchers)/2))
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)
	return ctrl
}

func (s *workerSuite) newWorker(c *gc.C) (*configWorker, string, chan string) {
	// Buffer the channel, so we don't drop signals if we're not ready.
	states := make(chan string, 10)

	// Create a unix socket for testing.
	tmpDir := c.MkDir()
	socket := path.Join(tmpDir, "test.socket")

	w, err := newWorker(WorkerConfig{
		Logger:            s.logger,
		Clock:             clock.WallClock,
		SocketName:        socket,
		NewSocketListener: NewSocketListener,
	}, states)
	c.Assert(err, jc.ErrorIsNil)
	return w, socket, states
}

func (s *workerSuite) ensureStartup(c *gc.C, states chan string) {
	select {
	case state := <-states:
		c.Assert(state, gc.Equals, stateStarted)
	case <-time.After(coretesting.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *workerSuite) ensureReload(c *gc.C, states chan string) {
	select {
	case state := <-states:
		c.Assert(state, gc.Equals, stateReload)
	case <-time.After(coretesting.ShortWait * 100):
		c.Fatalf("timed out waiting for reload")
	}
}

func (s *workerSuite) requestReload(c *gc.C, socket string) {
	resp, err := newRequest(c, socket, "/reload")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
}

func (s *workerSuite) ensureReloadRequestRefused(c *gc.C, socket string) {
	_, err := newRequest(c, socket, "/reload")
	c.Assert(err, jc.ErrorIs, syscall.ECONNREFUSED)
}

func (s *workerSuite) ensureEndpointNotFound(c *gc.C, socket, method string) {
	resp, err := newRequest(c, socket, method)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusNotFound)
}

func ensureDone(c *gc.C, watcher ConfigWatcher) {
	select {
	case <-watcher.Done():
	case <-time.After(coretesting.ShortWait):
		c.Fatal("should have received a done signal")
	}
}

func newRequest(c *gc.C, socket, method string) (*http.Response, error) {
	serverURL := "http://localhost:8080" + method
	req, err := http.NewRequest(
		http.MethodGet,
		serverURL,
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	return client(socket).Do(req)
}

// Return an *http.Client with custom transport that allows it to connect to
// the given Unix socket.
func client(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (conn net.Conn, err error) {
				return net.Dial("unix", socketPath)
			},
		},
	}
}
