// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type caasProvisionerSuite struct {
	testing.IsolationSuite

	api *StorageProvisionerAPIv4

	storageBackend       *MockStorageBackend
	filesystemAttachment *MockFilesystemAttachment
	volumeAttachment     *MockVolumeAttachment
	entityFinder         *MockEntityFinder
	lifer                *MockLifer
	backend              *MockBackend
	resources            *MockResources
}

var _ = gc.Suite(&caasProvisionerSuite{})

func (s *caasProvisionerSuite) TestWatchApplications(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string)

	watcher := watchertest.NewMockStringsWatcher(ch)
	s.backend.EXPECT().WatchApplications().DoAndReturn(func() state.StringsWatcher {
		// Enqueue
		go func() {
			select {
			case ch <- []string{"application-mariadb"}:
			case <-time.After(testing.LongWait):
				c.Fatalf("timed out waiting to send")
			}
		}()
		return watcher
	})
	s.resources.EXPECT().Register(watcher).Return("1")

	result, err := s.api.WatchApplications(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.StringsWatcherId, gc.Equals, "1")
	c.Check(result.Changes, jc.DeepEquals, []string{"application-mariadb"})
}

func (s *caasProvisionerSuite) TestWatchApplicationsClosed(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string)
	close(ch)

	watcher := watchertest.NewMockStringsWatcher(ch)
	s.backend.EXPECT().WatchApplications().Return(watcher)

	_, err := s.api.WatchApplications(context.Background())
	c.Assert(err, gc.ErrorMatches, `.*tomb: still alive`)
}

func (s *caasProvisionerSuite) TestRemoveVolumeAttachment(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// It is expected that the detachment of mariadb has been remove prior.

	s.storageBackend.EXPECT().RemoveVolumeAttachment(names.NewUnitTag("mariadb/0"), names.NewVolumeTag("0"), false).Return(errors.Errorf(`removing attachment of volume 0 from unit mariadb/0: volume attachment is not dying`))
	s.storageBackend.EXPECT().RemoveVolumeAttachment(names.NewUnitTag("mariadb/0"), names.NewVolumeTag("1"), false).Return(nil)
	s.storageBackend.EXPECT().RemoveVolumeAttachment(names.NewUnitTag("mysql/2"), names.NewVolumeTag("4"), false).Return(errors.NotFoundf(`removing attachment of volume 4 from unit mysql/2: volume "4" on "unit mysql/2"`))
	s.storageBackend.EXPECT().RemoveVolumeAttachment(names.NewUnitTag("mariadb/0"), names.NewVolumeTag("42"), false).Return(errors.NotFoundf(`removing attachment of volume 42 from unit mariadb/0: volume "42" on "unit mariadb/0"`))

	results, err := s.api.RemoveAttachment(context.Background(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{{
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "volume-0",
		}, {
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "volume-1",
		}, {
			MachineTag:    "unit-mysql-2",
			AttachmentTag: "volume-4",
		}, {
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "volume-42",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "removing attachment of volume 0 from unit mariadb/0: volume attachment is not dying"}},
			{Error: nil},
			{Error: &params.Error{Message: `removing attachment of volume 4 from unit mysql/2: volume "4" on "unit mysql/2" not found`, Code: "not found"}},
			{Error: &params.Error{Message: `removing attachment of volume 42 from unit mariadb/0: volume "42" on "unit mariadb/0" not found`, Code: "not found"}},
		},
	})
}

func (s *caasProvisionerSuite) TestRemoveFilesystemAttachments(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// It is expected that the detachment of mariadb has been remove prior.

	s.storageBackend.EXPECT().RemoveFilesystemAttachment(names.NewUnitTag("mariadb/0"), names.NewFilesystemTag("0"), false).Return(errors.Errorf(`removing attachment of filesystem 0 from unit mariadb/0: filesystem attachment is not dying`))
	s.storageBackend.EXPECT().RemoveFilesystemAttachment(names.NewUnitTag("mariadb/0"), names.NewFilesystemTag("1"), false).Return(nil)
	s.storageBackend.EXPECT().RemoveFilesystemAttachment(names.NewUnitTag("mysql/2"), names.NewFilesystemTag("4"), false).Return(errors.NotFoundf(`removing attachment of filesystem 4 from unit mysql/2: filesystem "4" on "unit mysql/2"`))
	s.storageBackend.EXPECT().RemoveFilesystemAttachment(names.NewUnitTag("mariadb/0"), names.NewFilesystemTag("42"), false).Return(errors.NotFoundf(`removing attachment of filesystem 42 from unit mariadb/0: filesystem "42" on "unit mariadb/0"`))

	results, err := s.api.RemoveAttachment(context.Background(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{{
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-0",
		}, {
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-1",
		}, {
			MachineTag:    "unit-mysql-2",
			AttachmentTag: "filesystem-4",
		}, {
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-42",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "removing attachment of filesystem 0 from unit mariadb/0: filesystem attachment is not dying"}},
			{Error: nil},
			{Error: &params.Error{Message: `removing attachment of filesystem 4 from unit mysql/2: filesystem "4" on "unit mysql/2" not found`, Code: "not found"}},
			{Error: &params.Error{Message: `removing attachment of filesystem 42 from unit mariadb/0: filesystem "42" on "unit mariadb/0" not found`, Code: "not found"}},
		},
	})
}

func (s *caasProvisionerSuite) TestFilesystemLife(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.entityFinder.EXPECT().FindEntity(names.NewFilesystemTag("0")).Return(entity{
		Lifer: s.lifer,
	}, nil)
	s.lifer.EXPECT().Life().Return(state.Alive)

	s.entityFinder.EXPECT().FindEntity(names.NewFilesystemTag("1")).Return(entity{
		Lifer: s.lifer,
	}, nil)
	s.lifer.EXPECT().Life().Return(state.Alive)

	s.entityFinder.EXPECT().FindEntity(names.NewFilesystemTag("42")).Return(entity{
		Lifer: s.lifer,
	}, errors.NotFoundf(`filesystem "42"`))

	args := params.Entities{Entities: []params.Entity{{Tag: "filesystem-0"}, {Tag: "filesystem-1"}, {Tag: "filesystem-42"}}}
	result, err := s.api.Life(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: life.Alive},
			{Life: life.Alive},
			{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: `filesystem "42" not found`,
			}},
		},
	})
}

