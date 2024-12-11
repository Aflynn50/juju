// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/collections/set"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coreresource "github.com/juju/juju/core/resource"
	coreresourcestore "github.com/juju/juju/core/resource/store"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

type State struct {
	*domain.StateBase
	clock  clock.Clock
	logger logger.Logger
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory, clock clock.Clock, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		clock:     clock,
		logger:    logger,
	}
}

// DeleteApplicationResources deletes all resources associated with a given
// application ID. It checks that resources are not linked to a file store,
// image store, or unit before deletion.
// The method uses several SQL statements to prepare and execute the deletion
// process within a transaction. If related records are found in any store,
// deletion is halted and an error is returned, preventing any deletion which
// can led to inconsistent state due to foreign key constraints.
func (st *State) DeleteApplicationResources(
	ctx context.Context,
	applicationID application.ID,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	type uuids []string
	appIdentity := resourceIdentity{ApplicationUUID: applicationID.String()}

	// SQL statement to list all resources for an application.
	listAppResourcesStmt, err := st.Prepare(`
SELECT resource_uuid AS &resourceIdentity.uuid 
FROM application_resource 
WHERE application_uuid = $resourceIdentity.application_uuid`, appIdentity)
	if err != nil {
		return errors.Capture(err)
	}

	// SQL statement to check there is no related resources in resource_file_store.
	noFileStoreStmt, err := st.Prepare(`
SELECT resource_uuid AS &resourceIdentity.uuid 
FROM resource_file_store
WHERE resource_uuid IN ($uuids[:])`, resourceIdentity{}, uuids{})
	if err != nil {
		return errors.Capture(err)
	}

	// SQL statement to check there is no related resources in resource_image_store.
	noImageStoreStmt, err := st.Prepare(`
SELECT resource_uuid AS &resourceIdentity.uuid 
FROM resource_image_store
WHERE resource_uuid IN ($uuids[:])`, resourceIdentity{}, uuids{})
	if err != nil {
		return errors.Capture(err)
	}

	// SQL statement to check there is no related resources in unit_resource.
	noUnitResourceStmt, err := st.Prepare(`
SELECT resource_uuid AS &resourceIdentity.uuid 
FROM unit_resource
WHERE resource_uuid IN ($uuids[:])`, resourceIdentity{}, uuids{})
	if err != nil {
		return errors.Capture(err)
	}

	// SQL statement to delete resources from resource_retrieved_by.
	deleteFromRetrievedByStmt, err := st.Prepare(`
DELETE FROM resource_retrieved_by
WHERE resource_uuid IN ($uuids[:])`, uuids{})
	if err != nil {
		return errors.Capture(err)
	}

	// SQL statement to delete resources from application_resource.
	deleteFromApplicationResourceStmt, err := st.Prepare(`
DELETE FROM application_resource
WHERE resource_uuid IN ($uuids[:])`, uuids{})
	if err != nil {
		return errors.Capture(err)
	}

	// SQL statement to delete resources from resource.
	deleteFromResourceStmt, err := st.Prepare(`
DELETE FROM resource
WHERE uuid IN ($uuids[:])`, uuids{})
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) (err error) {
		// list all resources for an application.
		var resources []resourceIdentity
		err = tx.Query(ctx, listAppResourcesStmt, appIdentity).GetAll(&resources)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		resUUIDs := make(uuids, 0, len(resources))
		for _, res := range resources {
			resUUIDs = append(resUUIDs, res.UUID)
		}

		checkLink := func(message string, stmt *sqlair.Statement) error {
			var resources []resourceIdentity
			err := tx.Query(ctx, stmt, resUUIDs).GetAll(&resources)
			switch {
			case errors.Is(err, sqlair.ErrNoRows): // Happy path
				return nil
			case err != nil:
				return err
			}
			return errors.Errorf("%s: %w", message, resourceerrors.CleanUpStateNotValid)
		}

		// check there are no related resources in resource_file_store.
		if err = checkLink("resource linked to file store data", noFileStoreStmt); err != nil {
			return errors.Capture(err)
		}

		// check there are no related resources in resource_image_store.
		if err = checkLink("resource linked to image store data", noImageStoreStmt); err != nil {
			return errors.Capture(err)
		}

		// check there are no related resources in unit_resource.
		if err = checkLink("resource linked to unit", noUnitResourceStmt); err != nil {
			return errors.Capture(err)
		}

		// delete resources from resource_retrieved_by.
		if err = tx.Query(ctx, deleteFromRetrievedByStmt, resUUIDs).Run(); err != nil {
			return errors.Capture(err)
		}

		safedelete := func(stmt *sqlair.Statement) error {
			var outcome sqlair.Outcome
			err = tx.Query(ctx, stmt, resUUIDs).Get(&outcome)
			if err != nil {
				return errors.Capture(err)
			}
			num, err := outcome.Result().RowsAffected()
			if err != nil {
				return errors.Capture(err)
			}
			if num != int64(len(resUUIDs)) {
				return errors.Errorf("expected %d rows to be deleted, got %d", len(resUUIDs), num)
			}
			return nil
		}

		// delete resources from application_resource.
		err = safedelete(deleteFromApplicationResourceStmt)
		if err != nil {
			return errors.Capture(err)
		}

		// delete resources from resource.
		return safedelete(deleteFromResourceStmt)
	})
}

