// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/database/schema"
	"github.com/juju/juju/internal/database/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type migrationSuite struct {
	testing.DqliteSuite
}

var _ = gc.Suite(&migrationSuite{})

func (s *migrationSuite) TestMigrationSuccess(c *gc.C) {
	patches := schema.New()
	patches.Add(
		schema.MakePatch("CREATE TABLE band(name TEXT PRIMARY KEY);"),
		schema.MakePatch("INSERT INTO band VALUES (?);", "Blood Incantation"),
	)

	db := s.DB()
	m := NewDBMigration(&txnRunner{db: db}, loggertesting.WrapCheckLog(c), patches)
	c.Assert(m.Apply(context.Background()), jc.ErrorIsNil)

	rows, err := db.Query("SELECT * from band;")
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { _ = rows.Close() })

	var band string
	c.Assert(rows.Next(), jc.IsTrue)
	c.Assert(rows.Scan(&band), jc.ErrorIsNil)
	c.Check(band, gc.Equals, "Blood Incantation")
}
