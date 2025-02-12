// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	api "github.com/juju/juju/api/client/resources"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreresource "github.com/juju/juju/core/resource"
	coreresourcetesting "github.com/juju/juju/core/resource/testing"
	domainresource "github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	charmresource "github.com/juju/juju/internal/charm/resource"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/rpc/params"
)

type ResourcesHandlerSuite struct {
	testing.IsolationSuite

	authErr  error
	username string
	recorder *httptest.ResponseRecorder
	handler  *ResourceHandler

	resourceUUID    coreresource.UUID
	resourceName    string
	resourceContent string
	resource        coreresource.Resource
	resourceReader  io.ReadCloser
	applicationName string

	resourceService       *MockResourceService
	resourceServiceGetter *MockResourceServiceGetter
}

var _ = gc.Suite(&ResourcesHandlerSuite{})

func (s *ResourcesHandlerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.authErr = nil
	s.username = "youknowwho"

	s.resourceUUID = coreresourcetesting.GenResourceUUID(c)

	s.resourceName = "foo"
	s.applicationName = "app"

	s.resourceContent = "stuff"
	s.resourceReader = io.NopCloser(strings.NewReader(s.resourceContent))

	// Generating the fingerprint exhausts the reader so a new one is used.
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(s.resourceContent))
	c.Assert(err, jc.ErrorIsNil)
	s.resource = coreresource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: s.resourceName,
				Type: charmresource.TypeFile,
				Path: "foo.tgz",
			},
			Fingerprint: fp,
			Size:        int64(len(s.resourceContent)),
		},
		UUID:        s.resourceUUID,
		RetrievedBy: s.username,
	}

	s.recorder = httptest.NewRecorder()
}

func (s *ResourcesHandlerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.resourceService = NewMockResourceService(ctrl)
	s.resourceServiceGetter = NewMockResourceServiceGetter(ctrl)

	s.handler = NewResourceHandler(
		s.authFunc,
		func(context.Context) error { return nil },
		s.resourceServiceGetter,
		loggertesting.WrapCheckLog(c),
	)

	return ctrl
}

func (s *ResourcesHandlerSuite) serveHTTP(req *http.Request) {
	s.resourceServiceGetter.EXPECT().Resource(gomock.Any()).Return(s.resourceService, nil)
	s.handler.ServeHTTP(s.recorder, req)
}

func (s *ResourcesHandlerSuite) authFunc(_ *http.Request, _ ...string) (
	names.Tag, error,
) {
	if s.authErr != nil {
		return nil, errors.Trace(s.authErr)
	}

	tag := names.NewUserTag(s.username)
	return tag, nil
}

func (s *ResourcesHandlerSuite) TestExpectedAuthTags(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: Create auth function that checks the expected tags.
	expectedTags := set.NewStrings(names.UserTagKind, names.MachineTagKind, names.ControllerAgentTagKind, names.ApplicationTagKind)
	authFunc := func(req *http.Request, tagKinds ...string) (names.Tag, error) {
		c.Assert(tagKinds, jc.SameContents, expectedTags.Values())
		tag := names.NewUserTag(s.username)
		return tag, nil
	}

	s.handler = NewResourceHandler(
		authFunc,
		func(context.Context) error { return nil },
		s.resourceServiceGetter,
		loggertesting.WrapCheckLog(c),
	)

	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		s.applicationName,
		s.resourceName,
	).Return(s.resourceUUID, nil)

	s.resourceService.EXPECT().OpenResource(
		gomock.Any(),
		s.resourceUUID,
	).Return(s.resource, s.resourceReader, nil)

	req := s.newDownloadRequest(c)

	// Act:
	s.serveHTTP(req)

	// Assert:
	s.checkResp(c, http.StatusOK, "application/octet-stream", s.resourceContent)
}

func (s *ResourcesHandlerSuite) TestAuthFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	failure, expected := apiFailure("<failure>", "")
	s.authErr = failure

	req := s.newDownloadRequest(c)

	// Act:
	s.handler.ServeHTTP(s.recorder, req)

	// Assert:
	s.checkResp(c, http.StatusInternalServerError, "application/json", expected)
}

func (s *ResourcesHandlerSuite) TestUnsupportedMethod(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	req, err := http.NewRequest("POST", s.requestURL(), nil)
	c.Assert(err, jc.ErrorIsNil)

	// Act:
	s.serveHTTP(req)

	// Assert:
	_, expected := apiFailure(`unsupported method: "POST"`, params.CodeMethodNotAllowed)
	s.checkResp(c, http.StatusMethodNotAllowed, "application/json", expected)
}

func (s *ResourcesHandlerSuite) TestGetSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	req := s.newDownloadRequest(c)

	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		s.applicationName,
		s.resourceName,
	).Return(s.resourceUUID, nil)

	s.resourceService.EXPECT().OpenResource(
		gomock.Any(),
		s.resourceUUID,
	).Return(s.resource, s.resourceReader, nil)

	// Act:
	s.serveHTTP(req)

	// Assert:
	s.checkResp(c, http.StatusOK, "application/octet-stream", s.resourceContent)
}

