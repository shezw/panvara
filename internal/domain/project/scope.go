/*
   Panvara
   internal/domain/project/scope.go    2026-07-18
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package project

import "fmt"

// Scope is the exact project and deployment-environment isolation boundary.
type Scope struct {
	projectID     ID
	environmentID EnvironmentID
}

// NewScope validates and constructs an exact project/environment scope.
func NewScope(projectID ID, environmentID EnvironmentID) (Scope, error) {
	scope := Scope{projectID: projectID, environmentID: environmentID}
	if err := scope.Validate(); err != nil {
		return Scope{}, err
	}
	return scope, nil
}

// Validate rejects incomplete project or environment boundaries.
func (scope Scope) Validate() error {
	if !scope.projectID.Valid() {
		return fmt.Errorf("project scope has an invalid project id")
	}
	if !scope.environmentID.Valid() {
		return fmt.Errorf("project scope has an invalid environment id")
	}
	return nil
}

// ProjectID returns the stable project boundary.
func (scope Scope) ProjectID() ID {
	return scope.projectID
}

// EnvironmentID returns the stable deployment-environment boundary.
func (scope Scope) EnvironmentID() EnvironmentID {
	return scope.environmentID
}
