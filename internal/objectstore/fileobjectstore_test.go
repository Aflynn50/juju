// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/objectstore"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type fileObjectStoreSuite struct {
	baseSuite
}

var _ = gc.Suite(&fileObjectStoreSuite{})

func (s *fileObjectStoreSuite) TestGetMetadataNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	store := s.newFileObjectStore(c, c.MkDir())
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{}, errors.NotFound).Times(2)

	_, _, err := store.Get(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *fileObjectStoreSuite) TestGetMetadataFoundNoFile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{
		Hash512_384: "blah",
		Hash256:     "blah",
		Path:        "foo",
		Size:        666,
	}, nil).Times(2)

	_, _, err := store.Get(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *fileObjectStoreSuite) TestGetMetadataAndFileFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	namespace := "inferi"
	fileName := "foo"
	size, hash512_384, hash256 := s.createFile(c, s.filePath(path, namespace), fileName, "some content")

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        fileName,
		Size:        size,
	}, nil)

	file, fileSize, err := store.Get(context.Background(), fileName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(size, gc.Equals, fileSize)
	c.Assert(s.readFile(c, file), gc.Equals, "some content")
}

func (s *fileObjectStoreSuite) TestGetMetadataAndFileNotFoundThenFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Attempt to read the file before it exists. This should fail.
	// Then attempt to read the file after it exists. This should succeed.

	path := c.MkDir()

	namespace := "inferi"
	fileName := "foo"
	size, hash512_384, hash256 := s.createFile(c, s.filePath(path, namespace), fileName, "some content")

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        fileName,
		Size:        size,
	}, errors.NotFoundf("not found"))
	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        fileName,
		Size:        size,
	}, nil)

	file, fileSize, err := store.Get(context.Background(), fileName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(size, gc.Equals, fileSize)
	c.Assert(s.readFile(c, file), gc.Equals, "some content")
}

func (s *fileObjectStoreSuite) TestGetMetadataAndFileFoundWithIncorrectSize(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	namespace := "inferi"
	fileName := "foo"
	size, hash512_384, hash256 := s.createFile(c, s.filePath(path, namespace), fileName, "some content")

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        fileName,
		Size:        size + 1,
	}, nil).Times(2)

	_, _, err := store.Get(context.Background(), fileName)
	c.Assert(err, gc.ErrorMatches, `.*size mismatch.*`)
}

func (s *fileObjectStoreSuite) TestPut(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hash512_384 := s.calculateHexHash512_384(c, "some content")
	hash256 := s.calculateHexHash256(c, "some content")

	s.expectClaim(hash512_384, 1)
	s.expectRelease(hash512_384, 1)

	path := c.MkDir()

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        "foo",
		Size:        12,
	}).Return(uuid, nil)

	received, err := store.Put(context.Background(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received.Validate(), jc.ErrorIsNil)
	c.Check(received, gc.Equals, uuid)
}

func (s *fileObjectStoreSuite) TestPutFileAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hash512_384 := s.calculateHexHash512_384(c, "some content")
	hash256 := s.calculateHexHash256(c, "some content")

	s.expectClaim(hash512_384, 2)
	s.expectRelease(hash512_384, 2)

	path := c.MkDir()

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        "foo",
		Size:        12,
	}).Return(uuid, nil).Times(2)

	uuid0, err := store.Put(context.Background(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(uuid0.Validate(), jc.ErrorIsNil)

	uuid1, err := store.Put(context.Background(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(uuid1.Validate(), jc.ErrorIsNil)

	c.Check(uuid0, gc.Equals, uuid1)
}

func (s *fileObjectStoreSuite) TestPutCleansUpFileOnMetadataFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// If the file is not referenced by another metadata entry, then the file
	// should be left to be cleaned by the object store later on.

	hash512_384 := s.calculateHexHash512_384(c, "some content")
	hash256 := s.calculateHexHash256(c, "some content")

	s.expectClaim(hash512_384, 1)
	s.expectRelease(hash512_384, 1)

	path := c.MkDir()

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        "foo",
		Size:        12,
	}).Return(uuid, errors.Errorf("boom"))

	_, err := store.Put(context.Background(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, gc.ErrorMatches, `.*boom`)

	s.expectFileDoesExist(c, path, hash512_384)
}

func (s *fileObjectStoreSuite) TestPutDoesNotCleansUpFileOnMetadataFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// The file should not be cleaned up if the file is referenced by another
	// metadata entry. In this case we need to ensure that the file is not
	// cleaned up if the metadata service returns an error.

	hash512_384 := s.calculateHexHash512_384(c, "some content")
	hash256 := s.calculateHexHash256(c, "some content")

	s.expectClaim(hash512_384, 2)
	s.expectRelease(hash512_384, 2)

	path := c.MkDir()

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        "foo",
		Size:        12,
	}).Return(uuid, nil)

	_, err := store.Put(context.Background(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, jc.ErrorIsNil)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        "foo",
		Size:        12,
	}).Return(uuid, errors.Errorf("boom"))

	_, err = store.Put(context.Background(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, gc.ErrorMatches, `.*boom`)

	s.expectFileDoesExist(c, path, hash512_384)
}

