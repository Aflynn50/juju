// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
    gc "gopkg.in/check.v1"

    schematesting "github.com/juju/juju/domain/schema/testing"
)

type resourceSuite struct {
    schematesting.ModelSuite
}

var _ = gc.Suite(&resourceSuite{})

func (s *resourceSuite) TestSetResource(* gc.C) {

}