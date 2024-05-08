// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"testing"

	"github.com/juju/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/authentication_mock.go github.com/juju/juju/apiserver/authentication ExpirableStorageBakery
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/bakerystorage_mock.go github.com/juju/juju/state/bakerystorage BakeryConfig,ExpirableStorage
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/bakery_mock.go github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery FirstPartyCaveatChecker
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/http_mock.go net/http RoundTripper
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/crossmodel_mock.go github.com/juju/juju/apiserver/common/crossmodel OfferBakeryInterface,Backend

func TestAll(t *testing.T) {
	gc.TestingT(t)
}

func (o *OfferBakery) SetBakery(bakery authentication.ExpirableStorageBakery) {
	o.bakery = bakery
}

func (o *AuthContext) SetClock(clk clock.Clock) {
	o.offerBakery.setClock(clk)
}
