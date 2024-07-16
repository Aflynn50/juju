CREATE TABLE machine (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    net_node_uuid TEXT NOT NULL,
    life_id INT NOT NULL,
    base TEXT,
    nonce TEXT,
    password_hash_algorithm_id TEXT,
    password_hash TEXT,
    clean BOOLEAN,
    force_destroyed BOOLEAN,
    placement TEXT,
    agent_started_at DATETIME,
    hostname TEXT,
    is_controller BOOLEAN,
    CONSTRAINT fk_machine_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_machine_life
    FOREIGN KEY (life_id)
    REFERENCES life (id)
);

CREATE UNIQUE INDEX idx_name
ON machine (name);

CREATE UNIQUE INDEX idx_machine_net_node
ON machine (net_node_uuid);

CREATE TABLE machine_constraint (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    constraint_uuid TEXT NOT NULL,
    CONSTRAINT fk_machine_constraint_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_constraint_constraint
    FOREIGN KEY (constraint_uuid)
    REFERENCES "constraint" (uuid)
);

CREATE TABLE machine_tool (
    machine_uuid TEXT NOT NULL,
    tool_url TEXT NOT NULL,
    tool_version TEXT NOT NULL,
    tool_sha256 TEXT NOT NULL,
    tool_size INT NOT NULL,
    CONSTRAINT fk_machine_principal_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    PRIMARY KEY (machine_uuid, tool_url)
);

CREATE TABLE machine_volume (
    machine_uuid TEXT NOT NULL,
    volume_uuid TEXT NOT NULL,
    CONSTRAINT fk_machine_volume_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_volume_volume
    FOREIGN KEY (volume_uuid)
    REFERENCES storage_volume (uuid),
    PRIMARY KEY (machine_uuid, volume_uuid)
);

CREATE TABLE machine_filesystem (
    machine_uuid TEXT NOT NULL,
    filesystem_uuid TEXT NOT NULL,
    CONSTRAINT fk_machine_filesystem_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_filesystem_filesystem
    FOREIGN KEY (filesystem_uuid)
    REFERENCES storage_filesystem (uuid),
    PRIMARY KEY (machine_uuid, filesystem_uuid)
);

CREATE TABLE machine_requires_reboot (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    created_at DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    CONSTRAINT fk_machine_requires_reboot_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

CREATE TABLE machine_status (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    status INT NOT NULL,
    CONSTRAINT fk_machine_constraint_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_constraint_status
    FOREIGN KEY (status)
    REFERENCES machine_status_values (id)
);
