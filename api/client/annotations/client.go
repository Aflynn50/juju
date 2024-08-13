// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client allows access to the annotations API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the annotations API.
func NewClient(st base.APICallCloser, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(st, "Annotations", options...)
	return &Client{ClientFacade: frontend, facade: backend}
}

// Get returns annotations that have been set on the given entities.
func (c *Client) Get(ctx context.Context, tags []string) ([]params.AnnotationsGetResult, error) {
	annotations := params.AnnotationsGetResults{}
	if err := c.facade.FacadeCall(ctx, "Get", entitiesFromTags(tags), &annotations); err != nil {
		return annotations.Results, errors.Trace(err)
	}
	return annotations.Results, nil
}

// Set sets entity annotation pairs.
func (c *Client) Set(ctx context.Context, annotations map[string]map[string]string) ([]params.ErrorResult, error) {
	args := params.AnnotationsSet{entitiesAnnotations(annotations)}
	results := new(params.ErrorResults)
	if err := c.facade.FacadeCall(ctx, "Set", args, results); err != nil {
		return nil, errors.Trace(err)
	}
	return results.Results, nil
}

func entitiesFromTags(tags []string) params.Entities {
	entities := []params.Entity{}
	for _, tag := range tags {
		entities = append(entities, params.Entity{tag})
	}
	return params.Entities{entities}
}

func entitiesAnnotations(annotations map[string]map[string]string) []params.EntityAnnotations {
	all := []params.EntityAnnotations{}
	for tag, pairs := range annotations {
		one := params.EntityAnnotations{
			EntityTag:   tag,
			Annotations: pairs,
		}
		all = append(all, one)
	}
	return all
}