// DeleteUnitResources removes the association of a unit, identified by UUID,
// with any of its' application's resources. It initiates a transaction and
// executes an SQL statement to delete rows from the unit_resource table.
// Returns an error if the operation fails at any point in the process.
func (st *State) DeleteUnitResources(
	ctx context.Context,
	uuid coreunit.UUID,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	unit := unitResource{UnitUUID: uuid.String()}
	stmt, err := st.Prepare(`DELETE FROM unit_resource WHERE unit_uuid = $unitResource.unit_uuid`, unit)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Capture(tx.Query(ctx, stmt, unit).Run())
	})
}

// GetResourceUUID returns the UUID of the resource specified by natural key of
// application and resource name.
func (st *State) GetResourceUUID(
	ctx context.Context,
	applicationID application.ID,
	name string,
) (coreresource.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	// Define the resource identity based on the provided application ID and
	// name.
	resource := resourceIdentity{
		ApplicationUUID: applicationID.String(),
		Name:            name,
	}

	// Prepare the SQL statement to retrieve the resource UUID.
	stmt, err := st.Prepare(`
SELECT uuid as &resourceIdentity.uuid 
FROM v_application_resource
WHERE name = $resourceIdentity.name 
AND application_uuid = $resourceIdentity.application_uuid
`, resource)
	if err != nil {
		return "", errors.Capture(err)
	}

	// Execute the SQL transaction.
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, resource).Get(&resource)
		if errors.Is(err, sqlair.ErrNoRows) {
			return resourceerrors.ResourceNotFound
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return coreresource.UUID(resource.UUID), nil
}

