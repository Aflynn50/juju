// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package storage -destination domain_mock.go github.com/juju/juju/apiserver/facades/client/storage StorageService

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}
