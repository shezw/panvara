-- Panvara
-- db/migrations/0005_project_access_administration.sql    2026-07-19
--
-- @link    : https://github.com/shezw/panvara
-- @author  : shezw
-- @email   : hello@shezw.com

ALTER TABLE panvara_principal
    ADD COLUMN kind text,
    ADD COLUMN display_name text,
    ADD COLUMN disabled_at timestamptz;

UPDATE panvara_principal
SET kind = 'bootstrap',
    display_name = principal_id,
    disabled_at = CASE WHEN status = 'disabled' THEN updated_at ELSE NULL END;

ALTER TABLE panvara_principal
    ALTER COLUMN kind SET NOT NULL,
    ALTER COLUMN display_name SET NOT NULL,
    ADD CONSTRAINT panvara_principal_kind_check CHECK (
        kind IN ('bootstrap', 'service')
    ),
    ADD CONSTRAINT panvara_principal_kind_identity_check CHECK (
        (
            kind = 'service'
            AND principal_id ~ '^svc:[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
        )
        OR
        kind = 'bootstrap'
    ),
    ADD CONSTRAINT panvara_principal_display_name_check CHECK (
        octet_length(display_name) BETWEEN 1 AND 128
        AND display_name = btrim(display_name)
        AND display_name !~ '[[:cntrl:]]'
    ),
    ADD CONSTRAINT panvara_principal_disabled_state_check CHECK (
        (status = 'active' AND disabled_at IS NULL)
        OR
        (status = 'disabled' AND disabled_at IS NOT NULL)
    ),
    ADD CONSTRAINT panvara_principal_disabled_time_check CHECK (
        disabled_at IS NULL
        OR (disabled_at >= created_at AND disabled_at <= updated_at)
    );

CREATE FUNCTION panvara_enforce_principal_terminal_update()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'panvara principal lifecycle facts cannot be deleted'
            USING ERRCODE = '55000';
    END IF;
    IF NEW.project_id IS DISTINCT FROM OLD.project_id
        OR NEW.principal_id IS DISTINCT FROM OLD.principal_id
        OR NEW.kind IS DISTINCT FROM OLD.kind
        OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'panvara principal identity and provenance are immutable'
            USING ERRCODE = '55000';
    END IF;
    IF OLD.status = 'disabled' AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'panvara disabled principal facts are immutable'
            USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER panvara_principal_terminal_update
BEFORE UPDATE OR DELETE ON panvara_principal
FOR EACH ROW
EXECUTE FUNCTION panvara_enforce_principal_terminal_update();

ALTER TABLE panvara_access_grant
    ADD COLUMN granted_by_principal_id text,
    ADD COLUMN revoked_by_principal_id text,
    ADD COLUMN updated_at timestamptz;

UPDATE panvara_access_grant
SET granted_by_principal_id = principal_id,
    revoked_by_principal_id = CASE WHEN revoked_at IS NULL THEN NULL ELSE principal_id END,
    updated_at = GREATEST(granted_at, COALESCE(revoked_at, granted_at));

ALTER TABLE panvara_access_grant
    ALTER COLUMN granted_by_principal_id SET NOT NULL,
    ALTER COLUMN updated_at SET NOT NULL,
    ADD CONSTRAINT panvara_access_grant_actor_state_check CHECK (
        (revoked_at IS NULL AND revoked_by_principal_id IS NULL)
        OR
        (revoked_at IS NOT NULL AND revoked_by_principal_id IS NOT NULL)
    ),
    ADD CONSTRAINT panvara_access_grant_updated_time_check CHECK (
        updated_at >= granted_at
        AND (revoked_at IS NULL OR (revoked_at >= granted_at AND revoked_at <= updated_at))
    ),
    ADD CONSTRAINT panvara_access_grant_granted_by_fk
        FOREIGN KEY (project_id, granted_by_principal_id)
        REFERENCES panvara_principal (project_id, principal_id) ON DELETE RESTRICT,
    ADD CONSTRAINT panvara_access_grant_revoked_by_fk
        FOREIGN KEY (project_id, revoked_by_principal_id)
        REFERENCES panvara_principal (project_id, principal_id) ON DELETE RESTRICT;