// ListResources returns the list of resource for the given application.
func (st *State) ListResources(
	ctx context.Context,
	applicationID application.ID,
) (resource.ApplicationResources, error) {
	db, err := st.DB()
	if err != nil {
		return resource.ApplicationResources{}, errors.Capture(err)
	}

	// Prepare the application ID to query resources by application.
	appID := resourceIdentity{
		ApplicationUUID: applicationID.String(),
	}

	// Prepare the statement to get resources for the given application.
	getResourcesQuery := `
SELECT &resourceView.* 
FROM v_resource
WHERE application_uuid = $resourceIdentity.application_uuid`
	getResourcesStmt, err := st.Prepare(getResourcesQuery, appID, resourceView{})
	if err != nil {
		return resource.ApplicationResources{}, errors.Capture(err)
	}

	// Prepare the statement to check if a resource has been polled.
	checkPolledQuery := `
SELECT &resourceIdentity.uuid 
FROM v_application_resource
WHERE application_uuid = $resourceIdentity.application_uuid
AND uuid = $resourceIdentity.uuid
AND last_polled IS NOT NULL`
	checkPolledStmt, err := st.Prepare(checkPolledQuery, appID)
	if err != nil {
		return resource.ApplicationResources{}, errors.Capture(err)
	}

	// Prepare the statement to get units related to a resource.
	getUnitsQuery := `
SELECT &unitResource.*
FROM unit_resource
WHERE unit_resource.resource_uuid = $resourceIdentity.uuid`
	getUnitStmt, err := st.Prepare(getUnitsQuery, appID, unitResource{})
	if err != nil {
		return resource.ApplicationResources{}, errors.Capture(err)
	}

	var result resource.ApplicationResources
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) (err error) {
		// Map to hold unit-specific resources
		resByUnit := map[coreunit.UUID]resource.UnitResources{}
		// resource found for the application
		var resources []resourceView

		// Query to get all resources for the given application.
		err = tx.Query(ctx, getResourcesStmt, appID).GetAll(&resources)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil // nothing found
		}
		if err != nil {
			return errors.Capture(err)
		}

		// Process each resource from the application to check polled state
		// and if they are associated with a unit.
		for _, res := range resources {
			resId := resourceIdentity{UUID: res.UUID, ApplicationUUID: res.ApplicationUUID}

			// Check to see if the resource has already been polled.
			err = tx.Query(ctx, checkPolledStmt, resId).Get(&resId)
			if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Capture(err)
			}
			hasBeenPolled := !errors.Is(err, sqlair.ErrNoRows)

			// Fetch units related to the resource.
			var units []unitResource
			err = tx.Query(ctx, getUnitStmt, resId).GetAll(&units)
			if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Capture(err)
			}

			r, err := res.toResource()
			if err != nil {
				return errors.Capture(err)
			}
			// Add each resource.
			result.Resources = append(result.Resources, r)

			// Add the charm resource or an empty one,
			// depending ons polled status.
			charmRes := charmresource.Resource{}
			if hasBeenPolled {
				charmRes, err = res.toCharmResource()
				if err != nil {
					return errors.Capture(err)
				}
			}
			result.RepositoryResources = append(result.RepositoryResources, charmRes)

			// Sort by unit to generate unit resources.
			for _, unit := range units {
				unitRes, ok := resByUnit[coreunit.UUID(unit.UnitUUID)]
				if !ok {
					unitRes = resource.UnitResources{ID: coreunit.UUID(unit.UnitUUID)}
				}
				ur, err := res.toResource()
				if err != nil {
					return errors.Capture(err)
				}
				unitRes.Resources = append(unitRes.Resources, ur)
				resByUnit[coreunit.UUID(unit.UnitUUID)] = unitRes
			}
		}
		// Collect and sort unit resources.
		units := slices.SortedFunc(maps.Values(resByUnit), func(r1, r2 resource.UnitResources) int {
			return strings.Compare(r1.ID.String(), r2.ID.String())
		})
		result.UnitResources = append(result.UnitResources, units...)

		return nil
	})

	// Return the list of application resources along with unit resources.
	return result, errors.Capture(err)
}

// GetResource returns the identified resource.
// Returns a [resourceerrors.ResourceNotFound] if no such resource exists.
func (st *State) GetResource(ctx context.Context,
	resourceUUID coreresource.UUID) (resource.Resource, error) {
	db, err := st.DB()
	if err != nil {
		return resource.Resource{}, errors.Capture(err)
	}
	resourceParam := resourceIdentity{
		UUID: resourceUUID.String(),
	}
	resourceOutput := resourceView{}

	stmt, err := st.Prepare(`
SELECT &resourceView.*
FROM v_resource
WHERE uuid = $resourceIdentity.uuid`,
		resourceParam, resourceOutput)
	if err != nil {
		return resource.Resource{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, resourceParam).Get(&resourceOutput)
		if errors.Is(err, sqlair.ErrNoRows) {
			return resourceerrors.ResourceNotFound
		}
		return errors.Capture(err)
	})
	if err != nil {
		return resource.Resource{}, errors.Capture(err)
	}

	return resourceOutput.toResource()
}

