// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination migrations_mock_test.go github.com/juju/juju/domain/status/modelmigration ImportService,ExportService

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}
