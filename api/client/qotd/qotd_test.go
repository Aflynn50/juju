// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package qotd_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/qotd"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
)

type qotdMockSuite struct{}

var _ = gc.Suite(&qotdMockSuite{})

func (s *qotdMockSuite) TestSetQOTDAuthor(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	user := names.NewUserTag("bobby")
	author := "William Shakespear"
	args := params.SetQOTDAuthorArgs{
		Entity: params.Entity{
			Tag: user.String(),
		},
		Author: author,
	}
	result := params.SetQOTDAuthorResult{
		Error: nil,
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetQOTDAuthor", args, &result).SetArg(3, result).Return(nil)

	qotdClient := qotd.NewClientFromCaller(mockFacadeCaller)

	found, err := qotdClient.SetQOTDAuthor(context.Background(), user, author)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.Equals, result)
}

func (s *qotdMockSuite) TestSetQOTDAuthorErrorResult(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	user := names.NewUserTag("bobby")
	author := "William Shakespear"
	args := params.SetQOTDAuthorArgs{
		Entity: params.Entity{
			Tag: user.String(),
		},
		Author: author,
	}
	errorMsg := "apiserver go boom"
	result := params.SetQOTDAuthorResult{}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetQOTDAuthor", args, &result).SetArg(3, params.SetQOTDAuthorResult{
		Error: apiservererrors.ServerError(errors.New(errorMsg)),
	}).Return(nil)

	qotdClient := qotd.NewClientFromCaller(mockFacadeCaller)

	found, err := qotdClient.SetQOTDAuthor(context.Background(), user, author)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Error, gc.ErrorMatches, errorMsg)
}

func (s *qotdMockSuite) TestQOTDDetailsFacadeCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	user := names.NewUserTag("bobby")
	author := "William Shakespear"
	args := params.SetQOTDAuthorArgs{
		Entity: params.Entity{
			Tag: user.String(),
		},
		Author: author,
	}
	result := params.SetQOTDAuthorResult{}
	errorMsg := "facade call go boom"
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetQOTDAuthor", args, &result).Return(errors.New(errorMsg))

	qotdClient := qotd.NewClientFromCaller(mockFacadeCaller)
	_, err := qotdClient.SetQOTDAuthor(context.Background(), user, author)
	c.Assert(err, gc.ErrorMatches, errorMsg)
}