// StoreResource records a stored resource along with who retrieved it.
// Returns [resourceerrors.ResourceNotFound] if the resource UUID cannot be found.
// Returns [resourceerrors.StoredResourceNotFound] if the stored resource at the
// storageID cannot be found.
// Returns [resourceerrors.ResourceAlreadyStored] if the resource is already
// associated with a stored resource blob.
// Returns [resourceerrors.RetrievedByTypeNotValid] if the retrieved by type is
// invalid.
func (st *State) StoreResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
	storageID coreresourcestore.ID,
	retrievedBy string,
	retrievedByType resource.RetrievedByType,
	incrementCharmModifiedVersion bool,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		kind, err := st.getResourceType(ctx, tx, resourceUUID)
		if err != nil {
			return errors.Errorf("getting resource type: %w", err)
		}

		switch kind {
		case charmresource.TypeFile:
			err = st.insertFileResource(ctx, tx, resourceUUID, storageID)
			if err != nil {
				return errors.Errorf("inserting stored file resource information: %w", err)
			}
		case charmresource.TypeContainerImage:
			err = st.insertImageResource(ctx, tx, resourceUUID, storageID)
			if err != nil {
				return errors.Errorf("inserting stored container image resource information: %w", err)
			}
		default:
			return errors.Errorf("unknown resource type: %q", kind.String())
		}
		if err != nil {
			return errors.Capture(err)
		}

		if retrievedBy != "" {
			err := st.insertRetrievedBy(ctx, tx, resourceUUID, retrievedBy, retrievedByType)
			if err != nil {
				return errors.Errorf("inserting retrieval information for resource %s: %w", resourceUUID, err)
			}
		}

		if incrementCharmModifiedVersion {
			err := st.incrementCharmModifiedVersion(ctx, tx, resourceUUID)
			if err != nil {
				return errors.Errorf("inserting retrieval information for resource %s: %w", resourceUUID, err)
			}
		}

		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// getResourceType finds the type of the given resource from the resource table.
func (st *State) getResourceType(
	ctx context.Context,
	tx *sqlair.TX,
	resourceUUID coreresource.UUID,
) (charmresource.Type, error) {
	resKind := resourceKind{
		UUID: resourceUUID.String(),
	}
	getResourceType, err := st.Prepare(`
SELECT crk.name AS &resourceKind.name
FROM resource AS r
JOIN application_resource AS ar ON r.uuid = ar.resource_uuid
JOIN charm_resource AS cr ON r.charm_uuid = cr.charm_uuid AND r.charm_resource_name = cr.name
JOIN charm_resource_kind AS crk ON cr.kind_id = crk.id
WHERE r.uuid = $resourceKind.uuid
`, resKind)
	if err != nil {
		return 0, errors.Capture(err)
	}

	err = tx.Query(ctx, getResourceType, resKind).Get(&resKind)
	if errors.Is(err, sqlair.ErrNoRows) {
		return 0, resourceerrors.ResourceNotFound
	} else if err != nil {
		return 0, errors.Capture(err)
	}

	kind, err := charmresource.ParseType(resKind.Name)
	if err != nil {
		return 0, errors.Errorf("parsing resource kind: %w", err)
	}
	return kind, err
}

