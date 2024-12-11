// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/objectstore"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/resource/store"
	resourcestoretesting "github.com/juju/juju/core/resource/store/testing"
	coreresourcetesting "github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type resourceSuite struct {
	schematesting.ModelSuite

	state *State

	constants struct {
		fakeApplicationUUID1 string
		fakeApplicationUUID2 string
		fakeUnitUUID1        string
		fakeUnitUUID2        string
	}
}

var _ = gc.Suite(&resourceSuite{})

const fakeCharmUUID = "fake-charm-uuid"

func (s *resourceSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	s.constants.fakeApplicationUUID1 = "fake-application-1-uuid"
	s.constants.fakeApplicationUUID2 = "fake-application-2-uuid"
	s.constants.fakeUnitUUID1 = "fake-unit-1-uuid"
	s.constants.fakeUnitUUID2 = "fake-unit-2-uuid"

	// Populate DB with two application and a charm
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		fakeNetNodeUUID := "fake-net-node-uuid"

		_, err = tx.ExecContext(ctx, `INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, 'app', 0)`, fakeCharmUUID)
		if err != nil {
			return errors.Capture(err)
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO net_node (uuid) VALUES (?)`, fakeNetNodeUUID)
		if err != nil {
			return errors.Capture(err)
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO application (uuid, name, life_id, charm_uuid) VALUES (?, ?, ?, ?),(?, ?, ?, ?)`,
			s.constants.fakeApplicationUUID1, "app1", 0 /* alive */, fakeCharmUUID,
			s.constants.fakeApplicationUUID2, "app2", 0 /* alive */, fakeCharmUUID)
		if err != nil {
			return errors.Capture(err)
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid) VALUES (?, ?, ?, ?, ?),(?, ?, ?, ?, ?)`,
			s.constants.fakeUnitUUID1, "unit1", 0 /* alive */, s.constants.fakeApplicationUUID1, fakeNetNodeUUID,
			s.constants.fakeUnitUUID2, "unit2", 0 /* alive */, s.constants.fakeApplicationUUID2, fakeNetNodeUUID)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("failed to populate DB with applications: %v", errors.ErrorStack(err)))
}

// TestDeleteApplicationResources is a test method that verifies the deletion of resources
// associated with a specific application in the database.
func (s *resourceSuite) TestDeleteApplicationResources(c *gc.C) {
	// Arrange: populate db with some resources, belonging to app1 (2 res) and app2 (1 res)
	res1 := resourceData{
		UUID:            "app1-res1-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "res1",
		// populate table "resource_retrieved_by"
		RetrievedByType: "user",
		RetrievedByName: "john",
	}
	res2 := resourceData{
		UUID:            "app1-res2-uuid",
		Name:            "res2",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
	}
	other := resourceData{
		UUID:            "res-uuid",
		Name:            "res3",
		ApplicationUUID: s.constants.fakeApplicationUUID2,
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) (err error) {
		for _, input := range []resourceData{res1, res2, other} {
			if err := input.insert(context.Background(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: delete resources from application 1
	err = s.state.DeleteApplicationResources(context.Background(), application.ID(s.constants.fakeApplicationUUID1))

	// Assert: check that resources have been deleted in expected tables
	// without errors
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) failed to delete resources from application 1: %v", errors.ErrorStack(err)))
	var remainingResources []resourceData
	var noRowsInRessourceRetrievedBy bool
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) (err error) {
		// fetch resources
		rows, err := tx.Query(`
SELECT uuid, charm_resource_name, application_uuid
FROM resource AS r
LEFT JOIN application_resource AS ar ON r.uuid = ar.resource_uuid`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var uuid string
			var resName string
			var appUUID string
			if err := rows.Scan(&uuid, &resName, &appUUID); err != nil {
				return err
			}
			remainingResources = append(remainingResources,
				resourceData{UUID: uuid, ApplicationUUID: appUUID,
					Name: resName})
		}
		// fetch resource_retrieved_by
		var discard string
		err = tx.QueryRow(`SELECT resource_uuid from resource_retrieved_by`).
			Scan(&discard)
		if errors.Is(err, sql.ErrNoRows) {
			noRowsInRessourceRetrievedBy = true
			return nil
		}
		return err
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) failed to check db: %v",
		errors.ErrorStack(err)))
	c.Check(noRowsInRessourceRetrievedBy, gc.Equals, true, gc.Commentf("(Assert) resource_retrieved_by table should be empty"))
	c.Check(remainingResources, jc.DeepEquals, []resourceData{other},
		gc.Commentf("(Assert) only resource from %q should be there",
			s.constants.fakeApplicationUUID2))
}

// TestDeleteApplicationResourcesErrorRemainingUnits tests resource deletion with linked units.
//
// This method populates the database with a resource linked to a unit, attempts to delete
// the application's resources, then verifies that an error is returned due to the remaining unit
// and that no resources have been deleted. This enforces constraints on cleaning up resources
// with active dependencies.
func (s *resourceSuite) TestDeleteApplicationResourcesErrorRemainingUnits(c *gc.C) {
	// Arrange: populate db with some resource a resource linked to a unit
	input := resourceData{
		UUID:            "app1-res1-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "res1",
		// Populate table resource_unit
		UnitUUID: s.constants.fakeUnitUUID1,
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) (err error) {
		return input.insert(context.Background(), tx)
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: delete resources from application 1
	err = s.state.DeleteApplicationResources(context.Background(), application.ID(s.constants.fakeApplicationUUID1))

	// Assert: check an error is returned and no resource deleted
	c.Check(err, jc.ErrorIs, resourceerrors.CleanUpStateNotValid,
		gc.Commentf("(Assert) unexpected error: %v", errors.ErrorStack(err)))
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) (err error) {
		// fetch resources
		var discard string
		return tx.QueryRow(`
SELECT uuid FROM v_resource
WHERE uuid = ? AND application_uuid = ? AND name = ?`,
			input.UUID, input.ApplicationUUID, input.Name,
		).Scan(&discard)
	})
	c.Check(err, jc.ErrorIsNil, gc.Commentf("(Assert) resource deleted or cannot check db: %v",
		errors.ErrorStack(err)))
}

// TestDeleteApplicationResourcesErrorRemainingObjectStoreData verifies that attempting to delete application
// resources will fail when there are remaining object store data linked to the resource,
// and no resource will be deleted.
func (s *resourceSuite) TestDeleteApplicationResourcesErrorRemainingObjectStoreData(c *gc.C) {
	// Arrange: populate db with some resource linked with some data
	input := resourceData{
		UUID:            "res1-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "res1",
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) (err error) {
		// Insert the data
		if err := input.insert(context.Background(), tx); err != nil {
			return errors.Capture(err)
		}
		// Insert some data linked to the resource
		if _, err := tx.Exec(`
INSERT INTO object_store_metadata (uuid, sha_256, sha_384,size) 
VALUES ('store-uuid','','',0)`); err != nil {
			return errors.Capture(err)
		}
		if _, err := tx.Exec(`
INSERT INTO resource_file_store (resource_uuid, store_uuid) 
VALUES (?,'store-uuid')`, input.UUID); err != nil {
			return errors.Capture(err)
		}
		return
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: delete resources from application 1
	err = s.state.DeleteApplicationResources(context.Background(), application.ID(s.constants.fakeApplicationUUID1))

	// Assert: check an error is returned and no resource deleted
	c.Check(err, jc.ErrorIs, resourceerrors.CleanUpStateNotValid,
		gc.Commentf("(Assert) unexpected error: %v", errors.ErrorStack(err)))
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) (err error) {
		// fetch resources
		var discard string
		return tx.QueryRow(`
SELECT uuid FROM v_resource
WHERE uuid = ? AND application_uuid = ? AND name = ?`,
			input.UUID, input.ApplicationUUID, input.Name,
		).Scan(&discard)
	})
	c.Check(err, jc.ErrorIsNil, gc.Commentf("(Assert) resource deleted or cannot check db: %v",
		errors.ErrorStack(err)))
}

// TestDeleteApplicationResourcesErrorRemainingImageStoreData verifies that attempting to delete application
// resources will fail when there are remaining image store data linked to the resource,
// and no resource will be deleted.
func (s *resourceSuite) TestDeleteApplicationResourcesErrorRemainingImageStoreData(c *gc.C) {
	// Arrange: populate db with some resource linked with some data
	input := resourceData{
		UUID:            "res1-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "res1",
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) (err error) {
		// Insert the data
		if err := input.insert(context.Background(), tx); err != nil {
			return errors.Capture(err)
		}
		// Insert some data linked to the resource
		if _, err := tx.Exec(`
INSERT INTO resource_container_image_metadata_store (storage_key, registry_path) 
VALUES ('store-uuid','')`); err != nil {
			return errors.Capture(err)
		}
		if _, err := tx.Exec(`
INSERT INTO resource_image_store (resource_uuid, store_storage_key) 
VALUES (?,'store-uuid')`, input.UUID); err != nil {
			return errors.Capture(err)
		}
		return
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: delete resources from application 1
	err = s.state.DeleteApplicationResources(context.Background(), application.ID(s.constants.fakeApplicationUUID1))

	// Assert: check an error is returned and no resource deleted
	c.Check(err, jc.ErrorIs, resourceerrors.CleanUpStateNotValid,
		gc.Commentf("(Assert) unexpected error: %v", errors.ErrorStack(err)))
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) (err error) {
		// fetch resources
		var discard string
		return tx.QueryRow(`
SELECT uuid FROM v_resource
WHERE uuid = ? AND application_uuid = ? AND name = ?`,
			input.UUID, input.ApplicationUUID, input.Name,
		).Scan(&discard)
	})
	c.Check(err, jc.ErrorIsNil, gc.Commentf("(Assert) resource deleted or cannot check db: %v",
		errors.ErrorStack(err)))
}

// TestDeleteUnitResources verifies that resources linked to a specific unit are deleted correctly.
func (s *resourceSuite) TestDeleteUnitResources(c *gc.C) {
	// Arrange: populate db with some resource a resource linked to a unit
	resUnit1 := resourceData{
		UUID:            "res-unit1-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "res-unit1",
		// Populate table resource_unit
		UnitUUID: s.constants.fakeUnitUUID1,
	}
	other := resourceData{
		UUID:            "res-unit2-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "res-unit2",
		// Populate table resource_unit
		UnitUUID: s.constants.fakeUnitUUID2,
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) (err error) {
		for _, input := range []resourceData{resUnit1, other} {
			if err := input.insert(context.Background(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: delete resources from application 1
	err = s.state.DeleteUnitResources(context.Background(), unit.UUID(s.constants.fakeUnitUUID1))

	// Assert: check that resources link to unit 1 have been deleted in expected tables
	// without errors
	c.Assert(err, jc.ErrorIsNil,
		gc.Commentf("(Assert) failed to delete resources link to unit 1: %v",
			errors.ErrorStack(err)))
	var obtained []resourceData
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) (err error) {
		// fetch resources
		rows, err := tx.Query(`
SELECT uuid, name, application_uuid, unit_uuid
FROM v_resource AS rv
LEFT JOIN unit_resource AS ur ON rv.uuid = ur.resource_uuid`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var uuid string
			var resName string
			var appUUID string
			var unitUUID *string
			if err := rows.Scan(&uuid, &resName, &appUUID, &unitUUID); err != nil {
				return err
			}
			obtained = append(obtained,
				resourceData{UUID: uuid, ApplicationUUID: appUUID,
					Name: resName, UnitUUID: zeroPtr(unitUUID)})
		}
		return err
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) failed to check db: %v",
		errors.ErrorStack(err)))
	expectedResUnit1 := resUnit1
	expectedResUnit1.UnitUUID = ""
	c.Assert(obtained, jc.SameContents, []resourceData{expectedResUnit1, other}, gc.Commentf("(Assert) unexpected resources: %v", obtained))
}

// TestGetApplicationResourceID tests that the resource ID can be correctly
// retrieved from the database, given a name and an application
func (s *resourceSuite) TestGetApplicationResourceID(c *gc.C) {
	// Arrange: Populate state with two resources on application 1.
	found := resourceData{
		UUID:            "resource-uuid-found",
		Name:            "resource-name-found",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
	}
	other := resourceData{
		UUID:            "resource-uuid-other",
		Name:            "resource-name-other",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) (err error) {
		for _, input := range []resourceData{found, other} {
			if err := input.insert(context.Background(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: Get application resource ID
	id, err := s.state.GetResourceUUID(
		context.Background(),
		application.ID(s.constants.fakeApplicationUUID1),
		found.Name,
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to get application resource ID: %v", errors.ErrorStack(err)))
	c.Assert(id, gc.Equals, coreresource.UUID(found.UUID),
		gc.Commentf("(Act) unexpected application resource ID"))
}

// TestGetApplicationResourceIDNotFound verifies the behavior when attempting
// to retrieve a resource ID for a non-existent resource within a specified
// application.
func (s *resourceSuite) TestGetApplicationResourceIDNotFound(c *gc.C) {
	// Arrange: No resources
	// Act: Get application resource ID
	_, err := s.state.GetResourceUUID(
		context.Background(),
		application.ID(s.constants.fakeApplicationUUID1),
		"resource-name-not-found",
	)
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceNotFound, gc.Commentf("(Act) unexpected error"))
}

// TestGetResourceNotFound verifies that attempting to retrieve a non-existent
// resource results in a ResourceNotFound error.
func (s *resourceSuite) TestGetResourceNotFound(c *gc.C) {
	// Arrange : no resource
	resID := coreresource.UUID("resource-id")

	// Act
	_, err := s.state.GetResource(context.Background(), resID)

	// Assert
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceNotFound, gc.Commentf("(Assert) unexpected error"))
}

// TestGetResource verifies the successful retrieval of a resource from the
// database by its ID.
func (s *resourceSuite) TestGetResource(c *gc.C) {
	// Arrange : a simple resource
	resID := coreresource.UUID("resource-id")
	now := time.Now().Truncate(time.Second).UTC()
	expected := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "resource-name",
				Path:        "/path/to/resource",
				Description: "this is a test resource",
				Type:        charmresource.TypeFile,
			},
			Revision: 42,
			Origin:   0,
			// todo(gfouillet): handle size/fingerprint
			//Fingerprint: charmresource.Fingerprint{},
			//Size:        0,
		},
		UUID:            resID,
		ApplicationID:   application.ID(s.constants.fakeApplicationUUID1),
		RetrievedBy:     "johnDoe",
		RetrievedByType: "user",
		Timestamp:       now,
	}
	input := resourceData{
		UUID:            resID.String(),
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Revision:        expected.Revision,
		OriginType:      "uploaded", // 0 in db
		CreatedAt:       now,
		Name:            expected.Name,
		Type:            charmresource.TypeFile,
		Path:            expected.Path,
		Description:     expected.Description,
		RetrievedByType: string(expected.RetrievedByType),
		RetrievedByName: expected.RetrievedBy,
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := input.insert(context.Background(), tx)
		return errors.Capture(err)
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act
	obtained, err := s.state.GetResource(context.Background(), resID)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute GetResource: %v", errors.ErrorStack(err)))

	// Assert
	c.Assert(obtained, jc.DeepEquals, expected, gc.Commentf("(Assert) resource different than expected"))
}

// TestSetRepositoryResource ensures that the SetRepositoryResources function
// updates the resource poll dates correctly.
func (s *resourceSuite) TestSetRepositoryResource(c *gc.C) {
	// Arrange : Insert 4 resources, two have been already polled, and two other not yet.
	now := time.Now().Truncate(time.Second).UTC()
	previousPoll := now.Add(-1 * time.Hour)
	defaultResource := resourceData{
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       now,
		RetrievedByType: "user",
		RetrievedByName: "John Doe",
	}
	notPolled := []resourceData{
		defaultResource.DeepCopy(),
		defaultResource.DeepCopy(),
	}
	notPolled[0].UUID = "not-polled-id-1"
	notPolled[0].Name = "not-polled-1"
	notPolled[1].UUID = "not-polled-id-2"
	notPolled[1].Name = "not-polled-2"
	alreadyPolled := []resourceData{
		defaultResource.DeepCopy(),
		defaultResource.DeepCopy(),
	}
	alreadyPolled[0].UUID = "polled-id-1"
	alreadyPolled[0].Name = "polled-1"
	alreadyPolled[1].UUID = "polled-id-2"
	alreadyPolled[1].Name = "polled-2"
	for i := range alreadyPolled {
		alreadyPolled[i].PolledAt = previousPoll
	}

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		for _, input := range append(notPolled, alreadyPolled...) {
			if err := input.insert(context.Background(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: update resource 1 and 2 (not 3)
	err = s.state.SetRepositoryResources(context.Background(), resource.SetRepositoryResourcesArgs{
		ApplicationID: application.ID(s.constants.fakeApplicationUUID1),
		Info: []charmresource.Resource{{
			Meta: charmresource.Meta{
				Name: "not-polled-1",
			},
		}, {
			Meta: charmresource.Meta{
				Name: "polled-1",
			},
		}},
		LastPolled: now,
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute TestSetRepositoryResource: %v", errors.ErrorStack(err)))

	// Assert
	type obtainedRow struct {
		ResourceUUID string
		LastPolled   *time.Time
	}
	var obtained []obtainedRow
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT uuid, last_polled FROM resource`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row obtainedRow
			if err := rows.Scan(&row.ResourceUUID, &row.LastPolled); err != nil {
				return err
			}
			obtained = append(obtained, row)
		}
		return err
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) failed to get expected changes in db: %v", errors.ErrorStack(err)))
	c.Assert(obtained, jc.SameContents, []obtainedRow{
		{
			ResourceUUID: "polled-id-1", // updated
			LastPolled:   &now,
		},
		{
			ResourceUUID: "polled-id-2",
			LastPolled:   &previousPoll, // not updated
		},
		{
			ResourceUUID: "not-polled-id-1", // created
			LastPolled:   &now,
		},
		{
			ResourceUUID: "not-polled-id-2", // not polled
			LastPolled:   nil,
		},
	})
}

