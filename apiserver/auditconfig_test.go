// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"math"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	servertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type auditConfigSuite struct {
	testing.ApiServerSuite
}

var _ = gc.Suite(&auditConfigSuite{})

func (s *auditConfigSuite) openAPIWithoutLogin(c *gc.C) api.Connection {
	info := s.ControllerModelApiInfo()
	info.Tag = nil
	info.Password = ""
	info.Macaroons = nil
	info.SkipLogin = true
	conn, err := api.Open(context.Background(), info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	return conn
}

func (s *auditConfigSuite) TestLoginAddsAuditConversationEventually(c *gc.C) {
	log := &servertesting.FakeAuditLog{}
	s.WithAuditLogConfig = &auditlog.Config{
		Enabled: true,
		Target:  log,
	}

	userTag := names.NewUserTag("bobbrown")
	password := "password"
	s.createModelAdminUser(c, userTag, password)

	conn := s.openAPIWithoutLogin(c)

	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag:       userTag.String(),
		Credentials:   password,
		CLIArgs:       "hey you guys",
		ClientVersion: jujuversion.Current.String(),
	}
	loginTime := s.Clock.Now()
	err := conn.APICall(context.Background(), "Admin", 3, "", "Login", request, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserInfo, gc.NotNil)
	// Nothing's logged at this point because there haven't been any
	// interesting requests.
	log.CheckCallNames(c)

	var addResults params.AddMachinesResults
	addReq := &params.AddMachines{
		MachineParams: []params.AddMachineParams{{
			Jobs: []model.MachineJob{"JobHostUnits"},
		}},
	}
	addMachinesTime := s.Clock.Now()
	err = conn.APICall(context.Background(), "MachineManager", machineManagerFacadeVersion, "", "AddMachines", addReq, &addResults)
	c.Assert(err, jc.ErrorIsNil)

	log.CheckCallNames(c, "AddConversation", "AddRequest", "AddResponse")

	convo := log.Calls()[0].Args[0].(auditlog.Conversation)
	mc := jc.NewMultiChecker()
	mc.AddExpr("_.ConversationID", gc.HasLen, 16)
	mc.AddExpr("_.ConnectionID", jc.Ignore)
	mc.AddExpr("_.When", jc.Satisfies, func(s string) bool {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return false
		}
		return math.Abs(t.Sub(loginTime).Seconds()) < 1.0
	})
	c.Assert(convo, mc, auditlog.Conversation{
		Who:       userTag.Id(),
		What:      "hey you guys",
		ModelName: "controller",
		ModelUUID: s.ControllerModelUUID(),
	})

	auditReq := log.Calls()[1].Args[0].(auditlog.Request)
	mc = jc.NewMultiChecker()
	mc.AddExpr("_.ConversationID", jc.Ignore)
	mc.AddExpr("_.ConnectionID", jc.Ignore)
	mc.AddExpr("_.RequestID", jc.Ignore)
	mc.AddExpr("_.When", jc.Satisfies, func(s string) bool {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return false
		}
		return math.Abs(t.Sub(addMachinesTime).Seconds()) < 1.0
	})
	c.Assert(auditReq, mc, auditlog.Request{
		Facade:  "MachineManager",
		Method:  "AddMachines",
		Version: machineManagerFacadeVersion,
	})
}

func (s *auditConfigSuite) TestAuditLoggingFailureOnInterestingRequest(c *gc.C) {
	log := &servertesting.FakeAuditLog{}
	log.SetErrors(errors.Errorf("bad news bears"))
	s.WithAuditLogConfig = &auditlog.Config{
		Enabled: true,
		Target:  log,
	}

	userTag := names.NewUserTag("bobbrown")
	password := "password"
	s.createModelAdminUser(c, userTag, password)

	conn := s.openAPIWithoutLogin(c)

	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag:       userTag.String(),
		Credentials:   password,
		CLIArgs:       "hey you guys",
		ClientVersion: jujuversion.Current.String(),
	}
	err := conn.APICall(context.Background(), "Admin", 3, "", "Login", request, &result)
	// No error yet since logging the conversation is deferred until
	// something happens.
	c.Assert(err, jc.ErrorIsNil)

	var addResults params.AddMachinesResults
	addReq := &params.AddMachines{
		MachineParams: []params.AddMachineParams{{
			Jobs: []model.MachineJob{"JobHostUnits"},
		}},
	}
	err = conn.APICall(context.Background(), "MachineManager", machineManagerFacadeVersion, "", "AddMachines", addReq, &addResults)
	c.Assert(err, gc.ErrorMatches, "bad news bears")
}