// insertFileResource checks that the storage ID corresponds to stored object
// store metadata and then records that the resource is stored at the provided
// storage ID.
func (st *State) insertFileResource(
	ctx context.Context,
	tx *sqlair.TX,
	resourceUUID coreresource.UUID,
	storageID coreresourcestore.ID,
) error {
	// Get the object store UUID of the stored resource blob.
	if !storageID.IsObjectStoreUUID() {
		return errors.Errorf("cannot insert file resource that is not stored in the object store")
	}
	uuid, err := storageID.ObjectStoreUUID()
	if err != nil {
		return errors.Capture(err)
	}

	// Check the resource blob is stored in the object store.
	storedResource := storedFileResource{
		ResourceUUID:    resourceUUID.String(),
		ObjectStoreUUID: uuid.String(),
	}
	checkObjectStoreMetadataStmt, err := st.Prepare(`
SELECT uuid AS &storedFileResource.store_uuid
FROM object_store_metadata
WHERE uuid = $storedFileResource.store_uuid
`, storedResource)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, checkObjectStoreMetadataStmt, storedResource).Get(&storedResource)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("checking object store for resource %s: %w", resourceUUID, resourceerrors.StoredResourceNotFound)
	} else if err != nil {
		return errors.Errorf("checking object store for resource %s: %w", resourceUUID, err)
	}

	// Check if the resource has already been stored.
	checkResourceFileStoreStmt, err := st.Prepare(`
SELECT &storedFileResource.*
FROM   resource_file_store
WHERE  resource_uuid = $storedFileResource.resource_uuid
`, storedResource)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, checkResourceFileStoreStmt, storedResource).Get(&storedResource)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("checking if resource %s already stored: %w", resourceUUID, err)
	} else if err == nil {
		// If a row was found, return that the resource is already stored.
		return resourceerrors.ResourceAlreadyStored
	}

	// Record where the resource is stored.
	insertStoredResourceStmt, err := st.Prepare(`
INSERT INTO resource_file_store (*)
VALUES ($storedFileResource.*)
`, storedResource)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, insertStoredResourceStmt, storedResource).Get(&outcome)
	if err != nil {
		return errors.Errorf("resource %s: %w", resourceUUID, err)
	}

	if rows, err := outcome.Result().RowsAffected(); err != nil {
		return errors.Capture(err)
	} else if rows != 1 {
		return errors.Errorf("expected 1 row to be inserted, got %d", rows)
	}

	return nil
}

// insertImageResource checks that the storage ID corresponds to stored
// container image store metadata and then records that the resource is stored
// at the provided storage ID.
func (st *State) insertImageResource(
	ctx context.Context,
	tx *sqlair.TX,
	resourceUUID coreresource.UUID,
	storageID coreresourcestore.ID,
) error {
	// Get the container image metadata storage key.
	if !storageID.IsContainerImageMetadataID() {
		return errors.Errorf("cannot insert container image metadata resource that is not stored in the container image metadata store")
	}
	storageKey, err := storageID.ContainerImageMetadataStoreID()
	if err != nil {
		return errors.Capture(err)
	}

	// Check the resource is stored in the container image metadata store.
	storedResource := storedContainerImageResource{
		ResourceUUID: resourceUUID.String(),
		StorageKey:   storageKey,
	}
	checkContainerImageStoreStmt, err := st.Prepare(`
SELECT storage_key AS &storedContainerImageResource.store_storage_key
FROM resource_container_image_metadata_store
WHERE storage_key = $storedContainerImageResource.store_storage_key
`, storedResource)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, checkContainerImageStoreStmt, storedResource).Get(&storedResource)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("checking container image metadata store for resource %s: %w", resourceUUID, resourceerrors.StoredResourceNotFound)
	} else if err != nil {
		return errors.Errorf("checking container image metadata store for resource %s: %w", resourceUUID, err)
	}

	// Check if the resource has already been stored.
	checkResourceImageStoreStmt, err := st.Prepare(`
SELECT &storedContainerImageResource.*
FROM   resource_image_store
WHERE  resource_uuid = $storedContainerImageResource.resource_uuid
`, storedResource)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, checkResourceImageStoreStmt, storedResource).Get(&storedResource)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("checking if resource %s already stored: %w", resourceUUID, err)
	} else if err == nil {
		// If a row was found, return that the resource is already stored.
		return resourceerrors.ResourceAlreadyStored
	}

	// Record where the resource is stored.
	insertStoredResourceStmt, err := st.Prepare(`
INSERT INTO resource_image_store (*)
VALUES ($storedContainerImageResource.*)
`, storedResource)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, insertStoredResourceStmt, storedResource).Get(&outcome)
	if err != nil {
		return errors.Errorf("resource %s: %w", resourceUUID, err)
	}

	if rows, err := outcome.Result().RowsAffected(); err != nil {
		return errors.Capture(err)
	} else if rows != 1 {
		return errors.Errorf("expected 1 row to be inserted, got %d", rows)
	}

	return nil
}

