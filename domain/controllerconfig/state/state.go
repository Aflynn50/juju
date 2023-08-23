// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// ControllerConfig returns the current configuration in the database.
func (st *State) ControllerConfig(ctx context.Context) (map[string]any, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	q := "SELECT key, value FROM controller_config"

	var result map[string]any
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, q)
		if err != nil {
			return errors.Trace(err)
		}

		result, err = controllerConfigFromRows(rows)
		return errors.Trace(err)
	})
	return result, err
}

// UpdateControllerConfig allows changing some of the configuration
// for the controller. Changes passed in updateAttrs will be applied
// to the current config, and keys in removeAttrs will be unset (and
// so revert to their defaults). Only a subset of keys can be changed
// after bootstrapping.
func (st *State) UpdateControllerConfig(ctx context.Context, updateAttrs map[string]any, removeAttrs []string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	dq := "DELETE FROM controller_config WHERE key = ?"

	uq := `
INSERT INTO controller_config (key, value)
VALUES (?, ?)
  ON CONFLICT(key) DO UPDATE SET value=?`

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// Remove the attributes
		for _, r := range removeAttrs {
			if _, err := tx.ExecContext(ctx, dq, r); err != nil {
				return errors.Trace(err)
			}
		}

		// Update the attributes.
		for k := range updateAttrs {
			if _, err := tx.ExecContext(ctx, uq, k, updateAttrs[k], updateAttrs[k]); err != nil {
				return errors.Trace(err)
			}
		}

		return nil
	})

	return errors.Trace(err)
}

// AllKeysQuery returns a SQL statement that will return
// all known controller configuration keys.
func (*State) AllKeysQuery() string {
	return "SELECT key FROM controller_config"
}

// controllerConfigFromRows returns controller config info from rows returned from the backing DB.
func controllerConfigFromRows(rows *sql.Rows) (map[string]any, error) {
	result := make(map[string]any)

	for rows.Next() {
		var key string
		var value any

		if err := rows.Scan(&key, &value); err != nil {
			_ = rows.Close()
			return nil, errors.Trace(err)
		}

		result[key] = value
	}

	return result, errors.Trace(rows.Err())
}