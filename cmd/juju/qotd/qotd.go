// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package qotd

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/errors"
)

// setQOTDAuthorCommand is the base of the set-qotd-author command.
type setQOTDAuthorCommand struct {
	// ControllerCommandBase is used because this is a command that interacts
	// with the controller.
	modelcmd.ControllerCommandBase

	// author is the author the user has specified.
	author string
	// out is responsible for outputting the response to the user in the correct
	// format.
	out cmd.Output
}

// NewSetQOTDAuthorCommand returns a command to set the quote of the day author.
func NewSetQOTDAuthorCommand() cmd.Command {
	cmd := &setQOTDAuthorCommand{}
	return modelcmd.WrapBase(cmd)
}

// Info defines the name of the command and the command documentation. It
// is part of the Command interface in the juju/cmd package.
func (c *setQOTDAuthorCommand) Info() *cmd.Info {
	// jujucmd.Info adds flags common to all juju cli commands>
	return jujucmd.Info(&cmd.Info{
		Name:     "set-qotd-author",
		Purpose:  "Set the quote of the day author:",
		Args:     "<quote-of-the-day-author>",
		Doc:      "Sets the author of the quote of the day",
		Examples: "juju set-qotd-author \"Nelson Mandela\"\n",
		SeeAlso: []string{
			"is",
			"unleash",
		},
	})
}

// SetFlags adds flags to the command. It is part of the Command interface in
// the juju/cmd package.
func (c *setQOTDAuthorCommand) SetFlags(f *gnuflag.FlagSet) {
	// Collect the default output formatters.
	formatters := make(map[string]cmd.Formatter, len(cmd.DefaultFormatters))
	for k, v := range cmd.DefaultFormatters {
		formatters[k] = v.Formatter
	}
	// Add the output related command flags and set the default formatter to
	// "smart". This will automatically format strings for output.
	c.out.AddFlags(f, "smart", formatters)
}

// Init initializes the command before running it. It collects the use supplied
// arguments and throws an error if they are not as expected. It is part of the
// Command interface in the juju/cmd package.
func (c *setQOTDAuthorCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.Errorf("No quote author specified")
	case 1:
		c.author = args[0]
		return nil
	default:
		//  CheckEmpty checks that there are no extra arguments.
		return cmd.CheckEmpty(args[1:])
	}
}

// Run executes the action of the command. It is part of the Command interface
// in the juju/cmd package.
func (c *setQOTDAuthorCommand) Run(ctx *cmd.Context) error {
	// For now, just tell the user what they wrote.
	return c.out.Write(ctx, "Quote author set to \""+c.author+"\"")
}
