// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/rpc/params"
)

type ApplicationService interface {
	// GetUnitWorkloadStatus returns the workload status of the specified unit, returning an
	// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
	GetUnitWorkloadStatus(context.Context, coreunit.Name) (*corestatus.StatusInfo, error)

	// SetUnitWorkloadStatus sets the workload status of the specified unit, returning an
	// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
	SetUnitWorkloadStatus(context.Context, coreunit.Name, *corestatus.StatusInfo) error
}

// UnitStatusSetter defines the API used to set the workload status of a unit.
type UnitStatusSetter struct {
	clock              clock.Clock
	applicationService ApplicationService
	getCanModify       GetAuthFunc
}

// NewUnitStatusSetter returns a new UnitStatusSetter.
func NewUnitStatusSetter(applicationService ApplicationService, clock clock.Clock, getCanModify GetAuthFunc) *UnitStatusSetter {
	return &UnitStatusSetter{
		applicationService: applicationService,
		getCanModify:       getCanModify,
		clock:              clock,
	}
}

// SetStatus sets the workload status of the specified units.
func (s *UnitStatusSetter) SetStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	canModify, err := s.getCanModify()
	if err != nil {
		return params.ErrorResults{}, err
	}

	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	now := s.clock.Now()

	for i, arg := range args.Entities {
		tag, err := names.ParseUnitTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if !canModify(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		sInfo := corestatus.StatusInfo{
			Status:  corestatus.Status(arg.Status),
			Message: arg.Info,
			Data:    arg.Data,
			Since:   &now,
		}
		err = s.applicationService.SetUnitWorkloadStatus(ctx, unitName, &sInfo)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("unit %q", unitName))
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return result, nil
}

// UnitStatusGetter defines the API used to get the workload status of a unit.
type UnitStatusGetter struct {
	clock              clock.Clock
	applicationService ApplicationService
	getCanAccess       GetAuthFunc
}

// NewUnitStatusGetter returns a new UnitStatusGetter.
func NewUnitStatusGetter(applicationService ApplicationService, clock clock.Clock, getCanAccess GetAuthFunc) *UnitStatusGetter {
	return &UnitStatusGetter{
		applicationService: applicationService,
		getCanAccess:       getCanAccess,
		clock:              clock,
	}
}

// Status returns the workload status of the specified units.
func (s *UnitStatusGetter) Status(ctx context.Context, args params.Entities) (params.StatusResults, error) {
	canAccess, err := s.getCanAccess()
	if err != nil {
		return params.StatusResults{}, err
	}

	result := params.StatusResults{
		Results: make([]params.StatusResult, len(args.Entities)),
	}

	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		sInfo, err := s.applicationService.GetUnitWorkloadStatus(ctx, unitName)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("unit %q", unitName))
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i] = params.StatusResult{
			Status: sInfo.Status.String(),
			Info:   sInfo.Message,
			Data:   sInfo.Data,
			Since:  sInfo.Since,
		}
	}
	return result, nil
}
