// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/blockcommand"
)

type exportSuite struct {
	coordinator *MockCoordinator
	service     *MockExportService
}

var _ = gc.Suite(&exportSuite{})

func (s *exportSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockExportService(ctrl)

	return ctrl
}

func (s *exportSuite) newExportOperation() *exportOperation {
	return &exportOperation{
		service: s.service,
	}
}

func (s *exportSuite) TestExport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	s.service.EXPECT().GetBlocks(gomock.Any()).Return([]blockcommand.Block{
		{Type: blockcommand.ChangeBlock, Message: "foo"},
		{Type: blockcommand.RemoveBlock, Message: "bar"},
		{Type: blockcommand.DestroyBlock, Message: "baz"},
	}, nil)

	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(dst.Blocks(), jc.DeepEquals, map[string]string{
		"all-changes":   "foo",
		"remove-object": "bar",
		"destroy-model": "baz",
	})
}

func (s *exportSuite) TestExportEmptyBlocks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	s.service.EXPECT().GetBlocks(gomock.Any()).Return([]blockcommand.Block{}, nil)

	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(dst.Blocks(), jc.DeepEquals, map[string]string{})
}
