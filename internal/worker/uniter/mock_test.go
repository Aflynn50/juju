// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
	"github.com/juju/juju/internal/worker/uniter/storage"
	"github.com/juju/juju/rpc/params"
)

type dummyStorageAccessor struct {
	storage.StorageAccessor
}

func (*dummyStorageAccessor) UnitStorageAttachments(_ names.UnitTag) ([]params.StorageAttachmentId, error) {
	return nil, nil
}

type dummySecretsAccessor struct {
	remotestate.SecretsClient
}

func (a *dummySecretsAccessor) SecretMetadata() ([]secrets.SecretOwnerMetadata, error) {
	return nil, nil
}

func (*dummySecretsAccessor) GetConsumerSecretsRevisionInfo(string, []string) (map[string]secrets.SecretRevisionInfo, error) {
	return nil, nil
}

type nopResolver struct{}

func (nopResolver) NextOp(resolver.LocalState, remotestate.Snapshot, operation.Factory) (operation.Operation, error) {
	return nil, resolver.ErrNoOperation
}
