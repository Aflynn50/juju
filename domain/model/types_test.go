// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/instance"
	modeltesting "github.com/juju/juju/core/model/testing"
	usertesting "github.com/juju/juju/core/user/testing"
)

type typesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&typesSuite{})

// ptr returns a reference to a copied value of type T.
func ptr[T any](i T) *T {
	return &i
}

// TestModelCreationArgsValidation is aserting all the validation cases that the
// [GlobalModelCreationArgs.Validate] function checks for.
func (*typesSuite) TestModelCreationArgsValidation(c *gc.C) {
	userUUID := usertesting.GenUserUUID(c)

	tests := []struct {
		Args    GlobalModelCreationArgs
		Name    string
		ErrTest error
	}{
		{
			Name: "Test invalid name",
			Args: GlobalModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "",
				Owner:       userUUID,
			},
			ErrTest: errors.NotValid,
		},
		{
			Name: "Test invalid owner",
			Args: GlobalModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       "",
			},
			ErrTest: errors.NotValid,
		},
		{
			Name: "Test invalid cloud",
			Args: GlobalModelCreationArgs{
				Cloud:       "",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       userUUID,
			},
			ErrTest: errors.NotValid,
		},
		{
			Name: "Test invalid cloud region",
			Args: GlobalModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "",
				Name:        "my-awesome-model",
				Owner:       userUUID,
			},
			ErrTest: nil,
		},
		{
			Name: "Test invalid credential key",
			Args: GlobalModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Credential: credential.Key{
					Owner: usertesting.GenNewName(c, "wallyworld"),
				},
				Name:  "my-awesome-model",
				Owner: userUUID,
			},
			ErrTest: errors.NotValid,
		},
		{
			Name: "Test happy path without credential key",
			Args: GlobalModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Owner:       userUUID,
			},
			ErrTest: nil,
		},
		{
			Name: "Test happy path with credential key",
			Args: GlobalModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Credential: credential.Key{
					Cloud: "cloud",
					Owner: usertesting.GenNewName(c, "wallyworld"),
					Name:  "mycred",
				},
				Name:  "my-awesome-model",
				Owner: userUUID,
			},
			ErrTest: nil,
		},
	}

	for i, test := range tests {
		c.Logf("testing %q: %d %v", test.Name, i, test.Args)

		err := test.Args.Validate()
		if test.ErrTest == nil {
			c.Check(err, jc.ErrorIsNil, gc.Commentf("%s", test.Name))
		} else {
			c.Check(err, jc.ErrorIs, test.ErrTest, gc.Commentf("%s", test.Name))
		}
	}
}

// TestModelImportArgsValidation is aserting all the validation cases that the
// [ModelImportArgs.Validate] function checks for.
func (*typesSuite) TestModelImportArgsValidation(c *gc.C) {
	userUUID := usertesting.GenUserUUID(c)

	tests := []struct {
		Args    ModelImportArgs
		Name    string
		ErrTest error
	}{
		{
			Name: "Test happy path with valid model id",
			Args: ModelImportArgs{
				GlobalModelCreationArgs: GlobalModelCreationArgs{
					Cloud:       "my-cloud",
					CloudRegion: "my-region",
					Credential: credential.Key{
						Cloud: "cloud",
						Owner: usertesting.GenNewName(c, "wallyworld"),
						Name:  "mycred",
					},
					Name:  "my-awesome-model",
					Owner: userUUID,
				},
				ID: modeltesting.GenModelUUID(c),
			},
		},
		{
			Name: "Test invalid model id",
			Args: ModelImportArgs{
				GlobalModelCreationArgs: GlobalModelCreationArgs{
					Cloud:       "my-cloud",
					CloudRegion: "my-region",
					Credential: credential.Key{
						Cloud: "cloud",
						Owner: usertesting.GenNewName(c, "wallyworld"),
						Name:  "mycred",
					},
					Name:  "my-awesome-model",
					Owner: userUUID,
				},
				ID: "not valid",
			},
			ErrTest: errors.NotValid,
		},
	}

	for i, test := range tests {
		c.Logf("testing %q: %d %v", test.Name, i, test.Args)

		err := test.Args.Validate()
		if test.ErrTest == nil {
			c.Check(err, jc.ErrorIsNil, gc.Commentf("%s", test.Name))
		} else {
			c.Check(err, jc.ErrorIs, test.ErrTest, gc.Commentf("%s", test.Name))
		}
	}
}

