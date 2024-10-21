// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageregistry

import (
	"github.com/juju/juju/internal/storage"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type trackedWorkerSuite struct {
	baseSuite

	states   chan string
	registry *MockProviderRegistry
	provider *MockProvider
}

var _ = gc.Suite(&trackedWorkerSuite{})

func (s *trackedWorkerSuite) TestKilled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, err := NewTrackedWorker(s.registry)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CheckKill(c, w)

	w.Kill()
}

func (s *trackedWorkerSuite) TestStorageProviderTypesWithCommon(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.registry.EXPECT().StorageProviderTypes().Return([]storage.ProviderType{"ebs"}, nil)

	w, err := NewTrackedWorker(s.registry)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CheckKill(c, w)

	types, err := w.(*trackedWorker).StorageProviderTypes()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(types, gc.DeepEquals, []storage.ProviderType{"ebs", "loop", "rootfs", "tmpfs"})
}

func (s *trackedWorkerSuite) TestStorageProviderTypesWithEmptyProviderTypes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.registry.EXPECT().StorageProviderTypes().Return([]storage.ProviderType{}, nil)

	w, err := NewTrackedWorker(s.registry)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CheckKill(c, w)

	types, err := w.(*trackedWorker).StorageProviderTypes()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(types, gc.DeepEquals, []storage.ProviderType{"loop", "rootfs", "tmpfs"})
}

func (s *trackedWorkerSuite) TestStorageProvider(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.registry.EXPECT().StorageProvider(storage.ProviderType("rootfs")).Return(s.provider, nil)

	w, err := NewTrackedWorker(s.registry)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CheckKill(c, w)

	provider, err := w.(*trackedWorker).StorageProvider(storage.ProviderType("rootfs"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(provider, gc.DeepEquals, s.provider)
}

func (s *trackedWorkerSuite) setupMocks(c *gc.C) *gomock.Controller {
	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	ctrl := s.baseSuite.setupMocks(c)

	s.registry = NewMockProviderRegistry(ctrl)
	s.provider = NewMockProvider(ctrl)

	return ctrl
}
