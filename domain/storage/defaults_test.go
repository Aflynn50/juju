// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/charm/v13"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/storage"
)

type defaultsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&defaultsSuite{})

func makeStorageDefaults(b, f string) domainstorage.StorageDefaults {
	var result domainstorage.StorageDefaults
	if b != "" {
		result.DefaultBlockSource = &b
	}
	if f != "" {
		result.DefaultFilesystemSource = &f
	}
	return result
}

func (s *defaultsSuite) assertAddApplicationStorageDirectivesDefaults(c *gc.C, pool string, cons, expect map[string]storage.Directive) {
	err := domainstorage.StorageDirectivesWithDefaults(
		map[string]charm.Storage{
			"data":    {Name: "data", Type: charm.StorageBlock, CountMin: 1, CountMax: -1},
			"allecto": {Name: "allecto", Type: charm.StorageBlock, CountMin: 0, CountMax: -1},
		},
		coremodel.IAAS,
		makeStorageDefaults(pool, ""),
		cons,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, expect)
}

func (s *defaultsSuite) TestAddApplicationStorageDirectivesNoConstraintsUsed(c *gc.C) {
	storageCons := map[string]storage.Directive{
		"data": makeStorageDirective("", 0, 0),
	}
	expectedCons := map[string]storage.Directive{
		"data":    makeStorageDirective("loop", 1024, 1),
		"allecto": makeStorageDirective("loop", 1024, 0),
	}
	s.assertAddApplicationStorageDirectivesDefaults(c, "loop-pool", storageCons, expectedCons)
}

func (s *defaultsSuite) TestAddApplicationStorageDirectivesJustCount(c *gc.C) {
	storageCons := map[string]storage.Directive{
		"data": makeStorageDirective("", 0, 1),
	}
	expectedCons := map[string]storage.Directive{
		"data":    makeStorageDirective("loop-pool", 1024, 1),
		"allecto": makeStorageDirective("loop", 1024, 0),
	}
	s.assertAddApplicationStorageDirectivesDefaults(c, "loop-pool", storageCons, expectedCons)
}

func (s *defaultsSuite) TestAddApplicationStorageDirectivesDefaultPool(c *gc.C) {
	storageCons := map[string]storage.Directive{
		"data": makeStorageDirective("", 2048, 1),
	}
	expectedCons := map[string]storage.Directive{
		"data":    makeStorageDirective("loop-pool", 2048, 1),
		"allecto": makeStorageDirective("loop", 1024, 0),
	}
	s.assertAddApplicationStorageDirectivesDefaults(c, "loop-pool", storageCons, expectedCons)
}

func (s *defaultsSuite) TestAddApplicationStorageDirectivesConstraintPool(c *gc.C) {
	storageCons := map[string]storage.Directive{
		"data": makeStorageDirective("loop-pool", 2048, 1),
	}
	expectedCons := map[string]storage.Directive{
		"data":    makeStorageDirective("loop-pool", 2048, 1),
		"allecto": makeStorageDirective("loop", 1024, 0),
	}
	s.assertAddApplicationStorageDirectivesDefaults(c, "", storageCons, expectedCons)
}

func (s *defaultsSuite) TestAddApplicationStorageDirectivesNoUserDefaultPool(c *gc.C) {
	storageCons := map[string]storage.Directive{
		"data": makeStorageDirective("", 2048, 1),
	}
	expectedCons := map[string]storage.Directive{
		"data":    makeStorageDirective("loop", 2048, 1),
		"allecto": makeStorageDirective("loop", 1024, 0),
	}
	s.assertAddApplicationStorageDirectivesDefaults(c, "", storageCons, expectedCons)
}

func (s *defaultsSuite) TestAddApplicationStorageDirectivesDefaultSizeFallback(c *gc.C) {
	storageCons := map[string]storage.Directive{
		"data": makeStorageDirective("loop-pool", 0, 1),
	}
	expectedCons := map[string]storage.Directive{
		"data":    makeStorageDirective("loop-pool", 1024, 1),
		"allecto": makeStorageDirective("loop", 1024, 0),
	}
	s.assertAddApplicationStorageDirectivesDefaults(c, "loop-pool", storageCons, expectedCons)
}

func (s *defaultsSuite) TestAddApplicationStorageDirectivesDefaultSizeFromCharm(c *gc.C) {
	storageCons := map[string]storage.Directive{
		"multi1to10": makeStorageDirective("loop", 0, 3),
	}
	expectedCons := map[string]storage.Directive{
		"multi1to10": makeStorageDirective("loop", 1024, 3),
		"multi2up":   makeStorageDirective("loop", 2048, 2),
	}
	err := domainstorage.StorageDirectivesWithDefaults(
		map[string]charm.Storage{
			"multi1to10": {Name: "multi1to10", Type: charm.StorageBlock, CountMin: 1, CountMax: 10},
			"multi2up":   {Name: "multi2up", Type: charm.StorageBlock, CountMin: 2, CountMax: -1, MinimumSize: 2 * 1024},
		},
		coremodel.IAAS,
		makeStorageDefaults("", ""),
		storageCons,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageCons, jc.DeepEquals, expectedCons)
}