// insertRetrievedBy updates the retrieved by table to record who retrieved the currently stored resource.
// in the retrieved_by table, and if not, adds the given retrieved by name and
// type.
func (st *State) insertRetrievedBy(
	ctx context.Context,
	tx *sqlair.TX,
	resourceUUID coreresource.UUID,
	retrievedBy string,
	retrievedByType resource.RetrievedByType,
) error {
	// Verify if the resource has already been retrieved.
	resID := resourceIdentity{UUID: resourceUUID.String()}
	checkAlreadyRetrievedQuery := `
SELECT resource_uuid AS &resourceIdentity.uuid 
FROM   resource_retrieved_by
WHERE  resource_uuid = $resourceIdentity.uuid`
	checkAlreadyRetrievedStmt, err := st.Prepare(checkAlreadyRetrievedQuery, resID)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, checkAlreadyRetrievedStmt, resID).Get(&resID)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Capture(err)
	} else if err == nil {
		// If the resource has already been retrieved, the return an error.
		return resourceerrors.ResourceAlreadyStored
	}

	// Get the retrieved by type ID.
	type getRetrievedByType struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}
	retrievedTypeParam := getRetrievedByType{Name: string(retrievedByType)}
	getRetrievedByTypeStmt, err := st.Prepare(`
SELECT &getRetrievedByType.* 
FROM   resource_retrieved_by_type 
WHERE  name = $getRetrievedByType.name`, retrievedTypeParam)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, getRetrievedByTypeStmt, retrievedTypeParam).Get(&retrievedTypeParam)
	if errors.Is(err, sqlair.ErrNoRows) {
		return resourceerrors.RetrievedByTypeNotValid
	}
	if err != nil {
		return errors.Capture(err)
	}

	// Insert retrieved by.
	type setRetrievedBy struct {
		ResourceUUID      string `db:"resource_uuid"`
		RetrievedByTypeID int    `db:"retrieved_by_type_id"`
		Name              string `db:"name"`
	}
	retrievedByParam := setRetrievedBy{
		ResourceUUID:      resourceUUID.String(),
		Name:              retrievedBy,
		RetrievedByTypeID: retrievedTypeParam.ID,
	}
	insertRetrievedByStmt, err := st.Prepare(`
INSERT INTO resource_retrieved_by (resource_uuid, retrieved_by_type_id, name)
VALUES      ($setRetrievedBy.*)`, retrievedByParam)
	if err != nil {
		return errors.Capture(err)
	}

	return errors.Capture(tx.Query(ctx, insertRetrievedByStmt, retrievedByParam).Run())
}

// incrementCharmModifiedVersion increments the charm modified version on any
// application associated with a resource.
func (st *State) incrementCharmModifiedVersion(ctx context.Context, tx *sqlair.TX, resourceUUID coreresource.UUID) error {
	resID := resourceIdentity{UUID: resourceUUID.String()}
	updateCharmModifiedVersionStmt, err := st.Prepare(`
UPDATE application
SET    charm_modified_version = IFNULL(charm_modified_version ,0) + 1
WHERE  uuid IN (
    SELECT application_uuid
    FROM   application_resource
    WHERE  resource_uuid = $resourceIdentity.uuid
)
`, resID)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, updateCharmModifiedVersionStmt, resID).Get(&outcome)
	if err != nil {
		return errors.Errorf("updating charm modified version: %w", err)
	}

	rows, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Capture(err)
	} else if rows < 1 {
		return errors.Errorf("updating charm modified version: expected more than 1 row affected, got %d", rows)
	}

	type test struct {
		CMV int `db:"charm_modified_version"`
	}
	var t test

	stmt, err := st.Prepare(`
SELECT &test.charm_modified_version
FROM   application a
JOIN   application_resource ar ON ar.application_uuid = a.uuid
WHERE  resource_uuid = $resourceIdentity.uuid
`, resID, t)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, resID).Get(&t)
	if err != nil {
		return errors.Errorf("updating charm modified version: %w", err)
	}
	st.logger.Criticalf("cmv is %d", t.CMV)

	return nil
}

