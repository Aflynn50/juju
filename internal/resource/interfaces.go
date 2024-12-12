// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"io"
	"net/url"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/environs/config"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
	"github.com/juju/juju/state"
)

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

type ResourceService interface {
	// OpenResource returns the details of and a reader for the resource.
	//
	// The following error types can be expected to be returned:
	//   - [resourceerrors.ResourceNotFound] if the specified resource does not
	//     exist.
	//   - [resourceerrors.StoredResourceNotFound] if the specified resource is not
	//     in the resource store.
	OpenResource(ctx context.Context, resourceUUID resource.UUID) (resource.Resource, io.ReadCloser, error)

	// GetResourceUUID returns the UUID of the resource specified by natural key
	// of application and resource name.
	GetResourceUUID(ctx context.Context, applicationID coreapplication.ID, name string) (resource.UUID, error)
}

// Resources represents the methods used by the resource opener from state.Resources.
type Resources interface {
	// GetResource returns the identified resource.
	GetResource(applicationID, name string) (resource.Resource, error)
	// OpenResource returns the metadata for a resource and a reader for the resource.
	OpenResource(applicationID, name string) (resource.Resource, io.ReadCloser, error)
	// OpenResourceForUniter returns the metadata for a resource and a reader for the resource.
	OpenResourceForUniter(unitName, resName string) (resource.Resource, io.ReadCloser, error)
	// SetResource adds the resource to blob storage and updates the metadata.
	SetResource(applicationID, userID string, res charmresource.Resource, r io.Reader, _ state.IncrementCharmModifiedVersionType) (resource.Resource, error)
}

// ResourceGetter provides the functionality for getting a resource file.
type ResourceGetter interface {
	// GetResource returns a reader for the resource's data. That data
	// is streamed from the charm store. The charm's revision, if any,
	// is ignored. If the identified resource is not in the charm store
	// then errors.NotFound is returned.
	//
	// But if you write any code that assumes a NotFound error returned
	// from this method means that the resource was not found, you fail
	// basic logic.
	GetResource(ResourceRequest) (ResourceData, error)
}

// CharmHub represents methods required from a charmhub client talking to the
// charmhub api used by the local CharmHubClient
type CharmHub interface {
	DownloadResource(ctx context.Context, resourceURL *url.URL) (r io.ReadCloser, err error)
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}
