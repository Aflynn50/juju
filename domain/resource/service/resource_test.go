// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	applicationtesting "github.com/juju/juju/core/application/testing"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	coreresourcestore "github.com/juju/juju/core/resource/store"
	storetesting "github.com/juju/juju/core/resource/store/testing"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	unittesting "github.com/juju/juju/core/unit/testing"
	containerimageresourcestoreerrors "github.com/juju/juju/domain/containerimageresourcestore/errors"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	charmresource "github.com/juju/juju/internal/charm/resource"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type resourceServiceSuite struct {
	jujutesting.IsolationSuite

	state               *MockState
	resourceStoreGetter *MockResourceStoreGetter
	resourceStore       *MockResourceStore

	service *Service
}

var _ = gc.Suite(&resourceServiceSuite{})

func (s *resourceServiceSuite) TestDeleteApplicationResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().DeleteApplicationResources(gomock.Any(),
		appUUID).Return(nil)

	err := s.service.DeleteApplicationResources(context.
		Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestDeleteApplicationResourcesBadArgs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.DeleteApplicationResources(context.
		Background(), "not an application ID")
	c.Assert(err, jc.ErrorIs, resourceerrors.ApplicationIDNotValid,
		gc.Commentf("Application ID should be stated as not valid"))
}

func (s *resourceServiceSuite) TestDeleteApplicationResourcesUnexpectedError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	stateError := errors.New("unexpected error")

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().DeleteApplicationResources(gomock.Any(),
		appUUID).Return(stateError)

	err := s.service.DeleteApplicationResources(context.
		Background(), appUUID)
	c.Assert(err, jc.ErrorIs, stateError,
		gc.Commentf("Should return underlying error"))
}

func (s *resourceServiceSuite) TestDeleteUnitResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().DeleteUnitResources(gomock.Any(),
		unitUUID).Return(nil)

	err := s.service.DeleteUnitResources(context.
		Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestDeleteUnitResourcesBadArgs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.DeleteUnitResources(context.
		Background(), "not an unit UUID")
	c.Assert(err, jc.ErrorIs, resourceerrors.UnitUUIDNotValid,
		gc.Commentf("Unit UUID should be stated as not valid"))
}

func (s *resourceServiceSuite) TestDeleteUnitResourcesUnexpectedError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	stateError := errors.New("unexpected error")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().DeleteUnitResources(gomock.Any(),
		unitUUID).Return(stateError)

	err := s.service.DeleteUnitResources(context.
		Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, stateError,
		gc.Commentf("Should return underlying error"))
}

func (s *resourceServiceSuite) TestGetResourceUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	retID := resourcetesting.GenResourceUUID(c)
	s.state.EXPECT().GetResourceUUID(
		gomock.Any(),
		applicationtesting.GenApplicationUUID(c),
		"test-resource",
	).Return(retID, nil)

	ret, err := s.service.GetResourceUUID(
		context.Background(),
		applicationtesting.GenApplicationUUID(c),
		"test-resource",
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ret, gc.Equals, retID)
}

func (s *resourceServiceSuite) TestGetResourceUUIDBadID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetResourceUUID(context.Background(), "", "test-resource")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *resourceServiceSuite) TestGetResourceUUIDBadName(c *gc.C) {
	defer s.setupMocks(c).Finish()
	_, err := s.service.GetResourceUUID(context.Background(), applicationtesting.GenApplicationUUID(c), "")
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceNameNotValid)
}

func (s *resourceServiceSuite) TestListResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)
	expectedList := resource.ApplicationResources{
		Resources: []resource.Resource{{
			RetrievedBy:     "admin",
			RetrievedByType: resource.Application,
		}},
	}
	s.state.EXPECT().ListResources(gomock.Any(), id).Return(expectedList, nil)

	obtainedList, err := s.service.ListResources(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedList, gc.DeepEquals, expectedList)
}

