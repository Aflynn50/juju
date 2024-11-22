// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package qotd

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// Client allows access to the QOTD API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the QOTD API.
func NewClient(st base.APICallCloser, options ...base.Option) *Client {
	frontend, backend := base.NewClientFacade(st, "QOTD", options...)
	return &Client{ClientFacade: frontend, facade: backend}
}

// SetQOTDAuthor set the qotd of the day author using the QOTD API.
func (c *Client) SetQOTDAuthor(ctx context.Context, user names.UserTag, author string) (params.SetQOTDAuthorResult, error) {
	args := params.SetQOTDAuthorArgs{
		Entity: params.Entity{Tag: user.String()},
		Author: author,
	}
	result := params.SetQOTDAuthorResult{}
	if err := c.facade.FacadeCall(ctx, "SetQOTDAuthor", args, &result); err != nil {
		return params.SetQOTDAuthorResult{}, errors.Trace(err)
	}
	return result, nil
}
