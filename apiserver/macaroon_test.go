// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"net/http"
	"net/url"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/access"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = gc.Suite(&macaroonLoginSuite{})

type macaroonLoginSuite struct {
	remoteUser user.Name
	jujutesting.MacaroonSuite
}

func (s *macaroonLoginSuite) SetUpTest(c *gc.C) {
	s.remoteUser = usertesting.GenNewName(c, "testuser@somewhere")
	s.MacaroonSuite.SetUpTest(c)
}

func (s *macaroonLoginSuite) TestPublicKeyLocatorErrorIsNotPersistent(c *gc.C) {
	s.AddModelUser(c, s.remoteUser)
	s.AddControllerUser(c, s.remoteUser, permission.LoginAccess)
	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}
	workingTransport := http.DefaultTransport
	failingTransport := errorTransport{
		fallback: workingTransport,
		location: s.DischargerLocation(),
		err:      errors.New("some error"),
	}
	s.PatchValue(&http.DefaultTransport, failingTransport)
	info := s.ControllerModelApiInfo()
	_, err := s.login(c, info)
	c.Assert(err, gc.ErrorMatches, `.*: some error .*`)

	http.DefaultTransport = workingTransport

	// The error doesn't stick around.
	_, err = s.login(c, info)
	c.Assert(err, jc.ErrorIsNil)

	// Once we've succeeded, we shouldn't try again.
	http.DefaultTransport = failingTransport

	_, err = s.login(c, info)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *macaroonLoginSuite) login(c *gc.C, info *api.Info) (params.LoginResult, error) {
	cookieJar := jujutesting.NewClearableCookieJar()

	infoSkipLogin := *info
	infoSkipLogin.SkipLogin = true
	infoSkipLogin.Macaroons = nil
	client := s.OpenAPI(c, &infoSkipLogin, cookieJar)
	defer client.Close()

	var (
		request params.LoginRequest
		result  params.LoginResult
	)
	err := client.APICall(context.Background(), "Admin", 3, "", "Login", &request, &result)
	if err != nil {
		return params.LoginResult{}, errors.Annotatef(err, "cannot log in")
	}

	cookieURL := &url.URL{
		Scheme: "https",
		Host:   "localhost",
		Path:   "/",
	}

	bakeryClient := httpbakery.NewClient()

	mac := result.BakeryDischargeRequired
	if mac == nil {
		var err error
		mac, err = bakery.NewLegacyMacaroon(result.DischargeRequired)
		c.Assert(err, jc.ErrorIsNil)
	}
	err = bakeryClient.HandleError(context.Background(), cookieURL, &httpbakery.Error{
		Message: result.DischargeRequiredReason,
		Code:    httpbakery.ErrDischargeRequired,
		Info: &httpbakery.ErrorInfo{
			Macaroon:     mac,
			MacaroonPath: "/",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	// Add the macaroons that have been saved by HandleError to our login request.
	request.Macaroons = httpbakery.MacaroonsForURL(bakeryClient.Client.Jar, cookieURL)

	err = client.APICall(context.Background(), "Admin", 3, "", "Login", &request, &result)
	return result, err
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToControllerNoAccess(c *gc.C) {
	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}
	info := s.APIInfo(c)
	// Log in to the controller, not the model.
	info.ModelTag = names.ModelTag{}

	_, err := s.login(c, info)
	assertPermissionDenied(c, err)
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToControllerLoginAccess(c *gc.C) {
	s.AddControllerUser(c, permission.EveryoneUserName, permission.LoginAccess)

	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}
	info := s.APIInfo(c)
	// Log in to the controller, not the model.
	info.ModelTag = names.ModelTag{}

	result, err := s.login(c, info)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result.UserInfo, gc.NotNil)
	c.Check(result.UserInfo.Identity, gc.Equals, names.NewUserTag(s.remoteUser.Name()).String())
	c.Check(result.UserInfo.ControllerAccess, gc.Equals, "login")
	c.Check(result.UserInfo.ModelAccess, gc.Equals, "")
	c.Check(result.Servers, gc.DeepEquals, params.FromProviderHostsPorts(parseHostPortsFromAddress(c, info.Addrs...)))
}

func parseHostPortsFromAddress(c *gc.C, addresses ...string) []network.ProviderHostPorts {
	hps := make([]network.ProviderHostPorts, len(addresses))
	for i, add := range addresses {
		hp, err := network.ParseProviderHostPorts(add)
		c.Assert(err, jc.ErrorIsNil)
		hps[i] = hp
	}
	return hps
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToControllerSuperuserAccess(c *gc.C) {
	s.AddControllerUser(c, permission.EveryoneUserName, permission.SuperuserAccess)
	var remoteUserTag = names.NewUserTag(s.remoteUser.Name())

	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}
	info := s.APIInfo(c)
	// Log in to the controller, not the model.
	info.ModelTag = names.ModelTag{}

	result, err := s.login(c, info)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result.UserInfo, gc.NotNil)
	c.Check(result.UserInfo.Identity, gc.Equals, remoteUserTag.String())
	c.Check(result.UserInfo.ControllerAccess, gc.Equals, "superuser")
	c.Check(result.UserInfo.ModelAccess, gc.Equals, "")
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToModelNoExplicitAccess(c *gc.C) {
	// If we have a remote user which the controller knows nothing about,
	// and the macaroon is discharged successfully, and the user is attempting
	// to log into a model, that is permission denied.
	s.AddControllerUser(c, permission.EveryoneUserName, permission.LoginAccess)
	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}
	info := s.APIInfo(c)

	_, err := s.login(c, info)
	assertPermissionDenied(c, err)
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToModelWithExplicitAccess(c *gc.C) {
	s.testRemoteUserLoginToModelWithExplicitAccess(c, false)
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToModelWithExplicitAccessAndAllowModelAccess(c *gc.C) {
	s.testRemoteUserLoginToModelWithExplicitAccess(c, true)
}

func (s *macaroonLoginSuite) testRemoteUserLoginToModelWithExplicitAccess(c *gc.C, allowModelAccess bool) {
	apiserver.SetAllowModelAccess(s.Server, allowModelAccess)

	accessService := s.ControllerDomainServices(c).Access()
	err := accessService.UpdatePermission(context.Background(), access.UpdatePermissionArgs{
		Subject: s.remoteUser,
		Change:  permission.Grant,
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Model,
				Key:        s.ControllerModelUUID(),
			},
			Access: permission.WriteAccess,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}

	_, err = s.login(c, s.ControllerModelApiInfo())
	if allowModelAccess {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		assertPermissionDenied(c, err)
	}
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToModelWithControllerAccess(c *gc.C) {
	s.AddModelUser(c, s.remoteUser)
	s.AddControllerUser(c, s.remoteUser, permission.SuperuserAccess)

	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}
	info := s.APIInfo(c)

	result, err := s.login(c, info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserInfo, gc.NotNil)
	c.Check(result.UserInfo.Identity, gc.Equals, names.NewUserTag(s.remoteUser.Name()).String())
	c.Check(result.UserInfo.ControllerAccess, gc.Equals, "superuser")
	c.Check(result.UserInfo.ModelAccess, gc.Equals, "write")
}

func (s *macaroonLoginSuite) TestLoginToModelSuccess(c *gc.C) {
	s.AddModelUser(c, s.remoteUser)
	s.AddControllerUser(c, s.remoteUser, permission.LoginAccess)
	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	client, err := api.Open(context.Background(), s.APIInfo(c), api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer client.Close()

	// The auth tag has been correctly returned by the server.
	c.Assert(client.AuthTag(), gc.Equals, names.NewUserTag(s.remoteUser.Name()))
}

func (s *macaroonLoginSuite) TestFailedToObtainDischargeLogin(c *gc.C) {
	s.DischargerLogin = func() string {
		return ""
	}
	client, err := api.Open(context.Background(), s.APIInfo(c), api.DialOpts{})
	c.Assert(err, gc.ErrorMatches, `cannot get discharge from "https://.*": third party refused discharge: cannot discharge: login denied by discharger`)
	c.Assert(client, gc.Equals, nil)
}

func (s *macaroonLoginSuite) TestConnectStream(c *gc.C) {
	s.AddModelUser(c, s.remoteUser)
	s.AddControllerUser(c, s.remoteUser, permission.LoginAccess)

	catcher := api.UrlCatcher{}
	s.PatchValue(&api.WebsocketDial, catcher.RecordLocation)

	dischargeCount := 0
	s.DischargerLogin = func() string {
		dischargeCount++
		return s.remoteUser.Name()
	}

	// First log into the regular API.
	client, err := api.Open(context.Background(), s.APIInfo(c), api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dischargeCount, gc.Equals, 1)

	// Then check that ConnectStream works OK and that it doesn't need
	// to discharge again.
	conn, err := client.ConnectStream(context.Background(), "/path", nil)
	c.Assert(err, gc.IsNil)
	defer conn.Close()

	connectURL, err := url.Parse(catcher.Location())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(connectURL.Path, gc.Equals, "/model/"+s.ControllerModelUUID()+"/path")
	c.Check(dischargeCount, gc.Equals, 1)
}

func (s *macaroonLoginSuite) TestConnectStreamFailedDischarge(c *gc.C) {
	s.AddModelUser(c, s.remoteUser)
	s.AddControllerUser(c, s.remoteUser, permission.LoginAccess)

	// This is really a test for ConnectStream, but to test ConnectStream's
	// discharge failing logic, we need an actual endpoint to test against,
	// and the debug-log endpoint makes a convenient example.

	var dischargeError bool
	s.DischargerLogin = func() string {
		if dischargeError {
			return ""
		}
		return s.remoteUser.Name()
	}

	// Make an API connection that uses a cookie jar
	// that allows us to remove all cookies.
	jar := jujutesting.NewClearableCookieJar()
	client := s.OpenAPI(c, nil, jar)

	// Ensure that the discharger won't discharge and try
	// logging in again. We should succeed in getting past
	// authorization because we have the cookies (but
	// the actual debug-log endpoint will return an error).
	dischargeError = true
	logArgs := url.Values{"noTail": []string{"true"}}
	conn, err := client.ConnectStream(context.Background(), "/log", logArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conn, gc.NotNil)
	conn.Close()

	// Then delete all the cookies by deleting the cookie jar
	// and try again. The login should fail.
	jar.Clear()

	conn, err = client.ConnectStream(context.Background(), "/log", logArgs)
	c.Assert(err, gc.ErrorMatches, `cannot get discharge from "https://.*": third party refused discharge: cannot discharge: login denied by discharger`)
	c.Assert(conn, gc.IsNil)
}

func (s *macaroonLoginSuite) TestConnectStreamWithDischargedMacaroons(c *gc.C) {
	s.AddModelUser(c, s.remoteUser)
	s.AddControllerUser(c, s.remoteUser, permission.LoginAccess)

	// If the connection was created with already-discharged macaroons
	// (rather than acquiring them through the discharge dance), they
	// wouldn't get attached to the websocket request.
	// https://bugs.launchpad.net/juju/+bug/1650451
	catcher := api.UrlCatcher{}
	s.PatchValue(&api.WebsocketDial, catcher.RecordLocation)

	mac, err := macaroon.New([]byte("abc-123"), []byte("aurora gone"), "shankil butchers", macaroon.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)

	s.DischargerLogin = func() string {
		return s.remoteUser.Name()
	}

	info := s.APIInfo(c)
	info.Macaroons = []macaroon.Slice{{mac}}
	client := s.OpenAPI(c, info, nil)

	host := api.PreferredHost(info)
	if host == "" {
		host = info.Addrs[0]
	}

	bClient, ok := client.BakeryClient().(*httpbakery.Client)
	c.Assert(ok, jc.IsTrue)
	dischargedMacaroons := httpbakery.MacaroonsForURL(bClient.Jar, api.CookieURLFromHost(host))
	c.Assert(len(dischargedMacaroons), gc.Equals, 1)

	// Mirror the situation in migration logtransfer - the macaroon is
	// now stored in the auth service (so no further discharge is
	// needed), but we use a different client to connect to the log
	// stream, so the macaroon isn't in the cookie jar despite being
	// in the connection info.

	// Then check that ConnectStream works OK and that it doesn't need
	// to discharge again.
	s.DischargerLogin = nil

	info2 := s.APIInfo(c)
	info2.Macaroons = dischargedMacaroons

	client2 := s.OpenAPI(c, info2, nil)
	conn, err := client2.ConnectStream(context.Background(), "/path", nil)
	c.Assert(err, gc.IsNil)
	defer conn.Close()

	headers := catcher.Headers()
	c.Assert(headers.Get(httpbakery.BakeryProtocolHeader), gc.Equals, "3")
	c.Assert(headers.Get("Cookie"), jc.HasPrefix, "macaroon-")
	assertHeaderMatchesMacaroon(c, headers, dischargedMacaroons[0])
}

func assertHeaderMatchesMacaroon(c *gc.C, header http.Header, macaroon macaroon.Slice) {
	req := http.Request{Header: header}
	actualCookie := req.Cookies()[0]
	expectedCookie, err := httpbakery.NewCookie(nil, macaroon)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actualCookie.Name, gc.Equals, expectedCookie.Name)
	c.Assert(actualCookie.Value, gc.Equals, expectedCookie.Value)
}
