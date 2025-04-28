// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	httptest "net/http/httptest"
	"strings"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/httpcontext"
	apitesting "github.com/juju/juju/apiserver/testing"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	coreerrors "github.com/juju/juju/core/errors"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/semversion"
	agentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

// TODO (tlm) tests for tools handlers have been commented as the product moves
// to DQlite. As of writing these tests need to be added back after the switch.
// -

type toolsSuite struct {
	agentBinaryStore *MockAgentBinaryStore
	blockChecker     *MockBlockChecker
}

var _ = gc.Suite(&toolsSuite{})

func (s *toolsSuite) SetUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.agentBinaryStore = NewMockAgentBinaryStore(ctrl)
	s.blockChecker = NewMockBlockChecker(ctrl)
	return ctrl
}

func (s *toolsSuite) agentBinaryStoreGetter(_ *http.Request) (AgentBinaryStore, error) {
	return s.agentBinaryStore, nil
}

func (s *toolsSuite) assertJSONErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) {
	toolsResponse := s.assertResponse(c, resp, expCode)
	c.Check(toolsResponse.ToolsList, gc.IsNil)
	c.Check(toolsResponse.Error, gc.NotNil)
	c.Check(toolsResponse.Error.Message, gc.Matches, expError)
}

func (s *toolsSuite) assertResponse(c *gc.C, resp *http.Response, expStatus int) params.ToolsResult {
	body := apitesting.AssertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var toolsResponse params.ToolsResult
	err := json.Unmarshal(body, &toolsResponse)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Body: %s", body))
	return toolsResponse
}

func (s *toolsSuite) assertUploadResponse(c *gc.C, resp *http.Response, agentTools *tools.Tools) {
	toolsResponse := s.assertResponse(c, resp, http.StatusOK)
	c.Check(toolsResponse.Error, gc.IsNil)
	c.Check(toolsResponse.ToolsList, jc.DeepEquals, tools.List{agentTools})
}

func (s *toolsSuite) blockCheckGetter(_ context.Context) (BlockChecker, error) {
	return s.blockChecker, nil
}

func (s *toolsSuite) TestAddBackTests(c *gc.C) {
	c.Skip(`
TODO (tlm): Add back in tests for tools handlers. The following tests need to
added back in:

# Overall
- Test only supported method are allowed for each handler.

# ToolsDownloader
- Test download happy path.
- Test download streams from simple streams and also saves to agent binary store.
`)
}

// TestUploadInvalidAgentBinaryVersion tests that when uploading an agent binary
// for a version that doesn't parse as a version we get a bad request status
// back.
func (s *toolsSuite) TestUploadInvalidAgentBinaryVersion(c *gc.C) {
	defer s.SetUpMocks(c).Finish()

	handler := newToolsUploadHandler(s.blockCheckGetter, s.agentBinaryStoreGetter)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tools?binaryVersion=foobarinvalid", nil)

	handler.ServeHTTP(res, req)
	s.assertJSONErrorResponse(c, res.Result(), http.StatusBadRequest, `invalid agent binary version \"foobarinvalid\"`)
}

// TestUploadMissingAgentBinaryVersion checks that is an agent binary is
// uploaded but the version is missing this results in a bad request status.
func (s *toolsSuite) TestUploadMissingAgentBinaryVersion(c *gc.C) {
	defer s.SetUpMocks(c).Finish()

	handler := newToolsUploadHandler(s.blockCheckGetter, s.agentBinaryStoreGetter)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tools", nil)

	handler.ServeHTTP(res, req)
	s.assertJSONErrorResponse(c, res.Result(), http.StatusBadRequest, `expected binaryVersion argument`)
}

// TestUploadBadContentType tests that if an upload is attempted with a bad
// content type we back a bad request status.
func (s *toolsSuite) TestUploadBadContentType(c *gc.C) {
	defer s.SetUpMocks(c).Finish()

	handler := newToolsUploadHandler(s.blockCheckGetter, s.agentBinaryStoreGetter)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tools?binaryVersion=4.0.0-ubuntu-amd64", nil)
	req.Header.Add("Content-Type", "fud")

	handler.ServeHTTP(res, req)
	s.assertJSONErrorResponse(c, res.Result(), http.StatusBadRequest, `expected Content-Type: application/x-tar-gz, got: fud`)
}

