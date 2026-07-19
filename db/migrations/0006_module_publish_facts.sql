-- Panvara
-- db/migrations/0006_module_publish_facts.sql    2026-07-19
--
--      ______     __  __     ______     ______     __     __
--     /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
--     \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
--      \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
--       \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com
--
-- @link    : https://github.com/shezw/panvara
-- @author  : shezw
-- @email   : hello@shezw.com

ALTER TABLE panvara_module_revision
    DROP CONSTRAINT panvara_module_revision_bootstrap_origin_check,
    ADD CONSTRAINT panvara_module_revision_origin_check CHECK (
        (
            origin = 'bootstrap'
            AND registered_by = 'system:bootstrap'
        )
        OR
        (
            origin = 'publish'
            AND registered_by <> 'system:bootstrap'
            AND registered_by ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
        )
    );

-- This candidate key lets a Release reference every immutable Plan identity
-- used by Publish, rather than merely pointing at a plan_id and trusting a
-- second mutable projection of its facts.
ALTER TABLE panvara_module_draft_plan
    ADD COLUMN publish_baseline_revision_identity text
        GENERATED ALWAYS AS (COALESCE(baseline_revision_hash, '')) STORED,
    ADD CONSTRAINT panvara_module_draft_plan_publish_identity_key
    UNIQUE (project_id, module_name, plan_id),
    ADD CONSTRAINT panvara_module_draft_plan_release_reference_key
    UNIQUE (
        project_id,
        module_name,
        draft_id,
        plan_id,
        draft_generation,
        validation_id,
        plan_hash,
        publish_baseline_revision_identity,
        candidate_revision_hash,
        candidate_data_schema_format,
        candidate_data_schema_fingerprint,
        source_hash,
        outcome,
        risk
    );