func (s *auditConfigSuite) TestAuditLoggingUsesExcludeMethods(c *gc.C) {
	log := &servertesting.FakeAuditLog{}
	s.WithAuditLogConfig = &auditlog.Config{
		Enabled:        true,
		ExcludeMethods: set.NewStrings("MachineManager.AddMachines"),
		Target:         log,
	}

	userTag := names.NewUserTag("bobbrown")
	password := "password"
	s.createModelAdminUser(c, userTag, password)

	conn := s.openAPIWithoutLogin(c)

	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag:       userTag.String(),
		Credentials:   password,
		CLIArgs:       "hey you guys",
		ClientVersion: jujuversion.Current.String(),
	}
	err := conn.APICall(context.Background(), "Admin", 3, "", "Login", request, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserInfo, gc.NotNil)
	// Nothing's logged at this point because there haven't been any
	// interesting requests.
	log.CheckCallNames(c)

	var addResults params.AddMachinesResults
	addReq := &params.AddMachines{
		MachineParams: []params.AddMachineParams{{
			Jobs: []model.MachineJob{"JobHostUnits"},
		}},
	}
	err = conn.APICall(context.Background(), "MachineManager", machineManagerFacadeVersion, "", "AddMachines", addReq, &addResults)
	c.Assert(err, jc.ErrorIsNil)

	// Still nothing logged - the AddMachines call has been filtered out.
	log.CheckCallNames(c)

	// Call something else.
	destroyReq := &params.DestroyMachinesParams{
		MachineTags: []string{addResults.Machines[0].Machine},
	}
	err = conn.APICall(context.Background(), "MachineManager", machineManagerFacadeVersion, "", "DestroyMachineWithParams", destroyReq, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Now the conversation and both requests are logged.
	log.CheckCallNames(c, "AddConversation", "AddRequest", "AddResponse", "AddRequest", "AddResponse")

	req1 := log.Calls()[1].Args[0].(auditlog.Request)
	c.Assert(req1.Facade, gc.Equals, "MachineManager")
	c.Assert(req1.Method, gc.Equals, "AddMachines")

	req2 := log.Calls()[3].Args[0].(auditlog.Request)
	c.Assert(req2.Facade, gc.Equals, "MachineManager")
	c.Assert(req2.Method, gc.Equals, "DestroyMachineWithParams")
}

func (s *auditConfigSuite) TestNewServerValidatesConfig(c *gc.C) {
	cfg := testing.DefaultServerConfig(c, nil)
	cfg.GetAuditConfig = nil
	cfg.DomainServicesGetter = s.DomainServicesGetter(c, s.NoopObjectStore(c), s.NoopLeaseManager(c))

	srv, err := apiserver.NewServer(context.Background(), cfg)
	c.Assert(err, gc.ErrorMatches, "missing GetAuditConfig not valid")
	c.Assert(srv, gc.IsNil)
}

func (s *auditConfigSuite) createModelAdminUser(c *gc.C, userTag names.UserTag, password string) {
	accessService := s.ControllerDomainServices(c).Access()

	_, _, err := accessService.AddUser(context.Background(), service.AddUserArg{
		Name:        user.NameFromTag(userTag),
		DisplayName: userTag.Name(),
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword(password)),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = accessService.CreatePermission(context.Background(), permission.UserAccessSpec{
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Model,
				Key:        s.ControllerModelUUID(),
			},
			Access: permission.AdminAccess,
		},
		User: user.NameFromTag(userTag),
	})
	c.Assert(err, jc.ErrorIsNil)
}