func (s *caasProvisionerSuite) TestVolumeLife(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.entityFinder.EXPECT().FindEntity(names.NewVolumeTag("0")).Return(entity{
		Lifer: s.lifer,
	}, nil)
	s.lifer.EXPECT().Life().Return(state.Alive)

	s.entityFinder.EXPECT().FindEntity(names.NewVolumeTag("1")).Return(entity{
		Lifer: s.lifer,
	}, nil)
	s.lifer.EXPECT().Life().Return(state.Alive)

	s.entityFinder.EXPECT().FindEntity(names.NewVolumeTag("42")).Return(entity{
		Lifer: s.lifer,
	}, errors.NotFoundf(`volume "42"`))

	args := params.Entities{Entities: []params.Entity{{Tag: "volume-0"}, {Tag: "volume-1"}, {Tag: "volume-42"}}}
	result, err := s.api.Life(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: life.Alive},
			{Life: life.Alive},
			{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: `volume "42" not found`,
			}},
		},
	})
}

func (s *caasProvisionerSuite) TestFilesystemAttachmentLife(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.storageBackend.EXPECT().FilesystemAttachment(names.NewUnitTag("mariadb/0"), names.NewFilesystemTag("0")).Return(s.filesystemAttachment, nil)
	s.filesystemAttachment.EXPECT().Life().Return(state.Alive)

	s.storageBackend.EXPECT().FilesystemAttachment(names.NewUnitTag("mariadb/0"), names.NewFilesystemTag("1")).Return(s.filesystemAttachment, nil)
	s.filesystemAttachment.EXPECT().Life().Return(state.Alive)

	s.storageBackend.EXPECT().FilesystemAttachment(names.NewUnitTag("mariadb/0"), names.NewFilesystemTag("42")).Return(s.filesystemAttachment, errors.NotFoundf(`filesystem "42" on "unit mariadb/0"`))

	results, err := s.api.AttachmentLife(context.Background(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{{
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-0",
		}, {
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-1",
		}, {
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-42",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: life.Alive},
			{Life: life.Alive},
			{Error: &params.Error{Message: `filesystem "42" on "unit mariadb/0" not found`, Code: "not found"}},
		},
	})
}

func (s *caasProvisionerSuite) TestVolumeAttachmentLife(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.storageBackend.EXPECT().VolumeAttachment(names.NewUnitTag("mariadb/0"), names.NewVolumeTag("0")).Return(s.volumeAttachment, nil)
	s.volumeAttachment.EXPECT().Life().Return(state.Alive)

	s.storageBackend.EXPECT().VolumeAttachment(names.NewUnitTag("mariadb/0"), names.NewVolumeTag("1")).Return(s.volumeAttachment, nil)
	s.volumeAttachment.EXPECT().Life().Return(state.Alive)

	s.storageBackend.EXPECT().VolumeAttachment(names.NewUnitTag("mariadb/0"), names.NewVolumeTag("42")).Return(s.volumeAttachment, errors.NotFoundf(`volume "42" on "unit mariadb/0"`))

	results, err := s.api.AttachmentLife(context.Background(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{{
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "volume-0",
		}, {
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "volume-1",
		}, {
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "volume-42",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: life.Alive},
			{Life: life.Alive},
			{Error: &params.Error{Message: `volume "42" on "unit mariadb/0" not found`, Code: "not found"}},
		},
	})
}

func (s *caasProvisionerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.storageBackend = NewMockStorageBackend(ctrl)
	s.filesystemAttachment = NewMockFilesystemAttachment(ctrl)
	s.volumeAttachment = NewMockVolumeAttachment(ctrl)
	s.entityFinder = NewMockEntityFinder(ctrl)
	s.lifer = NewMockLifer(ctrl)
	s.backend = NewMockBackend(ctrl)
	s.resources = NewMockResources(ctrl)

	s.api = &StorageProvisionerAPIv4{
		LifeGetter: common.NewLifeGetter(s.entityFinder, func(context.Context) (common.AuthFunc, error) {
			return func(names.Tag) bool {
				return true
			}, nil
		}),
		sb:        s.storageBackend,
		st:        s.backend,
		resources: s.resources,
		getAttachmentAuthFunc: func(context.Context) (func(names.Tag, names.Tag) bool, error) {
			return func(names.Tag, names.Tag) bool { return true }, nil
		},
	}

	return ctrl
}

type entity struct {
	state.Lifer
	state.Entity
}
