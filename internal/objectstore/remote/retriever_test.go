// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remote

import (
	"bytes"
	"context"
	"fmt"
	io "io"
	"math/rand/v2"
	"sync/atomic"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/s3client"
	"github.com/juju/juju/internal/worker/apiremotecaller"
)

type retrieverSuite struct {
	testing.IsolationSuite

	remoteCallers    *MockAPIRemoteCallers
	remoteConnection *MockRemoteConnection
	apiConnection    *MockConnection
	client           *MockBlobsClient
	clock            *MockClock
}

var _ = gc.Suite(&retrieverSuite{})

func (s *retrieverSuite) TestRetrieverWithNoAPIRemotes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return(nil)

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	_, _, err := ret.Retrieve(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, NoRemoteConnections)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieverAlreadyKilled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ret := s.newRetriever(c)

	workertest.CleanKill(c, ret)

	_, _, err := ret.Retrieve(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, tomb.ErrDying)
}

func (s *retrieverSuite) TestRetrieverAlreadyContextCancelled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := ret.Retrieve(ctx, "foo")
	c.Assert(err, jc.ErrorIs, context.Canceled)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieverWithAPIRemotes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	client := &httprequest.Client{
		BaseURL: "http://example.com",
	}

	b := io.NopCloser(bytes.NewBufferString("hello world"))
	s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").Return(b, int64(11), nil)

	s.apiConnection.EXPECT().RootHTTPClient().Return(client, nil)
	s.apiConnection.EXPECT().Broken().DoAndReturn(func() <-chan struct{} {
		return make(chan struct{})
	}).AnyTimes()

	s.remoteConnection.EXPECT().Connection(gomock.Any()).DoAndReturn(func(ctx context.Context) <-chan api.Connection {
		ch := make(chan api.Connection, 1)
		ch <- s.apiConnection
		return ch
	})
	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{s.remoteConnection})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	readerCloser, size, err := ret.Retrieve(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that the reader is closed, otherwise the retriever will leak.
	// You can test this, by commenting out this line!
	defer readerCloser.Close()

	result, err := io.ReadAll(readerCloser)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, []byte("hello world"))
	c.Check(size, gc.Equals, int64(11))

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieverWithAPIRemotesRace(c *gc.C) {
	defer s.setupMocks(c).Finish()

	client := &httprequest.Client{
		BaseURL: "http://example.com",
	}

	b := io.NopCloser(bytes.NewBufferString("hello world"))

	// Ensure the first one blocks until the second one is called.

	done := make(chan struct{})
	started := make(chan struct{})

	fns := []func(context.Context, string, string) (io.ReadCloser, int64, error){
		func(ctx context.Context, s1, s2 string) (io.ReadCloser, int64, error) {
			select {
			case <-started:
			case <-time.After(testing.LongWait):
				c.Fatalf("timed out waiting for started")
			}

			select {
			case <-done:
			case <-time.After(testing.LongWait):
				c.Fatalf("timed out waiting for done")
			}

			select {
			case <-ctx.Done():
			case <-time.After(testing.LongWait):
				c.Fatalf("timed out waiting for context to be done")
			}
			return nil, 0, ctx.Err()
		},
		func(ctx context.Context, s1, s2 string) (io.ReadCloser, int64, error) {
			defer close(done)

			select {
			case <-started:
			case <-time.After(testing.LongWait):
				c.Fatalf("timed out waiting for started")
			}

			return b, int64(11), nil
		},
	}

	// Shuffle the functions to ensure we detect any issues that are dependant
	// on order.
	rand.Shuffle(len(fns), func(i, j int) {
		fns[i], fns[j] = fns[j], fns[i]
	})

	gomock.InOrder(
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").DoAndReturn(fns[0]),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").DoAndReturn(fns[1]),
	)

	var attempts int64
	s.apiConnection.EXPECT().RootHTTPClient().DoAndReturn(func() (*httprequest.Client, error) {
		n := atomic.AddInt64(&attempts, 1)
		if n == 2 {
			close(started)
		}
		return client, nil
	}).Times(2)
	s.apiConnection.EXPECT().Broken().DoAndReturn(func() <-chan struct{} {
		return make(chan struct{})
	}).AnyTimes()

	s.remoteConnection.EXPECT().Connection(gomock.Any()).DoAndReturn(func(ctx context.Context) <-chan api.Connection {
		ch := make(chan api.Connection, 1)
		ch <- s.apiConnection
		return ch
	}).Times(2)

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{
		s.remoteConnection,
		s.remoteConnection,
	})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	readerCloser, size, err := ret.Retrieve(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that the reader is closed, otherwise the retriever will leak.
	// You can test this, by commenting out this line!
	defer readerCloser.Close()

	result, err := io.ReadAll(readerCloser)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, []byte("hello world"))
	c.Check(size, gc.Equals, int64(11))

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieverWithAPIRemotesRaceNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	client := &httprequest.Client{
		BaseURL: "http://example.com",
	}

	b := io.NopCloser(bytes.NewBufferString("hello world"))

	started := make(chan struct{})

	notFound := func(ctx context.Context, s1, s2 string) (io.ReadCloser, int64, error) {
		select {
		case <-started:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for started")
		}
		return nil, 0, BlobNotFound
	}

	fns := []func(context.Context, string, string) (io.ReadCloser, int64, error){
		notFound,
		notFound,
		func(ctx context.Context, s1, s2 string) (io.ReadCloser, int64, error) {
			select {
			case <-started:
			case <-time.After(testing.LongWait):
				c.Fatalf("timed out waiting for started")
			}
			return b, int64(11), nil
		},
	}

	// Shuffle the functions to ensure we detect any issues that are dependant
	// on order.
	rand.Shuffle(len(fns), func(i, j int) {
		fns[i], fns[j] = fns[j], fns[i]
	})

	gomock.InOrder(
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").DoAndReturn(fns[0]),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").DoAndReturn(fns[1]),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").DoAndReturn(fns[2]),
	)

	var attempts int64
	s.apiConnection.EXPECT().RootHTTPClient().DoAndReturn(func() (*httprequest.Client, error) {
		n := atomic.AddInt64(&attempts, 1)
		if n == 3 {
			close(started)
		}
		return client, nil
	}).Times(3)
	s.apiConnection.EXPECT().Broken().DoAndReturn(func() <-chan struct{} {
		return make(chan struct{})
	}).AnyTimes()

	s.remoteConnection.EXPECT().Connection(gomock.Any()).DoAndReturn(func(ctx context.Context) <-chan api.Connection {
		ch := make(chan api.Connection, 1)
		ch <- s.apiConnection
		return ch
	}).Times(3)

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{
		s.remoteConnection,
		s.remoteConnection,
		s.remoteConnection,
	})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	readerCloser, size, err := ret.Retrieve(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that the reader is closed, otherwise the retriever will leak.
	// You can test this, by commenting out this line!
	defer readerCloser.Close()

	result, err := io.ReadAll(readerCloser)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, []byte("hello world"))
	c.Check(size, gc.Equals, int64(11))

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieverWithAPIRemotesNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	client := &httprequest.Client{
		BaseURL: "http://example.com",
	}

	s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").Return(nil, 0, BlobNotFound).Times(3)

	s.apiConnection.EXPECT().RootHTTPClient().DoAndReturn(func() (*httprequest.Client, error) {
		return client, nil
	}).Times(3)
	s.apiConnection.EXPECT().Broken().DoAndReturn(func() <-chan struct{} {
		return make(chan struct{})
	}).AnyTimes()

	s.remoteConnection.EXPECT().Connection(gomock.Any()).DoAndReturn(func(ctx context.Context) <-chan api.Connection {
		ch := make(chan api.Connection, 1)
		ch <- s.apiConnection
		return ch
	}).Times(3)

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{
		s.remoteConnection,
		s.remoteConnection,
		s.remoteConnection,
	})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	_, _, err := ret.Retrieve(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, BlobNotFound)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieverWithAPIRemotesError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	client := &httprequest.Client{
		BaseURL: "http://example.com",
	}

	started := make(chan struct{})

	fail := func(ctx context.Context, namespace, sha256 string) (io.ReadCloser, int64, error) {
		select {
		case <-started:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for started")
		}
		return nil, 0, fmt.Errorf("boom")
	}

	gomock.InOrder(
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").DoAndReturn(func(ctx context.Context, namespace, sha256 string) (io.ReadCloser, int64, error) {
			select {
			case <-started:
			case <-time.After(testing.LongWait):
				c.Fatalf("timed out waiting for started")
			}
			return nil, 0, BlobNotFound
		}),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").DoAndReturn(fail).MaxTimes(2),
	)

	var attempts int64
	s.apiConnection.EXPECT().RootHTTPClient().DoAndReturn(func() (*httprequest.Client, error) {
		n := atomic.AddInt64(&attempts, 1)
		if n == 3 {
			close(started)
		}
		return client, nil
	}).Times(3)
	s.apiConnection.EXPECT().Broken().DoAndReturn(func() <-chan struct{} {
		return make(chan struct{})
	}).AnyTimes()

	s.remoteConnection.EXPECT().Connection(gomock.Any()).DoAndReturn(func(ctx context.Context) <-chan api.Connection {
		ch := make(chan api.Connection, 1)
		ch <- s.apiConnection
		return ch
	}).Times(3)

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{
		s.remoteConnection,
		s.remoteConnection,
		s.remoteConnection,
	})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	_, _, err := ret.Retrieve(context.Background(), "foo")
	c.Assert(err, gc.ErrorMatches, "boom")

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieverWaitingForConnection(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	requested := make(chan struct{})
	s.remoteConnection.EXPECT().Connection(gomock.Any()).DoAndReturn(func(ctx context.Context) <-chan api.Connection {
		defer close(requested)

		ch := make(chan api.Connection, 1)
		return ch
	})
	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{s.remoteConnection})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	// If we're waiting for a connection, a cancel should stop the retriever.
	go func() {
		select {
		case <-requested:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for connection to be requested")
		}

		cancel()
	}()

	_, _, err := ret.Retrieve(ctx, "foo")
	c.Assert(err, jc.ErrorIs, context.Canceled)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieverClosedConnection(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.remoteConnection.EXPECT().Connection(gomock.Any()).DoAndReturn(func(ctx context.Context) <-chan api.Connection {
		ch := make(chan api.Connection)
		close(ch)
		return ch
	})
	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{s.remoteConnection})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	_, _, err := ret.Retrieve(ctx, "foo")
	c.Assert(err, jc.ErrorIs, NoRemoteConnection)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieverWithBrokenConnection(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &httprequest.Client{
		BaseURL: "http://example.com",
	}

	wait := make(chan struct{})

	// Trigger the broken connection, which will cause the GetObject to return
	// the ctx.Err(), which should be context.Canceled.

	s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").DoAndReturn(func(ctx context.Context, namespace, sha256 string) (io.ReadCloser, int64, error) {
		return nil, -1, ctx.Err()
	})

	s.apiConnection.EXPECT().RootHTTPClient().DoAndReturn(func() (*httprequest.Client, error) {
		select {
		case <-wait:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for wait")
		}
		return client, nil
	})
	s.apiConnection.EXPECT().Broken().DoAndReturn(func() <-chan struct{} {
		defer close(wait)

		ch := make(chan struct{})
		close(ch)
		return ch
	})

	s.remoteConnection.EXPECT().Connection(gomock.Any()).DoAndReturn(func(ctx context.Context) <-chan api.Connection {
		ch := make(chan api.Connection, 1)
		ch <- s.apiConnection
		return ch
	})
	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{s.remoteConnection})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	_, _, err := ret.Retrieve(ctx, "foo")
	c.Assert(err, jc.ErrorIs, context.Canceled)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) newRetriever(c *gc.C) *BlobRetriever {
	return NewBlobRetriever(s.remoteCallers, "namespace", func(url string, client s3client.HTTPClient, logger logger.Logger) (BlobsClient, error) {
		return s.client, nil
	}, s.clock, loggertesting.WrapCheckLog(c))
}

func (s *retrieverSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.remoteCallers = NewMockAPIRemoteCallers(ctrl)
	s.remoteConnection = NewMockRemoteConnection(ctrl)
	s.apiConnection = NewMockConnection(ctrl)

	s.client = NewMockBlobsClient(ctrl)
	s.clock = NewMockClock(ctrl)

	return ctrl
}
