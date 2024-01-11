// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"context"
	"encoding/json"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/network"
	proxytest "github.com/juju/juju/internal/proxy/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type connectionSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&connectionSuite{})

func (s *connectionSuite) TestCloseMultipleOk(c *gc.C) {
	conn := newRPCConnection()
	broken := make(chan struct{})
	close(broken)
	apiConn := api.NewTestingConnection(api.TestingConnectionParams{
		RPCConnection: conn,
		Clock:         &fakeClock{},
		Address:       "localhost:1234",
		Broken:        broken,
		Closed:        make(chan struct{}),
	})
	c.Assert(apiConn.Close(), gc.IsNil)
	c.Assert(apiConn.Close(), gc.IsNil)
	c.Assert(apiConn.Close(), gc.IsNil)
}

func (s *connectionSuite) apiConnection() api.Connection {
	conn := newRPCConnection()
	conn.response = &params.LoginResult{
		ControllerTag: coretesting.ControllerTag.String(),
		ModelTag:      coretesting.ModelTag.String(),
		ServerVersion: "2.3-rc2",
		Servers: [][]params.HostPort{
			{
				params.HostPort{
					Address: params.Address{
						Value: "fe80:abcd::1",
						CIDR:  "128",
					},
					Port: 1234,
				},
			},
		},
		UserInfo: &params.AuthUserInfo{
			Identity:         names.NewUserTag("fred").String(),
			ControllerAccess: "superuser",
		},
		Facades: []params.FacadeVersions{{
			Name:     "Client",
			Versions: []int{1, 2, 3, 4, 5, 6},
		}},
	}

	broken := make(chan struct{})
	close(broken)
	apiConn := api.NewTestingConnection(api.TestingConnectionParams{
		RPCConnection: conn,
		ModelTag:      coretesting.ModelTag.String(),
		Clock:         &fakeClock{},
		Address:       "localhost:1234",
		Broken:        broken,
		Closed:        make(chan struct{}),
	})
	s.AddCleanup(func(c *gc.C) {
		c.Assert(apiConn.Close(), jc.ErrorIsNil)
	})
	return apiConn
}

func (s *connectionSuite) TestAPIHostPortsAlwaysIncludesTheConnection(c *gc.C) {
	apiConn := s.apiConnection()
	err := apiConn.Login(context.Background(), names.NewUserTag("admin"), jujutesting.AdminSecret, "", nil)
	c.Assert(err, jc.ErrorIsNil)

	hostPortList := apiConn.APIHostPorts()
	c.Assert(len(hostPortList), gc.Equals, 2)
	c.Assert(len(hostPortList[0]), gc.Equals, 1)
	c.Assert(hostPortList[0][0].NetPort, gc.Equals, network.NetPort(1234))
	c.Assert(hostPortList[0][0].MachineAddress.Value, gc.Equals, "localhost")
	c.Assert(len(hostPortList[1]), gc.Equals, 1)
	c.Assert(hostPortList[1][0].NetPort, gc.Equals, network.NetPort(1234))
	c.Assert(hostPortList[1][0].MachineAddress.Value, gc.Equals, "fe80:abcd::1")
}

func (s *connectionSuite) TestAPIHostPortsDoesNotIncludeConnectionProxy(c *gc.C) {
	conn := newRPCConnection()
	conn.response = &params.LoginResult{
		ControllerTag: coretesting.ControllerTag.String(),
		ModelTag:      coretesting.ModelTag.String(),
		ServerVersion: "2.3-rc2",
		Servers: [][]params.HostPort{
			{
				params.HostPort{
					Address: params.Address{
						Value: "fe80:abcd::1",
						CIDR:  "128",
					},
					Port: 1234,
				},
			},
		},
	}

	broken := make(chan struct{})
	close(broken)
	apiConn := api.NewTestingConnection(api.TestingConnectionParams{
		RPCConnection: conn,
		ModelTag:      coretesting.ModelTag.String(),
		Clock:         &fakeClock{},
		Address:       "localhost:1234",
		Broken:        broken,
		Closed:        make(chan struct{}),
		Proxier:       proxytest.NewMockTunnelProxier(),
	})
	err := apiConn.Login(context.Background(), names.NewUserTag("admin"), jujutesting.AdminSecret, "", nil)
	c.Assert(err, jc.ErrorIsNil)

	hostPortList := apiConn.APIHostPorts()
	c.Assert(len(hostPortList), gc.Equals, 1)
	c.Assert(len(hostPortList[0]), gc.Equals, 1)
	c.Assert(hostPortList[0][0].NetPort, gc.Equals, network.NetPort(1234))
	c.Assert(hostPortList[0][0].MachineAddress.Value, gc.Equals, "fe80:abcd::1")
}

