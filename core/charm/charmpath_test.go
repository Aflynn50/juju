// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"os"
	"path/filepath"

	"github.com/juju/charm/v12"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/testcharms"
)

type charmPathSuite struct {
	repoPath string
}

var _ = gc.Suite(&charmPathSuite{})

func (s *charmPathSuite) SetUpTest(c *gc.C) {
	s.repoPath = c.MkDir()
}

func (s *charmPathSuite) cloneCharmDir(path, name string) string {
	return testcharms.Repo.ClonedDirPath(path, name)
}

func (s *charmPathSuite) TestNoPath(c *gc.C) {
	_, _, err := corecharm.NewCharmAtPath("")
	c.Assert(err, gc.ErrorMatches, "empty charm path")
}

func (s *charmPathSuite) TestInvalidPath(c *gc.C) {
	_, _, err := corecharm.NewCharmAtPath("/foo")
	c.Assert(err, gc.Equals, os.ErrNotExist)
}

func (s *charmPathSuite) TestRepoURL(c *gc.C) {
	_, _, err := corecharm.NewCharmAtPath("ch:foo")
	c.Assert(err, gc.Equals, os.ErrNotExist)
}

func (s *charmPathSuite) TestInvalidRelativePath(c *gc.C) {
	_, _, err := corecharm.NewCharmAtPath("./foo")
	c.Assert(err, gc.Equals, os.ErrNotExist)
}

func (s *charmPathSuite) TestRelativePath(c *gc.C) {
	s.cloneCharmDir(s.repoPath, "mysql")
	cwd, err := os.Getwd()
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = os.Chdir(cwd) }()
	c.Assert(os.Chdir(s.repoPath), jc.ErrorIsNil)
	_, _, err = corecharm.NewCharmAtPath("mysql")
	c.Assert(corecharm.IsInvalidPathError(err), jc.IsTrue)
}

func (s *charmPathSuite) TestNoCharmAtPath(c *gc.C) {
	_, _, err := corecharm.NewCharmAtPath(c.MkDir())
	c.Assert(err, gc.ErrorMatches, "charm not found.*")
}

func (s *charmPathSuite) TestCharm(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "dummy")
	s.cloneCharmDir(s.repoPath, "dummy")
	ch, url, err := corecharm.NewCharmAtPath(charmDir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Meta().Name, gc.Equals, "dummy")
	c.Assert(ch.Revision(), gc.Equals, 1)
	c.Assert(url, gc.DeepEquals, charm.MustParseURL("local:dummy-1"))
}

func (s *charmPathSuite) TestCharmArchive(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "dummy")
	s.cloneCharmDir(s.repoPath, "dummy")
	chDir, err := charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)

	dir := c.MkDir()
	archivePath := filepath.Join(dir, "archive.charm")
	file, err := os.Create(archivePath)
	c.Assert(err, jc.ErrorIsNil)
	defer file.Close()

	err = chDir.ArchiveTo(file)
	c.Assert(err, jc.ErrorIsNil)

	ch, url, err := corecharm.NewCharmAtPath(archivePath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Meta().Name, gc.Equals, "dummy")
	c.Assert(ch.Revision(), gc.Equals, 1)
	c.Assert(url, gc.DeepEquals, charm.MustParseURL("local:dummy-1"))
}

func (s *charmPathSuite) TestCharmWithManifest(c *gc.C) {
	repo := testcharms.RepoForSeries("focal")
	charmDir := repo.CharmDir("cockroach")
	ch, url, err := corecharm.NewCharmAtPath(charmDir.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Meta().Name, gc.Equals, "cockroachdb")
	c.Assert(ch.Revision(), gc.Equals, 0)
	c.Assert(url, gc.DeepEquals, charm.MustParseURL("local:cockroachdb-0"))
}

func (s *charmPathSuite) TestFindsSymlinks(c *gc.C) {
	realPath := testcharms.Repo.ClonedDirPath(c.MkDir(), "dummy")
	charmsPath := c.MkDir()
	linkPath := filepath.Join(charmsPath, "dummy")
	err := os.Symlink(realPath, linkPath)
	c.Assert(err, gc.IsNil)

	ch, url, err := corecharm.NewCharmAtPath(filepath.Join(charmsPath, "dummy"))
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)
	c.Assert(ch.Meta().Name, gc.Equals, "dummy")
	c.Assert(ch.Config().Options["title"].Default, gc.Equals, "My Title")
	c.Assert(ch.(*charm.CharmDir).Path, gc.Equals, linkPath)
	c.Assert(url, gc.DeepEquals, charm.MustParseURL("local:dummy-1"))
}
