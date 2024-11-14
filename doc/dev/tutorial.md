# Juju Developer Tutorial

To get acquainted with the insides of Juju, you're going to add a brand-new
feature. A quote-of-the-day service. With your addition, you will be able to set
an author for today's quote and Juju will fetch one of their venerable uttering.
Charms will then be able to fetch and use these words of high wisdom to increase
their effectiveness as they see fit.

**What you'll need:**
- A workstation with `git` and an editor suitable for development in golang.

**What you'll do:**
- Add a new command to the Juju CLI to set the author of today's quote.
- Add an API endpoint and facade method to the controller to save the new quote author.
- Add a domain to the controller for storing the quote authors.
- Add a worker to the controller which watches the state for changes, fetches quotes from new authors in state from
  [Zenquotes API](https://docs.zenquotes.io/zenquotes-documentation/#call-author), and saves these quotes in state.
- Add a hook tool for charms to get the quote of the day from the controller.
- Write a basic charm to fetch the quote of the day, and display it in its status.

## Set up and try to build Juju
Clone the main branch of the Juju repository onto your machine:
```bash
git clone -b main git@github.com:juju/juju.git 
```
Try compiling and installing this version of Juju to check you have all the
tools you need:
```a
cd juju
make build
```
If the `make` fails because you are missing any tools, install them.

# Add a new command to Juju
You are going to add a new command to Juju to set the author of the quote of the
day:
```
juju set-qotd-author
```
For now, this command will only print out the author you have set. For example:
```
$ juju set-qotd-author "Nelson Mandela"
You have set todays author to: Nelson Mandela
```

Open the juju repo in your editor and navigate to `cmd/juju`. This contains the
code for the Juju CLI commands.

Make a new folder called `qotd` and a file in it called `qotd.go`. Add the base
command definition and help information for the `set-qotd-author`:
```go
package qotd

import (
  "github.com/juju/cmd/v4"

  jujucmd "github.com/juju/juju/cmd"
  "github.com/juju/juju/cmd/modelcmd"
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
// implements command.Info from the juju/cmd package.
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
```

Here you have made use of the `juju/cmd` package here to define the command. Our
command needs to implement the `Command` interface defined in this package. We
can then register it with the Juju CLI.

> See more: [github.com/juju/cmd](https://github.com/juju/cmd), [`Command` interface](https://github.com/juju/cmd?tab=readme-ov-file#type-command)

Now, add the `SetFlags`, `Init` and `Run` methods to the command.
```go
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
```

There are two other methods that are part of the command interface:
`IsSuperCommand` and `AllowIntersperedFlags`. We do not need to provide these.
The `setQOTDAuthorComand` struct embeds the `modelcmd.ControllerCommandBase`
which in several layers down embeds `cmd.Command` from the command package. This
provides a default implementation.

## Testing the command

Juju uses the go check library for testing. To test the command you will add a
go check unit test suite.

In the `qotd` directory, create a `package_test.go`, and hook up go check to
work with the `go test` command:
```go
package qotd_test

import (
  stdtesting "testing"

  gc "gopkg.in/check.v1"
)

func TestPackage(t *stdtesting.T) {
  gc.TestingT(t)
}
```
Next, in the `qotd` directory, create a `qotd_test.go` file and add a go check
suite.

```go
package qotd_test

import (
  gc "gopkg.in/check.v1"

  "github.com/juju/juju/internal/testing"
)

type SetQOTDAuthorSuite struct {
  testing.BaseSuite
}

var _ = gc.Suite(&SetQOTDAuthorSuite{})
```
Embedding the `testing.BaseSuite` provides isolation for the test from the
system it is running on.

Now, add some tests of the command. We will test the command works as expected
and returns the right errors.

```go
func (s *SetQOTDAuthorSuite) TestSetQOTDAuthor(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, qotd.NewSetQOTDAuthorCommand(), "Nelson Mandela")
	c.Assert(err, jc.ErrorIsNil)
	stdout := cmdtesting.Stdout(context)
	c.Assert(stdout, gc.Equals, `Quote author set to "Nelson Mandela"\n`)
}

func (s *SetQOTDAuthorSuite) TestSetQOTDAuthorTooManyArgs(c *gc.C) {
_, err := cmdtesting.RunCommand(c, qotd.NewSetQOTDAuthorCommand(), "author", "arg-two")
c.Assert(err, gc.NotNil)
c.Assert(err.Error(), gc.Equals, `unrecognized args: ["arg-two"]`)
}
```

**Exercise:** Add a third test to check that the command throws an error when
no parameters are passed.

> See more: [Go Check](http://labix.org/gocheck)

## Wire up the command
The command is implemented, but it needs to be registered to appear when running
the Juju CLI.

Go to `cmd/juju/command/main.go` and find the `registerCommands` function.
Import the `qotd` package and register the new command at the bottom.
```go
    ...
	
    // Quote of the day command.
	r.Register(qotd.NewSetQOTDAuthorCommand())
}
```

## Compile and install Juju to try the new command.
From the root of the repo, run:
```
make install
```
Once it has finished, do `which juju` to check that it has correctly installed.
It should be installed in your `GOPATH` (defaults to `~/go`) at
`GOPATH/bin/juju`.

To admire your handiwork, run: 
```
juju set-qotd-author "Nelson Mandela"
```
