// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"
	"path"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/domain/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	SetResource(ctx context.Context, resource resource.Resource) error
	PutOCIImage(ctx context.Context, resource resource.Resource) error
	GetResourceUUID(ctx context.Context, application application.ID, name string) (resources.UUID, error)
	GetResource(ctx context.Context, uuid resources.UUID) (resource.Resource, error)
}

// Service defines a service for interacting with the underlying state.
type Service struct {
	st                State
	logger            logger.Logger
	objectStoreGetter objectstore.ModelObjectStoreGetter
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State, logger logger.Logger) *Service {
	return &Service{
		st:     st,
		logger: logger,
	}
}

func (s *Service) GetResourceUUID(ctx context.Context, application application.ID, name string) (resources.UUID, error) {
	return s.st.GetResourceUUID(ctx, application, name)
}

// SetResourceInfo sets information about the resource in state, but does not
// store the resource blob.
func (s *Service) SetResourceInfo(ctx context.Context, resource resource.Resource) error {
	// TODO set CreatedAt in the resource.
	err := s.st.SetResource(ctx, resource)
	if err != nil {
		return err
	}
	return nil
}

// SetResourceContent stores the resource content for a known resource.
func (s *Service) SetResourceContent(ctx context.Context, uuid resources.UUID, r io.ReadCloser) error {
	resource, err := s.st.GetResource(ctx, uuid)
	if err != nil {
		return err
	}
	switch resource.Type {
	case charmresource.TypeContainerImage:
		err = s.setFileResource(ctx, resource, r)
	case charmresource.TypeFile:
		err = s.setOCIImageResource(ctx, resource, r)
	}
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) SetResource(ctx context.Context, resource resource.Resource, r io.ReadCloser) error {

}

func (s *Service) setFileResource(ctx context.Context, resource resource.Resource, r io.ReadCloser) error {
	objectStore, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return err
	}

	path := storagePath(resource.Name, resource.ApplicationID)
	hash := resource.Fingerprint.String()
	err = objectStore.PutAndCheckHash(ctx, path, r, resource.Size, hash)
	if err != nil {
		return errors.Errorf("putting resource %q for application %q in object store: %w", resource.Name, resource.ApplicationID, err)
	}

	err = s.st.SetResource(ctx, resource)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) setOCIImageResource(ctx context.Context, resource resources.Resource, r io.ReadCloser) error {
	err := s.st.PutOCIImage()
	if err != nil {
		return err
	}

	err = s.st.SetResource(ctx, resource)
	if err != nil {
		return err
	}
	return nil
}

func (*Service) OpenApplicationResource() {}

func (*Service) OpenUnitResource() {}

// storagePath returns the path used as the location where the resource
// is stored in the object store.
func storagePath(name, applicationID string) string {
	return path.Join("application-"+applicationID, "resources", name)
}
