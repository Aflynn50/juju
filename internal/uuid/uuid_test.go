// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package uuid

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type uuidSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&uuidSuite{})

func (*uuidSuite) TestUUID(c *gc.C) {
	uuid, err := NewUUID()
	c.Assert(err, gc.IsNil)
	uuidCopy := uuid.Copy()
	uuidRaw := uuid.Raw()
	uuidStr := uuid.String()
	c.Assert(uuidRaw, gc.HasLen, 16)
	c.Assert(uuidStr, jc.Satisfies, IsValidUUIDString)
	uuid[0] = 0x00
	uuidCopy[0] = 0xFF
	c.Assert(uuid, gc.Not(gc.DeepEquals), uuidCopy)
	uuidRaw[0] = 0xFF
	c.Assert(uuid, gc.Not(gc.DeepEquals), uuidRaw)
	nextUUID, err := NewUUID()
	c.Assert(err, gc.IsNil)
	c.Assert(uuid, gc.Not(gc.DeepEquals), nextUUID)
}

func (*uuidSuite) TestIsValidUUIDFailsWhenNotValid(c *gc.C) {
	tests := []struct {
		input    string
		expected bool
	}{
		{
			input:    UUID{}.String(),
			expected: true,
		},
		{
			input:    "",
			expected: false,
		},
		{
			input:    "blah",
			expected: false,
		},
		{
			input:    "blah-9f484882-2f18-4fd2-967d-db9663db7bea",
			expected: false,
		},
		{
			input:    "9f484882-2f18-4fd2-967d-db9663db7bea-blah",
			expected: false,
		},
		{
			input:    "9f484882-2f18-4fd2-967d-db9663db7bea",
			expected: true,
		},
	}
	for i, t := range tests {
		c.Logf("Running test %d", i)
		c.Check(IsValidUUIDString(t.input), gc.Equals, t.expected)
	}
}

func (*uuidSuite) TestUUIDFromString(c *gc.C) {
	_, err := UUIDFromString("blah")
	c.Assert(err, gc.ErrorMatches, `invalid UUID: "blah"`)
	validUUID := "9f484882-2f18-4fd2-967d-db9663db7bea"
	uuid, err := UUIDFromString(validUUID)
	c.Assert(err, gc.IsNil)
	c.Assert(uuid.String(), gc.Equals, validUUID)
}