func (s *resourceServiceSuite) TestListResourcesBadID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	_, err := s.service.ListResources(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *resourceServiceSuite) TestGetResource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := resourcetesting.GenResourceUUID(c)
	expectedRes := resource.Resource{
		RetrievedBy:     "admin",
		RetrievedByType: resource.Application,
	}
	s.state.EXPECT().GetResource(gomock.Any(), id).Return(expectedRes, nil)

	obtainedRes, err := s.service.GetResource(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedRes, gc.DeepEquals, expectedRes)
}

func (s *resourceServiceSuite) TestGetResourceBadID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	_, err := s.service.GetResource(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

var fingerprint = []byte("123456789012345678901234567890123456789012345678")

func (s *resourceServiceSuite) TestSetApplicationResource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUID := resourcetesting.GenResourceUUID(c)
	s.state.EXPECT().SetApplicationResource(gomock.Any(), resourceUUID)

	err := s.service.SetApplicationResource(
		context.Background(),
		resourceUUID,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestSetApplicationResourceBadResourceUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetApplicationResource(context.Background(), "bad-uuid")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *resourceServiceSuite) TestStoreResource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUID := resourcetesting.GenResourceUUID(c)
	resourceType := charmresource.TypeFile

	reader := bytes.NewBufferString("spamspamspam")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	size := int64(42)

	retrievedBy := "bob"
	retrievedByType := resource.User

	storageID := storetesting.GenFileResourceStoreID(c, objectstoretesting.GenObjectStoreUUID(c))
	s.state.EXPECT().GetResource(gomock.Any(), resourceUUID).Return(
		resource.Resource{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Type: resourceType,
				},
				Fingerprint: fp,
				Size:        size,
			},
		}, nil,
	)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), resourceType).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Put(
		gomock.Any(),
		resourceUUID.String(),
		reader,
		size,
		coreresourcestore.NewFingerprint(fp.Fingerprint),
	).Return(storageID, nil)
	s.state.EXPECT().RecordStoredResource(gomock.Any(), resource.RecordStoredResourceArgs{
		ResourceUUID:                  resourceUUID,
		StorageID:                     storageID,
		RetrievedBy:                   retrievedBy,
		RetrievedByType:               retrievedByType,
		ResourceType:                  resourceType,
		IncrementCharmModifiedVersion: false,
	})

	err = s.service.StoreResource(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID:    resourceUUID,
			Reader:          reader,
			RetrievedBy:     retrievedBy,
			RetrievedByType: retrievedByType,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestStoreResourceRemovedOnRecordError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUID := resourcetesting.GenResourceUUID(c)
	resourceType := charmresource.TypeFile

	reader := bytes.NewBufferString("spamspamspam")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	size := int64(42)

	retrievedBy := "bob"
	retrievedByType := resource.User

	storageID := storetesting.GenFileResourceStoreID(c, objectstoretesting.GenObjectStoreUUID(c))
	s.state.EXPECT().GetResource(gomock.Any(), resourceUUID).Return(
		resource.Resource{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Type: resourceType,
				},
				Fingerprint: fp,
				Size:        size,
			},
		}, nil,
	)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), resourceType).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Put(
		gomock.Any(),
		resourceUUID.String(),
		reader,
		size,
		coreresourcestore.NewFingerprint(fp.Fingerprint),
	).Return(storageID, nil)

	// Return an error from recording the stored resource.
	expectedErr := errors.New("recording failed with massive error")
	s.state.EXPECT().RecordStoredResource(gomock.Any(), resource.RecordStoredResourceArgs{
		ResourceUUID:                  resourceUUID,
		StorageID:                     storageID,
		RetrievedBy:                   retrievedBy,
		RetrievedByType:               retrievedByType,
		ResourceType:                  resourceType,
		IncrementCharmModifiedVersion: false,
	}).Return(expectedErr)

	// Expect the removal of the resource.
	s.resourceStore.EXPECT().Remove(gomock.Any(), resourceUUID.String())

	err = s.service.StoreResource(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID:    resourceUUID,
			Reader:          reader,
			RetrievedBy:     retrievedBy,
			RetrievedByType: retrievedByType,
		},
	)
	c.Assert(err, jc.ErrorIs, expectedErr)
}

