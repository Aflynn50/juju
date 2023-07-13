// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"

	apisecrets "github.com/juju/juju/api/client/secrets"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/secrets"
)

type addSecretCommand struct {
	modelcmd.ModelCommandBase

	SecretUpsertContentCommand
	secretsAPIFunc func() (AddSecretsAPI, error)
}

// AddSecretsAPI is the secrets client API.
type AddSecretsAPI interface {
	CreateSecret(uri *secrets.URI, label, description string, data map[string]string) (string, error)
	Close() error
}

// NewAddSecretCommand returns a command to add a secret.
func NewAddSecretCommand() cmd.Command {
	c := &addSecretCommand{}
	c.secretsAPIFunc = c.secretsAPI
	return modelcmd.Wrap(c)
}

func (c *addSecretCommand) secretsAPI() (AddSecretsAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apisecrets.NewClient(root), nil
}

// Info implements cmd.Command.
func (c *addSecretCommand) Info() *cmd.Info {
	doc := `
Add a secret with a list of key values.

If a key has the '#base64' suffix, the value is already in base64 format and no
encoding will be performed, otherwise the value will be base64 encoded
prior to being stored.

If a key has the '#file' suffix, the value is read from the corresponding file.

A secret is owned by the model, meaning only the model admin
can manage it, ie grant/revoke access, update, remove etc.

Examples:
    add-secret token=34ae35facd4
    add-secret key#base64=AA==
    add-secret key#file=/path/to/file another-key=s3cret
    add-secret --label db-password \
        --info "my database password" \
        data#base64=s3cret== 
    add-secret --label db-password \
        --info "my database password" \
        --file=/path/to/file
`
	return jujucmd.Info(&cmd.Info{
		Name:    "add-secret",
		Args:    "[key[#base64|#file]=value...]",
		Purpose: "add a new secret",
		Doc:     doc,
	})
}

// Init implements cmd.Command.
func (c *addSecretCommand) Init(args []string) error {
	if err := c.SecretUpsertContentCommand.Init(args); err != nil {
		return err
	}
	if len(c.Data) == 0 {
		return errors.New("missing secret value or filename")
	}
	return nil
}

// Run implements cmd.Command.
func (c *addSecretCommand) Run(ctx *cmd.Context) error {
	secretsAPI, err := c.secretsAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer secretsAPI.Close()

	uri, err := secretsAPI.CreateSecret(nil, c.Label, c.Description, c.Data)
	if err != nil {
		return err
	}
	fmt.Fprintln(ctx.Stdout, uri)
	return nil
}
