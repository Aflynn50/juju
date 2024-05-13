// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type hashFileSystemAccessorSuite struct {
	baseSuite
}

var _ = gc.Suite(&hashFileSystemAccessorSuite{})

func (s *hashFileSystemAccessorSuite) TestHashExistsNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	err := os.MkdirAll(s.namespaceFilePath(dir), 0755)
	c.Assert(err, jc.ErrorIsNil)

	accessor := newHashFileSystemAccessor("namespace", dir, loggertesting.WrapCheckLog(c))
	err = accessor.HashExists(context.Background(), "hash")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *hashFileSystemAccessorSuite) TestHashExists(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	err := os.MkdirAll(s.namespaceFilePath(dir), 0755)
	c.Assert(err, jc.ErrorIsNil)

	_, err = os.Create(filepath.Join(s.namespaceFilePath(dir), "foo"))
	c.Assert(err, jc.ErrorIsNil)

	accessor := newHashFileSystemAccessor("namespace", dir, loggertesting.WrapCheckLog(c))
	err = accessor.HashExists(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *hashFileSystemAccessorSuite) TestGetByHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	err := os.MkdirAll(s.namespaceFilePath(dir), 0755)
	c.Assert(err, jc.ErrorIsNil)

	file, err := os.Create(filepath.Join(s.namespaceFilePath(dir), "foo"))
	c.Assert(err, jc.ErrorIsNil)

	_, err = fmt.Fprintln(file, "inferi")
	c.Assert(err, jc.ErrorIsNil)

	// Note this will include the new line character. This is on purpose and
	// is baked into the implementation.

	accessor := newHashFileSystemAccessor("namespace", dir, loggertesting.WrapCheckLog(c))
	reader, size, err := accessor.GetByHash(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(size, gc.Equals, int64(7))

	bytes, err := io.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(bytes), gc.Equals, "inferi\n")
}

func (s *hashFileSystemAccessorSuite) TestGetByHashNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	err := os.MkdirAll(s.namespaceFilePath(dir), 0755)
	c.Assert(err, jc.ErrorIsNil)

	accessor := newHashFileSystemAccessor("namespace", dir, loggertesting.WrapCheckLog(c))
	_, _, err = accessor.GetByHash(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *hashFileSystemAccessorSuite) TestDeleteByHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	err := os.MkdirAll(s.namespaceFilePath(dir), 0755)
	c.Assert(err, jc.ErrorIsNil)

	_, err = os.Create(filepath.Join(s.namespaceFilePath(dir), "foo"))
	c.Assert(err, jc.ErrorIsNil)

	accessor := newHashFileSystemAccessor("namespace", dir, loggertesting.WrapCheckLog(c))

	err = accessor.DeleteByHash(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	_, err = os.Stat(filepath.Join(s.namespaceFilePath(dir), "foo"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *hashFileSystemAccessorSuite) TestDeleteByHashNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	err := os.MkdirAll(s.namespaceFilePath(dir), 0755)
	c.Assert(err, jc.ErrorIsNil)

	accessor := newHashFileSystemAccessor("namespace", dir, loggertesting.WrapCheckLog(c))

	err = accessor.DeleteByHash(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *hashFileSystemAccessorSuite) namespaceFilePath(dir string) string {
	return filepath.Join(dir, "objectstore", "namespace")
}
