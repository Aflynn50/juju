// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/machine/testing"
)

type mapperSuite struct {
	jujutesting.IsolationSuite

	state *MockState
}

var _ = gc.Suite(&mapperSuite{})

func (s *mapperSuite) TestUuidToNameMapper(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	uuid0 := testing.GenUUID(c).String()
	uuid1 := testing.GenUUID(c).String()

	in := []string{uuid0, uuid1}
	out := map[string]machine.Name{
		uuid0: machine.Name("0"),
		uuid1: machine.Name("1"),
	}
	s.expectGetNamesForUUIDs(in, out)

	changesIn := []changestream.ChangeEvent{
		changeEventShim{
			changeType: 1,
			namespace:  "machine",
			changed:    uuid0,
		},
		changeEventShim{
			changeType: 2,
			namespace:  "machine",
			changed:    uuid1,
		},
	}

	service := s.getService()

	// Act
	changesOut, err := service.uuidToNameMapper(noContainersFilter)(context.Background(), changesIn)

	// Assert
	c.Assert(err, jc.ErrorIsNil)

	c.Check(changesOut, jc.SameContents, []changestream.ChangeEvent{
		changeEventShim{
			changeType: 1,
			namespace:  "machine",
			changed:    "0",
		},
		changeEventShim{
			changeType: 2,
			namespace:  "machine",
			changed:    "1",
		},
	})
}

func (s *mapperSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *mapperSuite) getService() *WatchableService {
	return &WatchableService{
		ProviderService: ProviderService{
			Service: Service{st: s.state},
		},
	}
}

func (s *mapperSuite) expectGetNamesForUUIDs(in []string, out map[string]machine.Name) {
	s.state.EXPECT().GetNamesForUUIDs(gomock.Any(), in).Return(out, nil)
}
