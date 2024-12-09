// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	coreresource "github.com/juju/juju/core/resource"
	coreresourcestore "github.com/juju/juju/core/resource/store"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

// State describes retrieval and persistence methods for resource.
type State interface {
	// DeleteApplicationResources removes all associated resources of a given
	// application identified by applicationID.
	DeleteApplicationResources(ctx context.Context, applicationID coreapplication.ID) error

	// DeleteUnitResources deletes the association of resources with a specific
	// unit.
	DeleteUnitResources(ctx context.Context, uuid coreunit.UUID) error

	// GetApplicationResourceID returns the ID of the application resource
	// specified by natural key of application and resource name.
	GetApplicationResourceID(ctx context.Context, args resource.GetApplicationResourceIDArgs) (coreresource.UUID, error)

	// ListResources returns the list of resource for the given application.
	ListResources(ctx context.Context, applicationID coreapplication.ID) (resource.ApplicationResources, error)

	// GetResource returns the identified resource.
	GetResource(ctx context.Context, resourceUUID coreresource.UUID) (resource.Resource, error)

	// LinkStoredResource links a storageUUID to a resourceUUID
	LinkStoredResource(ctx context.Context, resourceUUID coreresource.UUID, storageUUID coreresourcestore.ID) (resource.Resource, error)

	// LinkStoredResourceAndMarkChanged links a storageUUID to a resourceUUID
	// and marks on the application that something has changed.
	LinkStoredResourceAndMarkChanged(ctx context.Context, resourceUUID coreresource.UUID, storageUUID coreresourcestore.ID) (resource.Resource, error)

	// SetUnitResource sets the resource metadata for a specific unit.
	SetUnitResource(ctx context.Context, resourceUUID coreresource.UUID, unitUUID coreunit.UUID) (resource.Resource, error)

	// SetApplicationResource sets the resource application metadata an
	// application's resource.
	SetApplicationResource(ctx context.Context, resourceUUID coreresource.UUID, applicationID coreapplication.ID) (resource.Resource, error)

	// SetRepositoryResources sets the "polled" resource for the
	// application to the provided values. The current data for this
	// application/resource combination will be overwritten.
	SetRepositoryResources(ctx context.Context, config resource.SetRepositoryResourcesArgs) error
}

type ResourceStoreGetter interface {
	// GetResourceStore returns the appropriate ResourceStore for the
	// given resource type.
	GetResourceStore(context.Context, charmresource.Type) (coreresourcestore.ResourceStore, error)
}

// Service provides the API for working with resources.
type Service struct {
	st     State
	logger logger.Logger

	resourceStoreGetter ResourceStoreGetter
}

// NewService returns a new service reference wrapping the input state.
func NewService(
	st State,
	resourceStoreGetter ResourceStoreGetter,
	logger logger.Logger,
) *Service {
	return &Service{
		st:                  st,
		resourceStoreGetter: resourceStoreGetter,
		logger:              logger,
	}
}

// DeleteApplicationResources removes the resources of a specified application.
// It should be called after all resources have been unlinked from potential
// units by DeleteUnitResources and their associated data removed from store.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ApplicationIDNotValid] is returned if the application
//     ID is not valid.
//   - [resourceerrors.InvalidCleanUpState] is returned is there is
//     remaining units or stored resources which are still associated with
//     application resources.
func (s *Service) DeleteApplicationResources(
	ctx context.Context,
	applicationID coreapplication.ID,
) error {
	if err := applicationID.Validate(); err != nil {
		return resourceerrors.ApplicationIDNotValid
	}
	return s.st.DeleteApplicationResources(ctx, applicationID)
}

// DeleteUnitResources unlinks the resources associated to a unit by its UUID.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.UnitUUIDNotValid] is returned if the unit ID is not
//     valid.
func (s *Service) DeleteUnitResources(
	ctx context.Context,
	uuid coreunit.UUID,
) error {
	if err := uuid.Validate(); err != nil {
		return resourceerrors.UnitUUIDNotValid
	}
	return s.st.DeleteUnitResources(ctx, uuid)
}

// GetApplicationResourceID returns the ID of the application resource specified
// by natural key of application and resource name.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ResourceNameNotValid] if no resource name is provided
//     in the args.
//   - [coreerrors.NotValid] is returned if the application ID is not valid.
//   - [resourceerrors.ResourceNotFound] if no resource with name exists for
//     given application.
func (s *Service) GetApplicationResourceID(
	ctx context.Context,
	args resource.GetApplicationResourceIDArgs,
) (coreresource.UUID, error) {
	if err := args.ApplicationID.Validate(); err != nil {
		return "", errors.Errorf("application id: %w", err)
	}
	if args.Name == "" {
		return "", resourceerrors.ResourceNameNotValid
	}
	return s.st.GetApplicationResourceID(ctx, args)
}

