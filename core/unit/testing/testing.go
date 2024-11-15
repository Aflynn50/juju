// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreunit "github.com/juju/juju/core/unit"
)

// GenUnitUUID can be used in testing for generating a unit uuid that is checked
// for subsequent errors using the test suits go check instance.
func GenUnitUUID(c *gc.C) coreunit.UUID {
	uuid, err := coreunit.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}