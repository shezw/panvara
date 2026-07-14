-- Panvara
-- db/migrations/0001_flex_record.sql    2026-07-14
--
-- @link    : https://github.com/shezw/panvara
-- @author  : shezw
-- @email   : hello@shezw.com

CREATE TABLE IF NOT EXISTS flex_record (
    project_id uuid NOT NULL,
    module_name text NOT NULL,
    resource_name text NOT NULL,
    record_id uuid NOT NULL,
    revision_hash text NOT NULL,
    record_version bigint NOT NULL DEFAULT 1,
    data jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    deleted_at timestamptz,
    CONSTRAINT flex_record_pkey PRIMARY KEY (
        project_id,
        module_name,
        resource_name,
        revision_hash,
        record_id
    ),
    CONSTRAINT flex_record_module_name_check CHECK (
        module_name ~ '^[a-z][a-z0-9]*([.-][a-z0-9]+)*$'
        AND octet_length(module_name) <= 128
    ),
    CONSTRAINT flex_record_resource_name_check CHECK (
        resource_name ~ '^[a-z][a-z0-9_]*$'
        AND octet_length(resource_name) <= 128
    ),
    CONSTRAINT flex_record_revision_hash_check CHECK (
        revision_hash ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT flex_record_record_version_check CHECK (record_version > 0),
    CONSTRAINT flex_record_data_object_check CHECK (jsonb_typeof(data) = 'object'),
    CONSTRAINT flex_record_time_order_check CHECK (
        updated_at >= created_at
        AND (deleted_at IS NULL OR deleted_at >= created_at)
    )
);

CREATE INDEX IF NOT EXISTS flex_record_live_scope_page_idx
    ON flex_record (
        project_id,
        module_name,
        resource_name,
        revision_hash,
        created_at,
        record_id
    )
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS flex_unique (
    project_id uuid NOT NULL,
    module_name text NOT NULL,
    resource_name text NOT NULL,
    revision_hash text NOT NULL,
    record_id uuid NOT NULL,
    field_name text NOT NULL,
    canonical_value text NOT NULL,
    CONSTRAINT flex_unique_pkey PRIMARY KEY (
        project_id,
        module_name,
        resource_name,
        revision_hash,
        record_id,
        field_name
    ),
    CONSTRAINT flex_unique_scope_value_key UNIQUE (
        project_id,
        module_name,
        resource_name,
        revision_hash,
        field_name,
        canonical_value
    ),
    CONSTRAINT flex_unique_field_name_check CHECK (
        field_name ~ '^[a-z][a-z0-9_]*$'
        AND octet_length(field_name) <= 128
    ),
    CONSTRAINT flex_unique_canonical_value_check CHECK (
        octet_length(canonical_value) <= 512
    ),
    CONSTRAINT flex_unique_record_fk FOREIGN KEY (
        project_id,
        module_name,
        resource_name,
        revision_hash,
        record_id
    ) REFERENCES flex_record (
        project_id,
        module_name,
        resource_name,
        revision_hash,
        record_id
    ) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS flex_reference (
    project_id uuid NOT NULL,
    module_name text NOT NULL,
    revision_hash text NOT NULL,
    source_resource_name text NOT NULL,
    source_record_id uuid NOT NULL,
    field_name text NOT NULL,
    target_resource_name text NOT NULL,
    target_record_id uuid NOT NULL,
    CONSTRAINT flex_reference_pkey PRIMARY KEY (
        project_id,
        module_name,
        revision_hash,
        source_resource_name,
        source_record_id,
        field_name
    ),
    CONSTRAINT flex_reference_source_resource_check CHECK (
        source_resource_name ~ '^[a-z][a-z0-9_]*$'
        AND octet_length(source_resource_name) <= 128
    ),
    CONSTRAINT flex_reference_target_resource_check CHECK (
        target_resource_name ~ '^[a-z][a-z0-9_]*$'
        AND octet_length(target_resource_name) <= 128
    ),
    CONSTRAINT flex_reference_field_name_check CHECK (
        field_name ~ '^[a-z][a-z0-9_]*$'
        AND octet_length(field_name) <= 128
    ),
    CONSTRAINT flex_reference_source_record_fk FOREIGN KEY (
        project_id,
        module_name,
        source_resource_name,
        revision_hash,
        source_record_id
    ) REFERENCES flex_record (
        project_id,
        module_name,
        resource_name,
        revision_hash,
        record_id
    ) ON DELETE CASCADE,
    CONSTRAINT flex_reference_target_record_fk FOREIGN KEY (
        project_id,
        module_name,
        target_resource_name,
        revision_hash,
        target_record_id
    ) REFERENCES flex_record (
        project_id,
        module_name,
        resource_name,
        revision_hash,
        record_id
    ) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS flex_reference_target_idx
    ON flex_reference (
        project_id,
        module_name,
        revision_hash,
        target_resource_name,
        target_record_id
    );
