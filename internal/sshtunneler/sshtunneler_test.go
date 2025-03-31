// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"context"
	"encoding/base64"
	"net"
	"sync"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/lestrrat-go/jwx/v2/jwt"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	network "github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

type sshTunnelerSuite struct {
	state      *MockState
	controller *MockControllerInfo
	dialer     *MockSSHDial
}

var _ = gc.Suite(&sshTunnelerSuite{})

func (s *sshTunnelerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.controller = NewMockControllerInfo(ctrl)
	s.dialer = NewMockSSHDial(ctrl)

	return ctrl
}

func (s *sshTunnelerSuite) TestTunneler(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker, err := NewTunnelTracker(s.state, s.controller, s.dialer)
	c.Assert(err, jc.ErrorIsNil)

	sshConnArgs := state.SSHConnRequestArg{}

	s.controller.EXPECT().Addresses().Return([]network.SpaceAddress{
		{MachineAddress: network.NewMachineAddress("1.2.3.4")},
	})
	s.state.EXPECT().InsertSSHConnRequest(gomock.Any()).DoAndReturn(
		func(sra state.SSHConnRequestArg) error {
			sshConnArgs = sra
			return nil
		},
	)
	s.dialer.EXPECT().Dial(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)

	tunnelReqArgs := RequestArgs{
		unitName:  "foo/0",
		modelUUID: "model-uuid",
	}

	req, err := tunnelTracker.RequestTunnel(tunnelReqArgs)
	c.Assert(err, jc.ErrorIsNil)

	var tunnels []string
	for uuid := range tunnelTracker.tracker {
		tunnels = append(tunnels, uuid)
	}
	c.Assert(tunnels, gc.HasLen, 1)

	tID, err := tunnelTracker.AuthenticateTunnel("reverse-tunnel", sshConnArgs.Password)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tID, gc.Equals, tunnels[0])

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := tunnelTracker.PushTunnel(context.Background(), tID, nil)
		c.Check(err, jc.ErrorIsNil)
	}()

	_, err = req.Wait(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	wg.Wait()

	c.Assert(tunnelTracker.tracker, gc.HasLen, 0)
}

func (s *sshTunnelerSuite) TestGenerateBase64JWT(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker, err := NewTunnelTracker(s.state, s.controller, s.dialer)
	c.Assert(err, jc.ErrorIsNil)

	tunnelID := "test-tunnel-id"
	token, err := tunnelTracker.generateBase64JWT(tunnelID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(token, gc.Not(gc.Equals), "")

	rawToken, err := base64.StdEncoding.DecodeString(token)
	c.Assert(err, jc.ErrorIsNil)

	parsedToken, err := jwt.Parse(rawToken, jwt.WithKey(tunnelTracker.jwtAlg, tunnelTracker.sharedSecret))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(parsedToken.Subject(), gc.Equals, "reverse-tunnel")
	c.Assert(parsedToken.PrivateClaims()["tunnelID"], gc.Equals, tunnelID)
	c.Assert(parsedToken.Issuer(), gc.Equals, "sshtunneler")
	c.Assert(parsedToken.Expiration().Sub(parsedToken.IssuedAt()), gc.Equals, maxTimeout)
}

func (s *sshTunnelerSuite) TestGenerateEphemeralSSHKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker, err := NewTunnelTracker(s.state, s.controller, s.dialer)
	c.Assert(err, jc.ErrorIsNil)

	privateKey, publicKey, err := tunnelTracker.generateEphemeralSSHKey()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(privateKey, gc.Not(gc.IsNil))
	c.Assert(publicKey, gc.Not(gc.IsNil))
}

func (s *sshTunnelerSuite) TestAuthenticateTunnel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker, err := NewTunnelTracker(s.state, s.controller, s.dialer)
	c.Assert(err, jc.ErrorIsNil)

	tunnelID := "test-tunnel-id"
	token, err := tunnelTracker.generateBase64JWT(tunnelID)
	c.Assert(err, jc.ErrorIsNil)

	authTunnelID, err := tunnelTracker.AuthenticateTunnel("reverse-tunnel", token)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(authTunnelID, gc.Equals, tunnelID)
}

func (s *sshTunnelerSuite) TestPushTunnel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker, err := NewTunnelTracker(s.state, s.controller, s.dialer)
	c.Assert(err, jc.ErrorIsNil)

	tunnelID := "test-tunnel-id"
	tunnelReq := &tunnelRequest{
		recv: make(chan net.Conn),
	}
	tunnelTracker.tracker[tunnelID] = tunnelReq

	conn := &net.TCPConn{}

	go func() {
		select {
		case receivedConn := <-tunnelReq.recv:
			c.Check(receivedConn, gc.Equals, conn)
		case <-time.After(1 * time.Second):
			c.Fatal("timeout waiting for tunnel")
		}
	}()

	err = tunnelTracker.PushTunnel(context.Background(), tunnelID, conn)
	c.Check(err, jc.ErrorIsNil)

}