func (s *fileObjectStoreSuite) TestPutAndCheckHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hash512_384 := s.calculateHexHash512_384(c, "some content")
	hash256 := s.calculateHexHash256(c, "some content")

	s.expectClaim(hash512_384, 1)
	s.expectRelease(hash512_384, 1)

	path := c.MkDir()

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        "foo",
		Size:        12,
	}).Return(uuid, nil)

	uuid, err := store.PutAndCheckHash(context.Background(), "foo", strings.NewReader("some content"), 12, hash512_384)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(uuid.Validate(), jc.ErrorIsNil)
}

func (s *fileObjectStoreSuite) TestPutAndCheckHashWithInvalidHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hash512_384 := s.calculateHexHash512_384(c, "some content")

	fakeHash := fmt.Sprintf("%s0", hash512_384)

	path := c.MkDir()

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	_, err := store.PutAndCheckHash(context.Background(), "foo", strings.NewReader("some content"), 12, fakeHash)
	c.Assert(err, gc.ErrorMatches, `.*hash mismatch.*`)
}

func (s *fileObjectStoreSuite) TestPutAndCheckHashFileAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hash512_384 := s.calculateHexHash512_384(c, "some content")
	hash256 := s.calculateHexHash256(c, "some content")

	s.expectClaim(hash512_384, 2)
	s.expectRelease(hash512_384, 2)

	path := c.MkDir()

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        "foo",
		Size:        12,
	}).Return(uuid, nil).Times(2)

	uuid0, err := store.PutAndCheckHash(context.Background(), "foo", strings.NewReader("some content"), 12, hash512_384)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(uuid0.Validate(), jc.ErrorIsNil)

	uuid1, err := store.PutAndCheckHash(context.Background(), "foo", strings.NewReader("some content"), 12, hash512_384)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(uuid1.Validate(), jc.ErrorIsNil)

	c.Check(uuid0, gc.Equals, uuid1)
}

func (s *fileObjectStoreSuite) TestPutAndCheckHashCleansUpFileOnMetadataFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// If the file is not referenced by another metadata entry, then the file
	// should be left to cleaned up by the object store later on.

	hash512_384 := s.calculateHexHash512_384(c, "some content")
	hash256 := s.calculateHexHash256(c, "some content")

	s.expectClaim(hash512_384, 1)
	s.expectRelease(hash512_384, 1)

	path := c.MkDir()

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        "foo",
		Size:        12,
	}).Return("", errors.Errorf("boom"))

	_, err := store.PutAndCheckHash(context.Background(), "foo", strings.NewReader("some content"), 12, hash512_384)
	c.Assert(err, gc.ErrorMatches, `.*boom`)

	s.expectFileDoesExist(c, path, hash512_384)
}

func (s *fileObjectStoreSuite) TestPutAndCheckHashDoesNotCleansUpFileOnMetadataFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// The file should not be cleaned up if the file is referenced by another
	// metadata entry. In this case we need to ensure that the file is not
	// cleaned up if the metadata service returns an error.

	hash512_384 := s.calculateHexHash512_384(c, "some content")
	hash256 := s.calculateHexHash256(c, "some content")

	s.expectClaim(hash512_384, 2)
	s.expectRelease(hash512_384, 2)

	path := c.MkDir()

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        "foo",
		Size:        12,
	}).Return("", nil)

	_, err := store.PutAndCheckHash(context.Background(), "foo", strings.NewReader("some content"), 12, hash512_384)
	c.Assert(err, jc.ErrorIsNil)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        "foo",
		Size:        12,
	}).Return("", errors.Errorf("boom"))

	_, err = store.PutAndCheckHash(context.Background(), "foo", strings.NewReader("some content"), 12, hash512_384)
	c.Assert(err, gc.ErrorMatches, `.*boom`)

	s.expectFileDoesExist(c, path, hash512_384)
}

