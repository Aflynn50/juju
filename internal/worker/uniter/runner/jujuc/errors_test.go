// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type ErrorsSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&ErrorsSuite{})

func (t *ErrorsSuite) TestNotAvailableErr(c *tc.C) {
	err := jujuc.NotAvailable("the thing")
	c.Assert(err, tc.ErrorMatches, "the thing is not available")
	c.Assert(jujuc.IsNotAvailable(err), jc.IsTrue)
}