func (s *sshTunnelerSuite) TestDeleteTunnel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker, err := NewTunnelTracker(s.state, s.controller, s.dialer)
	c.Assert(err, jc.ErrorIsNil)

	tunnelID := "test-tunnel-id"
	tunnelReq := &tunnelRequest{}
	tunnelTracker.tracker[tunnelID] = tunnelReq

	tunnelTracker.delete(tunnelID)
	_, ok := tunnelTracker.tracker[tunnelID]
	c.Assert(ok, gc.Equals, false)
}

func (s *sshTunnelerSuite) TestRequestTunnel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker, err := NewTunnelTracker(s.state, s.controller, s.dialer)
	c.Assert(err, jc.ErrorIsNil)

	s.controller.EXPECT().Addresses().Return([]network.SpaceAddress{
		{MachineAddress: network.NewMachineAddress("1.2.3.4")},
	})
	s.state.EXPECT().InsertSSHConnRequest(gomock.Any()).Return(nil)

	tunnelReqArgs := RequestArgs{
		unitName:  "foo/0",
		modelUUID: "model-uuid",
	}

	req, err := tunnelTracker.RequestTunnel(tunnelReqArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(req, gc.Not(gc.IsNil))
	c.Check(req.privateKey, gc.Not(gc.IsNil))
}

func (s *sshTunnelerSuite) TestGetTunnel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker, err := NewTunnelTracker(s.state, s.controller, s.dialer)
	c.Assert(err, jc.ErrorIsNil)

	tunnelID := "test-tunnel-id"
	tunnelReq := &tunnelRequest{}
	tunnelTracker.tracker[tunnelID] = tunnelReq

	req, ok := tunnelTracker.getTunnel(tunnelID)
	c.Assert(ok, gc.Equals, true)
	c.Assert(req, gc.Equals, tunnelReq)
}

func (s *sshTunnelerSuite) TestAuthenticateTunnelInvalidUsername(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker, err := NewTunnelTracker(s.state, s.controller, s.dialer)
	c.Assert(err, jc.ErrorIsNil)

	_, err = tunnelTracker.AuthenticateTunnel("invalid-username", "some-password")
	c.Assert(err, gc.ErrorMatches, "invalid username")
}

func (s *sshTunnelerSuite) TestAuthenticateTunnelInvalidToken(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker, err := NewTunnelTracker(s.state, s.controller, s.dialer)
	c.Assert(err, jc.ErrorIsNil)

	_, err = tunnelTracker.AuthenticateTunnel("reverse-tunnel", "invalid-token")
	c.Assert(err, gc.ErrorMatches, "failed to decode token: .*")
}

func (s *sshTunnelerSuite) TestAuthenticateTunnelExpiredToken(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker, err := NewTunnelTracker(s.state, s.controller, s.dialer)
	c.Assert(err, jc.ErrorIsNil)

	token, err := jwt.NewBuilder().
		Issuer("sshtunneler").
		Subject("reverse-tunnel").
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(-1*maxTimeout)).
		Claim("tunnelID", "foo").
		Build()
	c.Assert(err, jc.ErrorIsNil)

	signedToken, err := jwt.Sign(token, jwt.WithKey(tunnelTracker.jwtAlg, tunnelTracker.sharedSecret))
	c.Assert(err, jc.ErrorIsNil)

	b64Token := base64.StdEncoding.EncodeToString(signedToken)

	_, err = tunnelTracker.AuthenticateTunnel("reverse-tunnel", b64Token)
	c.Assert(err, gc.ErrorMatches, `failed to parse token: "exp" not satisfied`)
}

func (s *sshTunnelerSuite) TestPushTunnelInvalidTunnelID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelTracker, err := NewTunnelTracker(s.state, s.controller, s.dialer)
	c.Assert(err, jc.ErrorIsNil)

	err = tunnelTracker.PushTunnel(context.Background(), "invalid-tunnel-id", nil)
	c.Assert(err, gc.ErrorMatches, "tunnel not found")
}

func (s *sshTunnelerSuite) TestWaitTimeout(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tunnelReq := &tunnelRequest{
		recv:    make(chan net.Conn),
		cleanup: func() {},
	}

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
	defer cancel()
	_, err := tunnelReq.Wait(ctx)
	c.Assert(err, gc.ErrorMatches, "waiting for tunnel: context deadline exceeded")
}
