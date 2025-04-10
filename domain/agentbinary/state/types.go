// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "github.com/juju/juju/domain/agentbinary"

// architectureRecord represents an architecture row in the database.
type architectureRecord struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

// objectStoreUUID is a database type for representing the uuid of an object
// store metadata row.
type objectStoreUUID struct {
	UUID string `db:"uuid"`
}

// agentBinaryRecord represents an agent binary entry in the database.
type agentBinaryRecord struct {
	Version         string `db:"version"`
	ArchitectureID  int    `db:"architecture_id"`
	ObjectStoreUUID string `db:"object_store_uuid"`
}

type metadataRecord struct {
	// Version is the version of the agent binary.
	Version string `db:"version"`
	// Size is the size of the agent binary in bytes.
	Size int64 `db:"size"`
	// SHA256 is the SHA256 hash of the agent binary.
	SHA256 string `db:"sha_256"`
}

type metadataRecords []metadataRecord

func (m metadataRecords) toMetadata() []agentbinary.Metadata {
	metadata := make([]agentbinary.Metadata, len(m))
	for i, record := range m {
		metadata[i] = agentbinary.Metadata{
			Version: record.Version,
			Size:    record.Size,
			SHA256:  record.SHA256,
		}
	}
	return metadata
}
