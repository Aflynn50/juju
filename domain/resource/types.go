// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"io"
	"time"

	"github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	coreresource "github.com/juju/juju/core/resource"
	coreresourcestore "github.com/juju/juju/core/resource/store"
	charmresource "github.com/juju/juju/internal/charm/resource"
)

// State indicates if a resource is downloaded onto the controller
// (available), or known to be available on charmhub (potential).
type State string

const (
	StatePotential = "potential"
	StateAvailable = "available"
)

// GetApplicationResourceIDArgs holds the arguments for the
// GetApplicationResourceID method.
type GetApplicationResourceIDArgs struct {
	ApplicationID application.ID
	Name          string
}

// SetRepositoryResourcesArgs holds the arguments for the
// SetRepositoryResources method.
type SetRepositoryResourcesArgs struct {
	// ApplicationID is the id of the application having these resources.
	ApplicationID application.ID
	// CharmID is the unique identifier for a charm to update resources.
	CharmID corecharm.ID
	// Info is a slice of resource data received from the repository.
	Info []charmresource.Resource
	// LastPolled indicates when the resource data was last polled.
	LastPolled time.Time
}

// StoreResourceArgs holds the arguments for resource storage methods.
type StoreResourceArgs struct {
	// ResourceUUID is the unique identifier of the resource.
	ResourceUUID coreresource.UUID
	// Reader is a reader for the resource blob.
	Reader io.Reader
	// RetrievedBy is the identity of the entity that retrieved the resource.
	// This field is optional.
	RetrievedBy string
	// RetrievedByType is the type of entity that retrieved the resource. This
	// field is optional.
	RetrievedByType coreresource.RetrievedByType
	// Size is the size in bytes of the resource blob.
	Size int64
	// Fingerprint is the hash of the resource blob.
	Fingerprint charmresource.Fingerprint
	// Origin is where the resource blob comes from.
	Origin charmresource.Origin
	// Revision indicates the resource revision.
	Revision int
}

// RecordStoredResourceArgs holds the arguments for record stored resource state
// method.
type RecordStoredResourceArgs struct {
	// ResourceUUID is the unique identifier of the resource.
	ResourceUUID coreresource.UUID
	// StorageID is the store ID of the resources' blob.
	StorageID coreresourcestore.ID
	// RetrievedBy is the identity of the entity that retrieved the resource.
	// This field is optional.
	RetrievedBy string
	// RetrievedByType is the type of entity that retrieved the resource. This
	// field is optional.
	RetrievedByType coreresource.RetrievedByType
	// ResourceType is the type of the resource
	ResourceType charmresource.Type
	// IncrementCharmModifiedVersion indicates weather the charm modified
	// version should be incremented or not.
	IncrementCharmModifiedVersion bool
	// Size is the size in bytes of the resource blob.
	Size int64
	// SHA384 is the hash of the resource blob.
	SHA384 string
	// Origin is where the resource blob comes from.
	Origin charmresource.Origin
	// Revision indicates the resource revision.
	Revision int
}

// SetResourcesArgs are the arguments for SetResource.
type SetResourcesArgs []SetResourcesArg

// SetResourcesArg is a single argument for the SetResources method.
type SetResourcesArg struct {
	// ApplicationName is the name of the application these resources are
	// associated with.
	ApplicationName string
	// ApplicationResources are the available resources on the application.
	Resources []SetResourceInfo
	// UnitResources contains information about the units using the resources in
	// ApplicationResources.
	UnitResources []SetUnitResourceInfo
}

// SetResourceInfo contains information about a single resource for the
// SetResources method.
type SetResourceInfo struct {
	// Name is the name of the resource.
	Name string
	// Origin identifies where the resource will come from.
	Origin charmresource.Origin
	// Revision is the charm store revision of the resource.
	Revision int
	// Timestamp is the time the resource was added to the model.
	Timestamp time.Time
}

// SetUnitResourceInfo contains information about a single unit resource for the
// SetResources method.
type SetUnitResourceInfo struct {
	// ResourceName is the name of the resource.
	ResourceName string
	// UnitName is the name of the unit using the resource.
	UnitName string
	// Timestamp is the time the resource was added to the model.
	Timestamp time.Time
}
