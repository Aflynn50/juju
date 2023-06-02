// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/mattn/go-sqlite3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain"
)

type serviceSuite struct {
	testing.IsolationSuite

	state     *MockState
	dbDeleter *MockDBDeleter
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestServiceCreate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := mustUUID(c)

	s.state.EXPECT().Create(gomock.Any(), uuid).Return(nil)

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Create(context.TODO(), uuid)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestServiceCreateError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := mustUUID(c)

	s.state.EXPECT().Create(gomock.Any(), uuid).Return(fmt.Errorf("boom"))

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Create(context.TODO(), uuid)
	c.Assert(err, gc.ErrorMatches, `creating model ".*": boom`)
}

func (s *serviceSuite) TestServiceCreateDuplicateError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := mustUUID(c)

	s.state.EXPECT().Create(gomock.Any(), uuid).Return(sqlite3.Error{
		ExtendedCode: sqlite3.ErrConstraintUnique,
	})

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Create(context.TODO(), uuid)
	c.Assert(err, gc.ErrorMatches, "creating model .*: record already exists")
	c.Assert(errors.Is(errors.Cause(err), domain.ErrDuplicate), jc.IsTrue)
}

func (s *serviceSuite) TestServiceCreateInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Create(context.TODO(), "invalid")
	c.Assert(err, gc.ErrorMatches, "validating model uuid.*")
}

func (s *serviceSuite) TestServiceDelete(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := mustUUID(c)

	s.state.EXPECT().Delete(gomock.Any(), uuid).Return(nil)
	s.dbDeleter.EXPECT().DeleteDB(uuid.String()).Return(nil)

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Delete(context.TODO(), uuid)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestServiceDeleteStateError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := mustUUID(c)

	s.state.EXPECT().Delete(gomock.Any(), uuid).Return(fmt.Errorf("boom"))

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Delete(context.TODO(), uuid)
	c.Assert(err, gc.ErrorMatches, `deleting model ".*": boom`)
}

func (s *serviceSuite) TestServiceDeleteNoRecordsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := mustUUID(c)

	s.state.EXPECT().Delete(gomock.Any(), uuid).Return(domain.ErrNoRecord)

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Delete(context.TODO(), uuid)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("no records should be idempotent"))
}

func (s *serviceSuite) TestServiceDeleteStateSqliteError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := mustUUID(c)

	s.state.EXPECT().Delete(gomock.Any(), uuid).Return(sqlite3.Error{
		Code:         sqlite3.ErrPerm,
		ExtendedCode: sqlite3.ErrCorruptVTab,
	})

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Delete(context.TODO(), uuid)
	c.Assert(err, gc.ErrorMatches, `deleting model ".*": access permission denied`)
}

func (s *serviceSuite) TestServiceDeleteManagerError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := mustUUID(c)

	s.state.EXPECT().Delete(gomock.Any(), uuid).Return(nil)
	s.dbDeleter.EXPECT().DeleteDB(uuid.String()).Return(fmt.Errorf("boom"))

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Delete(context.TODO(), uuid)
	c.Assert(err, gc.ErrorMatches, `stopping model ".*": boom`)
}

func (s *serviceSuite) TestServiceDeleteInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Delete(context.TODO(), "invalid")
	c.Assert(err, gc.ErrorMatches, "validating model uuid.*")
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.dbDeleter = NewMockDBDeleter(ctrl)

	return ctrl
}

func mustUUID(c *gc.C) UUID {
	return UUID(utils.MustNewUUID().String())
}