// TestSetRepositoryResourceUnknownResource validates that attempting to set
// repository resources for unknown resources logs the correct errors.
func (s *resourceSuite) TestSetRepositoryResourceUnknownResource(c *gc.C) {
	// Act: update unexisting resources
	err := s.state.SetRepositoryResources(context.Background(), resource.SetRepositoryResourcesArgs{
		ApplicationID: application.ID(s.constants.fakeApplicationUUID1),
		Info: []charmresource.Resource{{
			Meta: charmresource.Meta{
				Name: "not-a-resource-1",
			},
		}, {
			Meta: charmresource.Meta{
				Name: "not-a-resource-2",
			},
		}},
		LastPolled: time.Now(),
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute TestSetRepositoryResource: %v", errors.ErrorStack(err)))

	// Assert
	c.Check(c.GetTestLog(), jc.Contains, fmt.Sprintf("Resource not found for application app1 (%s)", s.constants.fakeApplicationUUID1), gc.Commentf("(Assert) application not found in log"))
	c.Check(c.GetTestLog(), jc.Contains, "not-a-resource-1", gc.Commentf("(Assert) missing resource name log"))
	c.Check(c.GetTestLog(), jc.Contains, "not-a-resource-2", gc.Commentf("(Assert) missing resource name log"))
}

// TestSetRepositoryResourceApplicationNotFound verifies that setting repository
// resources for a non-existent application results in an ApplicationNotFound error.
func (s *resourceSuite) TestSetRepositoryResourceApplicationNotFound(c *gc.C) {
	// Act: request an unexisting application
	err := s.state.SetRepositoryResources(context.Background(), resource.SetRepositoryResourcesArgs{
		ApplicationID: "not-an-application",
		Info:          []charmresource.Resource{{}}, // Non empty info
		LastPolled:    time.Now(),                   // not used
	})

	// Assert: check expected error
	c.Assert(err, jc.ErrorIs, resourceerrors.ApplicationNotFound, gc.Commentf("(Act) unexpected error: %v", errors.ErrorStack(err)))
}

func (s *resourceSuite) TestStoreResourceContainerImage(c *gc.C) {
	resID, storeID := s.createContainerImageResourceAndBlob(c)

	retrievedBy := "retrieved-by-app"
	retrievedByType := resource.Application
	// Act: store the resource.
	err := s.state.StoreResource(
		context.Background(),
		resID,
		storeID,
		retrievedBy,
		retrievedByType,
		false,
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute StoreResource: %v", errors.ErrorStack(err)))

	// Assert: Check that the resource has been linked to the stored blob
	var foundStorageKey string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT store_storage_key FROM resource_image_store
WHERE resource_uuid = ?`, resID).Scan(&foundStorageKey)
	})
	storageKey, err := storeID.ContainerImageMetadataStoreID()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(foundStorageKey, gc.Equals, storageKey)
	c.Check(err, jc.ErrorIsNil, gc.Commentf("(Assert) resource_image_store table not updated: %v", errors.ErrorStack(err)))

	// Assert: Check that retrieved by has been set.
	var foundRetrievedByType, foundRetrievedBy string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT rrb.name, rrbt.name AS type
FROM   resource_retrieved_by rrb
JOIN   resource_retrieved_by_type rrbt ON rrb.retrieved_by_type_id = rrbt.id
WHERE  resource_uuid = ?`, resID).Scan(&foundRetrievedBy, &foundRetrievedByType)
	})
	c.Check(foundRetrievedByType, gc.Equals, string(retrievedByType))
	c.Check(foundRetrievedBy, gc.Equals, retrievedBy)
}

