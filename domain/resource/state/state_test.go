// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	resourcestesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/domain/resource"
	schematesting "github.com/juju/juju/domain/schema/testing"
	charmresource "github.com/juju/juju/internal/charm/resource"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujutesting "github.com/juju/juju/internal/testing"
)

var fingerprint = []byte("123456789012345678901234567890123456789012345678")

type stateSuite struct {
	schematesting.ControllerSuite
	controllerModelUUID coremodel.UUID
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
}

func (s *stateSuite) TestControllerModelUUID(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(),loggertesting.WrapCheckLog(c))
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	res := resource.Resource{
		Meta: charmresource.Meta{
			Name:        "my-resource",
			Type:        charmresource.TypeFile,
			Path:        "filename.tgz",
			Description: "One line that is useful when operators need to push it.",
		},
		UUID:            resourcestesting.GenResourceID(c),
		ApplicationUUID:  applicationtesting.GenApplicationUUID(c),
		SuppliedBy:     "admin",
		SuppliedByType: resource.User,
		Origin:      charmresource.OriginStore,
		Revision:    ptr(1),
		Fingerprint: fp,
		Size:        1,

		CreatedAt:       time.Time{},
	st.SetResource(context.Background())
}
