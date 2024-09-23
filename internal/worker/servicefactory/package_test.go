// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"testing"

	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
	domaintesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package servicefactory -destination servicefactory_mock_test.go github.com/juju/juju/internal/servicefactory ControllerServiceFactory,ModelServiceFactory,ServiceFactory,ServiceFactoryGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package servicefactory -destination database_mock_test.go github.com/juju/juju/core/database DBDeleter
//go:generate go run go.uber.org/mock/mockgen -typed -package servicefactory -destination changestream_mock_test.go github.com/juju/juju/core/changestream WatchableDBGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package servicefactory -destination providertracker_mock_test.go github.com/juju/juju/core/providertracker Provider,ProviderFactory
//go:generate go run go.uber.org/mock/mockgen -typed -package servicefactory -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore,ObjectStoreGetter,ModelObjectStoreGetter

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	domaintesting.ControllerSuite

	logger    logger.Logger
	dbDeleter *MockDBDeleter
	dbGetter  *MockWatchableDBGetter

	serviceFactoryGetter     *MockServiceFactoryGetter
	controllerServiceFactory *MockControllerServiceFactory
	modelServiceFactory      *MockModelServiceFactory

	provider        *MockProvider
	providerFactory *MockProviderFactory

	objectStore            *MockObjectStore
	objectStoreGetter      *MockObjectStoreGetter
	modelObjectStoreGetter *MockModelObjectStoreGetter
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = loggertesting.WrapCheckLog(c)
	s.dbDeleter = NewMockDBDeleter(ctrl)
	s.dbGetter = NewMockWatchableDBGetter(ctrl)

	s.serviceFactoryGetter = NewMockServiceFactoryGetter(ctrl)
	s.controllerServiceFactory = NewMockControllerServiceFactory(ctrl)
	s.modelServiceFactory = NewMockModelServiceFactory(ctrl)

	s.provider = NewMockProvider(ctrl)
	s.providerFactory = NewMockProviderFactory(ctrl)

	s.objectStore = NewMockObjectStore(ctrl)
	s.objectStoreGetter = NewMockObjectStoreGetter(ctrl)
	s.modelObjectStoreGetter = NewMockModelObjectStoreGetter(ctrl)

	return ctrl
}
