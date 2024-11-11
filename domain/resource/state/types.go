// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "time"

type dbResource struct {
    // UUID is resources unique identifier.
    UUID string `db:"uuid"`
    // ApplicationUUID is the UUID of the application using the resource.
    ApplicationUUID string `db:"application_uuid"`
    // Name identifies the resource.
    Name string `db:"name"`
    // Origin
    OriginTypeID int       `db:"origin_type_id"`
    Size         int64     `db:"size"`
    Hash         string    `db:"hash"`
    HashTypeID   string    `db:"hash_type_id"`
    CreatedAt    time.Time `db:"created_at"`
}

type dbResourceMeta struct {
    // ApplicationUUID is the UUID of the application using the resource.
    ApplicationUUID string `db:"application_uuid"`

    // Name identifies the resource.
    Name string `db:"name"`

    // TypeID identifies the type of resource (e.g. "file").
    TypeID int `db:"type_id"`

    // Path is the path of the resource under the unit's data directory.
    Path string `db:"path"`

    // Description holds optional user-facing info for the resource.
    Description string `db:"description"`
}

type dbResourceType struct {
    Type string `db:"type"`
    ID   int    `db:"id"`
}
