// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type storageKindSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&storageKindSuite{})

// TestStorageKindDBValues ensures there's no skew between what's in the
// database table for charm_storage_kind and the typed consts used in the state packages.
func (s *storageKindSuite) TestStorageKindDBValues(c *gc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, kind FROM charm_storage_kind")
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	dbValues := make(map[StorageKind]string)
	for rows.Next() {
		var (
			id    int
			value string
		)

		c.Assert(rows.Scan(&id, &value), jc.ErrorIsNil)
		dbValues[StorageKind(id)] = value
	}
	c.Assert(dbValues, jc.DeepEquals, map[StorageKind]string{
		StorageKindFilesystem: "filesystem",
		StorageKindBlock:      "block",
	})
}
