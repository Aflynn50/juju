// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import (
	"context"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

const cleanerFacade = "Cleaner"

// API provides access to the Cleaner API facade.
type API struct {
	facade base.FacadeCaller
}

// NewAPI creates a new client-side Cleaner facade.
func NewAPI(caller base.APICaller, options ...Option) *API {
	facadeCaller := base.NewFacadeCaller(caller, cleanerFacade, options...)
	return &API{facade: facadeCaller}
}

// Cleanup calls the server-side Cleanup method.
func (api *API) Cleanup(ctx context.Context) error {
	return api.facade.FacadeCall(ctx, "Cleanup", nil, nil)
}

// WatchCleanups calls the server-side WatchCleanups method.
func (api *API) WatchCleanups(ctx context.Context) (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := api.facade.FacadeCall(ctx, "WatchCleanups", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewNotifyWatcher(api.facade.RawAPICaller(), result)
	return w, nil
}
