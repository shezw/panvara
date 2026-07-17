-- Panvara
-- db/migrations/0003_module_draft_workflow.sql    2026-07-16
--
-- @link    : https://github.com/shezw/panvara
-- @author  : shezw
-- @email   : hello@shezw.com

CREATE TABLE panvara_module_draft (
    project_id uuid NOT NULL,
    module_name text NOT NULL,
    draft_id uuid NOT NULL,
    baseline_revision_hash text,
    source_format text NOT NULL,
    source_hash text NOT NULL,
    source_bytes bytea NOT NULL,
    generation bigint NOT NULL,
    idempotency_key text NOT NULL,
    create_intent_hash text NOT NULL,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_by text NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT panvara_module_draft_pkey PRIMARY KEY (project_id, module_name, draft_id),
    CONSTRAINT panvara_module_draft_idempotency_key UNIQUE (project_id, module_name, idempotency_key),
    CONSTRAINT panvara_module_draft_uuidv7_check CHECK (
        draft_id::text ~ '^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
    ),
    CONSTRAINT panvara_module_draft_module_check CHECK (
        module_name ~ '^[a-z][a-z0-9]*([.-][a-z0-9]+)*$'
        AND octet_length(module_name) <= 128
    ),
    CONSTRAINT panvara_module_draft_baseline_check CHECK (
        baseline_revision_hash IS NULL OR baseline_revision_hash ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT panvara_module_draft_source_format_check CHECK (source_format IN ('json', 'yaml')),
    CONSTRAINT panvara_module_draft_source_hash_check CHECK (source_hash ~ '^sha256:[0-9a-f]{64}$'),
    CONSTRAINT panvara_module_draft_source_size_check CHECK (octet_length(source_bytes) <= 1048576),
    CONSTRAINT panvara_module_draft_source_utf8_check CHECK (convert_from(source_bytes, 'UTF8') IS NOT NULL),
    CONSTRAINT panvara_module_draft_generation_check CHECK (generation > 0),
    CONSTRAINT panvara_module_draft_idempotency_check CHECK (
        idempotency_key ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
    ),
    CONSTRAINT panvara_module_draft_intent_hash_check CHECK (create_intent_hash ~ '^sha256:[0-9a-f]{64}$'),
    CONSTRAINT panvara_module_draft_audit_check CHECK (
        created_by ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
        AND updated_by ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
        AND updated_at >= created_at
    ),
    CONSTRAINT panvara_module_draft_baseline_fk FOREIGN KEY (
        project_id, module_name, baseline_revision_hash
    ) REFERENCES panvara_module_revision (
        project_id, module_name, revision_hash
    ) ON DELETE RESTRICT
);

CREATE TABLE panvara_module_draft_validation (
    project_id uuid NOT NULL,
    module_name text NOT NULL,
    draft_id uuid NOT NULL,
    validation_id text NOT NULL,
    format_version integer NOT NULL,
    draft_generation bigint NOT NULL,
    baseline_revision_hash text,
    source_format text NOT NULL,
    source_hash text NOT NULL,
    valid boolean NOT NULL,
    issues jsonb NOT NULL,
    issues_hash text NOT NULL,
    candidate_revision_hash text,
    candidate_module_version text,
    candidate_data_schema_format integer,
    candidate_data_schema_fingerprint text,
    candidate_ir_bytes bytea,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT panvara_module_draft_validation_pkey PRIMARY KEY (
        project_id, module_name, draft_id, validation_id
    ),
    CONSTRAINT panvara_module_draft_validation_generation_key UNIQUE (
        project_id, module_name, draft_id, draft_generation, format_version
    ),
    CONSTRAINT panvara_module_draft_validation_draft_fk FOREIGN KEY (
        project_id, module_name, draft_id
    ) REFERENCES panvara_module_draft (project_id, module_name, draft_id) ON DELETE RESTRICT,
    CONSTRAINT panvara_module_draft_validation_identity_check CHECK (
        validation_id ~ '^sha256:[0-9a-f]{64}$'
        AND issues_hash ~ '^sha256:[0-9a-f]{64}$'
        AND source_hash ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT panvara_module_draft_validation_format_check CHECK (
        format_version = 1 AND source_format IN ('json', 'yaml') AND draft_generation > 0
    ),
    CONSTRAINT panvara_module_draft_validation_issues_check CHECK (
        jsonb_typeof(issues) = 'array'
        AND jsonb_array_length(issues) <= 100
        AND octet_length(issues::text) <= 262144
    ),
    CONSTRAINT panvara_module_draft_validation_result_check CHECK (
        (
            valid
            AND jsonb_array_length(issues) = 0
            AND candidate_revision_hash ~ '^sha256:[0-9a-f]{64}$'
            AND octet_length(candidate_module_version) BETWEEN 1 AND 128
            AND candidate_data_schema_format > 0
            AND candidate_data_schema_fingerprint ~ '^sha256:[0-9a-f]{64}$'
            AND octet_length(candidate_ir_bytes) BETWEEN 1 AND 16777216
            AND jsonb_typeof(convert_from(candidate_ir_bytes, 'UTF8')::jsonb) = 'object'
        ) OR (
            NOT valid
            AND jsonb_array_length(issues) BETWEEN 1 AND 100
            AND candidate_revision_hash IS NULL
            AND candidate_module_version IS NULL
            AND candidate_data_schema_format IS NULL
            AND candidate_data_schema_fingerprint IS NULL
            AND candidate_ir_bytes IS NULL
        )
    ),
    CONSTRAINT panvara_module_draft_validation_audit_check CHECK (
        created_by ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
    )
);

CREATE TABLE panvara_module_draft_plan (
    project_id uuid NOT NULL,
    module_name text NOT NULL,
    draft_id uuid NOT NULL,
    plan_id text NOT NULL,
    plan_hash text NOT NULL,
    format_version integer NOT NULL,
    draft_generation bigint NOT NULL,
    validation_id text NOT NULL,
    baseline_revision_hash text,
    baseline_data_schema_format integer,
    baseline_data_schema_fingerprint text,
    source_format text NOT NULL,
    source_hash text NOT NULL,
    candidate_revision_hash text NOT NULL,
    candidate_module_version text NOT NULL,
    candidate_data_schema_format integer NOT NULL,
    candidate_data_schema_fingerprint text NOT NULL,
    changes jsonb NOT NULL,
    risk_low integer NOT NULL,
    risk_review integer NOT NULL,
    risk_destructive integer NOT NULL,
    outcome text NOT NULL,
    risk text NOT NULL,
    data_schema_changed boolean NOT NULL,
    record_namespace_changed boolean NOT NULL,
    migration_execution_supported boolean NOT NULL,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT panvara_module_draft_plan_pkey PRIMARY KEY (
        project_id, module_name, draft_id, plan_id
    ),
    CONSTRAINT panvara_module_draft_plan_validation_key UNIQUE (
        project_id, module_name, draft_id, validation_id, format_version
    ),
    CONSTRAINT panvara_module_draft_plan_validation_fk FOREIGN KEY (
        project_id, module_name, draft_id, validation_id
    ) REFERENCES panvara_module_draft_validation (
        project_id, module_name, draft_id, validation_id
    ) ON DELETE RESTRICT,
    CONSTRAINT panvara_module_draft_plan_identity_check CHECK (
        plan_id ~ '^sha256:[0-9a-f]{64}$'
        AND plan_hash ~ '^sha256:[0-9a-f]{64}$'
        AND validation_id ~ '^sha256:[0-9a-f]{64}$'
        AND source_hash ~ '^sha256:[0-9a-f]{64}$'
        AND candidate_revision_hash ~ '^sha256:[0-9a-f]{64}$'
        AND candidate_data_schema_fingerprint ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT panvara_module_draft_plan_format_check CHECK (
        format_version = 1 AND draft_generation > 0 AND source_format IN ('json', 'yaml')
    ),
    CONSTRAINT panvara_module_draft_plan_baseline_check CHECK (
        (
            baseline_revision_hash IS NULL
            AND baseline_data_schema_format IS NULL
            AND baseline_data_schema_fingerprint IS NULL
        ) OR (
            baseline_revision_hash ~ '^sha256:[0-9a-f]{64}$'
            AND baseline_data_schema_format > 0
            AND baseline_data_schema_fingerprint ~ '^sha256:[0-9a-f]{64}$'
        )
    ),
    CONSTRAINT panvara_module_draft_plan_candidate_check CHECK (
        octet_length(candidate_module_version) BETWEEN 1 AND 128
        AND candidate_data_schema_format > 0
    ),
    CONSTRAINT panvara_module_draft_plan_changes_check CHECK (
        jsonb_typeof(changes) = 'array'
        AND jsonb_array_length(changes) <= 65536
        AND octet_length(changes::text) <= 16777216
    ),
    CONSTRAINT panvara_module_draft_plan_risk_check CHECK (
        risk_low >= 0 AND risk_review >= 0 AND risk_destructive >= 0
        AND outcome IN ('compatible', 'review_required', 'migration_required', 'unsupported')
        AND risk IN ('none', 'low', 'medium', 'high', 'critical')
        AND migration_execution_supported = false
    ),
    CONSTRAINT panvara_module_draft_plan_audit_check CHECK (
        created_by ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
    )
);

CREATE INDEX panvara_module_draft_project_module_time_idx
    ON panvara_module_draft (project_id, module_name, updated_at DESC, draft_id ASC);

CREATE INDEX panvara_module_draft_validation_generation_idx
    ON panvara_module_draft_validation (
        project_id, module_name, draft_id, draft_generation DESC, created_at DESC
    );

CREATE INDEX panvara_module_draft_plan_generation_idx
    ON panvara_module_draft_plan (
        project_id, module_name, draft_id, draft_generation DESC, created_at DESC
    );

CREATE FUNCTION panvara_reject_draft_snapshot_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'panvara draft validation and plan snapshots are immutable'
        USING ERRCODE = '55000';
END;
$$;

CREATE FUNCTION panvara_enforce_draft_head_update()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.project_id IS DISTINCT FROM OLD.project_id
       OR NEW.module_name IS DISTINCT FROM OLD.module_name
       OR NEW.draft_id IS DISTINCT FROM OLD.draft_id
       OR NEW.baseline_revision_hash IS DISTINCT FROM OLD.baseline_revision_hash
       OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
       OR NEW.create_intent_hash IS DISTINCT FROM OLD.create_intent_hash
       OR NEW.created_by IS DISTINCT FROM OLD.created_by
       OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'panvara draft identity, baseline, and creation provenance are immutable'
            USING ERRCODE = '55000';
    END IF;

    IF NEW.source_format IS NOT DISTINCT FROM OLD.source_format
       AND NEW.source_hash IS NOT DISTINCT FROM OLD.source_hash
       AND NEW.source_bytes IS NOT DISTINCT FROM OLD.source_bytes THEN
        IF NEW.generation IS DISTINCT FROM OLD.generation
           OR NEW.updated_by IS DISTINCT FROM OLD.updated_by
           OR NEW.updated_at IS DISTINCT FROM OLD.updated_at THEN
            RAISE EXCEPTION 'panvara identical draft source must be an audit-preserving no-op'
                USING ERRCODE = '55000';
        END IF;
    ELSIF NEW.generation <> OLD.generation + 1
          OR NEW.updated_at < OLD.updated_at THEN
        RAISE EXCEPTION 'panvara draft source changes require one generation increment'
            USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER panvara_module_draft_guarded_update
    BEFORE UPDATE ON panvara_module_draft
    FOR EACH ROW EXECUTE FUNCTION panvara_enforce_draft_head_update();

CREATE TRIGGER panvara_module_draft_validation_immutable_rows
    BEFORE UPDATE OR DELETE ON panvara_module_draft_validation
    FOR EACH ROW EXECUTE FUNCTION panvara_reject_draft_snapshot_mutation();

CREATE TRIGGER panvara_module_draft_validation_immutable_truncate
    BEFORE TRUNCATE ON panvara_module_draft_validation
    FOR EACH STATEMENT EXECUTE FUNCTION panvara_reject_draft_snapshot_mutation();

CREATE TRIGGER panvara_module_draft_plan_immutable_rows
    BEFORE UPDATE OR DELETE ON panvara_module_draft_plan
    FOR EACH ROW EXECUTE FUNCTION panvara_reject_draft_snapshot_mutation();

CREATE TRIGGER panvara_module_draft_plan_immutable_truncate
    BEFORE TRUNCATE ON panvara_module_draft_plan
    FOR EACH STATEMENT EXECUTE FUNCTION panvara_reject_draft_snapshot_mutation();
