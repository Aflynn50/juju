// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"errors"

	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/crossmodel"
)

type serviceSuite struct {
	testing.IsolationSuite

	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestUpdateExternalControllerSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	m1 := utils.MustNewUUID().String()
	m2 := utils.MustNewUUID().String()

	ec := crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(utils.MustNewUUID().String()),
		Alias:         "that-other-controller",
		Addrs:         []string{"10.10.10.10"},
		CACert:        "random-cert-string",
		ModelUUIDs:    []string{m1, m2},
	}

	s.state.EXPECT().UpdateExternalController(gomock.Any(), ec).Return(nil)

	err := NewService(s.state, nil).UpdateExternalController(context.Background(), ec)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateExternalControllerError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ec := crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(utils.MustNewUUID().String()),
		Alias:         "that-other-controller",
		Addrs:         []string{"10.10.10.10"},
		CACert:        "random-cert-string",
	}

	s.state.EXPECT().UpdateExternalController(gomock.Any(), ec).Return(errors.New("boom"))

	err := NewService(s.state, nil).UpdateExternalController(context.Background(), ec)
	c.Assert(err, gc.ErrorMatches, "updating external controller state: boom")
}

func (s *serviceSuite) TestRetrieveExternalControllerSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ctrlUUID := utils.MustNewUUID().String()
	ec := crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(ctrlUUID),
		Alias:         "that-other-controller",
		Addrs:         []string{"10.10.10.10"},
		CACert:        "random-cert-string",
	}

	s.state.EXPECT().Controller(gomock.Any(), ctrlUUID).Return(&ec, nil)

	res, err := NewService(s.state, nil).Controller(context.Background(), ctrlUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.Equals, &ec)
}

func (s *serviceSuite) TestRetrieveExternalControllerError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ctrlUUID := "ctrl1"
	s.state.EXPECT().Controller(gomock.Any(), ctrlUUID).Return(nil, errors.New("boom"))

	_, err := NewService(s.state, nil).Controller(context.Background(), ctrlUUID)
	c.Assert(err, gc.ErrorMatches, "retrieving external controller ctrl1: boom")
}

func (s *serviceSuite) TestRetrieveExternalControllerForModelSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := utils.MustNewUUID().String()
	ec := crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(modelUUID),
		Alias:         "that-other-controller",
		Addrs:         []string{"10.10.10.10"},
		CACert:        "random-cert-string",
	}

	s.state.EXPECT().ControllerForModel(gomock.Any(), modelUUID).Return(&ec, nil)

	res, err := NewService(s.state, nil).ControllerForModel(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.Equals, &ec)
}

func (s *serviceSuite) TestRetrieveExternalControllerForModelError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := "model1"
	s.state.EXPECT().ControllerForModel(gomock.Any(), modelUUID).Return(nil, errors.New("boom"))

	_, err := NewService(s.state, nil).ControllerForModel(context.Background(), modelUUID)
	c.Assert(err, gc.ErrorMatches, "retrieving external controller for model model1: boom")
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}