func (s *ResourcesHandlerSuite) TestGetNotFoundError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	req := s.newDownloadRequest(c)

	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		s.applicationName,
		s.resourceName,
	).Return("", resourceerrors.ResourceNotFound)

	// Act:
	s.serveHTTP(req)

	// Assert:
	s.checkErrResp(c, http.StatusNotFound, "application/json")
}

func (s *ResourcesHandlerSuite) TestPutSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:

	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		s.applicationName,
		s.resourceName,
	).Return(s.resourceUUID, nil)

	s.resourceService.EXPECT().GetResource(gomock.Any(), s.resourceUUID).Return(
		s.resource, nil,
	)

	s.resourceService.EXPECT().StoreResourceAndIncrementCharmModifiedVersion(gomock.Any(), domainresource.StoreResourceArgs{
		ResourceUUID:    s.resourceUUID,
		Reader:          s.resourceReader,
		RetrievedBy:     s.username,
		RetrievedByType: coreresource.User,
		Size:            s.resource.Size,
		Fingerprint:     s.resource.Fingerprint,
		Origin:          charmresource.OriginUpload,
		Revision:        -1,
	})

	// Second call to GetResource gets resource details after upload.
	expectedResource := s.resource
	expectedResource.Origin = charmresource.OriginUpload
	expectedResource.Revision = -1
	s.resourceService.EXPECT().GetResource(gomock.Any(), s.resourceUUID).Return(
		expectedResource, nil,
	)

	req := s.newUploadRequest(c)

	// Act:
	s.serveHTTP(req)

	// Assert: Check that the uploaded resources details are returned:
	expected := mustMarshalJSON(&params.UploadResult{
		Resource: params.Resource{
			CharmResource: api.CharmResource2API(expectedResource.Resource),
			UUID:          expectedResource.UUID.String(),
			Username:      expectedResource.RetrievedBy,
			Timestamp:     expectedResource.Timestamp,
		},
	})
	s.checkResp(c, http.StatusOK, "application/json", string(expected))
}

func (s *ResourcesHandlerSuite) TestPutChangeBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: Construct change allowed func and put it in resource handler.
	expectedError := apiservererrors.OperationBlockedError("test block")
	changeAllowedFunc := func(context.Context) error {
		return expectedError
	}
	s.handler = NewResourceHandler(
		s.authFunc,
		changeAllowedFunc,
		s.resourceServiceGetter,
		loggertesting.WrapCheckLog(c),
	)

	req := s.newUploadRequest(c)

	// Act:
	s.serveHTTP(req)

	// Assert:
	expected := mustMarshalJSON(&params.ErrorResult{apiservererrors.ServerError(expectedError)})
	s.checkResp(c, http.StatusBadRequest, "application/json", string(expected))
}

func (s *ResourcesHandlerSuite) TestPutSuccessDockerResource(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	req := s.newUploadRequest(c)

	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		s.applicationName,
		s.resourceName,
	).Return(s.resourceUUID, nil)

	res := s.resource
	res.Type = charmresource.TypeContainerImage
	s.resourceService.EXPECT().GetResource(gomock.Any(), s.resourceUUID).Return(
		res, nil,
	)

	s.resourceService.EXPECT().StoreResourceAndIncrementCharmModifiedVersion(gomock.Any(), domainresource.StoreResourceArgs{
		ResourceUUID:    s.resourceUUID,
		Reader:          s.resourceReader,
		RetrievedBy:     s.username,
		RetrievedByType: coreresource.User,
		Size:            s.resource.Size,
		Fingerprint:     s.resource.Fingerprint,
		Origin:          charmresource.OriginUpload,
		Revision:        -1,
	})

	// Second call to GetResource gets resource details after upload.
	expectedResource := res
	expectedResource.Origin = charmresource.OriginUpload
	expectedResource.Revision = -1
	s.resourceService.EXPECT().GetResource(gomock.Any(), s.resourceUUID).Return(
		expectedResource, nil,
	)

	// Act:
	s.serveHTTP(req)

	// Assert:
	expected := mustMarshalJSON(&params.UploadResult{
		Resource: params.Resource{
			CharmResource: api.CharmResource2API(expectedResource.Resource),
			UUID:          expectedResource.UUID.String(),
			Username:      expectedResource.RetrievedBy,
			Timestamp:     expectedResource.Timestamp,
		},
	})
	s.checkResp(c, http.StatusOK, "application/json", string(expected))
}