func (s *resourceSuite) TestStoreResourceFile(c *gc.C) {
	// Arrange: Create file resource.
	resID, storeID := s.createFileResourceAndBlob(c)

	// Act: store the resource.
	retrievedBy := "retrieved-by-unit"
	retrievedByType := resource.Unit
	err := s.state.StoreResource(
		context.Background(),
		coreresource.UUID(resID),
		storeID,
		retrievedBy,
		retrievedByType,
		false,
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute StoreResource: %v", errors.ErrorStack(err)))

	// Assert: Check that the resource has been linked to the stored blob
	var foundStoreUUID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT store_uuid FROM resource_file_store
WHERE resource_uuid = ?`, resID).Scan(&foundStoreUUID)
	})
	objectStoreUUID, err := storeID.ObjectStoreUUID()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(foundStoreUUID, gc.Equals, objectStoreUUID.String())
	c.Check(err, jc.ErrorIsNil, gc.Commentf("(Assert) resource_file_store table not updated: %v", errors.ErrorStack(err)))

	// Assert: Check that retrieved by has been set.
	var foundRetrievedByType, foundRetrievedBy string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT rrb.name, rrbt.name AS type
FROM   resource_retrieved_by rrb
JOIN   resource_retrieved_by_type rrbt ON rrb.retrieved_by_type_id = rrbt.id
WHERE  resource_uuid = ?`, resID).Scan(&foundRetrievedBy, &foundRetrievedByType)
	})
	c.Check(foundRetrievedByType, gc.Equals, string(retrievedByType))
	c.Check(foundRetrievedBy, gc.Equals, retrievedBy)
}

