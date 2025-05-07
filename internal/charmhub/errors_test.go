// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/charmhub/transport"
)

type ErrorsSuite struct {
	baseSuite
}

var _ = tc.Suite(&ErrorsSuite{})

func (s *ErrorsSuite) TestHandleBasicAPIErrors(c *tc.C) {
	var list transport.APIErrors
	err := handleBasicAPIErrors(list, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ErrorsSuite) TestHandleBasicAPIErrorsNotFound(c *tc.C) {
	list := transport.APIErrors{{Code: transport.ErrorCodeNotFound, Message: "foo"}}
	err := handleBasicAPIErrors(list, s.logger)
	c.Assert(err, tc.ErrorMatches, `charm or bundle not found`)
}

func (s *ErrorsSuite) TestHandleBasicAPIErrorsOther(c *tc.C) {
	list := transport.APIErrors{{Code: "other", Message: "foo"}}
	err := handleBasicAPIErrors(list, s.logger)
	c.Assert(err, tc.ErrorMatches, `foo`)
}