// ListResources returns the resource data for the given application including
// application, unit and repository resource data. Unit data is only included
// for machine units. Repository resource data is included if it exists.
//
// The following error types can be expected to be returned:
//   - [coreerrors.NotValid] is returned if the application ID is not valid.
//   - [resourceerrors.ApplicationDyingOrDead] for dead or dying
//     applications.
//   - [resourceerrors.ApplicationNotFound] when the specified application
//     does not exist.
//
// No error is returned if the provided application has no resource.
func (s *Service) ListResources(
	ctx context.Context,
	applicationID coreapplication.ID,
) (resource.ApplicationResources, error) {
	if err := applicationID.Validate(); err != nil {
		return resource.ApplicationResources{}, errors.Errorf("application id: %w", err)
	}
	return s.st.ListResources(ctx, applicationID)
}

// GetResource returns the identified application resource.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ApplicationNotFound] if the specified application does
//     not exist.
func (s *Service) GetResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
) (resource.Resource, error) {
	if err := resourceUUID.Validate(); err != nil {
		return resource.Resource{}, errors.Errorf("application id: %w", err)
	}
	return s.st.GetResource(ctx, resourceUUID)
}

<<<<<<< HEAD
// SetResource adds the application resource to blob storage and updates the metadata.
//
// The following error types can be expected to be returned:
//   - [coreerrors.NotValid] is returned if the application ID is not valid.
//   - [coreerrors.NotValid] is returned if the resource is not valid.
//   - [coreerrors.NotValid] is returned if the RetrievedByType is unknown while
//     RetrievedBy has a value.
//   - [resourceerrors.ApplicationNotFound] if the specified application does
//     not exist.
func (s *Service) SetResource(
=======
// StoreResource adds the application resource to blob storage and updates the
// metadata. It also sets the retrieved_by field.
func (s *Service) StoreResource(
>>>>>>> f439b76e1a (WIP)
	ctx context.Context,
	applicationID coreapplication.ID,
	retrievedBy string,
	retrievedByType resource.RetrievedByType,
	resourceUUID coreresource.UUID,
	reader io.Reader,
) (resource.Resource, error) {
	if err := applicationID.Validate(); err != nil {
		return resource.Resource{}, errors.Errorf("application id: %w", err)
	}
	if retrievedBy != "" && retrievedByType == resource.Unknown {
		return resource.Resource{},
			errors.Errorf("RetrievedByType cannot be unknown if RetrievedBy set: %w", resourceerrors.ArgumentNotValid)
	}
	if err := resourceUUID.Validate(); err != nil {
		return resource.Resource{}, errors.Errorf("resource uuid: %w", err)
	}

	res, err := s.st.GetResource(ctx, resourceUUID)
	if err != nil {
		return resource.Resource{}, err
	}

	store, err := s.resourceStoreGetter.GetResourceStore(ctx, res.Type)
	if err != nil {
		return resource.Resource{}, errors.Errorf("getting resource store for %s: %w", res.Type.String(), err)
	}

	storageUUID, err := store.Put(ctx, res.UUID.String(), reader, res.Size, coreresourcestore.NewFingerprint(res.Fingerprint.Fingerprint))
	if err != nil {
		return resource.Resource{}, errors.Errorf("getting resource from store: %w", err)
	}
	err = s.st.LinkStoredResource(ctx, resourceUUID, storageUUID, retrievedBy, retrievedByType)
	return res, err
}

func (s *Service) StoreResourceAndMarkChanged(
	ctx context.Context,
	ApplicationID coreapplication.ID,
	SuppliedBy string,
	SuppliedByType resource.RetrievedByType,
	Resource coreresource.UUID,
	Reader io.Reader,
) {
	//TODO Shouldn't this be a method on the application service really?
}

// OpenResource returns the details of and a reader for
// the resource.
//
// The following error types can be expected to be returned:
<<<<<<< HEAD
//   - [coreerrors.NotValid] is returned if the unit UUID is not valid.
//   - [coreerrors.NotValid] is returned if the resource UUID is not valid.
//   - [resourceerrors.ArgumentNotValid] is returned if the RetrievedByType is unknown while
//     RetrievedBy has a value.
//   - [resourceerrors.ResourceNotFound] if the specified resource doesn't exist
//   - [resourceerrors.UnitNotFound] if the specified unit doesn't exist
=======
//   - [resourceerrors.ResourceNotFound] if the specified resource does
//     not exist.
func (s *Service) OpenResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
) (resource.Resource, io.ReadCloser, error) {
	if err := resourceUUID.Validate(); err != nil {
		return resource.Resource{}, nil, errors.Errorf("resource id: %w", err)
	}

	res, err := s.st.GetResource(ctx, resourceUUID)
	if err != nil {
		return resource.Resource{}, nil, err
	}

	store, err := s.resourceStoreGetter.GetResourceStore(ctx, res.Type)
	if err != nil {
		return resource.Resource{}, nil, errors.Errorf("getting resource store for %s: %w", res.Type.String(), err)
	}

	reader, size, err := store.Get(ctx, res.UUID.String())
	if err != nil {
		return resource.Resource{}, nil, errors.Errorf("getting resource from store: %w", err)
	}

	if size != res.Size {
		return resource.Resource{}, nil, errors.Errorf("retrived resource size does not match expected resource size (%d != %d)", size, res.Size)
	}
	return res, reader, nil
}