func (s *resourceServiceSuite) TestStoreResourceBadUUID(c *gc.C) {
	err := s.service.StoreResource(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID: "bad-uuid",
		},
	)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *resourceServiceSuite) TestStoreResourceNilReader(c *gc.C) {
	err := s.service.StoreResource(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID: resourcetesting.GenResourceUUID(c),
			Reader:       nil,
		},
	)
	c.Assert(err, gc.ErrorMatches, "cannot have nil reader")
}

func (s *resourceServiceSuite) TestStoreResourceBadRetrievedBy(c *gc.C) {
	err := s.service.StoreResource(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID:    resourcetesting.GenResourceUUID(c),
			Reader:          bytes.NewBufferString("spam"),
			RetrievedBy:     "bob",
			RetrievedByType: resource.Unknown,
		},
	)
	c.Assert(err, jc.ErrorIs, resourceerrors.RetrievedByTypeNotValid)
}

func (s *resourceServiceSuite) TestStoreResourceAndIncrementCharmModifiedVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUID := resourcetesting.GenResourceUUID(c)
	resourceType := charmresource.TypeFile

	reader := bytes.NewBufferString("spamspamspam")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	size := int64(42)

	retrievedBy := "bob"
	retrievedByType := resource.User

	storageID := storetesting.GenFileResourceStoreID(c, objectstoretesting.GenObjectStoreUUID(c))
	s.state.EXPECT().GetResource(gomock.Any(), resourceUUID).Return(
		resource.Resource{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Type: resourceType,
				},
				Fingerprint: fp,
				Size:        size,
			},
		}, nil,
	)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), resourceType).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Put(
		gomock.Any(),
		resourceUUID.String(),
		reader,
		size,
		coreresourcestore.NewFingerprint(fp.Fingerprint),
	).Return(storageID, nil)
	s.state.EXPECT().RecordStoredResource(gomock.Any(), resource.RecordStoredResourceArgs{
		ResourceUUID:                  resourceUUID,
		StorageID:                     storageID,
		RetrievedBy:                   retrievedBy,
		RetrievedByType:               retrievedByType,
		ResourceType:                  resourceType,
		IncrementCharmModifiedVersion: true,
	})

	err = s.service.StoreResourceAndIncrementCharmModifiedVersion(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID:    resourceUUID,
			Reader:          reader,
			RetrievedBy:     retrievedBy,
			RetrievedByType: retrievedByType,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestStoreResourceAndIncrementCharmModifiedVersionBadUUID(c *gc.C) {
	err := s.service.StoreResourceAndIncrementCharmModifiedVersion(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID: "bad-uuid",
		},
	)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *resourceServiceSuite) TestStoreResourceAndIncrementCharmModifiedVersionNilReader(c *gc.C) {
	err := s.service.StoreResourceAndIncrementCharmModifiedVersion(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID: resourcetesting.GenResourceUUID(c),
			Reader:       nil,
		},
	)
	c.Assert(err, gc.ErrorMatches, "cannot have nil reader")
}

func (s *resourceServiceSuite) TestStoreResourceAndIncrementCharmModifiedVersionBadRetrievedBy(c *gc.C) {
	err := s.service.StoreResourceAndIncrementCharmModifiedVersion(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID:    resourcetesting.GenResourceUUID(c),
			Reader:          bytes.NewBufferString("spam"),
			RetrievedBy:     "bob",
			RetrievedByType: resource.Unknown,
		},
	)
	c.Assert(err, jc.ErrorIs, resourceerrors.RetrievedByTypeNotValid)
}

