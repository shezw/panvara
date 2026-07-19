/*
   Panvara
   internal/interfaces/httpapi/access_response.go    2026-07-19
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package httpapi

import (
	"time"

	applicationaccess "github.com/shezw/panvara/internal/application/access"
	domainaccess "github.com/shezw/panvara/internal/domain/access"
)

type principalResponse struct {
	ID          string                       `json:"id"`
	Kind        domainaccess.PrincipalKind   `json:"kind"`
	DisplayName string                       `json:"display_name"`
	Status      domainaccess.PrincipalStatus `json:"status"`
	CreatedAt   time.Time                    `json:"created_at"`
	UpdatedAt   time.Time                    `json:"updated_at"`
	DisabledAt  *time.Time                   `json:"disabled_at,omitempty"`
}

type principalListResponse struct {
	Data []principalResponse `json:"data"`
}

type credentialResponse struct {
	ID          string                        `json:"id"`
	PrincipalID string                        `json:"principal_id"`
	Label       string                        `json:"label"`
	Hint        string                        `json:"hint"`
	Status      domainaccess.CredentialStatus `json:"status"`
	IssuedBy    string                        `json:"issued_by"`
	IssuedAt    time.Time                     `json:"issued_at"`
	RevokedBy   string                        `json:"revoked_by,omitempty"`
	RevokedAt   *time.Time                    `json:"revoked_at,omitempty"`
}

type credentialListResponse struct {
	Data []credentialResponse `json:"data"`
}

type issuedCredentialResponse struct {
	Credential credentialResponse `json:"credential"`
	Token      string             `json:"token"`
}

type ownerGrantResponse struct {
	PrincipalID string     `json:"principal_id"`
	Role        string     `json:"role"`
	Active      bool       `json:"active"`
	GrantedBy   string     `json:"granted_by"`
	GrantedAt   time.Time  `json:"granted_at"`
	RevokedBy   string     `json:"revoked_by,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

type ownerGrantListResponse struct {
	Data []ownerGrantResponse `json:"data"`
}

func makePrincipalResponse(value domainaccess.Principal) principalResponse {
	return principalResponse{
		ID: value.ID(), Kind: value.Kind(), DisplayName: value.DisplayName(), Status: value.Status(),
		CreatedAt: value.CreatedAt().UTC(), UpdatedAt: value.UpdatedAt().UTC(), DisabledAt: value.DisabledAt(),
	}
}

func makePrincipalListResponse(values []domainaccess.Principal) principalListResponse {
	result := principalListResponse{Data: make([]principalResponse, 0, len(values))}
	for _, value := range values {
		result.Data = append(result.Data, makePrincipalResponse(value))
	}
	return result
}

func makeCredentialResponse(value domainaccess.Credential) credentialResponse {
	return credentialResponse{
		ID: value.ID().String(), PrincipalID: value.PrincipalID(), Label: value.Label(), Hint: value.Hint(),
		Status: value.Status(), IssuedBy: value.IssuedBy(), IssuedAt: value.IssuedAt().UTC(),
		RevokedBy: value.RevokedBy(), RevokedAt: value.RevokedAt(),
	}
}

func makeCredentialListResponse(values []domainaccess.Credential) credentialListResponse {
	result := credentialListResponse{Data: make([]credentialResponse, 0, len(values))}
	for _, value := range values {
		result.Data = append(result.Data, makeCredentialResponse(value))
	}
	return result
}

func makeIssuedCredentialResponse(value applicationaccess.IssuedCredential) issuedCredentialResponse {
	return issuedCredentialResponse{Credential: makeCredentialResponse(value.Credential), Token: value.Token}
}

func makeOwnerGrantResponse(value domainaccess.OwnerGrant) ownerGrantResponse {
	return ownerGrantResponse{
		PrincipalID: value.PrincipalID(), Role: value.Role(), Active: value.Active(),
		GrantedBy: value.GrantedBy(), GrantedAt: value.GrantedAt().UTC(),
		RevokedBy: value.RevokedBy(), RevokedAt: value.RevokedAt(),
	}
}

func makeOwnerGrantListResponse(values []domainaccess.OwnerGrant) ownerGrantListResponse {
	result := ownerGrantListResponse{Data: make([]ownerGrantResponse, 0, len(values))}
	for _, value := range values {
		result.Data = append(result.Data, makeOwnerGrantResponse(value))
	}
	return result
}