// TestFromCoreConstraints is concerned with testing the mapping from a
// [constraints.Value] to a [Constraints] object. Specifically the main thing we
// care about in this test is that spaces are either included or excluded
// correctly and that the rest of the values are set verbatim.
func (*typesSuite) TestFromCoreConstraints(c *gc.C) {
	tests := []struct {
		Comment string
		In      constraints.Value
		Out     Constraints
	}{
		{
			Comment: "Test every value get's set as described",
			In: constraints.Value{
				Arch:             ptr("test"),
				Container:        ptr(instance.LXD),
				CpuCores:         ptr(uint64(1)),
				CpuPower:         ptr(uint64(1)),
				Mem:              ptr(uint64(1024)),
				RootDisk:         ptr(uint64(100)),
				RootDiskSource:   ptr("source"),
				Tags:             ptr([]string{"tag1", "tag2"}),
				InstanceRole:     ptr("instance-role"),
				InstanceType:     ptr("instance-type"),
				VirtType:         ptr("kvm"),
				Zones:            ptr([]string{"zone1", "zone2"}),
				AllocatePublicIP: ptr(true),
				ImageID:          ptr("image-123"),
				Spaces:           ptr([]string{"space1", "space2", "^space3"}),
			},
			Out: Constraints{
				Arch:             ptr("test"),
				Container:        ptr(instance.LXD),
				CpuCores:         ptr(uint64(1)),
				CpuPower:         ptr(uint64(1)),
				Mem:              ptr(uint64(1024)),
				RootDisk:         ptr(uint64(100)),
				RootDiskSource:   ptr("source"),
				Tags:             ptr([]string{"tag1", "tag2"}),
				InstanceRole:     ptr("instance-role"),
				InstanceType:     ptr("instance-type"),
				VirtType:         ptr("kvm"),
				Zones:            ptr([]string{"zone1", "zone2"}),
				AllocatePublicIP: ptr(true),
				ImageID:          ptr("image-123"),
				Spaces: ptr([]SpaceConstraint{
					{SpaceName: "space1", Exclude: false},
					{SpaceName: "space2", Exclude: false},
					{SpaceName: "space3", Exclude: true},
				}),
			},
		},
		{
			Comment: "Test only excluded spaces",
			In: constraints.Value{
				Arch:   ptr("test"),
				Spaces: ptr([]string{"^space3"}),
			},
			Out: Constraints{
				Arch: ptr("test"),
				Spaces: ptr([]SpaceConstraint{
					{SpaceName: "space3", Exclude: true},
				}),
			},
		},
		{
			Comment: "Test only included spaces",
			In: constraints.Value{
				Arch:   ptr("test"),
				Spaces: ptr([]string{"space3"}),
			},
			Out: Constraints{
				Arch: ptr("test"),
				Spaces: ptr([]SpaceConstraint{
					{SpaceName: "space3", Exclude: false},
				}),
			},
		},
		{
			Comment: "Test no spaces",
			In: constraints.Value{
				Arch: ptr("test"),
			},
			Out: Constraints{
				Arch: ptr("test"),
			},
		},
	}

	for _, test := range tests {
		rval := FromCoreConstraints(test.In)
		c.Check(rval, jc.DeepEquals, test.Out, gc.Commentf(test.Comment))
	}
}

// TestToCoreConstraints is concerned with testing the mapping from a
// [Constraints] object to a [constraints.Value]. Specifically the main thing we
// care about in this test is that spaces are either included or excluded
// correctly and that the rest of the values are set verbatim.
func (*typesSuite) TestToCoreConstraints(c *gc.C) {
	tests := []struct {
		Comment string
		Out     constraints.Value
		In      Constraints
	}{
		{
			Comment: "Test every value get's set as described",
			In: Constraints{
				Arch:             ptr("test"),
				Container:        ptr(instance.LXD),
				CpuCores:         ptr(uint64(1)),
				CpuPower:         ptr(uint64(1)),
				Mem:              ptr(uint64(1024)),
				RootDisk:         ptr(uint64(100)),
				RootDiskSource:   ptr("source"),
				Tags:             ptr([]string{"tag1", "tag2"}),
				InstanceRole:     ptr("instance-role"),
				InstanceType:     ptr("instance-type"),
				VirtType:         ptr("kvm"),
				Zones:            ptr([]string{"zone1", "zone2"}),
				AllocatePublicIP: ptr(true),
				ImageID:          ptr("image-123"),
				Spaces: ptr([]SpaceConstraint{
					{SpaceName: "space1", Exclude: false},
					{SpaceName: "space2", Exclude: false},
					{SpaceName: "space3", Exclude: true},
				}),
			},
			Out: constraints.Value{
				Arch:             ptr("test"),
				Container:        ptr(instance.LXD),
				CpuCores:         ptr(uint64(1)),
				CpuPower:         ptr(uint64(1)),
				Mem:              ptr(uint64(1024)),
				RootDisk:         ptr(uint64(100)),
				RootDiskSource:   ptr("source"),
				Tags:             ptr([]string{"tag1", "tag2"}),
				InstanceRole:     ptr("instance-role"),
				InstanceType:     ptr("instance-type"),
				VirtType:         ptr("kvm"),
				Zones:            ptr([]string{"zone1", "zone2"}),
				AllocatePublicIP: ptr(true),
				ImageID:          ptr("image-123"),
				Spaces:           ptr([]string{"space1", "space2", "^space3"}),
			},
		},
		{
			Comment: "Test only excluded spaces",
			In: Constraints{
				Arch: ptr("test"),
				Spaces: ptr([]SpaceConstraint{
					{SpaceName: "space3", Exclude: true},
				}),
			},
			Out: constraints.Value{
				Arch:   ptr("test"),
				Spaces: ptr([]string{"^space3"}),
			},
		},
		{
			Comment: "Test only included spaces",
			In: Constraints{
				Arch: ptr("test"),
				Spaces: ptr([]SpaceConstraint{
					{SpaceName: "space3", Exclude: false},
				}),
			},
			Out: constraints.Value{
				Arch:   ptr("test"),
				Spaces: ptr([]string{"space3"}),
			},
		},
		{
			Comment: "Test no spaces",
			In: Constraints{
				Arch: ptr("test"),
			},
			Out: constraints.Value{
				Arch: ptr("test"),
			},
		},
	}

	for _, test := range tests {
		rval := ToCoreConstraints(test.In)
		c.Check(rval, jc.DeepEquals, test.Out, gc.Commentf(test.Comment))
	}
}
