// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/status"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/statushistory"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go -source=./service.go
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination charm_mock_test.go github.com/juju/juju/domain/application/service CharmStore,WatcherFactory
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination internal_charm_mock_test.go github.com/juju/juju/internal/charm Charm
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination constraints_mock_test.go github.com/juju/juju/core/constraints Validator
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination leader_mock_test.go github.com/juju/juju/core/leadership Ensurer

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	modelID model.UUID

	state                     *MockState
	charm                     *MockCharm
	charmStore                *MockCharmStore
	agentVersionGetter        *MockAgentVersionGetter
	provider                  *MockProvider
	supportedFeaturesProvider *MockSupportedFeatureProvider
	leadership                *MockEnsurer
	validator                 *MockValidator

	storageRegistryGetter corestorage.ModelStorageRegistryGetter
	clock                 *testclock.Clock

	service *ProviderService
}

func (s *baseSuite) setupMocksWithProvider(
	c *gc.C,
	providerGetter func(ctx context.Context) (Provider, error),
	supportFeaturesProviderGetter func(ctx context.Context) (SupportedFeatureProvider, error),
) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelID = modeltesting.GenModelUUID(c)

	s.agentVersionGetter = NewMockAgentVersionGetter(ctrl)
	s.provider = NewMockProvider(ctrl)
	s.supportedFeaturesProvider = NewMockSupportedFeatureProvider(ctrl)
	s.leadership = NewMockEnsurer(ctrl)

	s.state = NewMockState(ctrl)
	s.charm = NewMockCharm(ctrl)
	s.charmStore = NewMockCharmStore(ctrl)
	s.validator = NewMockValidator(ctrl)

	s.storageRegistryGetter = corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
		return storage.ChainedProviderRegistry{
			dummystorage.StorageProviders(),
			provider.CommonStorageProviders(),
		}
	})

	s.clock = testclock.NewClock(time.Time{})
	s.service = NewProviderService(
		s.state,
		s.leadership,
		s.storageRegistryGetter,
		s.modelID,
		s.agentVersionGetter,
		providerGetter,
		supportFeaturesProviderGetter,
		s.charmStore,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c)),
		s.clock,
		loggertesting.WrapCheckLog(c),
	)
	s.service.clock = s.clock

	return ctrl
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	return s.setupMocksWithStatusHistory(c, domain.NewStatusHistory(loggertesting.WrapCheckLog(c)))
}

func (s *baseSuite) setupMocksWithStatusHistory(c *gc.C, statusHistory StatusHistory) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelID = modeltesting.GenModelUUID(c)

	s.agentVersionGetter = NewMockAgentVersionGetter(ctrl)
	s.provider = NewMockProvider(ctrl)
	s.supportedFeaturesProvider = NewMockSupportedFeatureProvider(ctrl)
	s.leadership = NewMockEnsurer(ctrl)

	s.state = NewMockState(ctrl)
	s.charm = NewMockCharm(ctrl)
	s.charmStore = NewMockCharmStore(ctrl)
	s.validator = NewMockValidator(ctrl)

	s.storageRegistryGetter = corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
		return storage.ChainedProviderRegistry{
			dummystorage.StorageProviders(),
			provider.CommonStorageProviders(),
		}
	})

	s.clock = testclock.NewClock(time.Time{})
	s.service = NewProviderService(
		s.state,
		s.leadership,
		s.storageRegistryGetter,
		s.modelID,
		s.agentVersionGetter,
		func(ctx context.Context) (Provider, error) {
			return s.provider, nil
		},
		func(ctx context.Context) (SupportedFeatureProvider, error) {
			return s.supportedFeaturesProvider, nil
		},
		s.charmStore,
		statusHistory,
		s.clock,
		loggertesting.WrapCheckLog(c),
	)
	s.service.clock = s.clock

	return ctrl

}

func (s *baseSuite) minimalManifest() charm.Manifest {
	return charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk: charm.RiskStable,
				},
				Architectures: []string{"amd64"},
			},
		},
	}
}

type changeEvent struct {
	typ       changestream.ChangeType
	namespace string
	changed   string
}

var _ changestream.ChangeEvent = (*changeEvent)(nil)

func (c *changeEvent) Type() changestream.ChangeType {
	return c.typ
}

func (c *changeEvent) Namespace() string {
	return c.namespace
}

func (c *changeEvent) Changed() string {
	return c.changed
}

type statusHistoryRecord struct {
	ns statushistory.Namespace
	s  status.StatusInfo
}

type statusHistoryRecorder struct {
	records []statusHistoryRecord
}

// RecordStatus records the given status information.
// If the status data cannot be marshalled, it will not be recorded, instead
// the error will be logged under the data_error key.
func (r *statusHistoryRecorder) RecordStatus(ctx context.Context, ns statushistory.Namespace, s status.StatusInfo) error {
	r.records = append(r.records, statusHistoryRecord{ns: ns, s: s})
	return nil
}
