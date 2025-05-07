// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/backups"
	"github.com/juju/juju/internal/testing"
)

type archiveSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&archiveSuite{})

func (s *archiveSuite) TestNewCanonoicalArchivePaths(c *tc.C) {
	ap := backups.NewCanonicalArchivePaths()

	c.Check(ap.ContentDir, tc.Equals, "juju-backup")
	c.Check(ap.FilesBundle, tc.Equals, "juju-backup/root.tar")
	c.Check(ap.DBDumpDir, tc.Equals, "juju-backup/dump")
	c.Check(ap.MetadataFile, tc.Equals, "juju-backup/metadata.json")
}

func (s *archiveSuite) TestNewNonCanonicalArchivePaths(c *tc.C) {
	ap := backups.NewNonCanonicalArchivePaths("/tmp")

	c.Check(ap.ContentDir, jc.SamePath, "/tmp/juju-backup")
	c.Check(ap.FilesBundle, jc.SamePath, "/tmp/juju-backup/root.tar")
	c.Check(ap.DBDumpDir, jc.SamePath, "/tmp/juju-backup/dump")
	c.Check(ap.MetadataFile, jc.SamePath, "/tmp/juju-backup/metadata.json")
}
