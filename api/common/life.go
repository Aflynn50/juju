// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/rpc/params"
)

// Life requests the life cycle of the given entities from the given
// server-side API facade via the given caller.
func Life(ctx context.Context, caller base.FacadeCaller, tags []names.Tag) ([]params.LifeResult, error) {
	if len(tags) == 0 {
		return []params.LifeResult{}, nil
	}
	var result params.LifeResults
	entities := make([]params.Entity, len(tags))
	for i, t := range tags {
		entities[i] = params.Entity{Tag: t.String()}
	}
	args := params.Entities{Entities: entities}
	if err := caller.FacadeCall(ctx, "Life", args, &result); err != nil {
		return []params.LifeResult{}, err
	}
	return result.Results, nil
}

// OneLife requests the life cycle of the given entity from the given
// server-side API facade via the given caller.
func OneLife(ctx context.Context, caller base.FacadeCaller, tag names.Tag) (life.Value, error) {
	result, err := Life(ctx, caller, []names.Tag{tag})
	if err != nil {
		return "", err
	}
	if len(result) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(result))
	}
	if err := result[0].Error; err != nil {
		return "", err
	}
	return result[0].Life, nil
}