func (s *resourceSuite) TestStoreResourceIncrementCharmModifiedVersion(c *gc.C) {
	// Arrange: create resource and storage.
	resID, storeID := s.createContainerImageResourceAndBlob(c)
	initialCharmModifiedVersion := s.getCharmModifiedVersion(c, resID.String())
	resID2, storeID2 := s.createContainerImageResourceAndBlob(c)

	// Act: store the resource and increment the CMV.
	retrievedBy := "retrieved-by-app"
	retrievedByType := resource.Application
	err := s.state.StoreResource(
		context.Background(),
		resID,
		storeID,
		retrievedBy,
		retrievedByType,
		true,
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute StoreResource: %v", errors.ErrorStack(err)))

	foundCharmModifiedVersion1 := s.getCharmModifiedVersion(c, resID.String())

	err = s.state.StoreResource(
		context.Background(),
		resID2,
		storeID2,
		retrievedBy,
		retrievedByType,
		true,
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute StoreResource: %v", errors.ErrorStack(err)))

	foundCharmModifiedVersion2 := s.getCharmModifiedVersion(c, resID2.String())

	// Assert: Check the charm modified version has been incremented.
	c.Assert(foundCharmModifiedVersion1, gc.Equals, initialCharmModifiedVersion+1)
	c.Assert(foundCharmModifiedVersion2, gc.Equals, initialCharmModifiedVersion+2)
}

func (s *resourceSuite) TestStoreResourceDoNotIncrementCharmModifiedVersion(c *gc.C) {
	// Arrange: insert a resource and get charm modified version.
	resID, storeID := s.createContainerImageResourceAndBlob(c)
	initialCharmModifiedVersion := s.getCharmModifiedVersion(c, resID.String())

	// Act: store the resource.
	retrievedBy := "retrieved-by-app"
	retrievedByType := resource.Application
	err := s.state.StoreResource(
		context.Background(),
		resID,
		storeID,
		retrievedBy,
		retrievedByType,
		false,
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute StoreResource: %v", errors.ErrorStack(err)))

	// Assert: Check the charm modified version has not been incremented.
	foundCharmModifiedVersion := s.getCharmModifiedVersion(c, resID.String())
	c.Assert(foundCharmModifiedVersion, gc.Equals, initialCharmModifiedVersion)
}

func (s *resourceSuite) TestStoreResourceContainerImageAlreadyStored(c *gc.C) {
	// Arrange: insert a resource and generate 2 blobs.
	resID, storeID1 := s.createContainerImageResourceAndBlob(c)

	storageKey2 := "storage-key-2"
	storeID2 := resourcestoretesting.GenContainerImageMetadataResourceID(c, storageKey2)
	err := s.addContainerImage(storageKey2)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to add container image: %v", errors.ErrorStack(err)))

	// Arrange: store the first resource.
	retrievedBy := "retrieved-by-app"
	retrievedByType := resource.Application
	err = s.state.StoreResource(
		context.Background(),
		resID,
		storeID1,
		retrievedBy,
		retrievedByType,
		false,
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute StoreResource: %v", errors.ErrorStack(err)))

	// Act: try to store a second resource.
	err = s.state.StoreResource(
		context.Background(),
		resID,
		storeID2,
		retrievedBy,
		retrievedByType,
		false,
	)
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceAlreadyStored)
}

func (s *resourceSuite) TestStoreFileResourceAlreadyStored(c *gc.C) {
	// Arrange: insert a resource.
	resID, storeID1 := s.createFileResourceAndBlob(c)

	objectStoreUUID2 := objectstoretesting.GenObjectStoreUUID(c)
	storeID2 := resourcestoretesting.GenFileResourceStoreID(c, objectStoreUUID2)
	err := s.addObjectStoreBlobMetadata(objectStoreUUID2)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to add object store blob: %v", errors.ErrorStack(err)))

	// Arrange: store the first resource.
	retrievedBy := "retrieved-by-unit"
	retrievedByType := resource.Unit
	err = s.state.StoreResource(
		context.Background(),
		resID,
		storeID1,
		retrievedBy,
		retrievedByType,
		false,
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute StoreResource: %v", errors.ErrorStack(err)))

	// Act: try and store the second resource.
	err = s.state.StoreResource(
		context.Background(),
		resID,
		storeID2,
		retrievedBy,
		retrievedByType,
		false,
	)
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceAlreadyStored)
}

func (s *resourceSuite) TestStoreResourceResourceNotFound(c *gc.C) {
	// Arrange: add a stored resource.
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)
	storeID := resourcestoretesting.GenFileResourceStoreID(c, objectStoreUUID)
	err := s.addObjectStoreBlobMetadata(objectStoreUUID)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to add object store blob: %v", errors.ErrorStack(err)))

	// Act: try and store the resource.
	err = s.state.StoreResource(
		context.Background(),
		coreresource.UUID("bad-uuid"),
		storeID,
		"retrieved-by-unit",
		resource.Unit,
		false,
	)
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceNotFound)
}

func (s *resourceSuite) TestStoreResourceFileStoredResourceNotFound(c *gc.C) {
	// Arrange: insert a resource.
	resID := s.addResource(c, charmresource.TypeFile)

	// Arrange: generate a valid store ID.
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)
	storeID := resourcestoretesting.GenFileResourceStoreID(c, objectStoreUUID)

	// Act: try and store the resource.
	err := s.state.StoreResource(
		context.Background(),
		coreresource.UUID(resID),
		storeID,
		"retrieved-by-unit",
		resource.Unit,
		false,
	)
	c.Assert(err, jc.ErrorIs, resourceerrors.StoredResourceNotFound)
}

