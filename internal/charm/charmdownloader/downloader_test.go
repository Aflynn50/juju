// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"context"
	"net/url"
	"os"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type downloaderSuite struct {
	testing.IsolationSuite

	downloadClient *MockDownloadClient
}

var _ = gc.Suite(&downloaderSuite{})

func (s *downloaderSuite) TestDownload(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cURL, err := url.Parse("https://example.com/foo")
	c.Assert(err, jc.ErrorIsNil)

	s.downloadClient.EXPECT().Download(gomock.Any(), cURL, gomock.Any(), gomock.Any()).Return(&charmhub.Digest{
		SHA256: "sha256",
		SHA384: "sha384",
		Size:   123,
	}, nil)

	downloader := NewCharmDownloader(s.downloadClient, loggertesting.WrapCheckLog(c))
	result, err := downloader.Download(context.Background(), cURL, "sha256")
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the path is not empty and that the temp file still exists.
	c.Assert(result.Path, gc.Not(gc.Equals), "")

	_, err = os.Stat(result.Path)
	c.Check(err, jc.ErrorIsNil)
	c.Check(result.Size, gc.Equals, int64(123))
}

func (s *downloaderSuite) TestDownloadFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cURL, err := url.Parse("https://example.com/foo")
	c.Assert(err, jc.ErrorIsNil)

	var tmpPath string

	// Spy on the download call to get the path of the temp file.
	spy := func(_ context.Context, _ *url.URL, path string, _ ...charmhub.DownloadOption) (*charmhub.Digest, error) {
		tmpPath = path
		return &charmhub.Digest{
			SHA256: "sha256-ignored",
			SHA384: "sha384-ignored",
			Size:   123,
		}, errors.Errorf("boom")
	}
	s.downloadClient.EXPECT().Download(gomock.Any(), cURL, gomock.Any(), gomock.Any()).DoAndReturn(spy)

	downloader := NewCharmDownloader(s.downloadClient, loggertesting.WrapCheckLog(c))
	_, err = downloader.Download(context.Background(), cURL, "hash")
	c.Assert(err, gc.ErrorMatches, `.*boom`)

	_, err = os.Stat(tmpPath)
	c.Check(os.IsNotExist(err), jc.IsTrue)
}

func (s *downloaderSuite) TestDownloadInvalidDigestHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cURL, err := url.Parse("https://example.com/foo")
	c.Assert(err, jc.ErrorIsNil)

	var tmpPath string

	// Spy on the download call to get the path of the temp file.
	spy := func(_ context.Context, _ *url.URL, path string, _ ...charmhub.DownloadOption) (*charmhub.Digest, error) {
		tmpPath = path
		return &charmhub.Digest{
			SHA256: "sha256-ignored",
			SHA384: "sha384-ignored",
			Size:   123,
		}, nil
	}
	s.downloadClient.EXPECT().Download(gomock.Any(), cURL, gomock.Any(), gomock.Any()).DoAndReturn(spy)

	downloader := NewCharmDownloader(s.downloadClient, loggertesting.WrapCheckLog(c))
	_, err = downloader.Download(context.Background(), cURL, "hash")
	c.Assert(err, jc.ErrorIs, ErrInvalidDigestHash)

	_, err = os.Stat(tmpPath)
	c.Check(os.IsNotExist(err), jc.IsTrue)
}

func (s *downloaderSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.downloadClient = NewMockDownloadClient(ctrl)

	return ctrl
}
