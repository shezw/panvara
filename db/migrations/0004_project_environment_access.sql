-- Panvara
-- db/migrations/0004_project_environment_access.sql    2026-07-18
--
-- @link    : https://github.com/shezw/panvara
-- @author  : shezw
-- @email   : hello@shezw.com

CREATE TABLE panvara_project (
    project_id uuid NOT NULL,
    project_key text NOT NULL,
    default_locale text NOT NULL,
    default_time_zone text NOT NULL,
    default_currency text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT panvara_project_pkey PRIMARY KEY (project_id),
    CONSTRAINT panvara_project_key_unique UNIQUE (project_key),
    CONSTRAINT panvara_project_uuidv7_check CHECK (
        project_id::text ~ '^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
    ),
    CONSTRAINT panvara_project_key_check CHECK (
        project_key ~ '^[a-z0-9][a-z0-9._-]{0,63}$'
    ),
    CONSTRAINT panvara_project_locale_check CHECK (
        octet_length(default_locale) <= 128
        AND default_locale ~ '^[A-Za-z]{2,8}(-[A-Za-z0-9]{1,8})*$'
    ),
    CONSTRAINT panvara_project_time_zone_check CHECK (
        octet_length(default_time_zone) BETWEEN 1 AND 128
        AND default_time_zone <> 'Local'
    ),
    CONSTRAINT panvara_project_currency_check CHECK (
        default_currency ~ '^[A-Z]{3}$'
    ),
    CONSTRAINT panvara_project_status_check CHECK (status IN ('active', 'disabled')),
    CONSTRAINT panvara_project_time_order_check CHECK (updated_at >= created_at)
);

CREATE TABLE panvara_environment (
    project_id uuid NOT NULL,
    environment_id uuid NOT NULL,
    environment_key text NOT NULL,
    is_default boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT panvara_environment_pkey PRIMARY KEY (project_id, environment_id),
    CONSTRAINT panvara_environment_key_unique UNIQUE (project_id, environment_key),
    CONSTRAINT panvara_environment_uuidv7_check CHECK (
        environment_id::text ~ '^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
    ),
    CONSTRAINT panvara_environment_key_check CHECK (
        environment_key ~ '^[a-z0-9][a-z0-9._-]{0,63}$'
    ),
    CONSTRAINT panvara_environment_status_check CHECK (status IN ('active', 'disabled')),
    CONSTRAINT panvara_environment_time_order_check CHECK (updated_at >= created_at),
    CONSTRAINT panvara_environment_project_fk FOREIGN KEY (project_id)
        REFERENCES panvara_project (project_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX panvara_environment_one_default_per_project_idx
    ON panvara_environment (project_id)
    WHERE is_default;

CREATE TABLE panvara_principal (
    project_id uuid NOT NULL,
    principal_id text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT panvara_principal_pkey PRIMARY KEY (project_id, principal_id),
    CONSTRAINT panvara_principal_id_check CHECK (
        principal_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
    ),
    CONSTRAINT panvara_principal_status_check CHECK (status IN ('active', 'disabled')),
    CONSTRAINT panvara_principal_time_order_check CHECK (updated_at >= created_at),
    CONSTRAINT panvara_principal_project_fk FOREIGN KEY (project_id)
        REFERENCES panvara_project (project_id) ON DELETE RESTRICT
);

CREATE TABLE panvara_access_grant (
    project_id uuid NOT NULL,
    environment_id uuid NOT NULL,
    principal_id text NOT NULL,
    role text NOT NULL,
    granted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    revoked_at timestamptz,
    CONSTRAINT panvara_access_grant_pkey PRIMARY KEY (
        project_id,
        environment_id,
        principal_id,
        role
    ),
    CONSTRAINT panvara_access_grant_role_check CHECK (role = 'project.owner'),
    CONSTRAINT panvara_access_grant_time_order_check CHECK (
        revoked_at IS NULL OR revoked_at >= granted_at
    ),
    CONSTRAINT panvara_access_grant_environment_fk FOREIGN KEY (project_id, environment_id)
        REFERENCES panvara_environment (project_id, environment_id) ON DELETE RESTRICT,
    CONSTRAINT panvara_access_grant_principal_fk FOREIGN KEY (project_id, principal_id)
        REFERENCES panvara_principal (project_id, principal_id) ON DELETE RESTRICT
);

CREATE INDEX panvara_access_grant_active_lookup_idx
    ON panvara_access_grant (project_id, environment_id, principal_id, role)
    WHERE revoked_at IS NULL;
