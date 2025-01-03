// internal/api/role_handlers.go

package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/coopdloop/go-backend-sqlc/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

type UpdateUserRolesRequest struct {
	UserID int32    `json:"user_id"`
	Roles  []string `json:"roles"`
}

// GetAllRoles returns list of all available roles
func (s *Server) GetAllRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := s.db.ListRoles(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch roles")
		http.Error(w, "Failed to fetch roles", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(roles)
}

// GetUsersWithRoles returns all users and their roles
func (s *Server) GetUsersWithRoles(w http.ResponseWriter, r *http.Request) {
	// Check if user has admin role
	currentUser := r.Context().Value("user").(Claims)

	// Get user from database using Auth0 ID
	dbUser, err := s.db.GetUserByAuth0ID(r.Context(), currentUser.Sub)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user from database")
		http.Error(w, "Failed to verify permissions", http.StatusInternalServerError)
		return
	}

	userRoles, err := s.db.GetUserRoles(r.Context(), dbUser.ID)
	if err != nil {
		http.Error(w, "Failed to verify permissions", http.StatusInternalServerError)
		return
	}

	hasAdminAccess := false
	for _, role := range userRoles {
		if role.Name == "admin" || role.Name == "security_admin" {
			hasAdminAccess = true
			break
		}
	}

	if !hasAdminAccess {
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	users, err := s.db.GetUsersWithRoles(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch users with roles")
		http.Error(w, "Failed to fetch users", http.StatusInternalServerError)
		return
	}

	// Add this block to process roles: important
	for i := range users {
		if users[i].Roles != "" {
			users[i].Roles = strings.Join(strings.Split(strings.Trim(users[i].Roles, "{}"), ","), ",")
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// GetUserRoles gets roles for a specific user
func (s *Server) GetUserRoles(w http.ResponseWriter, r *http.Request) {
	// Get userID from URL parameters
	userID := chi.URLParam(r, "userID")

	// Convert userID to int32
	id, err := strconv.ParseInt(userID, 10, 32)
	if err != nil {
		log.Error().Err(err).Msg("Invalid user ID")
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Get user roles from database
	roles, err := s.db.GetUserRoles(r.Context(), int32(id))
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user roles")
		http.Error(w, "Failed to get user roles", http.StatusInternalServerError)
		return
	}

	// Convert roles to string array for response
	roleNames := make([]string, len(roles))
	for i, role := range roles {
		roleNames[i] = role.Name
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(roleNames)
}

// UpdateUserRoles updates roles for a user
func (s *Server) UpdateUserRoles(w http.ResponseWriter, r *http.Request) {
	var req UpdateUserRolesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get current user for granted_by
	currentUser := r.Context().Value("user").(Claims)
	adminUser, err := s.db.GetUserByAuth0ID(r.Context(), currentUser.Sub)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user from database")
		http.Error(w, "Failed to verify permissions", http.StatusInternalServerError)
		return
	}

	// Remove all existing roles
	err = s.db.RemoveAllUserRoles(r.Context(), req.UserID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to remove existing roles")
		http.Error(w, "Failed to update roles", http.StatusInternalServerError)
		return
	}

	// Add new roles
	for _, roleName := range req.Roles {
		role, err := s.db.GetRoleByName(r.Context(), roleName)
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			log.Error().Err(err).Str("role", roleName).Msg("Failed to get role")
			continue
		}

		err = s.db.AssignUserRole(r.Context(), db.AssignUserRoleParams{
			UserID:    req.UserID,
			RoleID:    role.ID,
			GrantedBy: adminUser.ID,
		})
		if err != nil {
			log.Error().Err(err).
				Int32("user_id", req.UserID).
				Str("role", roleName).
				Msg("Failed to assign role")
			continue
		}
	}

	// Publish event to SNS if configured
	// if s.sns != nil {
	// 	event := struct {
	// 		UserID    int32    `json:"user_id"`
	// 		Roles     []string `json:"roles"`
	// 		GrantedBy int32    `json:"granted_by"`
	// 		Action    string   `json:"action"`
	// 	}{
	// 		UserID:    req.UserID,
	// 		Roles:     req.Roles,
	// 		GrantedBy: adminUser.ID,
	// 		Action:    "update_roles",
	// 	}
	//
	// 	go func() {
	// 		if err := s.sns.PublishEvent(r.Context(), "user.roles_updated", event); err != nil {
	// 			log.Error().Err(err).Msg("Failed to publish role update event")
	// 		}
	// 	}()
	// }

	w.WriteHeader(http.StatusOK)
}
