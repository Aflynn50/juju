// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/application/charm"
	internalcharm "github.com/juju/juju/internal/charm"
)

type manifestSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&manifestSuite{})

var manifestTestCases = [...]struct {
	name   string
	input  charm.Manifest
	output internalcharm.Manifest
}{
	{
		name:   "empty",
		input:  charm.Manifest{},
		output: internalcharm.Manifest{},
	},
	{
		name: "full bases",
		input: charm.Manifest{
			Bases: []charm.Base{
				{
					Name: "ubuntu",
					Channel: charm.Channel{
						Track:  "latest",
						Risk:   charm.RiskStable,
						Branch: "foo",
					},
					Architectures: []string{"amd64"},
				},
			},
		},
		output: internalcharm.Manifest{
			Bases: []internalcharm.Base{
				{
					Name: "ubuntu",
					Channel: internalcharm.Channel{
						Track:  "latest",
						Risk:   internalcharm.Stable,
						Branch: "foo",
					},
					Architectures: []string{"amd64"},
				},
			},
		},
	},
}

func (s *manifestSuite) TestConvertManifest(c *gc.C) {
	for _, tc := range manifestTestCases {
		c.Logf("Running test case %q", tc.name)

		result, err := decodeManifest(tc.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, gc.DeepEquals, tc.output)

		// Ensure that the conversion is idempotent.
		converted, warnings, err := encodeManifest(&result)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(converted, jc.DeepEquals, tc.input)
		c.Check(warnings, gc.HasLen, 0)
	}
}

func (s *manifestSuite) TestConvertManifestWarnings(c *gc.C) {
	converted, warnings, err := encodeManifest(&internalcharm.Manifest{
		Bases: []internalcharm.Base{
			{
				Name: "ubuntu",
				Channel: internalcharm.Channel{
					Track:  "latest",
					Risk:   internalcharm.Stable,
					Branch: "foo",
				},
				Architectures: []string{"amd64", "i386", "arm64"},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(converted, jc.DeepEquals, charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Track:  "latest",
					Risk:   charm.RiskStable,
					Branch: "foo",
				},
				Architectures: []string{"amd64", "arm64"},
			},
		},
	})
	c.Check(warnings, gc.DeepEquals, []string{`unsupported architectures: i386 for "ubuntu" with channel: "latest/stable/foo"`})
}
