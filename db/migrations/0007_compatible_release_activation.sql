-- Panvara
-- db/migrations/0007_compatible_release_activation.sql    2026-08-02
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

-- A snapshot is the immutable, environment-scoped release epoch. Module rows
-- are normalized so a later multi-module activation can replace one binding
-- while copying every other binding without changing this authority model.
CREATE TABLE panvara_project_release_snapshot (
    project_id uuid NOT NULL,
    environment_id uuid NOT NULL,
    release_epoch bigint NOT NULL,
    cause text NOT NULL,
    activated_by_principal_id text,
    activated_by_credential_id uuid,
    request_id text NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT panvara_project_release_snapshot_pkey PRIMARY KEY (
        project_id,
        environment_id,
        release_epoch
    ),
    CONSTRAINT panvara_project_release_snapshot_epoch_check CHECK (
        release_epoch > 0
    ),
    CONSTRAINT panvara_project_release_snapshot_cause_check CHECK (
        cause IN ('bootstrap', 'activate')
    ),
    CONSTRAINT panvara_project_release_snapshot_request_id_check CHECK (
        octet_length(request_id) BETWEEN 1 AND 128
        AND request_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]*$'
    ),
    CONSTRAINT panvara_project_release_snapshot_actor_check CHECK (
        (
            cause = 'bootstrap'
            AND activated_by_principal_id IS NULL
            AND activated_by_credential_id IS NULL
            AND request_id = 'system:bootstrap'
        )
        OR
        (
            cause = 'activate'
            AND activated_by_principal_id IS NOT NULL
            AND activated_by_principal_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
            AND activated_by_credential_id IS NOT NULL
        )
    ),
    CONSTRAINT panvara_project_release_snapshot_environment_fk FOREIGN KEY (
        project_id,
        environment_id
    ) REFERENCES panvara_environment (
        project_id,
        environment_id
    ) ON DELETE RESTRICT,
    CONSTRAINT panvara_project_release_snapshot_actor_credential_fk FOREIGN KEY (
        project_id,
        environment_id,
        activated_by_credential_id,
        activated_by_principal_id
    ) REFERENCES panvara_api_credential (
        project_id,
        environment_id,
        credential_id,
        principal_id
    ) ON DELETE RESTRICT
);

ALTER TABLE panvara_module_release
    ADD CONSTRAINT panvara_module_release_activation_reference_key UNIQUE (
        project_id,
        environment_id,
        module_name,
        release_id,
        candidate_revision_hash
    );

ALTER TABLE panvara_module_revision_data_schema
    ADD CONSTRAINT panvara_module_revision_data_schema_activation_reference_key UNIQUE (
        project_id,
        module_name,
        revision_hash,
        data_schema_format,
        data_schema_fingerprint
    );

