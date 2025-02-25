// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errorutils_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package errorutils_test -destination environs_mock_test.go github.com/juju/juju/environs CredentialInvalidator

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
