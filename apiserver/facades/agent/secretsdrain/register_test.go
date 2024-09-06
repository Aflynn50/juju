// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain_test

import (
	"context"

	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/secretsdrain"
	"github.com/juju/juju/apiserver/facades/agent/secretsdrain/mocks"
)

type drainSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&drainSuite{})

func (s *drainSuite) TestNewSecretManagerAPIPermissionCheck(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	authorizer := mocks.NewMockAuthorizer(ctrl)
	authorizer.EXPECT().AuthUnitAgent().Return(false)

	_, err := secretsdrain.NewSecretsDrainAPI(context.Background(), facadetest.ModelContext{
		Auth_: authorizer,
	})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}
