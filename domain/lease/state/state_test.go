// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/domain/lease/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
	jujutesting "github.com/juju/juju/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite

	store *state.State
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)

	s.store = state.NewState(s.TxnRunnerFactory(), jujutesting.CheckLogger{Log: c})
}

func (s *stateSuite) TestClaimLeaseSuccessAndLeaseQueries(c *gc.C) {
	pgKey := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	pgReq := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	// Add 2 leases.
	err := s.store.ClaimLease(context.Background(), uuid.MustNewUUID(), pgKey, pgReq)
	c.Assert(err, jc.ErrorIsNil)

	mmKey := pgKey
	mmKey.Lease = "mattermost"

	mmReq := pgReq
	mmReq.Holder = "mattermost/0"

	err = s.store.ClaimLease(context.Background(), uuid.MustNewUUID(), mmKey, mmReq)
	c.Assert(err, jc.ErrorIsNil)

	// Check all the leases.
	leases, err := s.store.Leases(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leases, gc.HasLen, 2)
	c.Check(leases[pgKey].Holder, gc.Equals, "postgresql/0")
	c.Check(leases[pgKey].Expiry.After(time.Now().UTC()), jc.IsTrue)
	c.Check(leases[mmKey].Holder, gc.Equals, "mattermost/0")
	c.Check(leases[mmKey].Expiry.After(time.Now().UTC()), jc.IsTrue)

	// Check with a filter.
	leases, err = s.store.Leases(context.Background(), pgKey)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leases, gc.HasLen, 1)
	c.Check(leases[pgKey].Holder, gc.Equals, "postgresql/0")

	// Add a lease from a different group,
	// and check that the group returns the application leases.
	err = s.store.ClaimLease(context.Background(),
		uuid.MustNewUUID(),
		corelease.Key{
			Namespace: "singular-controller",
			ModelUUID: "controller-model-uuid",
			Lease:     "singular",
		},
		corelease.Request{
			Holder:   "machine/0",
			Duration: time.Minute,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	leases, err = s.store.LeaseGroup(context.Background(), "application-leadership", "model-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leases, gc.HasLen, 2)
	c.Check(leases[pgKey].Holder, gc.Equals, "postgresql/0")
	c.Check(leases[mmKey].Holder, gc.Equals, "mattermost/0")
}

func (s *stateSuite) TestClaimLeaseAlreadyHeld(c *gc.C) {
	key := corelease.Key{
		Namespace: "singular-controller",
		ModelUUID: "controller-model-uuid",
		Lease:     "singular",
	}

	req := corelease.Request{
		Holder:   "machine/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(context.Background(), uuid.MustNewUUID(), key, req)
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.ClaimLease(context.Background(), uuid.MustNewUUID(), key, req)
	c.Assert(err, jc.ErrorIs, corelease.ErrHeld)
}

func (s *stateSuite) TestExtendLeaseSuccess(c *gc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(context.Background(), uuid.MustNewUUID(), key, req)
	c.Assert(err, jc.ErrorIsNil)

	leases, err := s.store.Leases(context.Background(), key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leases, gc.HasLen, 1)

	// Save the expiry for later comparison.
	originalExpiry := leases[key].Expiry

	req.Duration = 2 * time.Minute
	err = s.store.ExtendLease(context.Background(), key, req)
	c.Assert(err, jc.ErrorIsNil)

	leases, err = s.store.Leases(context.Background(), key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leases, gc.HasLen, 1)

	// Check that we extended.
	c.Check(leases[key].Expiry.After(originalExpiry), jc.IsTrue)
}

func (s *stateSuite) TestExtendLeaseNotHeldInvalid(c *gc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ExtendLease(context.Background(), key, req)
	c.Assert(err, jc.ErrorIs, corelease.ErrInvalid)
}

func (s *stateSuite) TestRevokeLeaseSuccess(c *gc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(context.Background(), uuid.MustNewUUID(), key, req)
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.RevokeLease(context.Background(), key, req.Holder)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestRevokeLeaseNotHeldInvalid(c *gc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	err := s.store.RevokeLease(context.Background(), key, "not-the-holder")
	c.Assert(err, jc.ErrorIs, corelease.ErrInvalid)
}

func (s *stateSuite) TestPinUnpinLeaseAndPinQueries(c *gc.C) {
	pgKey := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	pgReq := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(context.Background(), uuid.MustNewUUID(), pgKey, pgReq)
	c.Assert(err, jc.ErrorIsNil)

	// One entity pins the lease.
	err = s.store.PinLease(context.Background(), pgKey, "machine/6")
	c.Assert(err, jc.ErrorIsNil)

	// The same lease/entity is a no-op without error.
	err = s.store.PinLease(context.Background(), pgKey, "machine/6")
	c.Assert(err, jc.ErrorIsNil)

	// Another entity pinning the same lease.
	err = s.store.PinLease(context.Background(), pgKey, "machine/7")
	c.Assert(err, jc.ErrorIsNil)

	pins, err := s.store.Pinned(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pins, gc.HasLen, 1)
	c.Check(pins[pgKey], jc.SameContents, []string{"machine/6", "machine/7"})

	// Unpin and check the leases.
	err = s.store.UnpinLease(context.Background(), pgKey, "machine/7")
	c.Assert(err, jc.ErrorIsNil)

	pins, err = s.store.Pinned(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pins, gc.HasLen, 1)
	c.Check(pins[pgKey], jc.SameContents, []string{"machine/6"})
}

func (s *stateSuite) TestLeaseOperationCancellation(c *gc.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(ctx, uuid.MustNewUUID(), key, req)
	c.Assert(err, gc.ErrorMatches, "context canceled")
}

func (s *stateSuite) TestWorkerDeletesExpiredLeases(c *gc.C) {
	// Insert 2 leases, one with an expiry time in the past,
	// another in the future.
	q := `
INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
VALUES (?, 1, 'some-model-uuid', ?, ?, datetime('now'), datetime('now', ?))`[1:]

	stmt, err := s.DB().Prepare(q)
	c.Assert(err, jc.ErrorIsNil)

	defer stmt.Close()

	_, err = stmt.Exec(uuid.MustNewUUID().String(), "postgresql", "postgresql/0", "+2 minutes")
	c.Assert(err, jc.ErrorIsNil)

	_, err = stmt.Exec(uuid.MustNewUUID().String(), "redis", "redis/0", "-2 minutes")
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.ExpireLeases(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	// Only the postgresql lease (expiring in the future) should remain.
	row := s.DB().QueryRow("SELECT name FROM LEASE")
	var name string
	err = row.Scan(&name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(row.Err(), jc.ErrorIsNil)

	c.Check(name, gc.Equals, "postgresql")
}