func (s *resourceSuite) TestStoreResourceContainerImageStoredResourceNotFound(c *gc.C) {
	// Arrange: insert a resource and generate a valid store ID.
	resID := s.addResource(c, charmresource.TypeContainerImage)
	storeID := resourcestoretesting.GenContainerImageMetadataResourceID(c, "bad-storage-key")

	// Act: try and store the resource.
	err := s.state.StoreResource(
		context.Background(),
		coreresource.UUID(resID),
		storeID,
		"retrieved-by-unit",
		resource.Unit,
		false,
	)
	c.Assert(err, jc.ErrorIs, resourceerrors.StoredResourceNotFound)
}

func (s *resourceSuite) TestStoreRetrievedByUnit(c *gc.C) {
	resourceUUID := s.addResource(c, charmresource.TypeFile)
	retrievedBy := "app-test/0"
	retrievedByType := resource.Unit
	err := s.setWithRetrievedBy(c, resourceUUID, retrievedBy, retrievedByType)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute StoreResource: %v", errors.ErrorStack(err)))
	foundRetrievedBy, foundRetrievedByType := s.getRetrievedByType(c, resourceUUID)
	c.Check(foundRetrievedBy, gc.Equals, retrievedBy)
	c.Check(foundRetrievedByType, gc.Equals, retrievedByType)
}

func (s *resourceSuite) TestStoreRetrievedByApplication(c *gc.C) {
	resourceUUID := s.addResource(c, charmresource.TypeFile)
	retrievedBy := "app-test"
	retrievedByType := resource.Application
	err := s.setWithRetrievedBy(c, resourceUUID, retrievedBy, retrievedByType)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute StoreResource: %v", errors.ErrorStack(err)))
	foundRetrievedBy, foundRetrievedByType := s.getRetrievedByType(c, resourceUUID)
	c.Check(foundRetrievedBy, gc.Equals, retrievedBy)
	c.Check(foundRetrievedByType, gc.Equals, retrievedByType)
}

func (s *resourceSuite) TestStoreRetrievedByUser(c *gc.C) {
	resourceUUID := s.addResource(c, charmresource.TypeFile)
	retrievedBy := "jim"
	retrievedByType := resource.User
	err := s.setWithRetrievedBy(c, resourceUUID, retrievedBy, retrievedByType)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute StoreResource: %v", errors.ErrorStack(err)))
	foundRetrievedBy, foundRetrievedByType := s.getRetrievedByType(c, resourceUUID)
	c.Check(foundRetrievedBy, gc.Equals, retrievedBy)
	c.Check(foundRetrievedByType, gc.Equals, retrievedByType)
}

func (s *resourceSuite) TestStoreRetrievedByUnknown(c *gc.C) {
	resourceUUID := s.addResource(c, charmresource.TypeFile)
	retrievedBy := "jim"
	retrievedByType := resource.Unknown
	err := s.setWithRetrievedBy(c, resourceUUID, retrievedBy, retrievedByType)
	c.Assert(err, jc.ErrorIs, resourceerrors.RetrievedByTypeNotValid)
}

func (s *resourceSuite) TestStoreRetrievedByNotSet(c *gc.C) {
	// Retrieve by should not be set if it is blank and the type is unknown.
	resourceUUID := s.addResource(c, charmresource.TypeFile)
	retrievedBy := ""
	retrievedByType := resource.Unknown
	err := s.setWithRetrievedBy(c, resourceUUID, retrievedBy, retrievedByType)
	c.Assert(err, jc.ErrorIsNil)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT rrb.name, rrbt.name AS type
FROM   resource_retrieved_by rrb
JOIN   resource_retrieved_by_type rrbt ON rrb.retrieved_by_type_id = rrbt.id
WHERE  resource_uuid = ?`, resourceUUID.String()).Scan(&retrievedBy, &retrievedByType)
	})
	c.Assert(err, jc.ErrorIs, sql.ErrNoRows)
}

// TestSetUnitResource verifies that a unit resource is correctly set when the
// resource has no initial supplier. It sets up a resource in the database,
// calls the SetUnitResource method, and checks if the resource is updated as
// expected.
func (s *resourceSuite) TestSetUnitResource(c *gc.C) {
	// Arrange: insert a resource.
	startTime := time.Now().Truncate(time.Second).UTC()
	resID := "resource-id"
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       startTime,
		Name:            "resource-name",
		Type:            charmresource.TypeFile,
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := input.insert(context.Background(), tx); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act set supplied by with application type
	err = s.state.SetUnitResource(
		context.Background(),
		coreresource.UUID(resID),
		unit.UUID(s.constants.fakeUnitUUID1),
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute SetUnitResource: %v", errors.ErrorStack(err)))

	// Assert: check the unit resource has been added.
	var addedAt time.Time
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT added_at FROM unit_resource
WHERE resource_uuid = ? and unit_uuid = ?`,
			resID, s.constants.fakeUnitUUID1).Scan(&addedAt)
	})
	c.Check(addedAt, jc.TimeBetween(startTime, time.Now()))
	c.Check(err, jc.ErrorIsNil, gc.Commentf("(Assert) unit_resource table not updated: %v", errors.ErrorStack(err)))
}

func (s *resourceSuite) TestSetUnitResourceWithContainerImageTypeResourceError(c *gc.C) {
	// Arrange: insert a container resource.
	resID := "resource-id"
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       time.Now().Truncate(time.Second).UTC(),
		Name:            "resource-name",
		Type:            charmresource.TypeContainerImage,
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := input.insert(context.Background(), tx); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: try and set the container resource as a unit resource.
	err = s.state.SetUnitResource(
		context.Background(),
		coreresource.UUID(resID),
		unit.UUID(s.constants.fakeUnitUUID1),
	)
	c.Assert(err, gc.ErrorMatches, "units can only be set with file resources, this resource has type "+charmresource.TypeContainerImage.String())
}

