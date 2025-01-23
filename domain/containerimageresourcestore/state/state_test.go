// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/resource/store"
	coreresourcetesting "github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/domain/containerimageresourcestore"
	"github.com/juju/juju/domain/containerimageresourcestore/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type containerImageMetadataSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&containerImageMetadataSuite{})

func (s *containerImageMetadataSuite) TestContainerImageMetadataPut(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	resourceUUID := coreresourcetesting.GenResourceUUID(c)
	ociImageMetadata := containerimageresourcestore.ContainerImageMetadata{
		RegistryPath: "testing@sha256:beef-deed",
		Username:     "docker-registry",
		Password:     "fragglerock",
	}
	resourceStorageUUID, err := st.PutContainerImageMetadata(
		context.Background(),
		resourceUUID.String(),
		ociImageMetadata.RegistryPath,
		ociImageMetadata.Username,
		ociImageMetadata.Password,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resourceStorageUUID, gc.Not(gc.Equals), "")

	retrievedRegistryPath, retrievedUsername, retrievedPassword := s.getContainerImageMetadata(c, resourceStorageUUID)
	c.Assert(retrievedRegistryPath, gc.Equals, ociImageMetadata.RegistryPath)
	c.Assert(retrievedUsername, gc.Equals, ociImageMetadata.Username)
	c.Assert(retrievedPassword, gc.Equals, ociImageMetadata.Password)
}

func (s *containerImageMetadataSuite) TestContainerImageMetadataPutOnlyRegistryName(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	resourceUUID := coreresourcetesting.GenResourceUUID(c)
	ociImageMetadata := containerimageresourcestore.ContainerImageMetadata{
		RegistryPath: "testing@sha256:beef-deed",
	}
	storageKey, err := st.PutContainerImageMetadata(
		context.Background(),
		resourceUUID.String(),
		ociImageMetadata.RegistryPath,
		"",
		"",
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageKey, gc.Not(gc.Equals), "")

	retrievedRegistryPath, retrievedUsername, retrievedPassword := s.getContainerImageMetadata(c, storageKey)
	c.Assert(retrievedRegistryPath, gc.Equals, ociImageMetadata.RegistryPath)
	c.Assert(retrievedUsername, gc.Equals, "")
	c.Assert(retrievedPassword, gc.Equals, "")
}

func (s *containerImageMetadataSuite) TestContainerImageMetadataPutTwice(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	resourceUUID := coreresourcetesting.GenResourceUUID(c)
	ociImageMetadata := containerimageresourcestore.ContainerImageMetadata{
		RegistryPath: "testing@sha256:beef-deed",
		Username:     "docker-registry",
		Password:     "fragglerock",
	}
	ociImageMetadata2 := containerimageresourcestore.ContainerImageMetadata{
		RegistryPath: "second-testing@sha256:beef-deed",
		Username:     "second-docker-registry",
		Password:     "second-fragglerock",
	}
	storageKey, err := st.PutContainerImageMetadata(
		context.Background(),
		resourceUUID.String(),
		ociImageMetadata.RegistryPath,
		ociImageMetadata.Username,
		ociImageMetadata.Password,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageKey, gc.Not(gc.Equals), "")

	_, err = st.PutContainerImageMetadata(
		context.Background(),
		resourceUUID.String(),
		ociImageMetadata2.RegistryPath,
		ociImageMetadata2.Username,
		ociImageMetadata2.Password,
	)
	c.Assert(err, jc.ErrorIs, errors.ContainerImageMetadataAlreadyStored)
}

func (s *containerImageMetadataSuite) TestContainerImageMetadataGet(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	uuid := coreresourcetesting.GenResourceUUID(c)
	ociImageMetadata := containerimageresourcestore.ContainerImageMetadata{
		StorageKey:   uuid.String(),
		RegistryPath: "testing@sha256:beef-deed",
		Username:     "docker-registry",
		Password:     "fragglerock",
	}
	s.putContainerImageMetadata(c, ociImageMetadata)
	retrieved, err := st.GetContainerImageMetadata(context.Background(), uuid.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(retrieved, gc.Equals, ociImageMetadata)
}

func (s *containerImageMetadataSuite) TestContainerImageMetadataGetBadUUID(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	storageKey := coreresourcetesting.GenResourceUUID(c).String()
	_, err := st.GetContainerImageMetadata(context.Background(), storageKey)
	c.Assert(err, jc.ErrorIs, errors.ContainerImageMetadataNotFound)
}

func (s *containerImageMetadataSuite) TestContainerImageMetadataRemove(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	storageKey := coreresourcetesting.GenResourceUUID(c)
	ociImageMetadata := containerimageresourcestore.ContainerImageMetadata{
		StorageKey:   storageKey.String(),
		RegistryPath: "testing@sha256:beef-deed",
		Username:     "docker-registry",
		Password:     "fragglerock",
	}
	s.putContainerImageMetadata(c, ociImageMetadata)

	err := st.RemoveContainerImageMetadata(context.Background(), storageKey.String())
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT 1
FROM resource_container_image_metadata_store
WHERE storage_key = ?`, storageKey.String()).Scan()
	})
	c.Assert(err, jc.ErrorIs, sql.ErrNoRows)
}

func (s *containerImageMetadataSuite) TestContainerImageMetadataRemoveBadUUID(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	resourceUUID := coreresourcetesting.GenResourceUUID(c)
	err := st.RemoveContainerImageMetadata(context.Background(), resourceUUID.String())
	c.Assert(err, jc.ErrorIs, errors.ContainerImageMetadataNotFound)
}

func (s *containerImageMetadataSuite) getContainerImageMetadata(c *gc.C, storageKey store.ID) (string, string, string) {
	id, err := storageKey.ContainerImageMetadataStoreID()
	c.Assert(err, jc.ErrorIsNil)
	var retrievedRegistryPath, retrievedUsername, retrievedPassword string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT registry_path, username, password 
FROM resource_container_image_metadata_store
WHERE storage_key = ?`, id).Scan(&retrievedRegistryPath, &retrievedUsername, &retrievedPassword)
	})
	c.Assert(err, jc.ErrorIsNil)
	return retrievedRegistryPath, retrievedUsername, retrievedPassword
}

func (s *containerImageMetadataSuite) putContainerImageMetadata(c *gc.C, metadata containerimageresourcestore.ContainerImageMetadata) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO resource_container_image_metadata_store
(storage_key, registry_path, username, password) VALUES (?, ?, ?, ?)
`, metadata.StorageKey, metadata.RegistryPath, metadata.Username, metadata.Password)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}