func (s *connectionSuite) TestTags(c *gc.C) {
	apiConn := s.apiConnection()
	// Even though we haven't called Login, the model tag should
	// still be set.
	modelTag, ok := apiConn.ModelTag()
	c.Check(ok, jc.IsTrue)
	c.Assert(modelTag, jc.DeepEquals, coretesting.ModelTag)
	err := apiConn.Login(context.Background(), jujutesting.AdminUser, jujutesting.AdminSecret, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	// Now that we've logged in, ModelTag should still be the same.
	modelTag, ok = apiConn.ModelTag()
	c.Check(ok, jc.IsTrue)
	c.Check(modelTag, jc.DeepEquals, coretesting.ModelTag)
	controllerTag := apiConn.ControllerTag()
	c.Check(controllerTag, gc.Equals, coretesting.ControllerTag)
}

func (s *connectionSuite) TestLoginSetsControllerAccess(c *gc.C) {
	apiConn := s.apiConnection()
	err := apiConn.Login(context.Background(), names.NewUserTag("admin"), jujutesting.AdminSecret, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiConn.ControllerAccess(), gc.Equals, "superuser")
}

func asMap(v interface{}) map[string]interface{} {
	var m map[string]interface{}
	d, _ := json.Marshal(v)
	_ = json.Unmarshal(d, &m)

	return m
}

var sampleRedirectError = func() *apiservererrors.RedirectError {
	hps, _ := network.ParseProviderHostPorts("1.1.1.1:12345", "2.2.2.2:7337")
	return &apiservererrors.RedirectError{
		Servers: []network.ProviderHostPorts{hps},
		CACert:  coretesting.ServerCert,
	}
}()

func (s *connectionSuite) TestLoginToMigratedModel(c *gc.C) {
	conn := newRPCConnection()
	conn.stub.SetErrors(&rpc.RequestError{
		Code: params.CodeRedirect,
		Info: asMap(params.RedirectErrorInfo{
			ControllerTag: coretesting.ControllerTag.String(),
			Servers:       params.FromProviderHostsPorts(sampleRedirectError.Servers),
			CACert:        sampleRedirectError.CACert,
		}),
	})
	broken := make(chan struct{})
	close(broken)
	apiConn := api.NewTestingConnection(api.TestingConnectionParams{
		RPCConnection: conn,
		ModelTag:      coretesting.ModelTag.String(),
		Clock:         &fakeClock{},
		Address:       "localhost:1234",
		Broken:        broken,
		Closed:        make(chan struct{}),
	})
	err := apiConn.Login(context.Background(), names.NewUserTag("admin"), jujutesting.AdminSecret, "", nil)

	redirErr, ok := errors.Cause(err).(*api.RedirectError)
	c.Assert(ok, gc.Equals, true)

	c.Assert(redirErr.Servers, jc.DeepEquals, []network.MachineHostPorts{{
		network.NewMachineHostPorts(12345, "1.1.1.1")[0],
		network.NewMachineHostPorts(7337, "2.2.2.2")[0],
	}})
	c.Assert(redirErr.CACert, gc.Equals, coretesting.ServerCert)
	c.Assert(redirErr.FollowRedirect, gc.Equals, false)
	c.Assert(redirErr.ControllerTag.String(), gc.Equals, coretesting.ControllerTag.String())
}

func (s *connectionSuite) TestBestFacadeVersion(c *gc.C) {
	apiConn := s.apiConnection()
	err := apiConn.Login(context.Background(), names.NewUserTag("admin"), jujutesting.AdminSecret, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(apiConn.BestFacadeVersion("Client"), gc.Equals, 6)
}

func (s *connectionSuite) TestAPIHostPortsMovesConnectedValueFirst(c *gc.C) {
	goodAddress := network.MachineHostPort{
		MachineAddress: network.NewMachineAddress("localhost", network.WithScope(network.ScopeMachineLocal)),
		NetPort:        1234,
	}
	// We intentionally set this to invalid values
	badValue := network.MachineHostPort{
		MachineAddress: network.NewMachineAddress("0.1.2.3", network.WithScope(network.ScopeMachineLocal)),
		NetPort:        1234,
	}
	badServer := []network.MachineHostPort{badValue}

	extraAddress := network.MachineHostPort{
		MachineAddress: network.NewMachineAddress("0.1.2.4", network.WithScope(network.ScopeMachineLocal)),
		NetPort:        5678,
	}
	extraAddress2 := network.MachineHostPort{
		MachineAddress: network.NewMachineAddress("0.1.2.1", network.WithScope(network.ScopeMachineLocal)),
		NetPort:        9012,
	}

	current := []network.HostPorts{
		{
			network.SpaceHostPort{
				SpaceAddress: network.SpaceAddress{MachineAddress: badValue.MachineAddress},
				NetPort:      badValue.NetPort,
			},
		},
		{
			network.SpaceHostPort{
				SpaceAddress: network.SpaceAddress{MachineAddress: extraAddress.MachineAddress},
				NetPort:      extraAddress.NetPort,
			},
			network.SpaceHostPort{
				SpaceAddress: network.SpaceAddress{MachineAddress: goodAddress.MachineAddress},
				NetPort:      goodAddress.NetPort,
			},
			network.SpaceHostPort{
				SpaceAddress: network.SpaceAddress{MachineAddress: extraAddress2.MachineAddress},
				NetPort:      extraAddress2.NetPort,
			},
		},
	}

	conn := newRPCConnection()
	conn.response = &params.LoginResult{
		ControllerTag: coretesting.ControllerTag.String(),
		ModelTag:      coretesting.ModelTag.String(),
		ServerVersion: "2.3-rc2",
		Servers:       params.FromHostsPorts(current),
		UserInfo: &params.AuthUserInfo{
			Identity:         names.NewUserTag("fred").String(),
			ControllerAccess: "superuser",
		},
		Facades: []params.FacadeVersions{{
			Name:     "Client",
			Versions: []int{1, 2, 3, 4, 5, 6},
		}},
	}

	broken := make(chan struct{})
	close(broken)

	apiConn := api.NewTestingConnection(api.TestingConnectionParams{
		RPCConnection: conn,
		ModelTag:      coretesting.ModelTag.String(),
		Clock:         &fakeClock{},
		Address:       "localhost:1234",
		Broken:        broken,
		Closed:        make(chan struct{}),
	})
	err := apiConn.Login(context.Background(), names.NewUserTag("admin"), jujutesting.AdminSecret, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	hostPorts := apiConn.APIHostPorts()
	// We should have rotate the server we connected to as the first item,
	// and the address of that server as the first address
	sortedServer := []network.MachineHostPort{
		goodAddress, extraAddress, extraAddress2,
	}
	expected := []network.MachineHostPorts{sortedServer, badServer}
	c.Check(hostPorts, gc.DeepEquals, expected)
}

type slideSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&slideSuite{})

var exampleHostPorts = []network.MachineHostPort{
	{MachineAddress: network.NewMachineAddress("0.1.2.3"), NetPort: 1234},
	{MachineAddress: network.NewMachineAddress("0.1.2.4"), NetPort: 5678},
	{MachineAddress: network.NewMachineAddress("0.1.2.1"), NetPort: 9012},
	{MachineAddress: network.NewMachineAddress("0.1.9.1"), NetPort: 8888},
}

func (s *slideSuite) TestSlideToFrontNoOp(c *gc.C) {
	servers := []network.MachineHostPorts{
		{exampleHostPorts[0]},
		{exampleHostPorts[1]},
	}
	// order should not have changed
	expected := []network.MachineHostPorts{
		{exampleHostPorts[0]},
		{exampleHostPorts[1]},
	}
	api.SlideAddressToFront(servers, 0, 0)
	c.Check(servers, gc.DeepEquals, expected)
}

func (s *slideSuite) TestSlideToFrontAddress(c *gc.C) {
	servers := []network.MachineHostPorts{
		{exampleHostPorts[0], exampleHostPorts[1], exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	// server order should not change, but ports should be switched
	expected := []network.MachineHostPorts{
		{exampleHostPorts[1], exampleHostPorts[0], exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	api.SlideAddressToFront(servers, 0, 1)
	c.Check(servers, gc.DeepEquals, expected)
}

func (s *slideSuite) TestSlideToFrontServer(c *gc.C) {
	servers := []network.MachineHostPorts{
		{exampleHostPorts[0], exampleHostPorts[1]},
		{exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	// server 1 should be slid to the front
	expected := []network.MachineHostPorts{
		{exampleHostPorts[2]},
		{exampleHostPorts[0], exampleHostPorts[1]},
		{exampleHostPorts[3]},
	}
	api.SlideAddressToFront(servers, 1, 0)
	c.Check(servers, gc.DeepEquals, expected)
}

func (s *slideSuite) TestSlideToFrontBoth(c *gc.C) {
	servers := []network.MachineHostPorts{
		{exampleHostPorts[0]},
		{exampleHostPorts[1], exampleHostPorts[2]},
		{exampleHostPorts[3]},
	}
	// server 1 should be slid to the front
	expected := []network.MachineHostPorts{
		{exampleHostPorts[2], exampleHostPorts[1]},
		{exampleHostPorts[0]},
		{exampleHostPorts[3]},
	}
	api.SlideAddressToFront(servers, 1, 1)
	c.Check(servers, gc.DeepEquals, expected)
}
