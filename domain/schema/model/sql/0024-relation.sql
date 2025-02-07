-- Copyright 2024 Canonical Ltd.
-- Licensed under the AGPLv3, see LICENCE file for details.

-- The application_endpoint ties an application's relation
-- definition to a endpoint binding via a space. Each relation
-- has 2 endpoints, unless it is a peer relation. The space
-- and charm relation combine represent the endpoint binding
-- of this application endpoint.
CREATE TABLE application_endpoint (
    uuid TEXT NOT NULL PRIMARY KEY,
    application_uuid TEXT NOT NULL,
    space_uuid TEXT NOT NULL,
    charm_relation_uuid TEXT NOT NULL,
    CONSTRAINT fk_application_uuid
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_space_uuid
    FOREIGN KEY (space_uuid)
    REFERENCES space (uuid),
    CONSTRAINT fk_charm_relation_uuid
    FOREIGN KEY (charm_relation_uuid)
    REFERENCES charm_relation (uuid)
);

-- The relation_endpoint table links a relation to a single
-- application endpoint. If the relation is of type peer,
-- there will be one row in the table. If the relation has
-- a provider and requirer endpoint, there will be two rows
-- in the table.
CREATE TABLE relation_endpoint (
    uuid TEXT NOT NULL PRIMARY KEY,
    relation_uuid TEXT NOT NULL,
    endpoint_uuid TEXT NOT NULL,
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid),
    CONSTRAINT fk_endpoint_uuid
    FOREIGN KEY (endpoint_uuid)
    REFERENCES application_endpoint (uuid)
);

CREATE UNIQUE INDEX idx_relation_endpoint
ON relation_endpoint (relation_uuid, endpoint_uuid);

-- The relation table represents a relation between two
-- applications, or a peer relation.
CREATE TABLE relation (
    uuid TEXT NOT NULL PRIMARY KEY,
    life_id INT NOT NULL,
    relation_id INT NOT NULL,
    CONSTRAINT fk_relation_life
    FOREIGN KEY (life_id)
    REFERENCES life (id)
);

-- The relation_unit table links a relation to a specific unit.
CREATE TABLE relation_unit (
    uuid TEXT NOT NULL PRIMARY KEY,
    relation_uuid TEXT NOT NULL,
    unit_uuid TEXT NOT NULL,
    in_scope BOOLEAN DEFAULT FALSE,
    departing BOOLEAN DEFAULT FALSE,
    CONSTRAINT fk_relation_unit_uuid
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid)
);

CREATE UNIQUE INDEX idx_relation_unit
ON relation_unit (relation_uuid, unit_uuid);

-- The relation_unit_setting holds key value pair settings
-- for a relation at the unit level. Keys must be unique
-- per unit.
CREATE TABLE relation_unit_setting (
    relation_unit_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT,
    CONSTRAINT chk_key_empty CHECK ("key" != ''),
    CONSTRAINT fk_relation_unit_uuid
    FOREIGN KEY (relation_unit_uuid)
    REFERENCES relation_unit (uuid),
    PRIMARY KEY (relation_unit_uuid, "key")
);

-- The relation_application_setting holds key value pair settings
-- for a relation at the application level. Keys must be unique
-- per application.
CREATE TABLE relation_application_setting (
    relation_endpoint_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT,
    CONSTRAINT chk_key_empty CHECK ("key" != ''),
    CONSTRAINT fk_relation_endpoint_uuid
    FOREIGN KEY (relation_endpoint_uuid)
    REFERENCES relation_endpoint (uuid),
    PRIMARY KEY (relation_endpoint_uuid, "key")
);

-- The relation_status maps a relation to its status
-- as defined in the relation_status_type table.
CREATE TABLE relation_status (
    relation_uuid TEXT NOT NULL PRIMARY KEY,
    relation_status_type_id TEXT NOT NULL,
    suspended_reason TEXT,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid),
    CONSTRAINT fk_relation_status_type_id
    FOREIGN KEY (relation_status_type_id)
    REFERENCES relation_status_type (id)
);

CREATE TABLE relation_status_type (
    id TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_relation_status_type_name
ON relation_status_type (name);

INSERT INTO relation_status_type VALUES
(0, 'joining'),
(1, 'joined'),
(2, 'broken'),
(3, 'suspending'),
(4, 'suspended');

-- The relation_sequence table is used to keep track of the
-- sequence number for relation IDs within a model. Each
-- relation must have an relation ID.
CREATE TABLE relation_sequence (
    -- The sequence number will start at 0 for each model and will be
    -- incremented.
    sequence INT NOT NULL DEFAULT 0
);

-- A unique constraint over a constant index ensures only 1 entry matching the
-- condition can exist.
CREATE UNIQUE INDEX idx_singleton_relation_sequence ON relation_sequence ((1));
