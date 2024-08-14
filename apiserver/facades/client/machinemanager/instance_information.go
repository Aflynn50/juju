// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/rpc/params"
)

// InstanceTypes returns instance type information for the cloud and region
// in which the current model is deployed.
func (mm *MachineManagerAPI) InstanceTypes(ctx context.Context, cons params.ModelInstanceTypesConstraints) (params.InstanceTypesResults, error) {
	invalidatorFunc, err := mm.credentialInvalidatorGetter()
	if err != nil {
		return params.InstanceTypesResults{}, errors.Trace(err)
	}
	callCtx := envcontext.WithCredentialInvalidator(ctx, invalidatorFunc)
	return instanceTypes(callCtx, mm.instanceTypeProvider, cons)
}

// instanceTypes reports back the results from the provider for what instance
// types are available for given constraints.
func instanceTypes(
	ctx envcontext.ProviderCallContext,
	providerGetter providertracker.ProviderGetter[environs.InstanceTypesFetcher],
	cons params.ModelInstanceTypesConstraints,
) (params.InstanceTypesResults, error) {
	fetcher, err := providerGetter(ctx)
	if err != nil {
		return params.InstanceTypesResults{}, fmt.Errorf(
			"cannot get instance fetcher for getting instance types: %w", err,
		)
	}

	result := make([]params.InstanceTypesResult, len(cons.Constraints))
	for i, c := range cons.Constraints {
		value := constraints.Value{}
		if c.Value != nil {
			value = *c.Value
		}
		itCons := newInstanceTypeConstraints(
			fetcher,
			value,
		)
		it, err := getInstanceTypes(ctx, itCons)
		if err != nil {
			it = params.InstanceTypesResult{Error: apiservererrors.ServerError(err)}
		}
		result[i] = it
	}

	return params.InstanceTypesResults{Results: result}, nil
}
