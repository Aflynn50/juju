// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
)

type typesSuite struct{}

var _ = tc.Suite(&typesSuite{})

func (s *typesSuite) TestValidate(c *tc.C) {
	// Arrange
	args := GetRelationUUIDForRemovalArgs{
		Endpoints: []string{"foo:require", "bar:provide"},
	}

	// Act
	err := args.Validate()

	// Assert
	c.Assert(err, jc.ErrorIsNil)
}

func (s *typesSuite) TestValidateFailEndpointsOne(c *tc.C) {
	// Arrange
	args := GetRelationUUIDForRemovalArgs{
		Endpoints: []string{"foo:require"},
	}

	// Act
	err := args.Validate()

	// Assert
	c.Assert(err, tc.NotNil)
}

func (s *typesSuite) TestValidateFailEndpointsMoreThanTwo(c *tc.C) {
	// Arrange
	args := GetRelationUUIDForRemovalArgs{
		Endpoints: []string{"foo:require", "bar:provide", "dead:beef"},
	}

	// Act
	err := args.Validate()

	// Assert
	c.Assert(err, tc.NotNil)
}
