// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"net/url"

	"github.com/juju/names/v5"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api/base"
	coretesting "github.com/juju/juju/internal/testing"
)

var _ base.APICallCloser = (*httpAPICallCloser)(nil)

// httpAPICallCloser implements base.APICallCloser.
type httpAPICallCloser struct {
	base.APICallCloser
	url *url.URL
}

// ModelTag implements base.APICallCloser.
func (*httpAPICallCloser) ModelTag() (names.ModelTag, bool) {
	return coretesting.ModelTag, true
}

// BestFacadeVersion implements base.APICallCloser.
func (*httpAPICallCloser) BestFacadeVersion(facade string) int {
	return 42
}

// HTTPClient implements base.APICallCloser. The returned HTTP client can be
// used to send requests to the testing server set up in httpFixture.run().
func (ac *httpAPICallCloser) HTTPClient() (*httprequest.Client, error) {
	return &httprequest.Client{
		BaseURL: ac.url.String(),
	}, nil
}