// TestSetUnitResourceAlreadySet checks if set unit resource correctly
// identifies an already set resource and skips updating.
func (s *resourceSuite) TestSetUnitResourceAlreadySet(c *gc.C) {
	// Arrange: insert a resource and data implying that everything is already
	// set.
	now := time.Now().Truncate(time.Second).UTC()
	previousInsertTime := now.Add(-1 * time.Hour)
	resID := "resource-id"
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       now,
		Name:            "resource-name",
		UnitUUID:        s.constants.fakeUnitUUID1,
		AddedAt:         previousInsertTime,
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return errors.Capture(input.insert(context.Background(), tx))
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act set supplied by with user type
	err = s.state.SetUnitResource(context.Background(),
		coreresource.UUID(resID),
		unit.UUID(s.constants.fakeUnitUUID1),
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute SetUnitResource: %v", errors.ErrorStack(err)))

	// Assert
	var addedAt time.Time
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT added_at FROM unit_resource
WHERE resource_uuid = ? and unit_uuid = ?`,
			resID, s.constants.fakeUnitUUID1).Scan(&addedAt)
	})
	c.Check(addedAt, gc.Equals, previousInsertTime)
	c.Check(err, jc.ErrorIsNil, gc.Commentf("(Assert) unit_resource table has been updated: %v", errors.ErrorStack(err)))
}

// TestSetUnitResourceNotFound verifies that attempting to set a resource for a
// unit when the resource does not exist results in a ResourceNotFound error.
// The test ensures that no updates are made to the unit_resource and
// resource_retrieved_by tables in the database.
func (s *resourceSuite) TestSetUnitResourceNotFound(c *gc.C) {
	// Arrange: No resource
	resID := "resource-id"

	// Act: set unknown resource
	err := s.state.SetUnitResource(context.Background(),
		coreresource.UUID(resID),
		unit.UUID(s.constants.fakeUnitUUID1),
	)

	// Assert: an error is returned, nothing is updated in the db
	c.Check(err, jc.ErrorIs, resourceerrors.ResourceNotFound)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var discard string
		err = tx.QueryRow(`SELECT * FROM unit_resource`).Scan(&discard)
		c.Check(err, jc.ErrorIs, sql.ErrNoRows, gc.Commentf("(Assert) unit_resource table has been updated: %v", errors.ErrorStack(err)))
		err = tx.QueryRow(`SELECT * FROM resource_retrieved_by`).Scan(&discard)
		c.Check(err, jc.ErrorIs, sql.ErrNoRows, gc.Commentf("(Assert) unit_resource table has been updated: %v", errors.ErrorStack(err)))
		return nil
	})
}

// TestSetUnitResourceUnitNotFound tests that setting a unit resource with an
// unexpected unit ID results in an error.
func (s *resourceSuite) TestSetUnitResourceUnitNotFound(c *gc.C) {
	// Arrange: insert a resource
	now := time.Now().Truncate(time.Second).UTC()
	resID := "resource-id"
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       now,
		Name:            "resource-name",
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return errors.Capture(input.insert(context.Background(), tx))
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act set supplied by with unexpected unit
	err = s.state.SetUnitResource(context.Background(),
		coreresource.UUID(resID),
		"unexpected-unit-id",
	)

	// Assert: an error is returned, nothing is updated in the db
	c.Check(err, jc.ErrorIs, resourceerrors.UnitNotFound)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var discard string
		err = tx.QueryRow(`SELECT * FROM unit_resource`).Scan(&discard)
		c.Check(err, jc.ErrorIs, sql.ErrNoRows, gc.Commentf("(Assert) unit_resource table has been updated: %v", errors.ErrorStack(err)))
		err = tx.QueryRow(`SELECT * FROM resource_retrieved_by`).Scan(&discard)
		c.Check(err, jc.ErrorIs, sql.ErrNoRows, gc.Commentf("(Assert) unit_resource table has been updated: %v", errors.ErrorStack(err)))
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceSuite) TestSetApplicationResource(c *gc.C) {
	// Arrange: insert a resource.
	resID := "resource-id"
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       time.Now().Truncate(time.Second).UTC(),
		Name:            "resource-name",
		Type:            charmresource.TypeContainerImage,
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := input.insert(context.Background(), tx); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	addedAt := time.Now().Truncate(time.Second).UTC()
	// Act set application resource.
	err = s.state.SetApplicationResource(
		context.Background(),
		coreresource.UUID(resID),
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute SetApplicationResource: %v", errors.ErrorStack(err)))

	// Assert
	var foundAddedAt time.Time
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT added_at FROM kubernetes_application_resource
WHERE resource_uuid = ?`,
			resID).Scan(&foundAddedAt)
	})
	c.Check(foundAddedAt, jc.TimeBetween(addedAt, time.Now()))
	c.Check(err, jc.ErrorIsNil, gc.Commentf("(Assert) kubernetes_application_resource table not updated: %v", errors.ErrorStack(err)))
}

func (s *resourceSuite) TestSetApplicationResourceNotFound(c *gc.C) {
	// Arrange: insert a resource.
	resID := "resource-id"

	// Act set application resource.
	err := s.state.SetApplicationResource(
		context.Background(),
		coreresource.UUID(resID),
	)
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceNotFound)
}

func (s *resourceSuite) TestSetApplicationResourceWithFileTypeResourceError(c *gc.C) {
	// Arrange: insert a resource.
	resID := "resource-id"
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       time.Now().Truncate(time.Second).UTC(),
		Name:            "resource-name",
		Type:            charmresource.TypeFile,
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := input.insert(context.Background(), tx); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act: set application resource.
	err = s.state.SetApplicationResource(
		context.Background(),
		coreresource.UUID(resID),
	)
	c.Assert(err, gc.ErrorMatches, "applications can only be set with container image resources, this resource has type "+charmresource.TypeFile.String())
}

