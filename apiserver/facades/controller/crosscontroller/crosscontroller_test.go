// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crosscontroller

import (
	"context"
	"errors"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var _ = gc.Suite(&CrossControllerSuite{})

type CrossControllerSuite struct {
	testing.IsolationSuite

	resources                *common.Resources
	watcher                  *mockNotifyWatcher
	localControllerInfo      func() ([]string, string, error)
	watchLocalControllerInfo func() state.NotifyWatcher
	api                      *CrossControllerAPI

	publicDnsAddress string
}

func (s *CrossControllerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })
	s.localControllerInfo = func() ([]string, string, error) {
		return []string{"addr1", "addr2"}, "ca-cert", nil
	}
	s.watchLocalControllerInfo = func() state.NotifyWatcher {
		return s.watcher
	}
	api, err := NewCrossControllerAPI(
		s.resources,
		func(context.Context) ([]string, string, error) { return s.localControllerInfo() },
		func(context.Context) (string, error) { return s.publicDnsAddress, nil },
		func() state.NotifyWatcher { return s.watchLocalControllerInfo() },
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
	s.watcher = newMockNotifyWatcher()
	s.AddCleanup(func(*gc.C) { _ = s.watcher.Stop() })
}

func (s *CrossControllerSuite) TestControllerInfo(c *gc.C) {
	results, err := s.api.ControllerInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ControllerAPIInfoResults{
		Results: []params.ControllerAPIInfoResult{{
			Addresses: []string{"addr1", "addr2"},
			CACert:    "ca-cert",
		}},
	})
}

func (s *CrossControllerSuite) TestControllerInfoWithDNSAddress(c *gc.C) {
	s.publicDnsAddress = "publicDNSaddr"
	results, err := s.api.ControllerInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ControllerAPIInfoResults{
		Results: []params.ControllerAPIInfoResult{{
			Addresses: []string{"publicDNSaddr", "addr1", "addr2"},
			CACert:    "ca-cert",
		}},
	})
}

func (s *CrossControllerSuite) TestControllerInfoError(c *gc.C) {
	s.localControllerInfo = func() ([]string, string, error) {
		return nil, "", errors.New("nope")
	}
	results, err := s.api.ControllerInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ControllerAPIInfoResults{
		Results: []params.ControllerAPIInfoResult{{
			Error: &params.Error{Message: "nope"},
		}},
	})
}

func (s *CrossControllerSuite) TestWatchControllerInfo(c *gc.C) {
	s.watcher.changes <- struct{}{} // initial value
	results, err := s.api.WatchControllerInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			NotifyWatcherId: "1",
		}},
	})
	c.Assert(s.resources.Get("1"), gc.Equals, s.watcher)
}

func (s *CrossControllerSuite) TestWatchControllerInfoError(c *gc.C) {
	s.watcher.tomb.Kill(errors.New("nope"))
	close(s.watcher.changes)

	results, err := s.api.WatchControllerInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			Error: &params.Error{Message: "nope"},
		}},
	})
	c.Assert(s.resources.Get("1"), gc.IsNil)
}

type stubControllerInfoGetter struct{}

func (stubControllerInfoGetter) APIHostPortsForClients(config controller.Config) ([]network.SpaceHostPorts, error) {
	return []network.SpaceHostPorts{{
		network.SpaceHostPort{
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.1.2.3",
					Scope: network.ScopeCloudLocal,
				},
				SpaceID: "0",
			},
			NetPort: 50000,
		},
		network.SpaceHostPort{
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "host-name",
					Scope: network.ScopePublic,
				},
				SpaceID: "0",
			},
			NetPort: 50000,
		},
	}}, nil
}

func (s *CrossControllerSuite) TestGetControllerInfo(c *gc.C) {
	addrs, cert, err := controllerInfo(stubControllerInfoGetter{}, controller.Config{
		"ca-cert": "ca-cert",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Public address is sorted first.
	c.Check(addrs, jc.DeepEquals, []string{"host-name:50000", "10.1.2.3:50000"})
	c.Check(cert, gc.Equals, "ca-cert")
}