// TestUploadZeroBytes asserts that uploading nothing to the handler results in
// a bad request state.
func (s *toolsSuite) TestUploadZeroBytes(c *gc.C) {
	defer s.SetUpMocks(c).Finish()

	body := strings.NewReader("")
	handler := newToolsUploadHandler(s.blockCheckGetter, s.agentBinaryStoreGetter)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tools?binaryVersion=4.0.0-ubuntu-amd64", body)
	req.Header.Add("Content-Type", "application/x-tar-gz")

	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	handler.ServeHTTP(res, req)
	s.assertJSONErrorResponse(c, res.Result(), http.StatusBadRequest, `no agent binaries uploaded`)
}

// TestUploadAgentBinaryServiceInvalidArch tests that if the agent binary store
// does not support the architecture being uploaded for we get back a bad
// request status.
func (s *toolsSuite) TestUploadAgentBinaryServiceInvalidArch(c *gc.C) {
	defer s.SetUpMocks(c).Finish()

	body := strings.NewReader("123456789")
	handler := newToolsUploadHandler(s.blockCheckGetter, s.agentBinaryStoreGetter)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tools?binaryVersion=4.0.0-ubuntu-amd64", body)
	req.Header.Add("Content-Type", "application/x-tar-gz")

	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.agentBinaryStore.EXPECT().AddAgentBinaryWithSHA256(
		gomock.Any(),
		gomock.Any(),
		coreagentbinary.Version{
			Number: semversion.MustParse("4.0.0"),
			Arch:   corearch.AMD64,
		},
		int64(9),
		"15e2b0d3c33891ebb0f1ef609ec419420c20e320ce94c65fbc8c3312448eb225",
	).Return(coreerrors.NotSupported)

	handler.ServeHTTP(res, req)
	s.assertJSONErrorResponse(c, res.Result(), http.StatusBadRequest, `unsupported architecture "amd64"`)
}

// TestUploadAgentBinaryServiceAlreadyExists tests that if the agent binary
// store already has an agent binary version for the uploaded version we get
// back a bad request status.
func (s *toolsSuite) TestUploadAgentBinaryServiceAlreadyExists(c *gc.C) {
	defer s.SetUpMocks(c).Finish()

	body := strings.NewReader("123456789")
	handler := newToolsUploadHandler(s.blockCheckGetter, s.agentBinaryStoreGetter)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tools?binaryVersion=4.0.0-ubuntu-amd64", body)
	req.Header.Add("Content-Type", "application/x-tar-gz")

	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.agentBinaryStore.EXPECT().AddAgentBinaryWithSHA256(
		gomock.Any(),
		gomock.Any(),
		coreagentbinary.Version{
			Number: semversion.MustParse("4.0.0"),
			Arch:   corearch.AMD64,
		},
		int64(9),
		"15e2b0d3c33891ebb0f1ef609ec419420c20e320ce94c65fbc8c3312448eb225",
	).Return(agentbinaryerrors.AlreadyExists)

	handler.ServeHTTP(res, req)
	s.assertJSONErrorResponse(
		c,
		res.Result(),
		http.StatusBadRequest,
		`agent binary already exists for version "4.0.0" and arch "amd64"`,
	)
}

// TestUploadAgentBinary tests the happy path of uploading agent binaries to the
// handler.
func (s *toolsSuite) TestUploadAgentBinary(c *gc.C) {
	defer s.SetUpMocks(c).Finish()

	body := strings.NewReader("123456789")
	handler := newToolsUploadHandler(s.blockCheckGetter, s.agentBinaryStoreGetter)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "https://[2001:0DB8::1]/tools?binaryVersion=4.0.0-ubuntu-amd64", body)
	req.Header.Add("Content-Type", "application/x-tar-gz")

	modelUUID := modeltesting.GenModelUUID(c)
	ctx := httpcontext.SetContextModelUUID(req.Context(), modelUUID)
	req = req.WithContext(ctx)

	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.agentBinaryStore.EXPECT().AddAgentBinaryWithSHA256(
		gomock.Any(),
		gomock.Any(),
		coreagentbinary.Version{
			Number: semversion.MustParse("4.0.0"),
			Arch:   corearch.AMD64,
		},
		int64(9),
		"15e2b0d3c33891ebb0f1ef609ec419420c20e320ce94c65fbc8c3312448eb225",
	).Return(nil)

	handler.ServeHTTP(res, req)
	c.Check(res.Result().StatusCode, gc.Equals, http.StatusOK)

	s.assertUploadResponse(c, res.Result(), &tools.Tools{
		Version: semversion.MustParseBinary("4.0.0-ubuntu-amd64"),
		URL:     fmt.Sprintf("https://[2001:0DB8::1]/model/%s/tools/4.0.0-ubuntu-amd64", modelUUID),
		SHA256:  "15e2b0d3c33891ebb0f1ef609ec419420c20e320ce94c65fbc8c3312448eb225",
		Size:    9,
	})
}