func (s *fileObjectStoreSuite) TestRemoveFileNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Test that we don't return an error if the file does not exist.
	// We just want to ensure that we don't return an error after the metadata
	// is removed.

	s.expectClaim("blah", 1)
	s.expectRelease("blah", 1)

	path := c.MkDir()

	fileName := "foo"

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		Hash512_384: "blah",
		Hash256:     "blah",
		Path:        fileName,
		Size:        666,
	}, nil)

	s.service.EXPECT().RemoveMetadata(gomock.Any(), "foo").Return(nil)

	err := store.Remove(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *fileObjectStoreSuite) TestRemove(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hash512_384 := s.calculateHexHash512_384(c, "some content")
	hash256 := s.calculateHexHash256(c, "some content")

	s.expectClaim(hash512_384, 2)
	s.expectRelease(hash512_384, 2)

	path := c.MkDir()

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        "foo",
		Size:        12,
	}).Return("", nil)

	_, err := store.Put(context.Background(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, jc.ErrorIsNil)

	s.expectFileDoesExist(c, path, hash512_384)

	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        "foo",
		Size:        12,
	}, nil)

	s.service.EXPECT().RemoveMetadata(gomock.Any(), "foo").Return(nil)

	err = store.Remove(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	s.expectFileDoesNotExist(c, path, hash512_384)
}

func (s *fileObjectStoreSuite) TestList(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	namespace := "inferi"
	fileName := "foo"
	size, hash512_384, hash256 := s.createFile(c, s.filePath(path, namespace), fileName, "some content")

	s.createDirectory(c, s.filePath(path, namespace), "other")

	store := s.newFileObjectStore(c, path).(*fileObjectStore)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().ListMetadata(gomock.Any()).Return([]objectstore.Metadata{{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        fileName,
		Size:        size,
	}}, nil)

	metadata, files, err := store.list(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(metadata, gc.DeepEquals, []objectstore.Metadata{{
		Hash512_384: hash512_384,
		Hash256:     hash256,
		Path:        fileName,
		Size:        size,
	}})
	c.Check(files, gc.DeepEquals, []string{hash512_384})
}

func (s *fileObjectStoreSuite) expectFileDoesNotExist(c *gc.C, path, hash string) {
	_, err := os.Stat(filepath.Join(path, defaultFileDirectory, "inferi", hash))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *fileObjectStoreSuite) expectFileDoesExist(c *gc.C, path, hash string) {
	_, err := os.Stat(filepath.Join(path, defaultFileDirectory, "inferi", hash))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *fileObjectStoreSuite) expectClaim(hash string, num int) {
	s.claimer.EXPECT().Claim(gomock.Any(), hash).Return(s.claimExtender, nil).Times(num)
	s.claimExtender.EXPECT().Extend(gomock.Any()).Return(nil).AnyTimes()
	s.claimExtender.EXPECT().Duration().Return(time.Second).AnyTimes()
}

func (s *fileObjectStoreSuite) expectRelease(hash string, num int) {
	s.claimer.EXPECT().Release(gomock.Any(), hash).Return(nil).Times(num)
}

func (s *fileObjectStoreSuite) filePath(path, namespace string) string {
	return filepath.Join(path, defaultFileDirectory, namespace)
}

func (s *fileObjectStoreSuite) newFileObjectStore(c *gc.C, path string) TrackedObjectStore {
	store, err := NewFileObjectStore(FileObjectStoreConfig{
		Namespace:       "inferi",
		RootDir:         path,
		MetadataService: s.service,
		Claimer:         s.claimer,
		Logger:          loggertesting.WrapCheckLog(c),
		Clock:           clock.WallClock,
	})
	c.Assert(err, gc.IsNil)

	return store
}

func (s *fileObjectStoreSuite) createDirectory(c *gc.C, path, name string) {
	err := os.MkdirAll(filepath.Join(path, name), 0755)
	c.Assert(err, jc.ErrorIsNil)
}
