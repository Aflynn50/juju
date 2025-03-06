// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/unit"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

var _ Observer = (*AgentPresence)(nil)

type AgentPresenceSuite struct {
	testing.IsolationSuite

	domainServicesGetter *MockDomainServicesGetter
	modelService         *MockModelService
	applicationService   *MockApplicationService
}

var _ = gc.Suite(&AgentPresenceSuite{})

func (s *AgentPresenceSuite) TestLoginForUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), uuid).Return(s.modelService, nil)
	s.modelService.EXPECT().ApplicationService().Return(s.applicationService)
	s.applicationService.EXPECT().SetUnitPresence(gomock.Any(), unit.Name("foo/666")).Return(nil)

	observer := s.newObserver(c)
	observer.Login(context.Background(), names.NewUnitTag("foo/666"), names.NewModelTag("bar"), uuid, false, "user data")
}

func (s *AgentPresenceSuite) TestLoginForMachine(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), uuid).Return(s.modelService, nil)

	// TODO (stickupkid): Once the machine domain is done, this should set
	// the machine presence.

	observer := s.newObserver(c)
	observer.Login(context.Background(), names.NewMachineTag("0"), names.NewModelTag("bar"), uuid, false, "user data")
}

func (s *AgentPresenceSuite) TestLoginForUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	observer := s.newObserver(c)
	observer.Login(context.Background(), names.NewUserTag("bob"), names.NewModelTag("bar"), uuid, false, "user data")
}

func (s *AgentPresenceSuite) TestLeaveForUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), uuid).Return(s.modelService, nil)
	s.modelService.EXPECT().ApplicationService().Return(s.applicationService).Times(2)
	s.applicationService.EXPECT().SetUnitPresence(gomock.Any(), unit.Name("foo/666")).Return(nil)
	s.applicationService.EXPECT().DeleteUnitPresence(gomock.Any(), unit.Name("foo/666")).Return(nil)

	observer := s.newObserver(c)
	observer.Login(context.Background(), names.NewUnitTag("foo/666"), names.NewModelTag("bar"), uuid, false, "user data")
	observer.Leave(context.Background())
}

func (s *AgentPresenceSuite) TestLeaveForUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	observer := s.newObserver(c)
	observer.Login(context.Background(), names.NewUserTag("bob"), names.NewModelTag("bar"), uuid, false, "user data")
	observer.Leave(context.Background())
}

func (s *AgentPresenceSuite) TestLeaveWithoutLogin(c *gc.C) {
	defer s.setupMocks(c).Finish()

	observer := s.newObserver(c)
	observer.Leave(context.Background())
}

func (s *AgentPresenceSuite) newObserver(c *gc.C) *AgentPresence {
	return NewAgentPresence(AgentPresenceConfig{
		DomainServicesGetter: s.domainServicesGetter,
		Logger:               loggertesting.WrapCheckLog(c),
	})
}

func (s *AgentPresenceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.domainServicesGetter = NewMockDomainServicesGetter(ctrl)
	s.modelService = NewMockModelService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)

	return ctrl
}