// SetUnitResource sets the unit as using the resource.
>>>>>>> f439b76e1a (WIP)
func (s *Service) SetUnitResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
	unitUUID coreunit.UUID,
) (resource.Resource, error) {
	res, err := s.st.SetUnitResource(ctx, resourceUUID, unitUUID)
	if err != nil {
		return resource.Resource{}, errors.Errorf("recording resource for unit: %w")
	}
	return res, nil
}

<<<<<<< HEAD
// OpenApplicationResource returns the details of and a reader for the resource.
//
// The following error types can be expected to be returned:
//   - [coreerrors.NotValid] is returned if the resource.UUID is not valid.
//   - [resourceerrors.ResourceNotFound] if the specified resource does
//     not exist.
func (s *Service) OpenApplicationResource(
=======
// SetApplicationResource sets the application as using the resource.
func (s *Service) SetApplicationResource(
>>>>>>> f439b76e1a (WIP)
	ctx context.Context,
	resourceUUID coreresource.UUID,
	applicationUUID coreapplication.ID,
) (resource.Resource, error) {
	res, err := s.st.SetApplicationResource(ctx, resourceUUID, applicationUUID)
	if err != nil {
		return resource.Resource{}, errors.Errorf("recording resource for application: %w")
	}
<<<<<<< HEAD
	res, err := s.st.OpenApplicationResource(ctx, resourceUUID)
	return res, &noopReadCloser{}, err
}

// OpenUnitResource returns metadata about the resource and a reader for
// the resource. The resource is associated with the unit once the reader is
// completely exhausted. Read progress is stored until the reader is completely
// exhausted. Typically used for File resource.
//
// The following error types can be returned:
//   - [coreerrors.NotValid] is returned if the resource.UUID is not valid.
//   - [coreerrors.NotValid] is returned if the unit UUID is not valid.
//   - [resourceerrors.ResourceNotFound] if the specified resource does
//     not exist.
//   - [resourceerrors.UnitNotFound] if the specified unit does
//     not exist.
func (s *Service) OpenUnitResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
	unitID coreunit.UUID,
) (resource.Resource, io.ReadCloser, error) {
	if err := unitID.Validate(); err != nil {
		return resource.Resource{}, nil, fmt.Errorf("unit id: %w", err)
	}
	if err := resourceUUID.Validate(); err != nil {
		return resource.Resource{}, nil, fmt.Errorf("resource id: %w", err)
	}
	res, err := s.st.OpenUnitResource(ctx, resourceUUID, unitID)
	return res, &noopReadCloser{}, err
=======
	return res, nil
>>>>>>> f439b76e1a (WIP)
}

// SetRepositoryResources sets the "polled" resource for the application to
// the provided values. These are resource collected from the repository for
// the application.
//
// The following error types can be expected to be returned:
//   - [coreerrors.NotValid] is returned if the Application ID is not valid.
//   - [resourceerrors.ArgumentNotValid] is returned if LastPolled is zero.
//   - [resourceerrors.ArgumentNotValid] is returned if the length of Info is zero.
//   - [resourceerrors.ApplicationNotFound] if the specified application does
//     not exist.
func (s *Service) SetRepositoryResources(
	ctx context.Context,
	args resource.SetRepositoryResourcesArgs,
) error {
	if err := args.ApplicationID.Validate(); err != nil {
		return errors.Errorf("application id: %w", err)
	}
	if len(args.Info) == 0 {
		return errors.Errorf("empty Info: %w", resourceerrors.ArgumentNotValid)
	}
	for _, info := range args.Info {
		if err := info.Validate(); err != nil {
			return errors.Errorf("resource: %w", err)
		}
	}
	if args.LastPolled.IsZero() {
		return errors.Errorf("zero LastPolled: %w", resourceerrors.ArgumentNotValid)
	}
	return s.st.SetRepositoryResources(ctx, args)
}
