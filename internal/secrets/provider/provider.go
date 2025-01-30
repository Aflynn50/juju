// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"sort"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/schema"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/configschema"
)

// SecretRevisions holds external revision ids for a list of secrets.
type SecretRevisions map[string]set.Strings

// Add adds a secret with revisions.
func (nm SecretRevisions) Add(uri *secrets.URI, revisionIDs ...string) {
	if _, ok := nm[uri.ID]; !ok {
		nm[uri.ID] = set.NewStrings(revisionIDs...)
		return
	}
	for _, rev := range revisionIDs {
		nm[uri.ID].Add(rev)
	}
}

// RevisionIDs returns all the secret revisions.
func (nm SecretRevisions) RevisionIDs() (result []string) {
	for _, revisions := range nm {
		result = append(result, revisions.SortedValues()...)
	}
	sort.Strings(result) // for testing.
	return result
}

const (
	// Auto uses either controller or k8s backends
	// depending on the model type.
	Auto = "auto"

	// Internal is the controller backend.
	Internal = "internal"
)

// ConfigAttrs defines config attributes for a secrets backend provider.
type ConfigAttrs map[string]interface{}

// ProviderConfig is implemented by providers that support config validation.
type ProviderConfig interface {
	// ConfigSchema returns the fields defining the provider config.
	ConfigSchema() configschema.Fields

	// ConfigDefaults returns default attribute values.
	ConfigDefaults() schema.Defaults

	// ValidateConfig returns an error if the new
	//provider config is not valid.
	ValidateConfig(oldCfg, newCfg ConfigAttrs, tokenRotateInterval *time.Duration) error
}

// SecretBackendProvider instances create secret backends.
type SecretBackendProvider interface {
	// Type is the type of the backend.
	Type() string

	// Initialise sets up the secrets backend to host secrets for
	// the specified model config.
	Initialise(cfg *ModelBackendConfig) error

	// CleanupSecrets removes any ACLs / resources associated
	// with the removed secrets.
	CleanupSecrets(ctx context.Context, cfg *ModelBackendConfig, accessor secrets.Accessor, removed SecretRevisions) error

	// CleanupModel removes any secrets / ACLs / resources
	// associated with the model config.
	CleanupModel(cfg *ModelBackendConfig) error

	// RestrictedConfig returns the config needed to create a
	// secrets backend client restricted to manage the specified
	// owned secrets and read shared secrets for the given entity tag.
	RestrictedConfig(ctx context.Context, adminCfg *ModelBackendConfig, sameController, forDrain bool, accessor secrets.Accessor, owned SecretRevisions, read SecretRevisions) (*BackendConfig, error)

	// NewBackend creates a secrets backend client using the
	// specified model config.
	NewBackend(cfg *ModelBackendConfig) (SecretsBackend, error)
}

// SupportAuthRefresh defines the methods to refresh auth tokens.
type SupportAuthRefresh interface {
	RefreshAuth(ctx context.Context, adminCfg BackendConfig, validFor time.Duration) (*BackendConfig, error)
}

// HasAuthRefresh returns true if the provider supports token refresh.
func HasAuthRefresh(p SecretBackendProvider) bool {
	_, ok := p.(SupportAuthRefresh)
	return ok
}
