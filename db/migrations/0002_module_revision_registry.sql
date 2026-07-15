-- Panvara
-- db/migrations/0002_module_revision_registry.sql    2026-07-15
--
-- @link    : https://github.com/shezw/panvara
-- @author  : shezw
-- @email   : hello@shezw.com

CREATE TABLE panvara_module_revision (
    project_id uuid NOT NULL,
    module_name text NOT NULL,
    revision_hash text NOT NULL,
    module_version text NOT NULL,
    spec_version text NOT NULL,
    ir_format integer NOT NULL,
    source_format text NOT NULL,
    source_hash text NOT NULL,
    source_bytes bytea NOT NULL,
    canonical_ir_bytes bytea NOT NULL,
    openapi_bytes bytea NOT NULL,
    manager_schema_bytes bytea NOT NULL,
    origin text NOT NULL,
    registered_by text NOT NULL,
    registered_at timestamptz NOT NULL,
    CONSTRAINT panvara_module_revision_pkey PRIMARY KEY (
        project_id,
        module_name,
        revision_hash
    ),
    CONSTRAINT panvara_module_revision_module_name_check CHECK (
        module_name ~ '^[a-z][a-z0-9]*([.-][a-z0-9]+)*$'
        AND octet_length(module_name) <= 128
    ),
    CONSTRAINT panvara_module_revision_hash_check CHECK (
        revision_hash ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT panvara_module_revision_version_check CHECK (
        octet_length(module_version) BETWEEN 1 AND 128
    ),
    CONSTRAINT panvara_module_revision_spec_version_check CHECK (
        octet_length(spec_version) BETWEEN 1 AND 128
    ),
    CONSTRAINT panvara_module_revision_ir_format_check CHECK (ir_format > 0),
    CONSTRAINT panvara_module_revision_source_format_check CHECK (
        source_format IN ('json', 'yaml')
    ),
    CONSTRAINT panvara_module_revision_source_hash_check CHECK (
        source_hash ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT panvara_module_revision_source_size_check CHECK (
        octet_length(source_bytes) BETWEEN 1 AND 1048576
    ),
    CONSTRAINT panvara_module_revision_canonical_ir_check CHECK (
        octet_length(canonical_ir_bytes) BETWEEN 1 AND 16777216
        AND jsonb_typeof(convert_from(canonical_ir_bytes, 'UTF8')::jsonb) = 'object'
    ),
    CONSTRAINT panvara_module_revision_openapi_check CHECK (
        octet_length(openapi_bytes) BETWEEN 1 AND 16777216
        AND jsonb_typeof(convert_from(openapi_bytes, 'UTF8')::jsonb) = 'object'
    ),
    CONSTRAINT panvara_module_revision_manager_schema_check CHECK (
        octet_length(manager_schema_bytes) BETWEEN 1 AND 16777216
        AND jsonb_typeof(convert_from(manager_schema_bytes, 'UTF8')::jsonb) = 'object'
    ),
    CONSTRAINT panvara_module_revision_bootstrap_origin_check CHECK (
        origin = 'bootstrap' AND registered_by = 'system:bootstrap'
    )
);

CREATE TABLE panvara_module_revision_data_schema (
    project_id uuid NOT NULL,
    module_name text NOT NULL,
    revision_hash text NOT NULL,
    data_schema_format integer NOT NULL,
    data_schema_fingerprint text NOT NULL,
    computed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT panvara_module_revision_data_schema_pkey PRIMARY KEY (
        project_id,
        module_name,
        revision_hash,
        data_schema_format
    ),
    CONSTRAINT panvara_module_revision_data_schema_format_check CHECK (
        data_schema_format > 0
    ),
    CONSTRAINT panvara_module_revision_data_schema_fingerprint_check CHECK (
        data_schema_fingerprint ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT panvara_module_revision_data_schema_revision_fk FOREIGN KEY (
        project_id,
        module_name,
        revision_hash
    ) REFERENCES panvara_module_revision (
        project_id,
        module_name,
        revision_hash
    ) ON DELETE RESTRICT
);

CREATE INDEX panvara_module_revision_project_module_time_idx
    ON panvara_module_revision (
        project_id,
        module_name,
        registered_at DESC,
        revision_hash ASC
    );

CREATE FUNCTION panvara_reject_module_revision_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'panvara module revisions are immutable'
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER panvara_module_revision_immutable_rows
    BEFORE UPDATE OR DELETE ON panvara_module_revision
    FOR EACH ROW
    EXECUTE FUNCTION panvara_reject_module_revision_mutation();

CREATE TRIGGER panvara_module_revision_immutable_truncate
    BEFORE TRUNCATE ON panvara_module_revision
    FOR EACH STATEMENT
    EXECUTE FUNCTION panvara_reject_module_revision_mutation();

CREATE TRIGGER panvara_module_revision_data_schema_immutable_rows
    BEFORE UPDATE OR DELETE ON panvara_module_revision_data_schema
    FOR EACH ROW
    EXECUTE FUNCTION panvara_reject_module_revision_mutation();

CREATE TRIGGER panvara_module_revision_data_schema_immutable_truncate
    BEFORE TRUNCATE ON panvara_module_revision_data_schema
    FOR EACH STATEMENT
    EXECUTE FUNCTION panvara_reject_module_revision_mutation();