// SetApplicationResource marks an existing resource as in use by a CAAS
// application.
// Returns [resourceerrors.ResourceNotFound] if the resource UUID cannot be
// found.
func (st *State) SetApplicationResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to check if the unit/resource is not already there.
	k8sAppResource := kubernetesApplicationResource{
		ResourceUUID: resourceUUID.String(),
		AddedAt:      st.clock.Now(),
	}
	checkK8sAppResourceAlreadyExistsStmt, err := st.Prepare(`
SELECT &kubernetesApplicationResource.*
FROM   kubernetes_application_resource
WHERE  kubernetes_application_resource.resource_uuid = $kubernetesApplicationResource.resource_uuid
`, k8sAppResource)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to insert a new link between unit and resource.
	insertK8sAppResourceStmt, err := st.Prepare(`
INSERT INTO kubernetes_application_resource (resource_uuid, added_at)
VALUES      ($kubernetesApplicationResource.*)
`, k8sAppResource)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check unit resource is not already inserted.
		err := tx.Query(ctx, checkK8sAppResourceAlreadyExistsStmt, k8sAppResource).Get(&k8sAppResource)
		if err == nil {
			// If the kubernetes application resource already exists, do nothing
			// and return.
			return nil
		}
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}

		// Check that this resource exists and is a container image resource.
		// Only container image resources can be set as kubernetes application
		// resource.
		resourceType, err := st.getResourceType(ctx, tx, resourceUUID)
		if err != nil {
			return errors.Capture(err)
		} else if resourceType != charmresource.TypeContainerImage {
			return errors.Errorf(
				"applications can only be set with container image resources, this resource has type %s",
				resourceType.String(),
			)
		}

		// Update kubernetes application resource table.
		var outcome sqlair.Outcome
		err = tx.Query(ctx, insertK8sAppResourceStmt, k8sAppResource).Get(&outcome)
		if err != nil {
			return errors.Capture(err)
		}

		// Validate that a single row was inserted.
		rows, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Capture(err)
		} else if rows != 1 {
			return errors.Errorf("inserting kubernetes application resource: expected 1 row affected, got %d", rows)
		}
		return nil
	})

	return err
}

// SetUnitResource sets the resource metadata for a specific unit.
// Returns [resourceerrors.UnitNotFound] if the unit id doesn't belong to an
// existing unit.
// Returns [resourceerrors.ResourceNotFound] if the resource id doesn't belong
// to an existing resource.
func (st *State) SetUnitResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
	unitUUID coreunit.UUID,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to check if the unit/resource is not already there.
	unitResourceInput := unitResource{
		ResourceUUID: resourceUUID.String(),
		UnitUUID:     unitUUID.String(),
		AddedAt:      st.clock.Now(),
	}
	checkUnitResourceQuery := `
SELECT &unitResource.*
FROM   unit_resource 
WHERE  unit_resource.resource_uuid = $unitResource.resource_uuid 
AND    unit_resource.unit_uuid = $unitResource.unit_uuid`
	checkUnitResourceStmt, err := st.Prepare(checkUnitResourceQuery, unitResourceInput)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to check that UnitUUID is valid UUID.
	uUUID := unitNameAndUUID{UnitUUID: unitUUID}
	checkValidUnitQuery := `
SELECT &unitNameAndUUID.uuid 
FROM   unit 
WHERE  uuid = $unitNameAndUUID.uuid`
	checkValidUnitStmt, err := st.Prepare(checkValidUnitQuery, uUUID)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to insert a new link between unit and resource.
	insertUnitResourceQuery := `
INSERT INTO unit_resource (unit_uuid, resource_uuid, added_at)
VALUES      ($unitResource.*)`
	insertUnitResourceStmt, err := st.Prepare(insertUnitResourceQuery, unitResourceInput)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check unit resource is not already inserted.
		err := tx.Query(ctx, checkUnitResourceStmt, unitResourceInput).Get(&unitResourceInput)
		if err == nil {
			return nil // nothing to do
		}
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}

		// Check the resource exists and is a container image resource.
		// Only container image resources can be set as kubernetes application
		// resource.
		resourceType, err := st.getResourceType(ctx, tx, resourceUUID)
		if err != nil {
			return errors.Capture(err)
		} else if resourceType != charmresource.TypeFile {
			return errors.Errorf("units can only be set with file resources, this resource has type %s",
				resourceType.String(),
			)
		}

		// Check unit exists.
		err = tx.Query(ctx, checkValidUnitStmt, uUUID).Get(&uUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("resource %s: %w", uUUID.UnitUUID, resourceerrors.UnitNotFound)
		}
		if err != nil {
			return errors.Capture(err)
		}

		// update unit resource table.
		err = tx.Query(ctx, insertUnitResourceStmt, unitResourceInput).Run()
		return errors.Capture(err)
	})

	return err
}

