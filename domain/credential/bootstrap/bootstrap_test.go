// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	userstate "github.com/juju/juju/domain/access/state"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite

	controllerUUID string
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.controllerUUID = s.SeedControllerUUID(c)
}

func (s *bootstrapSuite) TestInsertInitialControllerConfig(c *gc.C) {
	ctx := context.Background()

	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	userState := userstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = userState.AddUser(
		context.Background(), userUUID,
		"fred",
		"test user",
		userUUID,
		permission.ControllerForAccess(permission.SuperuserAccess, s.controllerUUID),
	)
	c.Assert(err, jc.ErrorIsNil)

	cld := cloud.Cloud{Name: "cirrus", Type: "ec2", AuthTypes: cloud.AuthTypes{cloud.UserPassAuthType}}
	err = cloudbootstrap.InsertCloud("fred", cld)(ctx, s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	cred := cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"foo": "bar"}, false)

	key := credential.Key{
		Cloud: "cirrus",
		Owner: "fred",
		Name:  "foo",
	}

	err = InsertCredential(key, cred)(ctx, s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	var owner, cloudName string
	row := s.DB().QueryRow(`
SELECT owner_uuid, cloud.name FROM cloud_credential
JOIN cloud ON cloud.uuid = cloud_credential.cloud_uuid
WHERE cloud_credential.name = ?`, "foo")
	c.Assert(row.Scan(&owner, &cloudName), jc.ErrorIsNil)
	c.Assert(owner, gc.Equals, userUUID.String())
	c.Assert(cloudName, gc.Equals, "cirrus")
}
