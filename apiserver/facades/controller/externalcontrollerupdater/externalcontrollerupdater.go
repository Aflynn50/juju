// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// EcService provides a subset of the external controller domain service methods.
type EcService interface {
	Controller(ctx context.Context, controllerUUID string) (*crossmodel.ControllerInfo, error)
	UpdateExternalController(ctx context.Context, ec crossmodel.ControllerInfo, modelUUIDs ...string) error
	Watch() (watcher.StringsWatcher, error)
}

// ExternalControllerUpdaterAPI provides access to the CrossModelRelations API facade.
type ExternalControllerUpdaterAPI struct {
	ecService EcService
	resources facade.Resources
}

// NewAPI creates a new server-side CrossModelRelationsAPI API facade backed
// by the given interfaces.
func NewAPI(
	resources facade.Resources,
	ecService EcService,
) (*ExternalControllerUpdaterAPI, error) {
	return &ExternalControllerUpdaterAPI{
		ecService: ecService,
		resources: resources,
	}, nil
}

// WatchExternalControllers watches for the addition and removal of external
// controller records to the local controller's database.
func (api *ExternalControllerUpdaterAPI) WatchExternalControllers() (params.StringsWatchResults, error) {
	w, err := api.ecService.Watch()
	if err != nil {
		return params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: apiservererrors.ServerError(errors.Annotate(err, "watching external controllers changes")),
			}},
		}, nil
	}
	changes, err := internal.FirstResult[[]string](w)
	if err != nil {
		return params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: apiservererrors.ServerError(errors.Annotate(err, "watching external controllers changes")),
			}},
		}, nil
	}
	return params.StringsWatchResults{
		Results: []params.StringsWatchResult{{
			StringsWatcherId: api.resources.Register(w),
			Changes:          changes,
		}},
	}, nil
}

// ExternalControllerInfo returns the info for the specified external controllers.
func (s *ExternalControllerUpdaterAPI) ExternalControllerInfo(args params.Entities) (params.ExternalControllerInfoResults, error) {
	result := params.ExternalControllerInfoResults{
		Results: make([]params.ExternalControllerInfoResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		controllerTag, err := names.ParseControllerTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		controllerInfo, err := s.ecService.Controller(context.TODO(), controllerTag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Result = &params.ExternalControllerInfo{
			ControllerTag: controllerTag.String(),
			Alias:         controllerInfo.Alias,
			Addrs:         controllerInfo.Addrs,
			CACert:        controllerInfo.CACert,
		}
	}
	return result, nil
}

// SetExternalControllerInfo saves the info for the specified external controllers.
func (s *ExternalControllerUpdaterAPI) SetExternalControllerInfo(args params.SetExternalControllersInfoParams) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Controllers)),
	}
	for i, arg := range args.Controllers {
		controllerTag, err := names.ParseControllerTag(arg.Info.ControllerTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err := s.ecService.UpdateExternalController(context.TODO(), crossmodel.ControllerInfo{
			ControllerTag: controllerTag,
			Alias:         arg.Info.Alias,
			Addrs:         arg.Info.Addrs,
			CACert:        arg.Info.CACert,
		}); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return result, nil
}