func (s *resourceSuite) TestSetApplicationResourceDoesNothingIfAlreadyExists(c *gc.C) {
	// Arrange: insert the charm resource, the resource and the initial
	// application resource.
	initialTime := time.Now()
	resID := "resource-id"
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       time.Now().Truncate(time.Second).UTC(),
		Name:            "resource-name",
		Type:            charmresource.TypeContainerImage,
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := input.insert(context.Background(), tx); err != nil {
			return errors.Capture(err)
		}
		return nil
	})

	// Set initial application resource.
	err = s.state.SetApplicationResource(
		context.Background(),
		coreresource.UUID(resID),
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to execute SetApplicationResource: %v", errors.ErrorStack(err)))

	// Act: set application resource a second time.
	intbetweenTime := time.Now()
	err = s.state.SetApplicationResource(
		context.Background(),
		coreresource.UUID(resID),
	)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) failed to execute second SetApplicationResource: %v", errors.ErrorStack(err)))

	// Assert: check that the application resource has the original added by
	// time.
	var foundAddedAt time.Time
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT added_at FROM kubernetes_application_resource
WHERE resource_uuid = ?`,
			resID).Scan(&foundAddedAt)
	})
	c.Check(foundAddedAt, jc.TimeBetween(initialTime, intbetweenTime))
	c.Check(err, jc.ErrorIsNil, gc.Commentf("(Assert) kubernetes_application_resource has been unexpectdly updated: %v", errors.ErrorStack(err)))
}

// TestListResourcesNoResources verifies that no resources are listed for an
// application when no resources exist. It checks that the resulting lists for
// unit resources, general resources, and repository resources are all empty.
func (s *resourceSuite) TestListResourcesNoResources(c *gc.C) {
	// Arrange: No resources
	// Act
	results, err := s.state.ListResources(context.Background(), application.ID(s.constants.fakeApplicationUUID1))
	// Assert
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) failed to list resources: %v", errors.ErrorStack(err)))
	c.Assert(results.UnitResources, gc.HasLen, 0)
	c.Assert(results.Resources, gc.HasLen, 0)
	c.Assert(results.RepositoryResources, gc.HasLen, 0)
}

// TestListResources tests the retrieval and organization of resources from the
// database.
func (s *resourceSuite) TestListResources(c *gc.C) {
	// Arrange
	now := time.Now().Truncate(time.Second).UTC()
	// Arrange : Insert several resources
	// - 1 with no unit nor polled
	// - 1 with unit but no polled
	// - 1 with polled but no unit
	// - 1 with polled and unit
	// - 1 not polled and another unit
	simpleRes := resourceData{
		UUID:            "simple-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "simple",
		CreatedAt:       now,
		Type:            charmresource.TypeFile,
	}
	polledRes := resourceData{
		UUID:            "polled-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "polled",
		CreatedAt:       now,
		PolledAt:        now,
		Type:            charmresource.TypeContainerImage,
	}
	unitRes := resourceData{
		UUID:            "unit-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "unit",
		CreatedAt:       now,
		UnitUUID:        s.constants.fakeUnitUUID1,
		AddedAt:         now,
		Type:            charmresource.TypeFile,
	}
	bothRes := resourceData{
		UUID:            "both-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "both",
		UnitUUID:        s.constants.fakeUnitUUID1,
		AddedAt:         now,
		PolledAt:        now,
		Type:            charmresource.TypeContainerImage,
	}
	anotherUnitRes := resourceData{
		UUID:            "another-unit-uuid",
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		Name:            "anotherUnit",
		CreatedAt:       now,
		UnitUUID:        s.constants.fakeUnitUUID2,
		AddedAt:         now,
		Type:            charmresource.TypeFile,
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		for _, input := range []resourceData{simpleRes, polledRes, unitRes, bothRes, anotherUnitRes} {
			if err := input.insert(context.Background(), tx); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v", errors.ErrorStack(err)))

	// Act
	results, err := s.state.ListResources(context.Background(), application.ID(s.constants.fakeApplicationUUID1))

	// Assert
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) failed to list resources: %v", errors.ErrorStack(err)))
	c.Assert(results.UnitResources, gc.DeepEquals, []resource.UnitResources{
		{
			ID: unit.UUID(s.constants.fakeUnitUUID1),
			Resources: []resource.Resource{
				unitRes.toResource(),
				bothRes.toResource(),
			},
		},
		{
			ID: unit.UUID(s.constants.fakeUnitUUID2),
			Resources: []resource.Resource{
				anotherUnitRes.toResource(),
			},
		},
	})
	c.Assert(results.Resources, gc.DeepEquals, []resource.Resource{
		simpleRes.toResource(),
		polledRes.toResource(),
		unitRes.toResource(),
		bothRes.toResource(),
		anotherUnitRes.toResource(),
	})
	c.Assert(results.RepositoryResources, gc.DeepEquals, []charmresource.Resource{
		{}, // not polled
		polledRes.toCharmResource(),
		{}, // not polled
		bothRes.toCharmResource(),
		{}, // not polled
	})
}

func (s *resourceSuite) addResource(c *gc.C, resType charmresource.Type) coreresource.UUID {
	createdAt := time.Now().Truncate(time.Second).UTC()
	resourceUUID := coreresource.UUID("resource-uuid")
	resID := resourceUUID.String()
	input := resourceData{
		UUID:            resID,
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       createdAt,
		Name:            "resource-name",
		Type:            resType,
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := input.insert(context.Background(), tx); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to add resource: %v", errors.ErrorStack(err)))
	return resourceUUID
}

func (s *resourceSuite) createFileResourceAndBlob(c *gc.C) (coreresource.UUID, store.ID) {
	// Arrange: insert a resource.
	createdAt := time.Now().Truncate(time.Second).UTC()
	resID := coreresourcetesting.GenResourceUUID(c)
	input := resourceData{
		UUID:            resID.String(),
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       createdAt,
		Name:            "resource-name",
		Type:            charmresource.TypeFile,
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := input.insert(context.Background(), tx); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to add resource: %v", errors.ErrorStack(err)))

	// Arrange: add a blob to the object store
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)
	storeID := resourcestoretesting.GenFileResourceStoreID(c, objectStoreUUID)
	err = s.addObjectStoreBlobMetadata(objectStoreUUID)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to add object store blob: %v", errors.ErrorStack(err)))

	return resID, storeID
}

func (s *resourceSuite) createContainerImageResourceAndBlob(c *gc.C) (coreresource.UUID, store.ID) {
	// Arrange: insert a resource.
	createdAt := time.Now().Truncate(time.Second).UTC()
	resID := coreresourcetesting.GenResourceUUID(c)
	input := resourceData{
		UUID:            resID.String(),
		ApplicationUUID: s.constants.fakeApplicationUUID1,
		CreatedAt:       createdAt,
		Name:            "resource-name",
		Type:            charmresource.TypeContainerImage,
	}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := input.insert(context.Background(), tx); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to add resource: %v", errors.ErrorStack(err)))

	// Arrange: add a container image using the resource UUID as the storage key.
	storageKey := resID.String()
	storeID := resourcestoretesting.GenContainerImageMetadataResourceID(c, storageKey)
	err = s.addContainerImage(storageKey)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to add container image: %v", errors.ErrorStack(err)))
	return resID, storeID
}

func (s *resourceSuite) addContainerImage(storageKey string) error {
	return s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO resource_container_image_metadata_store (storage_key, registry_path) 
VALUES      (?, 'testing@sha256:beef-deed')`, storageKey)
		return err
	})
}

func (s *resourceSuite) addObjectStoreBlobMetadata(uuid objectstore.UUID) error {

	return s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		// Use the uuid as the hash to avoid uniqueness issues while testing.
		_, err := tx.ExecContext(ctx, `
INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size) VALUES (?, ?, ?, 42)
`, uuid.String(), uuid.String(), uuid.String())
		return err
	})
}

// setWithRetrievedBy sets a resource with the specified retrievedBy and
// retrievedByType.
func (s *resourceSuite) setWithRetrievedBy(
	c *gc.C,
	resourceUUID coreresource.UUID,
	retrievedBy string,
	retrievedByType resource.RetrievedByType,
) error {
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)
	storeID := resourcestoretesting.GenFileResourceStoreID(c, objectStoreUUID)
	err := s.addObjectStoreBlobMetadata(objectStoreUUID)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to add object store blob: %v", errors.ErrorStack(err)))

	return s.state.StoreResource(
		context.Background(),
		resourceUUID,
		storeID,
		retrievedBy,
		retrievedByType,
		false,
	)
}

