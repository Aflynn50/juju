// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/instance"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	accessstate "github.com/juju/juju/domain/access/state"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	"github.com/juju/juju/domain/constraints"
	credentialbootstrap "github.com/juju/juju/domain/credential/bootstrap"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modelstate "github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/domain/model/state/testing"
	networkerrors "github.com/juju/juju/domain/network/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type baseSuite struct {
	schematesting.ControllerSuite

	adminUserUUID  coreuser.UUID
	cloudName      string
	credentialName string
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)

	controllerUUID := s.SeedControllerUUID(c)

	var err error
	s.adminUserUUID, err = coreuser.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	accessState := accessstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = accessState.AddUserWithPermission(
		context.Background(), s.adminUserUUID,
		coreuser.AdminUserName,
		coreuser.AdminUserName.Name(),
		false,
		s.adminUserUUID,
		permission.AccessSpec{
			Access: permission.SuperuserAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        controllerUUID,
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	s.cloudName = "test"
	fn := cloudbootstrap.InsertCloud(coreuser.AdminUserName, cloud.Cloud{
		Name:      s.cloudName,
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType},
	})

	err = fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	s.credentialName = "test"
	fn = credentialbootstrap.InsertCredential(
		credential.Key{
			Cloud: s.cloudName,
			Name:  s.credentialName,
			Owner: coreuser.AdminUserName,
		},
		cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
			"access-key": "val",
		}),
	)

	err = fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	testing.CreateInternalSecretBackend(c, s.ControllerTxnRunner())
}

type bootstrapSuite struct {
	baseSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestUUIDIsRespected(c *gc.C) {
	fn := CreateGlobalModelRecord(
		modeltesting.GenModelUUID(c),
		model.GlobalModelCreationArgs{
			Cloud: s.cloudName,
			Credential: credential.Key{
				Cloud: s.cloudName,
				Name:  s.credentialName,
				Owner: coreuser.AdminUserName,
			},
			Name:  "test",
			Owner: s.adminUserUUID,
		})

	err := fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
}

type modelBootstrapSuite struct {
	baseSuite
	schematesting.ModelSuite
}

var _ = gc.Suite(&modelBootstrapSuite{})

func (s *modelBootstrapSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.ModelSuite.SetUpTest(c)
}

func (s *modelBootstrapSuite) TestCreateModelDetails(c *gc.C) {
	controllerUUID := uuid.MustNewUUID()
	modelUUID := modeltesting.GenModelUUID(c)

	args := model.GlobalModelCreationArgs{
		Cloud: s.cloudName,
		Credential: credential.Key{
			Cloud: s.cloudName,
			Name:  s.credentialName,
			Owner: coreuser.AdminUserName,
		},
		Name:  "test",
		Owner: s.adminUserUUID,
	}

	// Create a model and then create a read-only model from it.
	fn := CreateGlobalModelRecord(modelUUID, args)
	err := fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	fn = CreateLocalModelRecord(modelUUID, controllerUUID, jujuversion.Current)
	err = fn(context.Background(), s.ControllerTxnRunner(), s.ModelTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	m := dbReadOnlyModel{}
	stmt, err := sqlair.Prepare(`SELECT &dbReadOnlyModel.* FROM model`, m)
	c.Assert(err, jc.ErrorIsNil)

	err = s.ModelTxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).Get(&m)
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(m.UUID, gc.Equals, modelUUID.String())
	c.Check(m.ControllerUUID, gc.Equals, controllerUUID.String())
	c.Check(m.Name, gc.Equals, args.Name)
	c.Check(m.IsControllerModel, gc.Equals, true)

	v := sqlair.M{}
	stmt, err = sqlair.Prepare(`SELECT &M.target_version FROM agent_version`, v)
	c.Assert(err, jc.ErrorIsNil)

	err = s.ModelTxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).Get(&v)
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(v["target_version"], gc.DeepEquals, jujuversion.Current.String())
}

// TestCreateModelUnsupportedCredential is asserting the fact that if we supply
// an empty credential to the model creation process and this type of credential
// isn't supported by the cloud then an error satisfying
// [modelerrors.CredentialNotValid] is returned.
func (s *modelBootstrapSuite) TestCreateModelUnsupportedCredential(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)

	fn := cloudbootstrap.InsertCloud(coreuser.AdminUserName, cloud.Cloud{
		Name:      "test-cloud",
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType},
	})
	err := fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	args := model.GlobalModelCreationArgs{
		// We assume here that the cloud made behind s.cloudName
		Cloud:      "test-cloud",
		Credential: credential.Key{},
		Name:       "test",
		Owner:      s.adminUserUUID,
	}

	// Create a model and then create a read-only model from it.
	fn = CreateGlobalModelRecord(modelUUID, args)
	err = fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Check(err, jc.ErrorIs, modelerrors.CredentialNotValid)
}

// TestCreateModelWithEmptyCredential is asserting that we can create models
// with empty cloud credentials when the cloud supports it.
func (s *modelBootstrapSuite) TestCreateModelWithEmptyCredential(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)

	fn := cloudbootstrap.InsertCloud(coreuser.AdminUserName, cloud.Cloud{
		Name:      "test-cloud",
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType},
	})
	err := fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	args := model.GlobalModelCreationArgs{
		// We assume here that the cloud made behind s.cloudName
		Cloud:      "test-cloud",
		Credential: credential.Key{},
		Name:       "test",
		Owner:      s.adminUserUUID,
	}

	fn = CreateGlobalModelRecord(modelUUID, args)
	err = fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Check(err, jc.ErrorIsNil)
}