func (s *resourceServiceSuite) TestSetUnitResource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUID := resourcetesting.GenResourceUUID(c)
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().SetUnitResource(gomock.Any(), resourceUUID, unitUUID).Return(nil)

	err := s.service.SetUnitResource(context.Background(), resourceUUID, unitUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestSetUnitResourceBadResourceUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	err := s.service.SetUnitResource(context.Background(), "bad-uuid", unitUUID)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *resourceServiceSuite) TestSetUnitResourceBadUnitUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUID := resourcetesting.GenResourceUUID(c)

	err := s.service.SetUnitResource(context.Background(), resourceUUID, "bad-uuid")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *resourceServiceSuite) TestOpenResource(c *gc.C) {
	defer s.setupMocks(c).Finish()
	id := resourcetesting.GenResourceUUID(c)
	reader := io.NopCloser(bytes.NewBufferString("spam"))
	resourceType := charmresource.TypeFile
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	size := int64(42)
	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Type: resourceType,
			},
			Fingerprint: fp,
			Size:        size,
		},
		UUID: id,
	}

	s.state.EXPECT().GetResource(gomock.Any(), id).Return(res, nil)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), resourceType).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Get(
		gomock.Any(),
		id.String(),
	).Return(reader, size, nil)

	obtainedRes, obtainedReader, err := s.service.OpenResource(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedRes, gc.DeepEquals, res)
	c.Assert(obtainedReader, gc.DeepEquals, reader)
}

func (s *resourceServiceSuite) TestOpenResourceFileNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	id := resourcetesting.GenResourceUUID(c)
	resourceType := charmresource.TypeFile
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	size := int64(42)
	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Type: resourceType,
			},
			Fingerprint: fp,
			Size:        size,
		},
	}

	s.state.EXPECT().GetResource(gomock.Any(), id).Return(res, nil)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), resourceType).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Get(
		gomock.Any(),
		id.String(),
	).Return(nil, 0, objectstoreerrors.ErrNotFound)

	_, _, err = s.service.OpenResource(context.Background(), id)
	c.Assert(err, jc.ErrorIs, resourceerrors.StoredResourceNotFound)
}

func (s *resourceServiceSuite) TestOpenResourceContainerImageNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	id := resourcetesting.GenResourceUUID(c)
	resourceType := charmresource.TypeContainerImage
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	size := int64(42)
	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Type: resourceType,
			},
			Fingerprint: fp,
			Size:        size,
		},
	}

	s.state.EXPECT().GetResource(gomock.Any(), id).Return(res, nil)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), resourceType).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Get(
		gomock.Any(),
		id.String(),
	).Return(nil, 0, containerimageresourcestoreerrors.ContainerImageMetadataNotFound)

	_, _, err = s.service.OpenResource(context.Background(), id)
	c.Assert(err, jc.ErrorIs, resourceerrors.StoredResourceNotFound)
}

func (s *resourceServiceSuite) TestOpenResourceBadID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, _, err := s.service.OpenResource(context.Background(), "id")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *resourceServiceSuite) TestSetRepositoryResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	args := resource.SetRepositoryResourcesArgs{
		ApplicationID: applicationtesting.GenApplicationUUID(c),
		Info: []charmresource.Resource{{

			Meta: charmresource.Meta{
				Name:        "my-resource",
				Type:        charmresource.TypeFile,
				Path:        "filename.tgz",
				Description: "One line that is useful when operators need to push it.",
			},
			Origin:      charmresource.OriginStore,
			Revision:    1,
			Fingerprint: fp,
			Size:        1,
		}},
		LastPolled: time.Now(),
	}
	s.state.EXPECT().SetRepositoryResources(gomock.Any(), args).Return(nil)

	err = s.service.SetRepositoryResources(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.resourceStoreGetter = NewMockResourceStoreGetter(ctrl)
	s.resourceStore = NewMockResourceStore(ctrl)

	s.service = NewService(s.state, s.resourceStoreGetter, loggertesting.WrapCheckLog(c))

	return ctrl
}