CREATE TABLE panvara_api_credential (
    project_id uuid NOT NULL,
    environment_id uuid NOT NULL,
    credential_id uuid NOT NULL,
    principal_id text NOT NULL,
    label text NOT NULL,
    secret_digest bytea NOT NULL,
    secret_hint text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    issued_by_principal_id text NOT NULL,
    revoked_by_principal_id text,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    revoked_at timestamptz,
    CONSTRAINT panvara_api_credential_pkey PRIMARY KEY (
        project_id,
        environment_id,
        credential_id
    ),
    CONSTRAINT panvara_api_credential_project_id_unique UNIQUE (
        project_id,
        credential_id
    ),
    CONSTRAINT panvara_api_credential_secret_unique UNIQUE (
        project_id,
        secret_digest
    ),
    CONSTRAINT panvara_api_credential_actor_identity_unique UNIQUE (
        project_id,
        environment_id,
        credential_id,
        principal_id
    ),
    CONSTRAINT panvara_api_credential_marker_identity_unique UNIQUE (
        project_id,
        environment_id,
        credential_id,
        principal_id,
        secret_digest,
        secret_hint
    ),
    CONSTRAINT panvara_api_credential_uuidv7_check CHECK (
        credential_id::text ~ '^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
    ),
    CONSTRAINT panvara_api_credential_label_check CHECK (
        octet_length(label) BETWEEN 1 AND 128
        AND label = btrim(label)
        AND label !~ '[[:cntrl:]]'
    ),
    CONSTRAINT panvara_api_credential_digest_check CHECK (
        octet_length(secret_digest) = 32
    ),
    CONSTRAINT panvara_api_credential_hint_check CHECK (
        octet_length(secret_hint) = 19
        AND secret_hint ~ '^sha256:[0-9a-f]{12}$'
        AND secret_hint = 'sha256:' || substring(encode(secret_digest, 'hex') FROM 1 FOR 12)
    ),
    CONSTRAINT panvara_api_credential_status_check CHECK (
        status IN ('active', 'revoked')
    ),
    CONSTRAINT panvara_api_credential_revoke_state_check CHECK (
        (status = 'active' AND revoked_at IS NULL AND revoked_by_principal_id IS NULL)
        OR
        (status = 'revoked' AND revoked_at IS NOT NULL AND revoked_by_principal_id IS NOT NULL)
    ),
    CONSTRAINT panvara_api_credential_time_order_check CHECK (
        updated_at >= created_at
        AND (revoked_at IS NULL OR (revoked_at >= created_at AND revoked_at <= updated_at))
    ),
    CONSTRAINT panvara_api_credential_environment_fk
        FOREIGN KEY (project_id, environment_id)
        REFERENCES panvara_environment (project_id, environment_id) ON DELETE RESTRICT,
    CONSTRAINT panvara_api_credential_principal_fk
        FOREIGN KEY (project_id, principal_id)
        REFERENCES panvara_principal (project_id, principal_id) ON DELETE RESTRICT,
    CONSTRAINT panvara_api_credential_issued_by_fk
        FOREIGN KEY (project_id, issued_by_principal_id)
        REFERENCES panvara_principal (project_id, principal_id) ON DELETE RESTRICT,
    CONSTRAINT panvara_api_credential_revoked_by_fk
        FOREIGN KEY (project_id, revoked_by_principal_id)
        REFERENCES panvara_principal (project_id, principal_id) ON DELETE RESTRICT
);

CREATE INDEX panvara_api_credential_active_lookup_idx
    ON panvara_api_credential (project_id, environment_id, credential_id, principal_id)
    WHERE status = 'active';

CREATE INDEX panvara_api_credential_principal_list_idx
    ON panvara_api_credential (project_id, environment_id, principal_id, created_at, credential_id);

CREATE FUNCTION panvara_enforce_api_credential_terminal_update()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'panvara credential lifecycle facts cannot be deleted'
            USING ERRCODE = '55000';
    END IF;
    IF NEW.project_id IS DISTINCT FROM OLD.project_id
        OR NEW.environment_id IS DISTINCT FROM OLD.environment_id
        OR NEW.credential_id IS DISTINCT FROM OLD.credential_id
        OR NEW.principal_id IS DISTINCT FROM OLD.principal_id
        OR NEW.issued_by_principal_id IS DISTINCT FROM OLD.issued_by_principal_id
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
        OR NEW.secret_digest IS DISTINCT FROM OLD.secret_digest
        OR NEW.secret_hint IS DISTINCT FROM OLD.secret_hint THEN
        RAISE EXCEPTION 'panvara credential identity, provenance, and secret evidence are immutable'
            USING ERRCODE = '55000';
    END IF;
    IF OLD.status = 'revoked' AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'panvara revoked credential facts are immutable'
            USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER panvara_api_credential_terminal_update
BEFORE UPDATE OR DELETE ON panvara_api_credential
FOR EACH ROW
EXECUTE FUNCTION panvara_enforce_api_credential_terminal_update();

