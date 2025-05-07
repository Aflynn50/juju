// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package assumes

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
)

type FeatureSetSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&FeatureSetSuite{})

func (s *FeatureSetSuite) TestAsList(c *tc.C) {
	var fs FeatureSet
	fs.Add(
		Feature{Name: "zzz"},
		Feature{Name: "abc"},
		Feature{Name: "efg"},
	)

	exp := []Feature{
		{Name: "abc"},
		{Name: "efg"},
		{Name: "zzz"},
	}

	c.Assert(fs.AsList(), tc.DeepEquals, exp, tc.Commentf("expected AsList() to return the feature list sorted by name"))
}

func (s *SatCheckerSuite) TestMerge(c *tc.C) {
	var fs1 FeatureSet
	fs1.Add(
		Feature{Name: "zzz"},
		Feature{Name: "efg"},
	)

	var fs2 FeatureSet
	fs2.Add(
		Feature{Name: "abc"},
		Feature{Name: "efg"},
	)

	exp := []Feature{
		{Name: "abc"},
		{Name: "efg"},
		{Name: "zzz"},
	}

	fs1.Merge(fs2)

	c.Assert(fs1.AsList(), tc.DeepEquals, exp, tc.Commentf("expected AsList() to return the union of the two sets with duplicates removed"))
}

func (s *SatCheckerSuite) TestGet(c *tc.C) {
	var fs FeatureSet
	fs.Add(
		Feature{Name: "zzz"},
	)

	_, found := fs.Get("zzz")
	c.Assert(found, jc.IsTrue)

	_, found = fs.Get("bogus!")
	c.Assert(found, jc.IsFalse)
}
