// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package qotd_test

import (
	"github.com/juju/cmd/v4/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/qotd"
	"github.com/juju/juju/internal/testing"
)

type SetQOTDAuthorSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&SetQOTDAuthorSuite{})

func (s *SetQOTDAuthorSuite) TestSetQOTDAuthor(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, qotd.NewSetQOTDAuthorCommand(), "Nelson Mandela")
	c.Assert(err, jc.ErrorIsNil)
	stdout := cmdtesting.Stdout(context)
	c.Assert(stdout, gc.Equals, `Quote author set to "Nelson Mandela"\n`)
}

func (s *SetQOTDAuthorSuite) TestSetQOTDAuthorNoArguments(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, qotd.NewSetQOTDAuthorCommand())
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, "No quote author specified")
}

func (s *SetQOTDAuthorSuite) TestSetQOTDAuthorTooManyArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, qotd.NewSetQOTDAuthorCommand(), "author", "arg-two")
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `unrecognized args: ["arg-two"]`)
}
