// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/cloudimagemetadata"
)

type metadataAccess interface {
	FindMetadata(cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error)
	SaveMetadata([]cloudimagemetadata.Metadata) error
	DeleteMetadata(imageId string) error
	ControllerTag() names.ControllerTag
	Model() (Model, error)
}

type Model interface {
	CloudRegion() string
}

var getState = func(st *state.State) metadataAccess {
	return stateShim{st}
}

type stateShim struct {
	*state.State
}

func (s stateShim) FindMetadata(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error) {
	return s.State.CloudImageMetadataStorage.FindMetadata(f)
}

func (s stateShim) SaveMetadata(m []cloudimagemetadata.Metadata) error {
	return s.State.CloudImageMetadataStorage.SaveMetadata(m)
}

func (s stateShim) DeleteMetadata(imageId string) error {
	return s.State.CloudImageMetadataStorage.DeleteMetadata(imageId)
}

// Model returns the Model for this state.
func (s stateShim) Model() (Model, error) {
	return s.State.Model()
}
