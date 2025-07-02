package mserve

import (
	"fmt"
	"github.com/Seann-Moser/credentials/oauth/oserver"
	"github.com/Seann-Moser/credentials/user"
	"github.com/Seann-Moser/rbac"
	"github.com/Seann-Moser/rbac/rbacServer"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-webauthn/webauthn/protocol"
	"net/http"
	"sort"
	"strings"
)

// endpointResourceName generates a standardized resource name for an API endpoint.
// It combines the given URL path and HTTP methods into a consistent string.
//
// The path should be a clean URL path (e.g., "/users/{id}/profile").
// The methods should be one or more HTTP methods (e.g., "GET", "POST", "PUT", "DELETE").
//
// Example:
// endpointResourceName("/users/{id}", "GET") would return "users.{id}.GET"
// endpointResourceName("/orders", "POST", "PUT") would return "orders.POST.PUT"
func endpointResourceName(service, path string, methods ...string) string {
	// Normalize the path: remove leading/trailing slashes and replace internal slashes with periods.
	// This helps create a more "flat" resource name.
	normalizedPath := strings.Trim(service+path, "/")
	normalizedPath = strings.ReplaceAll(normalizedPath, "/", ".")

	// Convert methods to uppercase to ensure consistency.
	// Sort methods alphabetically to ensure a consistent resource name regardless of input order.
	// E.g., ["GET", "POST"] should result in the same name as ["POST", "GET"].
	upperMethods := make([]string, len(methods))
	for i, m := range methods {
		upperMethods[i] = strings.ToUpper(m)
	}
	sort.Strings(upperMethods) // Sort for consistent order

	// Join the normalized path and sorted methods with periods.
	// This creates a unique identifier for the endpoint and its allowed operations.
	if len(upperMethods) > 0 {
		return fmt.Sprintf("%s.%s", normalizedPath, strings.Join(upperMethods, "."))
	}
	return normalizedPath // If no methods are provided, just use the normalized path
}

// Define Request/Response Body Structs for your new handlers
type AssignRoleToGroupRequest struct {
	GroupID string `json:"group_id"`
	RoleID  string `json:"role_id"`
}

// CreateRoleRequest uses rbac.Role directly as the body, which is good.
// The rbac.Role struct should be accessible for schema generation.

// MessageResponse is a common response for success messages.
type MessageResponse struct {
	Message string `json:"message"`
	RoleID  string `json:"role_id,omitempty"`       // For CreateRole response
	PermID  string `json:"permission_id,omitempty"` // For CreatePermission response
	UserID  string `json:"user_id,omitempty"`       // For CreateUser response
}