func (s *defaultsSuite) TestProviderFallbackToType(c *gc.C) {
	storageCons := map[string]storage.Directive{}
	expectedCons := map[string]storage.Directive{
		"data":  makeStorageDirective("loop", 1024, 1),
		"files": makeStorageDirective("rootfs", 1024, 1),
	}
	err := domainstorage.StorageDirectivesWithDefaults(
		map[string]charm.Storage{
			"data":  {Name: "data", Type: charm.StorageBlock, CountMin: 1, CountMax: 1},
			"files": {Name: "files", Type: charm.StorageFilesystem, CountMin: 1, CountMax: 1},
		},
		coremodel.IAAS,
		makeStorageDefaults("", ""),
		storageCons,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageCons, jc.DeepEquals, expectedCons)
}

func (s *defaultsSuite) TestProviderFallbackToTypeCaas(c *gc.C) {
	storageCons := map[string]storage.Directive{}
	expectedCons := map[string]storage.Directive{
		"files": makeStorageDirective("kubernetes", 1024, 1),
	}
	err := domainstorage.StorageDirectivesWithDefaults(
		map[string]charm.Storage{
			"files": {Name: "files", Type: charm.StorageFilesystem, CountMin: 1, CountMax: 1},
		},
		coremodel.CAAS,
		makeStorageDefaults("", ""),
		storageCons,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageCons, jc.DeepEquals, expectedCons)
}

func (s *defaultsSuite) TestProviderFallbackToTypeWithoutConstraints(c *gc.C) {
	storageCons := map[string]storage.Directive{}
	expectedCons := map[string]storage.Directive{
		"data":  makeStorageDirective("loop", 1024, 1),
		"files": makeStorageDirective("rootfs", 1024, 1),
	}
	err := domainstorage.StorageDirectivesWithDefaults(
		map[string]charm.Storage{
			"data":  {Name: "data", Type: charm.StorageBlock, CountMin: 1, CountMax: 1},
			"files": {Name: "files", Type: charm.StorageFilesystem, CountMin: 1, CountMax: 1},
		},
		coremodel.IAAS,
		makeStorageDefaults("ebs", "tmpfs"),
		storageCons,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageCons, jc.DeepEquals, expectedCons)
}

func (s *defaultsSuite) TestProviderFallbackToTypeWithoutConstraintsCaas(c *gc.C) {
	storageCons := map[string]storage.Directive{}
	expectedCons := map[string]storage.Directive{
		"files": makeStorageDirective("kubernetes", 1024, 1),
	}
	err := domainstorage.StorageDirectivesWithDefaults(
		map[string]charm.Storage{
			"files": {Name: "files", Type: charm.StorageFilesystem, CountMin: 1, CountMax: 1},
		},
		coremodel.CAAS,
		makeStorageDefaults("", "tmpfs"),
		storageCons,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageCons, jc.DeepEquals, expectedCons)
}

func (s *defaultsSuite) TestProviderFallbackToDefaults(c *gc.C) {
	storageCons := map[string]storage.Directive{
		"data":  makeStorageDirective("", 2048, 1),
		"files": makeStorageDirective("", 4096, 2),
	}
	expectedCons := map[string]storage.Directive{
		"data":  makeStorageDirective("ebs", 2048, 1),
		"files": makeStorageDirective("tmpfs", 4096, 2),
	}
	err := domainstorage.StorageDirectivesWithDefaults(
		map[string]charm.Storage{
			"data":  {Name: "data", Type: charm.StorageBlock, CountMin: 1, CountMax: 2},
			"files": {Name: "files", Type: charm.StorageFilesystem, CountMin: 1, CountMax: 2},
		},
		coremodel.IAAS,
		makeStorageDefaults("ebs", "tmpfs"),
		storageCons,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageCons, jc.DeepEquals, expectedCons)
}

func (s *defaultsSuite) TestProviderFallbackToDefaultsCaas(c *gc.C) {
	storageCons := map[string]storage.Directive{
		"files": makeStorageDirective("", 4096, 2),
	}
	expectedCons := map[string]storage.Directive{
		"files": makeStorageDirective("tmpfs", 4096, 2),
	}
	err := domainstorage.StorageDirectivesWithDefaults(
		map[string]charm.Storage{
			"files": {Name: "files", Type: charm.StorageFilesystem, CountMin: 1, CountMax: 2},
		},
		coremodel.CAAS,
		makeStorageDefaults("", "tmpfs"),
		storageCons,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageCons, jc.DeepEquals, expectedCons)
}
