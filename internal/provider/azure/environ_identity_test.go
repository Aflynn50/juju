// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/testing"
)

func (s *environSuite) TestSupportsInstanceRole(c *tc.C) {
	env, ok := s.openEnviron(c).(environs.InstanceRole)
	c.Assert(ok, tc.IsTrue)
	c.Assert(env.SupportsInstanceRoles(context.Background()), tc.IsTrue)
}

func (s *environSuite) TestCreateAutoInstanceRole(c *tc.C) {
	env, ok := s.openEnviron(c).(environs.InstanceRole)
	c.Assert(ok, tc.IsTrue)

	s.sender = s.initResourceGroupSenders(resourceGroupName)

	deployments := []*armresources.DeploymentExtended{{
		Name: to.Ptr("identity"),
		Properties: &armresources.DeploymentPropertiesExtended{
			ProvisioningState: to.Ptr(armresources.ProvisioningStateSucceeded),
		},
	}}
	s.sender = append(s.sender,
		// Managed identity.
		makeSender("/deployments", armresources.DeploymentListResult{Value: deployments}),
		// Role assignment.
		makeSender("/deployments", armresources.DeploymentListResult{Value: deployments}),
	)
	p := environs.BootstrapParams{
		ControllerConfig: map[string]interface{}{
			controller.ControllerUUIDKey: testing.ControllerTag.Id(),
		},
	}
	res, err := env.CreateAutoInstanceRole(context.Background(), p)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.Equals, fmt.Sprintf("%s/%s", resourceGroupName, "juju-controller-"+testing.ControllerTag.Id()))
}