func (s *ResourcesHandlerSuite) TestPutExtensionMismatch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	req := s.newUploadRequest(c)
	req.Header.Set("Content-Disposition", "form-data; filename=different.ext")

	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		s.applicationName,
		s.resourceName,
	).Return(s.resourceUUID, nil)

	s.resourceService.EXPECT().GetResource(gomock.Any(), s.resourceUUID).Return(
		s.resource, nil,
	)

	// Act:
	s.serveHTTP(req)

	// Assert:
	_, expected := apiFailure(`incorrect extension on resource upload "different.ext", expected ".tgz"`,
		"")
	s.checkResp(c, http.StatusInternalServerError, "application/json", expected)
}

// TestPutWithPending checks that clients can still upload resources marked as
// pending, though this concept is deprecated and no longer used by the
// controller.
func (s *ResourcesHandlerSuite) TestPutWithPending(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	req := s.newUploadRequest(c)
	req.URL.RawQuery += "&pendingid=some-unique-id"

	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		s.applicationName,
		s.resourceName,
	).Return(s.resourceUUID, nil)

	s.resourceService.EXPECT().GetResource(gomock.Any(), s.resourceUUID).Return(
		s.resource, nil,
	)

	s.resourceService.EXPECT().StoreResourceAndIncrementCharmModifiedVersion(gomock.Any(), domainresource.StoreResourceArgs{
		ResourceUUID:    s.resourceUUID,
		Reader:          s.resourceReader,
		RetrievedBy:     s.username,
		RetrievedByType: coreresource.User,
		Size:            s.resource.Size,
		Fingerprint:     s.resource.Fingerprint,
		Origin:          charmresource.OriginUpload,
		Revision:        -1,
	})

	// Second call to GetResource gets resource details after upload.
	expectedResource := s.resource
	expectedResource.Origin = charmresource.OriginUpload
	expectedResource.Revision = -1
	s.resourceService.EXPECT().GetResource(gomock.Any(), s.resourceUUID).Return(
		expectedResource, nil,
	)

	// Act:
	s.serveHTTP(req)

	// Assert: Check that the uploaded resources details are returned:
	expected := mustMarshalJSON(&params.UploadResult{
		Resource: params.Resource{
			CharmResource: api.CharmResource2API(expectedResource.Resource),
			UUID:          expectedResource.UUID.String(),
			Username:      expectedResource.RetrievedBy,
			Timestamp:     expectedResource.Timestamp,
		},
	})
	s.checkResp(c, http.StatusOK, "application/json", string(expected))
}

func (s *ResourcesHandlerSuite) TestPutNotFoundError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	req := s.newUploadRequest(c)

	s.resourceService.EXPECT().GetResourceUUIDByApplicationAndResourceName(
		gomock.Any(),
		s.applicationName,
		s.resourceName,
	).Return("", resourceerrors.ResourceNotFound)

	// Act:
	s.serveHTTP(req)

	// Assert:
	s.checkErrResp(c, http.StatusNotFound, "application/json")
}

func (s *ResourcesHandlerSuite) checkResp(c *gc.C, status int, ctype, body string) {
	c.Assert(s.recorder.Code, gc.Equals, status)
	hdr := s.recorder.Header()
	c.Check(hdr.Get("Content-Type"), gc.Equals, ctype)
	c.Check(hdr.Get("Content-Length"), gc.Equals, strconv.Itoa(len(body)))

	actualBody, err := io.ReadAll(s.recorder.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(actualBody), gc.Equals, body)
}

func (s *ResourcesHandlerSuite) checkErrResp(c *gc.C, status int, ctype string) {
	c.Assert(s.recorder.Code, gc.Equals, status)
	hdr := s.recorder.Header()
	c.Check(hdr.Get("Content-Type"), gc.Equals, ctype)
}

func (s *ResourcesHandlerSuite) newDownloadRequest(c *gc.C) *http.Request {
	req, err := http.NewRequest("GET", s.requestURL(), nil)
	c.Assert(err, jc.ErrorIsNil)

	return req
}

func (s *ResourcesHandlerSuite) newUploadRequest(c *gc.C) *http.Request {
	req, err := http.NewRequest("PUT", s.requestURL(), s.resourceReader)
	c.Assert(err, jc.ErrorIsNil)

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Length", fmt.Sprint(s.resource.Size))
	req.Header.Set("Content-SHA384", s.resource.Fingerprint.String())
	req.Header.Set("Content-Disposition", "form-data; filename="+s.resource.Path)

	return req
}

func (s *ResourcesHandlerSuite) requestURL() string {
	urlStr := "https://api:17017/applications/%s/resources/%s?:application=%s&:resource=%s"
	urlStr = fmt.Sprintf(urlStr, s.applicationName, s.resourceName, s.applicationName, s.resourceName)

	return urlStr
}

func apiFailure(msg, code string) (error, string) {
	failure := errors.New(msg)
	data := mustMarshalJSON(params.ErrorResult{
		Error: &params.Error{
			Message: msg,
			Code:    code,
		},
	})
	return failure, string(data)
}

func mustMarshalJSON(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
