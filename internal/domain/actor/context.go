/*
   Panvara
   internal/domain/actor/context.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package actor defines the authenticated or anonymous subject of a use case.
package actor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/shezw/panvara/internal/domain/project"
)

var actorIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var rolePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

const maxRoles = 64

// Context is explicit input to authorization-sensitive use cases. Dynamic
// resources may reference its ActorID but cannot redefine the identity model.
type Context struct {
	actorID   string
	projectID project.ID
	roles     []string
}

// New creates an authenticated actor scoped to one project.
func New(projectID, actorID string, roles []string) (Context, error) {
	actorID = strings.TrimSpace(actorID)
	parsedProjectID, err := project.ParseID(projectID)
	if err != nil {
		return Context{}, err
	}
	if !actorIDPattern.MatchString(actorID) {
		return Context{}, fmt.Errorf("invalid actor id %q", actorID)
	}
	normalizedRoles, err := normalizeRoles(roles)
	if err != nil {
		return Context{}, err
	}
	return Context{
		actorID:   actorID,
		projectID: parsedProjectID,
		roles:     normalizedRoles,
	}, nil
}

// NewAnonymous creates an unauthenticated actor scoped to one project.
func NewAnonymous(projectID string) (Context, error) {
	parsedProjectID, err := project.ParseID(projectID)
	if err != nil {
		return Context{}, err
	}
	return Context{projectID: parsedProjectID}, nil
}

// ActorID returns the stable Core actor identifier, or an empty string for an
// anonymous actor.
func (context Context) ActorID() string {
	return context.actorID
}

// ProjectID returns the project authorization boundary.
func (context Context) ProjectID() project.ID {
	return context.projectID
}

// Roles returns a stable defensive copy.
func (context Context) Roles() []string {
	return append([]string(nil), context.roles...)
}

// Anonymous reports whether no authenticated identity is present.
func (context Context) Anonymous() bool {
	return context.actorID == ""
}

// Valid reports whether the context has a project boundary and represents
// either an authenticated or anonymous actor created by a constructor.
func (context Context) Valid() bool {
	return context.projectID.Valid()
}

// HasRole checks a normalized role without exposing mutable state.
func (context Context) HasRole(role string) bool {
	index := sort.SearchStrings(context.roles, role)
	return index < len(context.roles) && context.roles[index] == role
}

func normalizeRoles(roles []string) ([]string, error) {
	if len(roles) > maxRoles {
		return nil, fmt.Errorf("actor has %d roles; maximum is %d", len(roles), maxRoles)
	}
	result := append([]string(nil), roles...)
	for _, role := range result {
		if !rolePattern.MatchString(role) {
			return nil, fmt.Errorf("invalid role %q", role)
		}
	}
	sort.Strings(result)
	for index := 1; index < len(result); index++ {
		if result[index] == result[index-1] {
			return nil, fmt.Errorf("duplicate role %q", result[index])
		}
	}
	return result, nil
}
