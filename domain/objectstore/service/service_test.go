// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/objectstore"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/uuid"
)

type serviceSuite struct {
	testing.IsolationSuite

	state          *MockState
	watcherFactory *MockWatcherFactory
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestGetMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := uuid.MustNewUUID().String()

	metadata := coreobjectstore.Metadata{
		Path: path,
		Hash: uuid.MustNewUUID().String(),
		Size: 666,
	}

	s.state.EXPECT().GetMetadata(gomock.Any(), path).Return(coreobjectstore.Metadata{
		Path: metadata.Path,
		Size: metadata.Size,
		Hash: metadata.Hash,
	}, nil)

	p, err := NewService(s.state).GetMetadata(context.Background(), path)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(p, gc.DeepEquals, metadata)
}

func (s *serviceSuite) TestListMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := uuid.MustNewUUID().String()

	metadata := coreobjectstore.Metadata{
		Path: path,
		Hash: uuid.MustNewUUID().String(),
		Size: 666,
	}

	s.state.EXPECT().ListMetadata(gomock.Any()).Return([]coreobjectstore.Metadata{{
		Path: metadata.Path,
		Hash: metadata.Hash,
		Size: metadata.Size,
	}}, nil)

	p, err := NewService(s.state).ListMetadata(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.DeepEquals, []coreobjectstore.Metadata{{
		Path: metadata.Path,
		Size: metadata.Size,
		Hash: metadata.Hash,
	}})
}

func (s *serviceSuite) TestPutMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := uuid.MustNewUUID().String()
	metadata := coreobjectstore.Metadata{
		Path: path,
		Hash: uuid.MustNewUUID().String(),
		Size: 666,
	}

	uuid := objectstoretesting.GenObjectStoreUUID(c)
	s.state.EXPECT().PutMetadata(gomock.Any(), gomock.AssignableToTypeOf(coreobjectstore.Metadata{})).DoAndReturn(func(ctx context.Context, data coreobjectstore.Metadata) (objectstore.UUID, error) {
		c.Check(data.Path, gc.Equals, metadata.Path)
		c.Check(data.Size, gc.Equals, metadata.Size)
		c.Check(data.Hash, gc.Equals, metadata.Hash)
		return uuid, nil
	})

	result, err := NewService(s.state).PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, uuid)
}

func (s *serviceSuite) TestRemoveMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := uuid.MustNewUUID().String()

	s.state.EXPECT().RemoveMetadata(gomock.Any(), key).Return(nil)

	err := NewService(s.state).RemoveMetadata(context.Background(), key)
	c.Assert(err, jc.ErrorIsNil)
}

// Test watch returns a watcher that watches the specified path.
func (s *serviceSuite) TestWatch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	watcher := watchertest.NewMockStringsWatcher(nil)
	defer workertest.DirtyKill(c, watcher)

	table := "objectstore"
	stmt := "SELECT key FROM objectstore"
	s.state.EXPECT().InitialWatchStatement().Return(table, stmt)

	s.watcherFactory.EXPECT().NewNamespaceWatcher(table, changestream.All, gomock.Any()).Return(watcher, nil)

	w, err := NewWatchableService(s.state, s.watcherFactory).Watch()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	return ctrl
}
