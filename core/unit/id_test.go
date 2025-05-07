// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/uuid"
)

type unitSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&unitSuite{})

func (*unitSuite) TestUUIDValidate(c *tc.C) {
	tests := []struct {
		uuid string
		err  error
	}{
		{
			uuid: "",
			err:  coreerrors.NotValid,
		},
		{
			uuid: "invalid",
			err:  coreerrors.NotValid,
		},
		{
			uuid: uuid.MustNewUUID().String(),
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.uuid)
		err := UUID(test.uuid).Validate()

		if test.err == nil {
			c.Check(err, tc.IsNil)
			continue
		}

		c.Check(err, jc.ErrorIs, test.err)
	}
}

func (*unitSuite) TestParseUUID(c *tc.C) {
	tests := []struct {
		uuid string
		err  error
	}{
		{
			uuid: "",
			err:  coreerrors.NotValid,
		},
		{
			uuid: "invalid",
			err:  coreerrors.NotValid,
		},
		{
			uuid: uuid.MustNewUUID().String(),
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.uuid)
		id, err := ParseID(test.uuid)

		if test.err == nil {
			if c.Check(err, tc.IsNil) {
				c.Check(id.String(), tc.Equals, test.uuid)
			}
			continue
		}

		c.Check(err, jc.ErrorIs, test.err)
	}
}