CREATE TABLE panvara_access_bootstrap_marker (
    project_id uuid NOT NULL,
    environment_id uuid NOT NULL,
    principal_id text NOT NULL,
    credential_id uuid NOT NULL,
    secret_digest bytea NOT NULL,
    secret_hint text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT panvara_access_bootstrap_marker_pkey PRIMARY KEY (
        project_id,
        environment_id
    ),
    CONSTRAINT panvara_access_bootstrap_marker_digest_check CHECK (
        octet_length(secret_digest) = 32
    ),
    CONSTRAINT panvara_access_bootstrap_marker_hint_check CHECK (
        octet_length(secret_hint) = 19
        AND secret_hint ~ '^sha256:[0-9a-f]{12}$'
        AND secret_hint = 'sha256:' || substring(encode(secret_digest, 'hex') FROM 1 FOR 12)
    ),
    CONSTRAINT panvara_access_bootstrap_marker_environment_fk
        FOREIGN KEY (project_id, environment_id)
        REFERENCES panvara_environment (project_id, environment_id) ON DELETE RESTRICT,
    CONSTRAINT panvara_access_bootstrap_marker_principal_fk
        FOREIGN KEY (project_id, principal_id)
        REFERENCES panvara_principal (project_id, principal_id) ON DELETE RESTRICT,
    CONSTRAINT panvara_access_bootstrap_marker_credential_fk
        FOREIGN KEY (
            project_id,
            environment_id,
            credential_id,
            principal_id,
            secret_digest,
            secret_hint
        ) REFERENCES panvara_api_credential (
            project_id,
            environment_id,
            credential_id,
            principal_id,
            secret_digest,
            secret_hint
        ) ON DELETE RESTRICT
);

CREATE TABLE panvara_security_audit_event (
    project_id uuid NOT NULL,
    environment_id uuid NOT NULL,
    event_id uuid NOT NULL,
    actor_principal_id text NOT NULL,
    actor_credential_id uuid NOT NULL,
    request_id text NOT NULL,
    action text NOT NULL,
    outcome text NOT NULL,
    target_kind text NOT NULL,
    target_id text NOT NULL,
    reason_code text,
    occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT panvara_security_audit_event_pkey PRIMARY KEY (
        project_id,
        environment_id,
        event_id
    ),
    CONSTRAINT panvara_security_audit_event_uuidv7_check CHECK (
        event_id::text ~ '^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
    ),
    CONSTRAINT panvara_security_audit_event_request_id_check CHECK (
        octet_length(request_id) BETWEEN 1 AND 128
        AND request_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]*$'
    ),
    CONSTRAINT panvara_security_audit_event_action_check CHECK (
        octet_length(action) BETWEEN 3 AND 128
        AND action ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*){1,7}$'
    ),
    CONSTRAINT panvara_security_audit_event_outcome_check CHECK (
        outcome IN ('success', 'denied')
    ),
    CONSTRAINT panvara_security_audit_event_target_kind_check CHECK (
        octet_length(target_kind) BETWEEN 3 AND 64
        AND target_kind ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*){0,3}$'
    ),
    CONSTRAINT panvara_security_audit_event_target_id_check CHECK (
        octet_length(target_id) BETWEEN 1 AND 256
        AND target_id = btrim(target_id)
        AND target_id !~ '[[:cntrl:]]'
    ),
    CONSTRAINT panvara_security_audit_event_reason_check CHECK (
        reason_code IS NULL
        OR (
            octet_length(reason_code) BETWEEN 1 AND 64
            AND reason_code ~ '^[a-z][a-z0-9_.:-]*$'
        )
    ),
    CONSTRAINT panvara_security_audit_event_outcome_reason_check CHECK (
        (outcome = 'success' AND reason_code IS NULL)
        OR
        (outcome = 'denied' AND reason_code IS NOT NULL)
    ),
    CONSTRAINT panvara_security_audit_event_environment_fk
        FOREIGN KEY (project_id, environment_id)
        REFERENCES panvara_environment (project_id, environment_id) ON DELETE RESTRICT,
    CONSTRAINT panvara_security_audit_event_actor_principal_fk
        FOREIGN KEY (project_id, actor_principal_id)
        REFERENCES panvara_principal (project_id, principal_id) ON DELETE RESTRICT,
    CONSTRAINT panvara_security_audit_event_actor_credential_fk
        FOREIGN KEY (
            project_id,
            environment_id,
            actor_credential_id,
            actor_principal_id
        ) REFERENCES panvara_api_credential (
            project_id,
            environment_id,
            credential_id,
            principal_id
        ) ON DELETE RESTRICT
);

CREATE INDEX panvara_security_audit_event_project_time_idx
    ON panvara_security_audit_event (project_id, environment_id, occurred_at, event_id);

CREATE FUNCTION panvara_reject_append_only_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'panvara immutable security facts are append-only'
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER panvara_access_bootstrap_marker_append_only
BEFORE UPDATE OR DELETE ON panvara_access_bootstrap_marker
FOR EACH ROW
EXECUTE FUNCTION panvara_reject_append_only_mutation();

CREATE TRIGGER panvara_security_audit_event_append_only
BEFORE UPDATE OR DELETE ON panvara_security_audit_event
FOR EACH ROW
EXECUTE FUNCTION panvara_reject_append_only_mutation();