type dbReadOnlyModel struct {
	UUID              string `db:"uuid"`
	ControllerUUID    string `db:"controller_uuid"`
	Name              string `db:"name"`
	Type              string `db:"type"`
	Cloud             string `db:"cloud"`
	CloudType         string `db:"cloud_type"`
	CloudRegion       string `db:"cloud_region"`
	CredentialOwner   string `db:"credential_owner"`
	CredentialName    string `db:"credential_name"`
	IsControllerModel bool   `db:"is_controller_model"`
}

func (s *modelBootstrapSuite) TestSetModelConstraints(c *gc.C) {
	controllerUUID := uuid.MustNewUUID()
	modelUUID := modeltesting.GenModelUUID(c)

	args := model.GlobalModelCreationArgs{
		Cloud: s.cloudName,
		Credential: credential.Key{
			Cloud: s.cloudName,
			Name:  s.credentialName,
			Owner: coreuser.AdminUserName,
		},
		Name:  "test",
		Owner: s.adminUserUUID,
	}

	// Create a model and then create a read-only model from it.
	fn := CreateGlobalModelRecord(modelUUID, args)
	err := fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	fn = CreateLocalModelRecord(modelUUID, controllerUUID, jujuversion.Current)
	err = fn(context.Background(), s.ControllerTxnRunner(), s.ModelTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	cons := coreconstraints.Value{
		Arch:      ptr("amd64"),
		Container: ptr(instance.LXD),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
	}
	fn = SetModelConstraints(cons)
	err = fn(context.Background(), s.ControllerTxnRunner(), s.ModelTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	modelState := modelstate.NewModelState(func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}, loggertesting.WrapCheckLog(c))

	expected := constraints.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.LXD),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
	}

	data, err := modelState.GetModelConstraints(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(data, jc.DeepEquals, expected)
}

// TestSetModelConstraintsFailedModelNotFound is asserting that if we set model
// constraints and the model does not exist we get back an error satisfying
// [modelerrors.NotFound].
func (s *modelBootstrapSuite) TestSetModelConstraintFailedModelNotFound(c *gc.C) {
	state := modelstate.NewModelState(func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}, loggertesting.WrapCheckLog(c))

	err := state.SetModelConstraints(context.Background(), constraints.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
	})
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestSetModelConstraintsInvalidContainerType asserts that if we set model
// constraints with an unknown/invalid container type we get back an error
// satisfying [machineerrors.InvalidContainerType] and no changes are made to
// the database.
func (s *modelBootstrapSuite) TestSetModelConstraintsInvalidContainerType(c *gc.C) {
	controllerUUID := uuid.MustNewUUID()
	modelUUID := modeltesting.GenModelUUID(c)

	args := model.GlobalModelCreationArgs{
		Cloud: s.cloudName,
		Credential: credential.Key{
			Cloud: s.cloudName,
			Name:  s.credentialName,
			Owner: coreuser.AdminUserName,
		},
		Name:  "test",
		Owner: s.adminUserUUID,
	}

	// Create a model and then create a read-only model from it.
	fn := CreateGlobalModelRecord(modelUUID, args)
	err := fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	fn = CreateLocalModelRecord(modelUUID, controllerUUID, jujuversion.Current)
	err = fn(context.Background(), s.ControllerTxnRunner(), s.ModelTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	state := modelstate.NewModelState(func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}, loggertesting.WrapCheckLog(c))

	cons := constraints.Constraints{
		Container: ptr(instance.ContainerType("noexist")),
		ImageID:   ptr("image-id"),
	}

	err = state.SetModelConstraints(context.Background(), cons)
	c.Check(err, jc.ErrorIs, machineerrors.InvalidContainerType)

	_, err = state.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.ConstraintsNotFound)
}

// TestSetModelConstraintFailedSpaceDoesNotExist asserts that if we set model
// constraints for a space that doesn't exist we get back an error satisfying
// [networkerrors.SpaceNotFound] and that no changes are made to the database.
func (s *modelBootstrapSuite) TestSetModelConstraintFailedSpaceDoesNotExist(c *gc.C) {
	controllerUUID := uuid.MustNewUUID()
	modelUUID := modeltesting.GenModelUUID(c)

	args := model.GlobalModelCreationArgs{
		Cloud: s.cloudName,
		Credential: credential.Key{
			Cloud: s.cloudName,
			Name:  s.credentialName,
			Owner: coreuser.AdminUserName,
		},
		Name:  "test",
		Owner: s.adminUserUUID,
	}

	// Create a model and then create a read-only model from it.
	fn := CreateGlobalModelRecord(modelUUID, args)
	err := fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	fn = CreateLocalModelRecord(modelUUID, controllerUUID, jujuversion.Current)
	err = fn(context.Background(), s.ControllerTxnRunner(), s.ModelTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	state := modelstate.NewModelState(func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}, loggertesting.WrapCheckLog(c))

	err = state.SetModelConstraints(context.Background(), constraints.Constraints{
		Spaces: ptr([]constraints.SpaceConstraint{
			{
				SpaceName: "space1",
				Exclude:   false,
			},
		}),
		ImageID: ptr("image-id"),
	})
	c.Check(err, jc.ErrorIs, networkerrors.SpaceNotFound)

	_, err = state.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.ConstraintsNotFound)
}

func ptr[T any](s T) *T {
	return &s
}