CREATE TABLE panvara_project_release_snapshot_module (
    project_id uuid NOT NULL,
    environment_id uuid NOT NULL,
    release_epoch bigint NOT NULL,
    module_name text NOT NULL,
    source_kind text NOT NULL,
    release_id uuid,
    runtime_revision_hash text NOT NULL,
    record_namespace_revision_hash text NOT NULL,
    data_schema_format integer NOT NULL,
    data_schema_fingerprint text NOT NULL,
    CONSTRAINT panvara_project_release_snapshot_module_pkey PRIMARY KEY (
        project_id,
        environment_id,
        release_epoch,
        module_name
    ),
    CONSTRAINT panvara_project_release_snapshot_module_name_check CHECK (
        module_name ~ '^[a-z][a-z0-9]*([.-][a-z0-9]+)*$'
        AND octet_length(module_name) <= 128
    ),
    CONSTRAINT panvara_project_release_snapshot_module_source_check CHECK (
        (source_kind = 'bootstrap' AND release_id IS NULL)
        OR
        (source_kind = 'release' AND release_id IS NOT NULL)
    ),
    CONSTRAINT panvara_project_release_snapshot_module_revision_check CHECK (
        runtime_revision_hash ~ '^sha256:[0-9a-f]{64}$'
        AND record_namespace_revision_hash ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT panvara_project_release_snapshot_module_data_schema_check CHECK (
        data_schema_format > 0
        AND data_schema_fingerprint ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT panvara_project_release_snapshot_module_snapshot_fk FOREIGN KEY (
        project_id,
        environment_id,
        release_epoch
    ) REFERENCES panvara_project_release_snapshot (
        project_id,
        environment_id,
        release_epoch
    ) ON DELETE RESTRICT,
    CONSTRAINT panvara_project_release_snapshot_module_runtime_revision_fk FOREIGN KEY (
        project_id,
        module_name,
        runtime_revision_hash
    ) REFERENCES panvara_module_revision (
        project_id,
        module_name,
        revision_hash
    ) ON DELETE RESTRICT,
    CONSTRAINT panvara_project_release_snapshot_module_record_revision_fk FOREIGN KEY (
        project_id,
        module_name,
        record_namespace_revision_hash
    ) REFERENCES panvara_module_revision (
        project_id,
        module_name,
        revision_hash
    ) ON DELETE RESTRICT,
    CONSTRAINT panvara_project_release_snapshot_module_runtime_schema_fk FOREIGN KEY (
        project_id,
        module_name,
        runtime_revision_hash,
        data_schema_format,
        data_schema_fingerprint
    ) REFERENCES panvara_module_revision_data_schema (
        project_id,
        module_name,
        revision_hash,
        data_schema_format,
        data_schema_fingerprint
    ) ON DELETE RESTRICT,
    CONSTRAINT panvara_project_release_snapshot_module_record_schema_fk FOREIGN KEY (
        project_id,
        module_name,
        record_namespace_revision_hash,
        data_schema_format,
        data_schema_fingerprint
    ) REFERENCES panvara_module_revision_data_schema (
        project_id,
        module_name,
        revision_hash,
        data_schema_format,
        data_schema_fingerprint
    ) ON DELETE RESTRICT,
    CONSTRAINT panvara_project_release_snapshot_module_release_fk FOREIGN KEY (
        project_id,
        environment_id,
        module_name,
        release_id,
        runtime_revision_hash
    ) REFERENCES panvara_module_release (
        project_id,
        environment_id,
        module_name,
        release_id,
        candidate_revision_hash
    ) ON DELETE RESTRICT
);

CREATE INDEX panvara_project_release_snapshot_module_release_idx
    ON panvara_project_release_snapshot_module (
        project_id,
        environment_id,
        module_name,
        release_id,
        release_epoch DESC
    )
    WHERE release_id IS NOT NULL;

CREATE TABLE panvara_environment_release_pointer (
    project_id uuid NOT NULL,
    environment_id uuid NOT NULL,
    active_epoch bigint NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT panvara_environment_release_pointer_pkey PRIMARY KEY (
        project_id,
        environment_id
    ),
    CONSTRAINT panvara_environment_release_pointer_epoch_check CHECK (
        active_epoch > 0
    ),
    CONSTRAINT panvara_environment_release_pointer_environment_fk FOREIGN KEY (
        project_id,
        environment_id
    ) REFERENCES panvara_environment (
        project_id,
        environment_id
    ) ON DELETE RESTRICT,
    CONSTRAINT panvara_environment_release_pointer_snapshot_fk FOREIGN KEY (
        project_id,
        environment_id,
        active_epoch
    ) REFERENCES panvara_project_release_snapshot (
        project_id,
        environment_id,
        release_epoch
    ) ON DELETE RESTRICT
);

CREATE FUNCTION panvara_reject_project_release_snapshot_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'panvara project release snapshots are append-only'
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER panvara_project_release_snapshot_append_only_rows
    BEFORE UPDATE OR DELETE ON panvara_project_release_snapshot
    FOR EACH ROW
    EXECUTE FUNCTION panvara_reject_project_release_snapshot_mutation();

CREATE TRIGGER panvara_project_release_snapshot_append_only_truncate
    BEFORE TRUNCATE ON panvara_project_release_snapshot
    FOR EACH STATEMENT
    EXECUTE FUNCTION panvara_reject_project_release_snapshot_mutation();

CREATE TRIGGER panvara_project_release_snapshot_module_append_only_rows
    BEFORE UPDATE OR DELETE ON panvara_project_release_snapshot_module
    FOR EACH ROW
    EXECUTE FUNCTION panvara_reject_project_release_snapshot_mutation();

CREATE TRIGGER panvara_project_release_snapshot_module_append_only_truncate
    BEFORE TRUNCATE ON panvara_project_release_snapshot_module
    FOR EACH STATEMENT
    EXECUTE FUNCTION panvara_reject_project_release_snapshot_mutation();

CREATE FUNCTION panvara_enforce_environment_release_pointer_update()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'TRUNCATE' OR TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'panvara environment release pointers cannot be removed'
            USING ERRCODE = '55000';
    END IF;
    IF NEW.project_id IS DISTINCT FROM OLD.project_id
        OR NEW.environment_id IS DISTINCT FROM OLD.environment_id THEN
        RAISE EXCEPTION 'panvara environment release pointer identity is immutable'
            USING ERRCODE = '55000';
    END IF;
    IF NEW.active_epoch <> OLD.active_epoch + 1 THEN
        RAISE EXCEPTION 'panvara environment release epoch must advance by exactly one'
            USING ERRCODE = '55000';
    END IF;
    IF NEW.updated_at < OLD.updated_at THEN
        RAISE EXCEPTION 'panvara environment release pointer time cannot move backwards'
            USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER panvara_environment_release_pointer_monotonic_rows
    BEFORE UPDATE OR DELETE ON panvara_environment_release_pointer
    FOR EACH ROW
    EXECUTE FUNCTION panvara_enforce_environment_release_pointer_update();

CREATE TRIGGER panvara_environment_release_pointer_reject_truncate
    BEFORE TRUNCATE ON panvara_environment_release_pointer
    FOR EACH STATEMENT
    EXECUTE FUNCTION panvara_enforce_environment_release_pointer_update();
