// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/storage/provider"
	coretesting "github.com/juju/juju/testing"
)

type upgradesSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&upgradesSuite{})

//nolint:unused
type expectUpgradedData struct {
	coll     *mgo.Collection
	expected []bson.M
	filter   bson.D
}

//nolint:unused
func upgradedData(coll *mgo.Collection, expected []bson.M) expectUpgradedData {
	return expectUpgradedData{
		coll:     coll,
		expected: expected,
	}
}

//nolint:unused
func (s *upgradesSuite) assertUpgradedData(c *gc.C, upgrade func(*StatePool) error, expect ...expectUpgradedData) {
	// Two rounds to check idempotency.
	for i := 0; i < 2; i++ {
		c.Logf("Run: %d", i)
		err := upgrade(s.pool)
		c.Assert(err, jc.ErrorIsNil)

		for _, expect := range expect {
			var docs []bson.M
			err = expect.coll.Find(expect.filter).Sort("_id").All(&docs)
			c.Assert(err, jc.ErrorIsNil)
			for i, d := range docs {
				doc := d
				delete(doc, "txn-queue")
				delete(doc, "txn-revno")
				delete(doc, "version")
				docs[i] = doc
			}
			c.Assert(docs, jc.DeepEquals, expect.expected,
				gc.Commentf("differences: %s", pretty.Diff(docs, expect.expected)))
		}
	}
}

//nolint:unused
func (s *upgradesSuite) makeModel(c *gc.C, name string, attr coretesting.Attrs) *State {
	uuid := utils.MustNewUUID()
	cfg := coretesting.CustomModelConfig(c, coretesting.Attrs{
		"name": name,
		"uuid": uuid.String(),
	}.Merge(attr))
	m, err := s.state.Model()
	c.Assert(err, jc.ErrorIsNil)
	_, st, err := s.controller.NewModel(ModelArgs{
		Type:                    ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   m.Owner(),
		StorageProviderRegistry: provider.CommonStorageProviders(),
	})
	c.Assert(err, jc.ErrorIsNil)
	return st
}

type bsonMById []bson.M

func (x bsonMById) Len() int { return len(x) }

func (x bsonMById) Swap(i, j int) { x[i], x[j] = x[j], x[i] }

func (x bsonMById) Less(i, j int) bool {
	return x[i]["_id"].(string) < x[j]["_id"].(string)
}

func (s *upgradesSuite) TestEnsureInitalRefCountForExternalSecretBackends(c *gc.C) {
	backendStore := NewSecretBackends(s.state)
	_, err := backendStore.CreateSecretBackend(CreateSecretBackendParams{
		ID:          "backend-id-1",
		Name:        "foo",
		BackendType: "vault",
	})
	c.Assert(err, jc.ErrorIsNil)
	backendRefCount, err := s.state.ReadBackendRefCount("backend-id-1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(backendRefCount, gc.Equals, 0)

	_, err = backendStore.CreateSecretBackend(CreateSecretBackendParams{
		ID:          "backend-id-2",
		Name:        "bar",
		BackendType: "vault",
	})
	c.Assert(err, jc.ErrorIsNil)
	ops, err := s.state.incBackendRevisionCountOps("backend-id-2", 3)
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.db().RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
	backendRefCount, err = s.state.ReadBackendRefCount("backend-id-2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(backendRefCount, gc.Equals, 3)

	ops, err = s.state.removeBackendRefCountOp("backend-id-1", true)
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.db().RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.state.ReadBackendRefCount("backend-id-1")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	expected := bsonMById{
		{
			// created by EnsureInitalRefCountForExternalSecretBackends
			"_id":      secretBackendRefCountKey("backend-id-1"),
			"refcount": 0,
		},
		{
			// no touch existing records.
			"_id":      secretBackendRefCountKey("backend-id-2"),
			"refcount": 3,
		},
	}
	sort.Sort(expected)

	refCountCollection, closer := s.state.db().GetRawCollection(globalRefcountsC)
	defer closer()

	expectedData := upgradedData(refCountCollection, expected)
	expectedData.filter = bson.D{{"_id", bson.M{"$regex": "secretbackend#revisions#.*"}}}
	s.assertUpgradedData(c, EnsureInitalRefCountForExternalSecretBackends, expectedData)
}
