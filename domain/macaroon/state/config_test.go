// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	macaroonerrors "github.com/juju/juju/domain/macaroon/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

var (
	testKey1 = bakery.MustGenerateKey()
	testKey2 = bakery.MustGenerateKey()
	testKey3 = bakery.MustGenerateKey()
	testKey4 = bakery.MustGenerateKey()
)

type configStateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&configStateSuite{})

func (s *configStateSuite) TestInitialise(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.InitialiseBakeryConfig(context.Background(), testKey1, testKey2, testKey3, testKey4)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *configStateSuite) TestInitialiseMultipleTimesFails(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.InitialiseBakeryConfig(context.Background(), testKey1, testKey2, testKey3, testKey4)
	c.Assert(err, jc.ErrorIsNil)

	err = st.InitialiseBakeryConfig(context.Background(), testKey1, testKey2, testKey3, testKey4)
	c.Assert(err, jc.ErrorIs, macaroonerrors.BakeryConfigAlreadyInitialised)
}

func (s *configStateSuite) TestGetKeys(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.InitialiseBakeryConfig(context.Background(), testKey1, testKey2, testKey3, testKey4)
	c.Assert(err, jc.ErrorIsNil)

	keypair, err := st.GetLocalUsersKey(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(keypair, gc.DeepEquals, testKey1)

	keypair, err = st.GetLocalUsersThirdPartyKey(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(keypair, gc.DeepEquals, testKey2)

	keypair, err = st.GetExternalUsersThirdPartyKey(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(keypair, gc.DeepEquals, testKey3)

	keypair, err = st.GetOffersThirdPartyKey(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(keypair, gc.DeepEquals, testKey4)
}

func (s *configStateSuite) TestGetKeysUninitialised(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetLocalUsersKey(context.Background())
	c.Check(err, jc.ErrorIs, macaroonerrors.NotInitialised)

	_, err = st.GetLocalUsersThirdPartyKey(context.Background())
	c.Check(err, jc.ErrorIs, macaroonerrors.NotInitialised)

	_, err = st.GetExternalUsersThirdPartyKey(context.Background())
	c.Check(err, jc.ErrorIs, macaroonerrors.NotInitialised)

	_, err = st.GetOffersThirdPartyKey(context.Background())
	c.Check(err, jc.ErrorIs, macaroonerrors.NotInitialised)
}
