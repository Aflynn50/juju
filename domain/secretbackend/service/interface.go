// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/secretbackend"
)

// State provides methods for working with secret backends.
type State interface {
	GetControllerModelCloudAndCredential(ctx context.Context) (cloud.Cloud, cloud.Credential, error)
	GetModelCloudAndCredential(ctx context.Context, modelUUID coremodel.UUID) (cloud.Cloud, cloud.Credential, error)

	CreateSecretBackend(ctx context.Context, params secretbackend.CreateSecretBackendParams) (string, error)
	UpdateSecretBackend(ctx context.Context, params secretbackend.UpdateSecretBackendParams) (string, error)
	DeleteSecretBackend(ctx context.Context, _ secretbackend.BackendIdentifier, deleteInUse bool) error
	ListSecretBackends(ctx context.Context) ([]*secretbackend.SecretBackend, error)
	ListSecretBackendIDs(ctx context.Context) ([]string, error)
	ListSecretBackendsForModel(ctx context.Context, modelUUID coremodel.UUID, includeEmpty bool) ([]*secretbackend.SecretBackend, error)
	ListInUseKubernetesSecretBackends(ctx context.Context) ([]*secretbackend.SecretBackend, error)
	GetSecretBackend(context.Context, secretbackend.BackendIdentifier) (*secretbackend.SecretBackend, error)
	SecretBackendRotated(ctx context.Context, backendID string, next time.Time) error

	SetModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID, secretBackendName string) error
	GetModelSecretBackendDetails(ctx context.Context, modelUUID coremodel.UUID) (secretbackend.ModelSecretBackend, error)

	InitialWatchStatementForSecretBackendRotationChanges() (string, string)
	GetSecretBackendRotateChanges(ctx context.Context, backendIDs ...string) ([]watcher.SecretBackendRotateChange, error)
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new namespace watcher
	// for events based on the input change mask.
	NewNamespaceWatcher(string, changestream.ChangeType, eventsource.NamespaceQuery) (watcher.StringsWatcher, error)

	// NewValueWatcher returns a watcher for a particular change
	// value in a namespace, based on the input change mask.
	NewValueWatcher(namespace, changeValue string, changeMask changestream.ChangeType) (watcher.NotifyWatcher, error)
}