// ErrorResponse is a common response for error messages.
type ErrorResponse struct {
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// ListRolesForGroupResponse just returns a slice of rbac.Role, so []*rbac.Role will be the body type.

// New structs for permission handlers
type AssignPermToRoleRequest struct {
	RoleID string `json:"role_id"`
	PermID string `json:"perm_id"`
}

// New structs for user handlers
// CreateUserRequest body is rbac.User directly.

type AssignUserRoleRequest struct {
	UserID string `json:"user_id"`
	RoleID string `json:"role_id"`
}

type AddRemoveUserGroupRequest struct {
	GroupID   string `json:"group_id"`
	UserID    string `json:"user_id"`
	GroupName string `json:"group_name,omitempty"` // GroupName might be optional or inferred
}

type HasPermissionResponse struct {
	HasPermission bool `json:"has_permission"`
}

type CanRequest struct {
	UserID   string `json:"user_id"`
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

type CanResponse struct {
	CanPerformAction bool `json:"can_perform_action"`
}

func makeUserEndpoints(u *user.Server) []*Endpoint {
	return []*Endpoint{
		{
			Handler: u.RegisterHandler,
			Path:    "/user/register",
			Methods: []string{http.MethodPost},
			Request: Request{
				Body: user.RegisterRequest{},
			},
			Responses: []Response{
				{
					Status: http.StatusCreated,
				},
				{Status: http.StatusConflict},
				{Status: http.StatusBadRequest},
				{Status: http.StatusInternalServerError},
			},
		},
		{
			Handler: u.LoginPasswordHandler,
			Path:    "/user/login",
			Methods: []string{http.MethodPost},
			Request: Request{
				Body: user.LoginRequest{},
			},
			Responses: []Response{
				{
					Status: http.StatusCreated,
				},
				{Status: http.StatusConflict},
				{Status: http.StatusBadRequest},
				{Status: http.StatusInternalServerError},
			},
		},
		{
			Handler: u.LoginTOTPHandler,
			Path:    "/user/login/totp",
			Methods: []string{http.MethodPost},
			Request: Request{
				Body: user.TOTPLoginRequest{},
			},
			Responses: []Response{
				{
					Status: http.StatusCreated,
				},
				{Status: http.StatusConflict},
				{Status: http.StatusBadRequest},
				{Status: http.StatusInternalServerError},
			},
		},
		{
			Handler: u.LogoutHandler,
			Path:    "/user/logout",
			Methods: []string{http.MethodGet},
			Responses: []Response{
				{
					Status: http.StatusOK,
				},
			},
		},
		{
			Handler: u.DeleteUserHandler,
			Path:    "/user/delete",
			Methods: []string{http.MethodDelete},
			Request: Request{
				Body: user.UserIDRequest{},
			},
			Responses: []Response{
				{
					Status: http.StatusOK,
				},
				{Status: http.StatusBadRequest},
				{Status: http.StatusInternalServerError},
			},
		},
		{
			Handler: u.ManageUserHandler,
			Path:    "/user/manage",
			Methods: []string{http.MethodPatch},
			Request: Request{
				Body: user.UpdateUserRolesRequest{},
			},
			Responses: []Response{
				{
					Status: http.StatusOK,
				},
				{Status: http.StatusBadRequest},
				{Status: http.StatusInternalServerError},
			},
		},
		{
			Handler: u.UserSettingsHandler,
			Path:    "/user/settings",
			Methods: []string{http.MethodPatch},
			Request: Request{
				Body: user.UserSettingsUpdateRequest{},
			},
			Responses: []Response{
				{
					Status: http.StatusOK,
				},
				{Status: http.StatusBadRequest},
				{Status: http.StatusInternalServerError},
			},
		},
		{
			Handler: u.BeginPasskeyRegistrationHandler,
			Path:    "/user/begin_passkey",
			Methods: []string{http.MethodGet},
			Request: Request{
				Body: user.BeginPasskeyRegistrationRequest{},
			},
			Responses: []Response{
				{
					Status: http.StatusOK,
					Body:   protocol.CredentialCreation{},
				},
				{Status: http.StatusUnauthorized},
				{Status: http.StatusInternalServerError},
			},
		},
		{
			Handler: u.FinishPasskeyRegistrationHandler,
			Path:    "/user/finish_passkey",
			Methods: []string{http.MethodPost},
			Responses: []Response{
				{
					Status: http.StatusOK,
				},
				{Status: http.StatusUnauthorized},
				{Status: http.StatusInternalServerError},
			},
			Request: Request{
				Body: protocol.CredentialCreationResponse{},
			},
		},
		{
			Handler: u.BeginPasskeyLoginHandler,
			Path:    "/user/login/begin_passkey",
			Methods: []string{http.MethodPost},
			Request: Request{
				Body: user.BeginPasskeyLoginRequest{},
			},
			Responses: []Response{
				{
					Status: http.StatusOK,
					Body:   protocol.CredentialCreation{},
					Headers: map[string]ROption{
						"X-WebAuthn-Session-ID": {
							Required: true,
						},
					},
				},
				{Status: http.StatusUnauthorized},
				{Status: http.StatusInternalServerError},
			},
		},
		{
			Handler: u.FinishPasskeyLoginHandler,
			Path:    "/user/login/finish_passkey",
			Methods: []string{http.MethodPost},
			Request: Request{Headers: map[string]ROption{
				"X-WebAuthn-Session-ID": {},
			},
				Body: protocol.CredentialAssertionResponse{}},
			Responses: []Response{
				{
					Status: http.StatusOK,
				},
				{Status: http.StatusUnauthorized},
				{Status: http.StatusInternalServerError},
			},
		},
		{
			Handler: u.DeletePasskeyHandler,
			Path:    "/user/delete_passkey",
			Methods: []string{http.MethodDelete},
			Request: Request{
				Body: user.DeletePasskeyRequest{},
			},
			Responses: []Response{
				{
					Status: http.StatusOK,
				},
				{Status: http.StatusUnauthorized},
				{Status: http.StatusForbidden},
				{Status: http.StatusInternalServerError},
			},
		},
		{
			Handler: u.GenerateTOTPSecretHandler,
			Path:    "/user/generate/totp",
			Methods: []string{http.MethodPost},
			Responses: []Response{
				{
					Status: http.StatusOK,
					Body:   user.GenerateTOTPResponse{},
				},
				{Status: http.StatusUnauthorized},
				{Status: http.StatusForbidden},
				{Status: http.StatusInternalServerError},
			},
		},
		{
			Handler: u.VerifyAndEnableTOTPHandler,
			Path:    "/user/verify/totp",
			Methods: []string{http.MethodPost},
			Request: Request{
				Body: user.VerifyTOTPRequest{},
			},
			Responses: []Response{
				{
					Status: http.StatusOK,
				},
				{Status: http.StatusUnauthorized},
				{Status: http.StatusForbidden},
				{Status: http.StatusInternalServerError},
			},
		},
		{
			Handler: u.DisableTOTPHandler,
			Path:    "/user/disable/totp",
			Methods: []string{http.MethodDelete},
			Responses: []Response{
				{
					Status: http.StatusOK,
				},
				{Status: http.StatusUnauthorized},
				{Status: http.StatusForbidden},
				{Status: http.StatusInternalServerError},
			},
		},
		{
			Handler: u.GetUser,
			Path:    "/user",
			Methods: []string{http.MethodGet},
			Responses: []Response{
				{
					Status: http.StatusOK,
					Body:   user.User{},
				},
				{Status: http.StatusUnauthorized},
				{Status: http.StatusForbidden},
				{Status: http.StatusInternalServerError},
			},
		},
	}
}

func makeRBACEndpoints(s *rbacServer.Server) []*Endpoint {
	return []*Endpoint{
		{
			Name:        "AssignRoleToGroup",
			Description: "Assigns a role to a group.",
			Methods:     []string{http.MethodPost},
			Path:        "/roles/assign-to-group",
			Handler:     s.AssignRoleToGroupHandler,
			Request: Request{
				Body: &AssignRoleToGroupRequest{},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "Role assigned successfully", Body: &MessageResponse{}},
				{Status: http.StatusBadRequest, Message: "Invalid request", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Internal server error", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionCreate},
				{Role: "role_manager", Access: rbac.ActionUpdate},
			},
		},
		{
			Name:        "UnassignRoleFromGroup",
			Description: "Unassigns a role from a group.",
			Methods:     []string{http.MethodPost},
			Path:        "/roles/unassign-from-group",
			Handler:     s.UnassignRoleFromGroupHandler,
			Request: Request{
				Body: &AssignRoleToGroupRequest{},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "Role unassigned successfully", Body: &MessageResponse{}},
				{Status: http.StatusBadRequest, Message: "Invalid request", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Internal server error", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionDelete},
				{Role: "role_manager", Access: rbac.ActionUpdate},
			},
		},
		{
			Name:        "ListRolesForGroup",
			Description: "Lists all roles assigned to a specific group.",
			Methods:     []string{http.MethodGet},
			Path:        "/roles/list-for-group",
			Handler:     s.ListRolesForGroupHandler,
			Request: Request{
				Params: map[string]ROption{
					"group_id": {Description: "ID of the group to list roles for.", Required: true, Type: openapi3.TypeString},
				},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "Roles listed successfully", Body: []*rbac.Role{}},
				{Status: http.StatusBadRequest, Message: "Missing group_id parameter", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Internal server error", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionRead},
				{Role: "viewer", Access: rbac.ActionRead},
			},
		},
		{
			Name:        "CreateRole",
			Description: "Creates a new RBAC role.",
			Methods:     []string{http.MethodPost},
			Path:        "/roles/create",
			Handler:     s.CreateRoleHandler,
			Request: Request{
				Body: &rbac.Role{},
			},
			Responses: []Response{
				{Status: http.StatusCreated, Message: "Role created successfully", Body: &MessageResponse{}},
				{Status: http.StatusBadRequest, Message: "Invalid request body", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Failed to create role", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionCreate},
				{Role: "role_manager", Access: rbac.ActionCreate},
			},
		},
		{
			Name:        "DeleteRole",
			Description: "Deletes an RBAC role by ID.",
			Methods:     []string{http.MethodDelete},
			Path:        "/roles/delete",
			Handler:     s.DeleteRoleHandler,
			Request: Request{
				Params: map[string]ROption{
					"id": {Description: "ID of the role to delete.", Required: true, Type: openapi3.TypeString},
				},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "Role deleted successfully", Body: &MessageResponse{}},
				{Status: http.StatusBadRequest, Message: "Missing role ID parameter", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Failed to delete role", Body: &ErrorResponse{}},
				{Status: http.StatusNotFound, Message: "Role not found", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionDelete},
				{Role: "role_manager", Access: rbac.ActionDelete},
			},
		},
		{
			Name:        "GetRole",
			Description: "Retrieves an RBAC role by ID.",
			Methods:     []string{http.MethodGet},
			Path:        "/roles/get",
			Handler:     s.GetRoleHandler,
			Request: Request{
				Params: map[string]ROption{
					"id": {Description: "ID of the role to retrieve.", Required: true, Type: openapi3.TypeString},
				},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "Role retrieved successfully", Body: &rbac.Role{}},
				{Status: http.StatusBadRequest, Message: "Missing role ID parameter", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Failed to retrieve role", Body: &ErrorResponse{}},
				{Status: http.StatusNotFound, Message: "Role not found", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionRead},
				{Role: "viewer", Access: rbac.ActionRead},
			},
		},
		{
			Name:        "ListAllRoles",
			Description: "Lists all available RBAC roles.",
			Methods:     []string{http.MethodGet},
			Path:        "/roles",
			Handler:     s.ListRoles,
			Responses: []Response{
				{Status: http.StatusOK, Message: "Roles listed successfully", Body: []*rbac.Role{}},
				{Status: http.StatusInternalServerError, Message: "Internal server error", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionRead},
				{Role: "viewer", Access: rbac.ActionRead},
			},
		},

		// New Permission Endpoints
		{
			Name:        "CreatePermission",
			Description: "Creates a new RBAC permission.",
			Methods:     []string{http.MethodPost},
			Path:        "/permissions/create",
			Handler:     s.CreatePermissionHandler,
			Request: Request{
				Body: &rbac.Permission{}, // Uses rbac.Permission directly
			},
			Responses: []Response{
				{Status: http.StatusCreated, Message: "Permission created successfully", Body: &MessageResponse{}},
				{Status: http.StatusBadRequest, Message: "Invalid request body", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Failed to create permission", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionCreate},
				{Role: "permission_manager", Access: rbac.ActionCreate},
			},
		},
		{
			Name:        "DeletePermission",
			Description: "Deletes an RBAC permission by ID.",
			Methods:     []string{http.MethodDelete},
			Path:        "/permissions/delete",
			Handler:     s.DeletePermissionHandler,
			Request: Request{
				Params: map[string]ROption{
					"id": {Description: "ID of the permission to delete.", Required: true, Type: openapi3.TypeString},
				},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "Permission deleted successfully", Body: &MessageResponse{}},
				{Status: http.StatusBadRequest, Message: "Missing permission ID parameter", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Failed to delete permission", Body: &ErrorResponse{}},
				{Status: http.StatusNotFound, Message: "Permission not found", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionDelete},
				{Role: "permission_manager", Access: rbac.ActionDelete},
			},
		},
		{
			Name:        "GetPermission",
			Description: "Retrieves an RBAC permission by ID.",
			Methods:     []string{http.MethodGet},
			Path:        "/permissions/get",
			Handler:     s.GetPermissionHandler,
			Request: Request{
				Params: map[string]ROption{
					"id": {Description: "ID of the permission to retrieve.", Required: true, Type: openapi3.TypeString},
				},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "Permission retrieved successfully", Body: &rbac.Permission{}},
				{Status: http.StatusBadRequest, Message: "Missing permission ID parameter", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Failed to retrieve permission", Body: &ErrorResponse{}},
				{Status: http.StatusNotFound, Message: "Permission not found", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionRead},
				{Role: "viewer", Access: rbac.ActionRead},
			},
		},
		{
			Name:        "AssignPermissionToRole",
			Description: "Assigns a permission to a role.",
			Methods:     []string{http.MethodPost},
			Path:        "/permissions/assign-to-role",
			Handler:     s.AssignPermissionToRoleHandler,
			Request: Request{
				Body: &AssignPermToRoleRequest{},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "Permission assigned successfully", Body: &MessageResponse{}},
				{Status: http.StatusBadRequest, Message: "Invalid request body", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Internal server error", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionCreate},
				{Role: "permission_manager", Access: rbac.ActionUpdate},
			},
		},
		{
			Name:        "RemovePermissionFromRole",
			Description: "Removes a permission from a role.",
			Methods:     []string{http.MethodPost},
			Path:        "/permissions/remove-from-role",
			Handler:     s.RemovePermissionFromRoleHandler,
			Request: Request{
				Body: &AssignPermToRoleRequest{},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "Permission removed successfully", Body: &MessageResponse{}},
				{Status: http.StatusBadRequest, Message: "Invalid request body", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Internal server error", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionDelete},
				{Role: "permission_manager", Access: rbac.ActionUpdate},
			},
		},
		{
			Name:        "ListPermissionsForRole",
			Description: "Lists all permissions assigned to a specific role.",
			Methods:     []string{http.MethodGet},
			Path:        "/permissions/list-for-role",
			Handler:     s.ListPermissionsForRoleHandler,
			Request: Request{
				Params: map[string]ROption{
					"role_id": {Description: "ID of the role to list permissions for.", Required: true, Type: openapi3.TypeString},
				},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "Permissions listed successfully", Body: []*rbac.Permission{}},
				{Status: http.StatusBadRequest, Message: "Missing role_id parameter", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Internal server error", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionRead},
				{Role: "viewer", Access: rbac.ActionRead},
			},
		},

		// New User Endpoints
		{
			Name:        "CreateUser",
			Description: "Creates a new RBAC user.",
			Methods:     []string{http.MethodPost},
			Path:        "/users/create",
			Handler:     s.CreateUserHandler,
			Request: Request{
				Body: &rbac.User{}, // Uses rbac.User directly
			},
			Responses: []Response{
				{Status: http.StatusCreated, Message: "User created successfully", Body: &MessageResponse{}},
				{Status: http.StatusBadRequest, Message: "Invalid request body", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Failed to create user", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionCreate},
				{Role: "user_manager", Access: rbac.ActionCreate},
			},
		},
		{
			Name:        "DeleteUser",
			Description: "Deletes an RBAC user by ID.",
			Methods:     []string{http.MethodDelete},
			Path:        "/users/delete",
			Handler:     s.DeleteUserHandler,
			Request: Request{
				Params: map[string]ROption{
					"id": {Description: "ID of the user to delete.", Required: true, Type: openapi3.TypeString},
				},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "User deleted successfully", Body: &MessageResponse{}},
				{Status: http.StatusBadRequest, Message: "Missing user ID parameter", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Failed to delete user", Body: &ErrorResponse{}},
				{Status: http.StatusNotFound, Message: "User not found", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionDelete},
				{Role: "user_manager", Access: rbac.ActionDelete},
			},
		},
		{
			Name:        "GetUser",
			Description: "Retrieves an RBAC user by ID.",
			Methods:     []string{http.MethodGet},
			Path:        "/users/get",
			Handler:     s.GetUserHandler,
			Request: Request{
				Params: map[string]ROption{
					"id": {Description: "ID of the user to retrieve.", Required: true, Type: openapi3.TypeString},
				},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "User retrieved successfully", Body: &rbac.User{}},
				{Status: http.StatusBadRequest, Message: "Missing user ID parameter", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Failed to retrieve user", Body: &ErrorResponse{}},
				{Status: http.StatusNotFound, Message: "User not found", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionRead},
				{Role: "viewer", Access: rbac.ActionRead},
			},
		},
		{
			Name:        "AssignRoleToUser",
			Description: "Assigns a role to a user.",
			Methods:     []string{http.MethodPost},
			Path:        "/users/assign-role",
			Handler:     s.AssignRoleToUserHandler,
			Request: Request{
				Body: &AssignUserRoleRequest{},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "Role assigned to user successfully", Body: &MessageResponse{}},
				{Status: http.StatusBadRequest, Message: "Invalid request body", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Internal server error", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionCreate},
				{Role: "user_manager", Access: rbac.ActionUpdate},
			},
		},
		{
			Name:        "UnassignRoleFromUser",
			Description: "Unassigns a role from a user.",
			Methods:     []string{http.MethodPost},
			Path:        "/users/unassign-role",
			Handler:     s.UnassignRoleFromUserHandler,
			Request: Request{
				Body: &AssignUserRoleRequest{},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "Role unassigned from user successfully", Body: &MessageResponse{}},
				{Status: http.StatusBadRequest, Message: "Invalid request body", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Internal server error", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionDelete},
				{Role: "user_manager", Access: rbac.ActionUpdate},
			},
		},
		{
			Name:        "ListRolesForUser",
			Description: "Lists all roles assigned to a specific user.",
			Methods:     []string{http.MethodGet},
			Path:        "/users/list-roles",
			Handler:     s.ListRolesForUserHandler,
			Request: Request{
				Params: map[string]ROption{
					"user_id": {Description: "ID of the user to list roles for.", Required: true, Type: openapi3.TypeString},
				},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "Roles listed successfully", Body: []*rbac.Role{}},
				{Status: http.StatusBadRequest, Message: "Missing user_id parameter", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Internal server error", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionRead},
				{Role: "viewer", Access: rbac.ActionRead},
			},
		},
		{
			Name:        "AddUserToGroup",
			Description: "Adds a user to a group.",
			Methods:     []string{http.MethodPost},
			Path:        "/users/add-to-group",
			Handler:     s.AddUserToGroupHandler,
			Request: Request{
				Body: &AddRemoveUserGroupRequest{},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "User added to group successfully", Body: &MessageResponse{}},
				{Status: http.StatusBadRequest, Message: "Invalid request body", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Internal server error", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionCreate},
				{Role: "user_manager", Access: rbac.ActionUpdate},
			},
		},
		{
			Name:        "RemoveUserFromGroup",
			Description: "Removes a user from a group.",
			Methods:     []string{http.MethodPost},
			Path:        "/users/remove-from-group",
			Handler:     s.RemoveUserFromGroupHandler,
			Request: Request{
				Body: &AddRemoveUserGroupRequest{},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "User removed from group successfully", Body: &MessageResponse{}},
				{Status: http.StatusBadRequest, Message: "Invalid request body", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Internal server error", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionDelete},
				{Role: "user_manager", Access: rbac.ActionUpdate},
			},
		},
		{
			Name:        "GetUsersByGroupID",
			Description: "Retrieves all users belonging to a specific group.",
			Methods:     []string{http.MethodGet},
			Path:        "/users/list-by-group",
			Handler:     s.GetUsersByGroupIDHandler,
			Request: Request{
				Params: map[string]ROption{
					"group_id": {Description: "ID of the group to list users for.", Required: true, Type: openapi3.TypeString},
				},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "Users listed successfully", Body: []*rbac.UserGroup{}},
				{Status: http.StatusBadRequest, Message: "Missing group_id parameter", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Internal server error", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionRead},
				{Role: "viewer", Access: rbac.ActionRead},
			},
		},
		{
			Name:        "GetGroupsByUserID",
			Description: "Retrieves all groups a specific user belongs to.",
			Methods:     []string{http.MethodGet},
			Path:        "/users/list-groups",
			Handler:     s.GetGroupsByUserIDHandler,
			Request: Request{
				Params: map[string]ROption{
					"user_id": {Description: "ID of the user to list groups for.", Required: true, Type: openapi3.TypeString},
				},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "Groups listed successfully", Body: []*rbac.UserGroup{}},
				{Status: http.StatusBadRequest, Message: "Missing user_id parameter", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Internal server error", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionRead},
				{Role: "viewer", Access: rbac.ActionRead},
			},
		},
		{
			Name:        "HasPermission",
			Description: "Checks if a user has a specific permission by permission ID.",
			Methods:     []string{http.MethodGet},
			Path:        "/users/has-permission",
			Handler:     s.HasPermissionHandler,
			Request: Request{
				Params: map[string]ROption{
					"user_id": {Description: "ID of the user.", Required: true, Type: openapi3.TypeString},
					"perm_id": {Description: "ID of the permission to check.", Required: true, Type: openapi3.TypeString},
				},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "Permission check result", Body: &HasPermissionResponse{}},
				{Status: http.StatusBadRequest, Message: "Missing user_id or perm_id parameters", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Internal server error", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionRead},
				{Role: "viewer", Access: rbac.ActionRead},
			},
		},
		{
			Name:        "Can",
			Description: "Checks if a user can perform a specific action on a resource.",
			Methods:     []string{http.MethodPost},
			Path:        "/users/can",
			Handler:     s.CanHandler,
			Request: Request{
				Body: &CanRequest{},
			},
			Responses: []Response{
				{Status: http.StatusOK, Message: "Authorization check result", Body: &CanResponse{}},
				{Status: http.StatusBadRequest, Message: "Invalid request body", Body: &ErrorResponse{}},
				{Status: http.StatusInternalServerError, Message: "Internal server error", Body: &ErrorResponse{}},
			},
			Roles: []Role{
				{Role: "admin", Access: rbac.ActionRead},
				{Role: "viewer", Access: rbac.ActionRead},
				{Role: "executor", Access: rbac.ActionAll},
			},
		},
	}
}

func makeEndpoints(handler oserver.Handler) []*Endpoint {
	//will need to clean this up
	return []*Endpoint{
		{
			Name:        "Authorize",
			Description: "Initiate OAuth 2.0 authorization code flow",
			Methods:     []string{http.MethodGet},
			Path:        "/authorize",
			Handler:     handler.Authorize,
			Internal:    false,
			Request: Request{
				Params: map[string]ROption{
					"response_type":         {},
					"client_id":             {},
					"redirect_uri":          {},
					"scope":                 {},
					"state":                 {},
					"code_challenge":        {},
					"code_challenge_method": {},
					"force_consent":         {},
				},
				Headers: nil,
				Body:    nil,
			},
			Responses: []Response{
				{
					Status:  http.StatusFound,
					Headers: map[string]ROption{"Location": {}},
				},
				{Status: http.StatusBadRequest},
			},
		},
		{
			Name:        "Consent",
			Description: "Initiate OAuth 2.0 authorization code flow",
			Methods:     []string{http.MethodPost},
			Path:        "/consent",
			Handler:     handler.Consent,
			Internal:    false,
			Request: Request{
				Headers: nil,
				Body:    oserver.AuthRequest{},
			},
			Responses: []Response{
				{
					Status:  http.StatusFound,
					Headers: map[string]ROption{"Location": {}},
				},
				{Status: http.StatusBadRequest},
			},
		},
		{
			Name:        "Token",
			Description: "Exchange code or refresh token for an access token",
			Methods:     []string{http.MethodPost},
			Path:        "/token",
			Handler:     handler.Token,
			Internal:    false,
			Request: Request{
				Params:  nil,
				Headers: map[string]ROption{"Content-Type": {}},
				Body:    oserver.TokenRequest{},
			},
			Scope: "oauth.server",
			Responses: []Response{
				{Status: http.StatusOK, Body: oserver.TokenResponse{}},
				{Status: http.StatusBadRequest},
			},
		},
		{
			Name:        "Revoke",
			Description: "Revoke an access or refresh token",
			Methods:     []string{http.MethodPost},
			Path:        "/revoke",
			Handler:     handler.Revoke,
			Internal:    false,
			Request: Request{
				Headers: map[string]ROption{"Content-Type": {}},
				Body:    oserver.RevocationRequest{},
			},
			Responses: []Response{
				{Status: http.StatusOK},
				{Status: http.StatusBadRequest},
			},
		},
		{
			Name:        "Introspect",
			Description: "Introspect an access or refresh token",
			Methods:     []string{http.MethodPost},
			Path:        "/introspect",
			Handler:     handler.Introspect,
			Internal:    false,
			Request: Request{
				Headers: map[string]ROption{"Content-Type": {}},
				Body:    oserver.IntrospectRequest{},
			},
			Responses: []Response{
				{Status: http.StatusOK, Body: oserver.IntrospectResponse{}},
				{Status: http.StatusBadRequest},
			},
		},
		{
			Name:        "JWKs",
			Description: "Retrieve the JSON Web Key Set",
			Methods:     []string{http.MethodGet},
			Path:        "/.well-known/jwks.json",
			Handler:     handler.JWKs,
			Internal:    false,
			Request:     Request{},
			Responses: []Response{
				{Status: http.StatusOK, Body: oserver.JWKSet{}},
				{Status: http.StatusInternalServerError},
			},
		},
		{
			Name:        "Register Client",
			Description: "Register a new OAuth client",
			Methods:     []string{http.MethodPost},
			Path:        "/clients",
			Handler:     handler.RegisterClient,
			Internal:    true,
			Request: Request{
				Headers: map[string]ROption{"Content-Type": {}},
				Body:    oserver.OAuthClient{},
			},
			Responses: []Response{
				{Status: http.StatusOK, Body: oserver.OAuthClient{}},
				{Status: http.StatusBadRequest},
			},
		},
		{
			Name:        "Get Client",
			Description: "Fetch an OAuth client by ID",
			Methods:     []string{http.MethodGet},
			Path:        "/clients/{id}",
			Handler:     handler.GetClient,
			Internal:    true,
			Request: Request{
				Params: map[string]ROption{"id": {}},
			},
			Responses: []Response{
				{Status: http.StatusOK, Body: oserver.OAuthClient{}},
				{Status: http.StatusNotFound},
			},
		},
		{
			Name:        "List Clients",
			Description: "List all OAuth clients for an account",
			Methods:     []string{http.MethodGet},
			Path:        "/clients",
			Handler:     handler.ListClients,
			Internal:    true,
			Request: Request{
				Params: map[string]ROption{"account_id": {}},
			},
			Responses: []Response{
				{Status: http.StatusOK, Body: []oserver.OAuthClient{}},
				{Status: http.StatusInternalServerError},
			},
		},
		{
			Name:        "Update Client",
			Description: "Update an existing OAuth client",
			Methods:     []string{http.MethodPut},
			Path:        "/clients/{id}",
			Handler:     handler.UpdateClient,
			Internal:    true,
			Request: Request{
				Params:  map[string]ROption{"id": {}},
				Headers: map[string]ROption{"Content-Type": {}},
				Body:    oserver.OAuthClient{},
			},
			Responses: []Response{
				{Status: http.StatusOK, Body: oserver.OAuthClient{}},
				{Status: http.StatusBadRequest},
			},
		},
		{
			Name:        "Delete Client",
			Description: "Delete an OAuth client",
			Methods:     []string{http.MethodDelete},
			Path:        "/clients/{id}",
			Handler:     handler.DeleteClient,
			Internal:    true,
			Request: Request{
				Params: map[string]ROption{"id": {}},
			},
			Responses: []Response{
				{Status: http.StatusNoContent},
				{Status: http.StatusBadRequest},
			},
		},
		{
			Name:        "Set Client Image",
			Description: "Upload or update a client’s image",
			Methods:     []string{http.MethodPost},
			Path:        "/clients/{id}/image",
			Handler:     handler.SetClientImage,
			Internal:    true,
			Request: Request{
				Params:  map[string]ROption{"id": {}},
				Headers: map[string]ROption{"Content-Type": {}},
			},
			Responses: []Response{
				{Status: http.StatusOK},
				{Status: http.StatusBadRequest},
			},
		},
		{
			Name:        "Send Client Image",
			Description: "Serve a client’s image",
			Methods:     []string{http.MethodGet},
			Path:        "/clients/{id}/image",
			Handler:     handler.SendClientImage,
			Internal:    true,
			Request: Request{
				Params: map[string]ROption{"id": {}},
			},
			Responses: []Response{
				{Status: http.StatusOK},
				{Status: http.StatusNotFound},
			},
		},
	}
}