CREATE TABLE panvara_module_release (
    project_id uuid NOT NULL,
    environment_id uuid NOT NULL,
    module_name text NOT NULL,
    release_id uuid NOT NULL,
    draft_id uuid NOT NULL,
    draft_generation bigint NOT NULL,
    validation_id text NOT NULL,
    plan_id text NOT NULL,
    plan_hash text NOT NULL,
    baseline_revision_hash text,
    publish_baseline_revision_identity text
        GENERATED ALWAYS AS (COALESCE(baseline_revision_hash, '')) STORED,
    candidate_revision_hash text NOT NULL,
    candidate_data_schema_format integer NOT NULL,
    candidate_data_schema_fingerprint text NOT NULL,
    source_hash text NOT NULL,
    outcome text NOT NULL,
    risk text NOT NULL,
    published_by_principal_id text NOT NULL,
    published_by_credential_id uuid NOT NULL,
    request_id text NOT NULL,
    published_at timestamptz NOT NULL,
    CONSTRAINT panvara_module_release_pkey PRIMARY KEY (
        project_id,
        environment_id,
        module_name,
        release_id
    ),
    CONSTRAINT panvara_module_release_plan_key UNIQUE (
        project_id,
        environment_id,
        module_name,
        plan_id
    ),
    CONSTRAINT panvara_module_release_uuidv7_check CHECK (
        release_id::text ~ '^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
    ),
    CONSTRAINT panvara_module_release_module_check CHECK (
        module_name ~ '^[a-z][a-z0-9]*([.-][a-z0-9]+)*$'
        AND octet_length(module_name) <= 128
    ),
    CONSTRAINT panvara_module_release_draft_check CHECK (
        draft_id::text ~ '^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
        AND draft_generation > 0
    ),
    CONSTRAINT panvara_module_release_identity_check CHECK (
        validation_id ~ '^sha256:[0-9a-f]{64}$'
        AND plan_id ~ '^sha256:[0-9a-f]{64}$'
        AND plan_hash ~ '^sha256:[0-9a-f]{64}$'
        AND (
            baseline_revision_hash IS NULL
            OR baseline_revision_hash ~ '^sha256:[0-9a-f]{64}$'
        )
        AND candidate_revision_hash ~ '^sha256:[0-9a-f]{64}$'
        AND candidate_data_schema_format > 0
        AND candidate_data_schema_fingerprint ~ '^sha256:[0-9a-f]{64}$'
        AND source_hash ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT panvara_module_release_outcome_check CHECK (
        outcome IN ('compatible', 'review_required', 'migration_required')
        AND risk IN ('none', 'low', 'medium', 'high')
    ),
    CONSTRAINT panvara_module_release_actor_check CHECK (
        published_by_principal_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
        AND request_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
    ),
    CONSTRAINT panvara_module_release_environment_fk FOREIGN KEY (
        project_id,
        environment_id
    ) REFERENCES panvara_environment (
        project_id,
        environment_id
    ) ON DELETE RESTRICT,
    CONSTRAINT panvara_module_release_draft_fk FOREIGN KEY (
        project_id,
        module_name,
        draft_id
    ) REFERENCES panvara_module_draft (
        project_id,
        module_name,
        draft_id
    ) ON DELETE RESTRICT,
    CONSTRAINT panvara_module_release_plan_fk FOREIGN KEY (
        project_id,
        module_name,
        draft_id,
        plan_id,
        draft_generation,
        validation_id,
        plan_hash,
        publish_baseline_revision_identity,
        candidate_revision_hash,
        candidate_data_schema_format,
        candidate_data_schema_fingerprint,
        source_hash,
        outcome,
        risk
    ) REFERENCES panvara_module_draft_plan (
        project_id,
        module_name,
        draft_id,
        plan_id,
        draft_generation,
        validation_id,
        plan_hash,
        publish_baseline_revision_identity,
        candidate_revision_hash,
        candidate_data_schema_format,
        candidate_data_schema_fingerprint,
        source_hash,
        outcome,
        risk
    ) ON DELETE RESTRICT,
    CONSTRAINT panvara_module_release_baseline_revision_fk FOREIGN KEY (
        project_id,
        module_name,
        baseline_revision_hash
    ) REFERENCES panvara_module_revision (
        project_id,
        module_name,
        revision_hash
    ) ON DELETE RESTRICT,
    CONSTRAINT panvara_module_release_candidate_revision_fk FOREIGN KEY (
        project_id,
        module_name,
        candidate_revision_hash
    ) REFERENCES panvara_module_revision (
        project_id,
        module_name,
        revision_hash
    ) ON DELETE RESTRICT,
    CONSTRAINT panvara_module_release_actor_principal_fk FOREIGN KEY (
        project_id,
        published_by_principal_id
    ) REFERENCES panvara_principal (
        project_id,
        principal_id
    ) ON DELETE RESTRICT,
    CONSTRAINT panvara_module_release_actor_credential_fk FOREIGN KEY (
        project_id,
        environment_id,
        published_by_credential_id,
        published_by_principal_id
    ) REFERENCES panvara_api_credential (
        project_id,
        environment_id,
        credential_id,
        principal_id
    ) ON DELETE RESTRICT
);

CREATE TABLE panvara_module_release_idempotency (
    project_id uuid NOT NULL,
    environment_id uuid NOT NULL,
    module_name text NOT NULL,
    idempotency_key text NOT NULL,
    intent_hash text NOT NULL,
    release_id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT panvara_module_release_idempotency_pkey PRIMARY KEY (
        project_id,
        environment_id,
        module_name,
        idempotency_key
    ),
    CONSTRAINT panvara_module_release_idempotency_key_check CHECK (
        idempotency_key ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
        AND intent_hash ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT panvara_module_release_idempotency_release_fk FOREIGN KEY (
        project_id,
        environment_id,
        module_name,
        release_id
    ) REFERENCES panvara_module_release (
        project_id,
        environment_id,
        module_name,
        release_id
    ) ON DELETE RESTRICT
);

CREATE INDEX panvara_module_release_scope_time_idx
    ON panvara_module_release (
        project_id,
        environment_id,
        module_name,
        published_at DESC,
        release_id ASC
    );

CREATE FUNCTION panvara_reject_module_release_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'panvara module release and idempotency facts are append-only'
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER panvara_module_release_append_only_rows
    BEFORE UPDATE OR DELETE ON panvara_module_release
    FOR EACH ROW
    EXECUTE FUNCTION panvara_reject_module_release_mutation();

CREATE TRIGGER panvara_module_release_append_only_truncate
    BEFORE TRUNCATE ON panvara_module_release
    FOR EACH STATEMENT
    EXECUTE FUNCTION panvara_reject_module_release_mutation();

CREATE TRIGGER panvara_module_release_idempotency_append_only_rows
    BEFORE UPDATE OR DELETE ON panvara_module_release_idempotency
    FOR EACH ROW
    EXECUTE FUNCTION panvara_reject_module_release_mutation();

CREATE TRIGGER panvara_module_release_idempotency_append_only_truncate
    BEFORE TRUNCATE ON panvara_module_release_idempotency
    FOR EACH STATEMENT
    EXECUTE FUNCTION panvara_reject_module_release_mutation();
