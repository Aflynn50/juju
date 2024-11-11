// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"time"

	"github.com/juju/utils/v4/hash"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/resources"
	charmresource "github.com/juju/juju/internal/charm/resource"
)

// Origin identifies where a charm's resource comes from.
type Origin string

// These are the valid resource origins.
const (
	OriginUpload Origin = "upload"
	OriginStore  Origin = "store"
)

type Resource struct {
	charmresource.Meta

	// UUID uniquely identifies a resource-application pair within the model.
	// UUID may be empty if the UUID (assigned by the model) is not known.
	UUID resources.UUID

	// Fingerprint is the SHA-384 checksum for the resource blob.
	Fingerprint hash.Fingerprint

	// Size is the size of the resource, in bytes.
	Size int64

	// ApplicationUUID identifies the application for the resource.
	ApplicationUUID application.ID

	// CreatedAt indicates when the resource was added to the model.
	CreatedAt time.Time

	// Origin identifies where the resource will come from.
	Origin Origin

	// Revision is the charm store revision of the resource.
	Revision *int

	// SuppliedBy is the name of who added the resource to the controller.
	// The name is a username if the resource is uploaded from the cli
	// by a specific user. If the resource is downloaded from a repository,
	// the ID of the unit which triggered the download is used.
	SuppliedBy string

	// SuppliedByType indicates what type of value the SuppliedBy name is:
	// application, username or unit.
	SuppliedByType SuppliedByType
}

// SuppliedByType indicates what the SuppliedBy name represents.
type SuppliedByType string

const (
	Unknown     SuppliedByType = "unknown"
	Application SuppliedByType = "application"
	Unit        SuppliedByType = "unit"
	User        SuppliedByType = "user"
)