// OpenApplicationResource returns the metadata for a resource.
func (st *State) OpenApplicationResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
) (resource.Resource, error) {
	return resource.Resource{}, nil
}

// OpenUnitResource returns the metadata for a resource. A unit
// resource is created to track the given unit and which resource
// its using.
func (st *State) OpenUnitResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
	unitID coreunit.UUID,
) (resource.Resource, error) {
	return resource.Resource{}, nil
}

// SetRepositoryResources sets the "polled" resources for the
// application to the provided values. The current data for this
// application/resource combination will be overwritten.
// Returns [resourceerrors.ApplicationNotFound] if the application id doesn't belong to a valid application.
func (st *State) SetRepositoryResources(
	ctx context.Context,
	config resource.SetRepositoryResourcesArgs,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to check that the application exists.
	appNameAndID := applicationNameAndID{
		ApplicationID: config.ApplicationID,
	}
	getAppNameQuery := `
SELECT name as &applicationNameAndID.name 
FROM application 
WHERE uuid = $applicationNameAndID.uuid
`
	getAppNameStmt, err := st.Prepare(getAppNameQuery, appNameAndID)
	if err != nil {
		return errors.Capture(err)
	}

	type resourceNames []string
	// Prepare statement to get impacted resources UUID.
	fetchResIdentity := resourceIdentity{ApplicationUUID: config.ApplicationID.String()}
	fetchUUIDsQuery := `
SELECT uuid as &resourceIdentity.uuid
FROM v_application_resource
WHERE  application_uuid = $resourceIdentity.application_uuid
AND name IN ($resourceNames[:])
`
	fetchUUIDsStmt, err := st.Prepare(fetchUUIDsQuery, fetchResIdentity, resourceNames{})
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to update lastPolled value.
	type lastPolledResource struct {
		UUID       string    `db:"uuid"`
		LastPolled time.Time `db:"last_polled"`
	}
	updateLastPolledQuery := `
UPDATE resource 
SET last_polled=$lastPolledResource.last_polled
WHERE uuid = $lastPolledResource.uuid
`
	updateLastPolledStmt, err := st.Prepare(updateLastPolledQuery, lastPolledResource{})
	if err != nil {
		return errors.Capture(err)
	}

	names := make([]string, 0, len(config.Info))
	for _, info := range config.Info {
		names = append(names, info.Name)
	}
	var resIdentities []resourceIdentity
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check application exists.
		err := tx.Query(ctx, getAppNameStmt, appNameAndID).Get(&appNameAndID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return resourceerrors.ApplicationNotFound
		}
		if err != nil {
			return errors.Capture(err)
		}

		// Fetch resources UUID.
		err = tx.Query(ctx, fetchUUIDsStmt, resourceNames(names), fetchResIdentity).GetAll(&resIdentities)
		if !errors.Is(err, sqlair.ErrNoRows) && err != nil {
			return errors.Capture(err)
		}

		if len(resIdentities) != len(names) {
			foundResources := set.NewStrings()
			for _, res := range resIdentities {
				foundResources.Add(res.Name)
			}
			st.logger.Errorf("Resource not found for application %s (%s), missing: %s",
				appNameAndID.Name, config.ApplicationID, set.NewStrings(names...).Difference(foundResources).Values())
		}

		// Update last polled resources.
		for _, res := range resIdentities {
			err := tx.Query(ctx, updateLastPolledStmt, lastPolledResource{
				UUID:       res.UUID,
				LastPolled: config.LastPolled,
			}).Run()
			if err != nil {
				return errors.Capture(err)
			}
		}

		return nil
	})
	return errors.Capture(err)
}
