// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/errors"
)

// GetProviderAvailabilityZones returns all the availability zones
// retrieved from the model's cloud provider.
func (s *ProviderService) GetProviderAvailabilityZones(ctx context.Context) (network.AvailabilityZones, error) {
	zoneProvider, err := s.providerWithZones(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return network.AvailabilityZones{}, nil
	}
	if err != nil {
		return network.AvailabilityZones{}, errors.Capture(err)
	}
	result, err := zoneProvider.AvailabilityZones(ctx)
	if err != nil {
		return network.AvailabilityZones{}, errors.Errorf("getting provider availability zones: %w", err)
	}
	return result, nil
}