func (s *resourceSuite) getRetrievedByType(c *gc.C, resourceUUID coreresource.UUID) (retrievedBy string, retrievedByType resource.RetrievedByType) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT rrb.name, rrbt.name AS type
FROM   resource_retrieved_by rrb
JOIN   resource_retrieved_by_type rrbt ON rrb.retrieved_by_type_id = rrbt.id
WHERE  resource_uuid = ?`, resourceUUID.String()).Scan(&retrievedBy, &retrievedByType)
	})
	c.Assert(err, jc.ErrorIsNil)
	return retrievedBy, retrievedByType
}

func (s *resourceSuite) getCharmModifiedVersion(c *gc.C, resID string) int {
	var charmModifiedVersion sql.NullInt64
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT a.charm_modified_version
FROM   application a
JOIN   application_resource ar ON a.uuid = ar.application_uuid
WHERE  ar.resource_uuid = ?`, resID).Scan(&charmModifiedVersion)
	})
	c.Assert(err, jc.ErrorIsNil)
	if charmModifiedVersion.Valid {
		return int(charmModifiedVersion.Int64)
	}
	return 0
}

// resourceData represents a structure containing meta-information about a resource in the system.
type resourceData struct {
	// from resource table
	UUID            string
	ApplicationUUID string
	Name            string
	Revision        int
	// OriginType is a string representing the source type of the resource
	// (should be a valid value from resource_origin_type or empty).
	OriginType string
	// State represents the current state of the resource (should be a valid
	// value resource_state or empty)
	State     string
	CreatedAt time.Time
	PolledAt  time.Time
	// RetrievedByType should be a valid value from resource_supplied_by_type.
	RetrievedByType string
	RetrievedByName string
	Type            charmresource.Type
	Path            string
	Description     string
	UnitUUID        string
	AddedAt         time.Time
}

// toCharmResource converts a resourceData object to a charmresource.Resource object.
func (d resourceData) toCharmResource() charmresource.Resource {
	return charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        d.Name,
			Type:        d.Type,
			Path:        d.Path,
			Description: d.Description,
		},
		Origin:   charmresource.Origin(OriginTypeID(d.OriginType)),
		Revision: d.Revision,
		// todo(gfouillet): deal with fingerprint & size
		Fingerprint: charmresource.Fingerprint{},
		Size:        0,
	}
}

// toResource converts a resourceData object to a resource.Resource object with
// enriched metadata.
func (d resourceData) toResource() resource.Resource {
	return resource.Resource{
		Resource:        d.toCharmResource(),
		UUID:            coreresource.UUID(d.UUID),
		ApplicationID:   application.ID(d.ApplicationUUID),
		RetrievedBy:     d.RetrievedByName,
		RetrievedByType: resource.RetrievedByType(d.RetrievedByType),
		Timestamp:       d.CreatedAt,
	}
}

// DeepCopy creates a deep copy of the resourceData instance and returns it.
func (d resourceData) DeepCopy() resourceData {
	result := d
	return result
}

// insert inserts the resource data into multiple related tables within a transaction.
// It populates charm_resource, resource, application_resource,
// resource_retrieved_by (if necessary), and unit_resource (if required).
func (input resourceData) insert(ctx context.Context, tx *sql.Tx) (err error) {
	//  Populate resource table
	nilZeroTime := func(t time.Time) *time.Time {
		if t.IsZero() {
			return nil
		}
		return &t
	}
	// Populate charm_resource table. Don't recreate the charm resource if it
	// already exists.
	_, err = tx.Exec(`
INSERT INTO charm_resource (charm_uuid, name, kind_id, path, description) 
VALUES (?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		fakeCharmUUID, input.Name, TypeID(input.Type), nilZero(input.Path), nilZero(input.Description))
	if err != nil {
		return errors.Capture(err)
	}

	// Populate resource table
	_, err = tx.Exec(`
INSERT INTO resource (uuid, charm_uuid, charm_resource_name, revision, origin_type_id, state_id, created_at, last_polled) 
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, input.UUID, fakeCharmUUID, input.Name, nilZero(input.Revision),
		OriginTypeID(input.OriginType), StateID(input.State), input.CreatedAt, nilZeroTime(input.PolledAt),
	)
	if err != nil {
		return errors.Capture(err)
	}

	// Populate application_resource table
	_, err = tx.Exec(`
INSERT INTO application_resource (resource_uuid, application_uuid) 
VALUES (?, ?)`, input.UUID, input.ApplicationUUID)
	if err != nil {
		return errors.Capture(err)
	}

	// Populate resource_retrieved_by table of necessary
	if input.RetrievedByName != "" {
		_, err = tx.Exec(`
INSERT INTO resource_retrieved_by (resource_uuid, retrieved_by_type_id, name) 
VALUES (?, ?, ?)`, input.UUID, RetrievedByTypeID(input.RetrievedByType), input.RetrievedByName)
		if err != nil {
			return errors.Capture(err)
		}
	}

	// Populate unit resource if required
	if input.UnitUUID != "" {
		_, err = tx.Exec(`
INSERT INTO unit_resource (resource_uuid, unit_uuid, added_at) 
VALUES (?, ?, ?)`, input.UUID, input.UnitUUID, input.AddedAt)
		return errors.Capture(err)
	}

	return
}

// nilZero returns a pointer to the input value unless the value is its type's
// zero value, in which case it returns nil.
func nilZero[T comparable](t T) *T {
	var zero T
	if t == zero {
		return nil
	}
	return &t
}

// zeroPtr returns the value pointed to by t or the zero value if the pointer is
// nil.
func zeroPtr[T comparable](t *T) T {
	var zero T
	if t == nil {
		return zero
	}
	return *t
}

// RetrievedByTypeID maps the RetrievedByType string to an integer ID based on
// predefined categories.
func RetrievedByTypeID(RetrievedByType string) int {
	res, _ := map[string]int{
		"user":        0,
		"unit":        1,
		"application": 2,
	}[RetrievedByType]
	return res
}

// TypeID returns the integer ID corresponding to the resource kind stored in d.Type.
func TypeID(Kind charmresource.Type) int {
	res, _ := map[charmresource.Type]int{
		charmresource.TypeFile:           0,
		charmresource.TypeContainerImage: 1,
	}[Kind]
	return res
}

// OriginTypeID maps the OriginType string to its corresponding integer ID
// based on predefined categories.
func OriginTypeID(OriginType string) int {
	res, _ := map[string]int{
		"upload": 0,
		"store":  1,
	}[OriginType]
	return res
}

// StateID returns the integer ID corresponding to the state stored in d.State.
func StateID(State string) int {
	res, _ := map[string]int{
		"available": 0,
		"potential": 1,
	}[State]
	return res
}
