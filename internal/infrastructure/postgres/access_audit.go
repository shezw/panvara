/*
   Panvara
   internal/infrastructure/postgres/access_audit.go    2026-07-19
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package postgres

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	applicationaccess "github.com/shezw/panvara/internal/application/access"
	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/project"
)

const (
	auditOutcomeSuccess = "success"
	auditOutcomeDenied  = "denied"
)

var (
	auditRequestIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	auditActionPattern     = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*){1,7}$`)
	auditTargetKindPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*){0,3}$`)
	auditTargetIDPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,255}$`)
	auditReasonPattern     = regexp.MustCompile(`^[a-z][a-z0-9_.:-]{0,63}$`)
)

// AuditDenied independently appends one authenticated authorization denial.
func (store *AccessAdminStore) AuditDenied(
	ctx context.Context,
	attempt applicationaccess.DeniedAttempt,
) error {
	if err := validateDeniedAttempt(attempt); err != nil {
		return err
	}
	return store.appendAudit(ctx, store.pool, auditFact{
		scope: attempt.Scope, actorID: attempt.ActorID, credentialID: attempt.CredentialID,
		requestID: attempt.RequestID, action: string(attempt.Operation),
		outcome: auditOutcomeDenied, targetKind: "authorization",
		targetID: string(attempt.Operation), reason: attempt.Reason,
		occurredAt: attempt.DeniedAt.UTC(),
	})
}

func (store *AccessAdminStore) appendMutationAudit(
	ctx context.Context,
	tx pgx.Tx,
	mutation applicationaccess.MutationContext,
	targetKind string,
	targetID string,
	at time.Time,
) error {
	return store.appendAudit(ctx, tx, auditFact{
		scope: mutation.Scope(), actorID: mutation.ActorID(), credentialID: mutation.CredentialID(),
		requestID: mutation.RequestID(), action: string(mutation.Operation()),
		outcome: auditOutcomeSuccess, targetKind: targetKind, targetID: targetID,
		occurredAt: at.UTC(),
	})
}

type auditExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type auditFact struct {
	scope        project.Scope
	actorID      string
	credentialID domainaccess.ID
	requestID    string
	action       string
	outcome      string
	targetKind   string
	targetID     string
	reason       string
	occurredAt   time.Time
}

func (store *AccessAdminStore) appendAudit(
	ctx context.Context,
	executor auditExecutor,
	fact auditFact,
) error {
	if err := validateAuditFact(ctx, executor, fact); err != nil {
		return err
	}
	eventID, err := store.auditIDs.New()
	if err != nil {
		return fmt.Errorf("generate PostgreSQL security audit id: %w", err)
	}
	if !eventID.Valid() {
		return fmt.Errorf("generate PostgreSQL security audit id: invalid UUIDv7")
	}
	var reason any
	if fact.reason != "" {
		reason = fact.reason
	}
	if _, err := executor.Exec(ctx, `
		INSERT INTO panvara_security_audit_event (
			project_id, environment_id, event_id,
			actor_principal_id, actor_credential_id, request_id,
			action, outcome, target_kind, target_id, reason_code, occurred_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, fact.scope.ProjectID().String(), fact.scope.EnvironmentID().String(),
		eventID.String(), fact.actorID, fact.credentialID.String(), fact.requestID,
		fact.action, fact.outcome, fact.targetKind, fact.targetID, reason,
		fact.occurredAt.UTC()); err != nil {
		return fmt.Errorf("append PostgreSQL security audit: %w", err)
	}
	return nil
}

func validateDeniedAttempt(attempt applicationaccess.DeniedAttempt) error {
	if err := validateAccessScope(attempt.Scope); err != nil {
		return err
	}
	if !principalIDPattern.MatchString(attempt.ActorID) || !attempt.CredentialID.Valid() ||
		!attempt.Operation.Valid() || !auditRequestIDPattern.MatchString(attempt.RequestID) ||
		!auditReasonPattern.MatchString(attempt.Reason) || attempt.DeniedAt.IsZero() {
		return fmt.Errorf("%w: invalid denied audit attempt", applicationaccess.ErrInvalid)
	}
	return nil
}

func validateAuditFact(ctx context.Context, executor auditExecutor, fact auditFact) error {
	if ctx == nil || executor == nil {
		return fmt.Errorf("%w: incomplete audit dependencies", applicationaccess.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateAccessScope(fact.scope); err != nil {
		return err
	}
	if !principalIDPattern.MatchString(fact.actorID) || !fact.credentialID.Valid() ||
		!auditRequestIDPattern.MatchString(fact.requestID) ||
		!auditActionPattern.MatchString(fact.action) ||
		!auditTargetKindPattern.MatchString(fact.targetKind) ||
		!auditTargetIDPattern.MatchString(fact.targetID) || fact.occurredAt.IsZero() {
		return fmt.Errorf("%w: invalid security audit fact", applicationaccess.ErrInvalid)
	}
	switch fact.outcome {
	case auditOutcomeSuccess:
		if fact.reason != "" {
			return fmt.Errorf("%w: successful audit cannot have a reason", applicationaccess.ErrInvalid)
		}
	case auditOutcomeDenied:
		if !auditReasonPattern.MatchString(fact.reason) {
			return fmt.Errorf("%w: denied audit reason is invalid", applicationaccess.ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: audit outcome is invalid", applicationaccess.ErrInvalid)
	}
	return nil
}
