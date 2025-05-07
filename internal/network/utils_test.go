// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"errors"
	"net"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/network"
)

type UtilsSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&UtilsSuite{})

func (s *UtilsSuite) TestSupportsIPv6Error(c *tc.C) {
	s.PatchValue(network.NetListen, func(netFamily, bindAddress string) (net.Listener, error) {
		c.Check(netFamily, tc.Equals, "tcp6")
		c.Check(bindAddress, tc.Equals, "[::1]:0")
		return nil, errors.New("boom!")
	})
	c.Check(network.SupportsIPv6(), jc.IsFalse)
}

func (s *UtilsSuite) TestSupportsIPv6OK(c *tc.C) {
	s.PatchValue(network.NetListen, func(_, _ string) (net.Listener, error) {
		return &mockListener{}, nil
	})
	c.Check(network.SupportsIPv6(), jc.IsTrue)
}

type mockListener struct {
	net.Listener
}

func (*mockListener) Close() error {
	return nil
}
