// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// architectureRecord represents a architecture entry in the database.
type architectureRecord struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

// objectStoreMeta represents the object store metadata in the database
// and exists is used to check if the object store metadata exists.
type objectStoreMeta struct {
	Exists bool   `db:"exists"`
	UUID   string `db:"uuid"`
	SHA256 string `db:"sha_256"`
	SHA384 string `db:"sha_384"`
	Size   int    `db:"size"`
}

// agentBinaryRecord represents an agent binary entry in the database.
type agentBinaryRecord struct {
	Version         string `db:"version"`
	ArchitectureID  int    `db:"architecture_id"`
	ObjectStoreUUID string `db:"object_store_uuid"`
}
