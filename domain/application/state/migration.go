// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/internal/errors"
)

// ExportApplications returns all the applications in the model.
func (st *State) GetApplicationsForExport(ctx context.Context) ([]application.ExportApplication, error) {
	db, err := st.DB()
	if err != nil {
		return nil, err
	}

	var app exportApplication
	query := `SELECT &exportApplication.* FROM v_application_export`
	stmt, err := st.Prepare(query, app)
	if err != nil {
		return nil, err
	}

	var (
		modelType model.ModelType
		apps      []exportApplication
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		modelType, err = st.getModelType(ctx, tx)
		if err != nil {
			return err
		}

		err := tx.Query(ctx, stmt).GetAll(&apps)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("failed to get applications for export: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	exportApps := make([]application.ExportApplication, len(apps))
	for i, app := range apps {
		locator, err := decodeCharmLocator(charmLocator{
			ReferenceName:  app.CharmReferenceName,
			Revision:       app.CharmRevision,
			SourceID:       app.CharmSourceID,
			ArchitectureID: app.CharmArchitectureID,
		})
		if err != nil {
			return nil, err
		}

		var providerID *string
		if app.K8sServiceProviderID.Valid {
			providerID = ptr(app.K8sServiceProviderID.String)
		}

		exportApps[i] = application.ExportApplication{
			UUID:                 app.UUID,
			Name:                 app.Name,
			ModelType:            modelType,
			CharmUUID:            app.CharmUUID,
			Life:                 app.Life,
			PasswordHash:         app.PasswordHash,
			Placement:            app.Placement,
			Exposed:              app.Exposed,
			Subordinate:          app.Subordinate,
			CharmModifiedVersion: app.CharmModifiedVersion,
			CharmUpgradeOnError:  app.CharmUpgradeOnError,
			CharmLocator: charm.CharmLocator{
				Name:         locator.Name,
				Revision:     locator.Revision,
				Source:       locator.Source,
				Architecture: locator.Architecture,
			},
			K8sServiceProviderID: providerID,
		}
	}
	return exportApps, nil
}
