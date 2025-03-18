// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type stateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestRelationExists(c *gc.C) {
	_, err := s.DB().Exec("INSERT INTO relation (uuid, life_id, relation_id) VALUES (?, ?, ?)",
		"some-relation-uuid", 0, "some-relation-id")
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RelationExists(context.Background(), "some-relation-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exists, gc.Equals, true)

	exists, err = st.RelationExists(context.Background(), "not-today-henry")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exists, gc.Equals, false)
}

func (s *stateSuite) TestRelationRemovalNormalSuccess(c *gc.C) {
	_, err := s.DB().Exec("INSERT INTO relation (uuid, life_id, relation_id) VALUES (?, ?, ?)",
		"some-relation-uuid", 0, "some-relation-id")
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.RelationAdvanceLifeAndScheduleRemoval(context.Background(), "removal-uuid", "some-relation-uuid", false)
	c.Assert(err, jc.ErrorIsNil)

	// Relation had life "alive" and should now be "dying".
	row := s.DB().QueryRow("SELECT life_id FROM relation where uuid = ?", "some-relation-uuid")
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(lifeID, gc.Equals, 1)

	// We should have a removal job scheduled immediately.
	row = s.DB().QueryRow(
		"SELECT removal_type_id, entity_uuid, force, scheduled_for FROM removal where uuid = ?",
		"removal-uuid",
	)
	var (
		removalTypeID int
		rUUID         string
		force         bool
		scheduledFor  time.Time
	)
	err = row.Scan(&removalTypeID, &rUUID, &force, &scheduledFor)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(removalTypeID, gc.Equals, 0)
	c.Check(rUUID, gc.Equals, "some-relation-uuid")
	c.Check(force, gc.Equals, false)
	c.Check(scheduledFor, jc.TimeBetween(time.Time{}, time.Now()))
}
func (s *stateSuite) TestRelationRemovalForcedDeadSuccess(c *gc.C) {
	_, err := s.DB().Exec("INSERT INTO relation (uuid, life_id, relation_id) VALUES (?, ?, ?)",
		"some-relation-uuid", 2, "some-relation-id")
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.RelationAdvanceLifeAndScheduleRemoval(context.Background(), "removal-uuid", "some-relation-uuid", true)
	c.Assert(err, jc.ErrorIsNil)

	// Relation had life "dead", which should be unchanged.
	row := s.DB().QueryRow("SELECT life_id FROM relation where uuid = ?", "some-relation-uuid")
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(lifeID, gc.Equals, 2)

	// We should have a removal job scheduled immediately.
	row = s.DB().QueryRow(
		"SELECT removal_type_id, entity_uuid, force, scheduled_for FROM removal where uuid = ?",
		"removal-uuid",
	)
	var (
		removalTypeID int
		rUUID         string
		force         bool
		scheduledFor  time.Time
	)
	err = row.Scan(&removalTypeID, &rUUID, &force, &scheduledFor)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(removalTypeID, gc.Equals, 0)
	c.Check(rUUID, gc.Equals, "some-relation-uuid")
	c.Check(force, gc.Equals, true)
	c.Check(scheduledFor, jc.TimeBetween(time.Time{}, time.Now()))
}
