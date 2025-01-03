package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/coopdloop/go-backend-sqlc/internal/db"
	"github.com/rs/zerolog/log"
)

// type CreateUserRequest struct {
//     Name     string `json:"name"`
//     Email    string `json:"email"`
// }

type ExtendedCreateUserRequest struct {
	Email   string         `json:"email"`
	Name    string         `json:"name"`
	Auth0ID string         `json:"auth0_id"`
	Picture sql.NullString `json:"picture"`
	Roles   []string       `json:"roles"`
}

func (s *Server) CreateUser(w http.ResponseWriter, r *http.Request) {
	// Check authorization (ensure only admins can create users)
	currentUser := r.Context().Value("user").(Claims)
	dbUser, err := s.db.GetUserByAuth0ID(r.Context(), currentUser.Sub)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user from database")
		http.Error(w, "Failed to verify permissions", http.StatusInternalServerError)
		return
	}

	// Verify admin role
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

	// Parse request body
	var req ExtendedCreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Generate a unique Auth0 ID if not provided
	if req.Auth0ID == "" {
		req.Auth0ID = fmt.Sprintf("manual-%d", time.Now().UnixNano())
	}

	// Create user
	newUser, err := s.db.CreateUser(r.Context(), db.CreateUserParams{
		Name:    req.Name,
		Email:   req.Email,
		Auth0ID: req.Auth0ID,
		Picture: req.Picture,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to create user")
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	// Assign roles if provided
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
			UserID:    newUser.ID,
			RoleID:    role.ID,
			GrantedBy: dbUser.ID,
		})
		if err != nil {
			log.Error().Err(err).
				Int32("user_id", newUser.ID).
				Str("role", roleName).
				Msg("Failed to assign role")
		}
	}

	// Return the created user
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newUser)
}

func (s *Server) UpdateUserProfile(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form data
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	// Get current user
	currentUser := r.Context().Value("user").(Claims)
	dbUser, err := s.db.GetUserByAuth0ID(r.Context(), currentUser.Sub)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user from database")
		http.Error(w, "Failed to verify user", http.StatusInternalServerError)
		return
	}

	// Get form values
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	// Handle picture upload (if provided)
	var pictureURL string
	pictureFile, handler, err := r.FormFile("picture")
	if err == nil {
		defer pictureFile.Close()

		// Generate a unique filename
		filename := fmt.Sprintf("%d_%s", dbUser.ID, handler.Filename)

		// Save file to storage (you'll need to implement this based on your storage solution)
		pictureURL, err = saveProfilePicture(pictureFile, filename)
		if err != nil {
			log.Error().Err(err).Msg("Failed to save profile picture")
			http.Error(w, "Failed to upload picture", http.StatusInternalServerError)
			return
		}
	}

	// Update user profile
	updatedUser, err := s.db.UpdateUserProfile(r.Context(), db.UpdateUserProfileParams{
		ID:   dbUser.ID,
		Name: name,
		Picture: sql.NullString{
			String: pictureURL,
			Valid:  pictureURL != "",
		},
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to update user profile")
		http.Error(w, "Failed to update profile", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedUser)
}

// Implement this function based on your file storage solution
func saveProfilePicture(file multipart.File, filename string) (string, error) {
	// Example implementation - you'll need to adapt this to your specific storage method
	// This could be local storage, S3, or another cloud storage solution
	uploadDir := "./uploads/profiles/"

	// Ensure upload directory exists
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		return "", err
	}

	// Create the file
	dst, err := os.Create(filepath.Join(uploadDir, filename))
	if err != nil {
		return "", err
	}
	defer dst.Close()

	// Copy the uploaded file
	_, err = io.Copy(dst, file)
	if err != nil {
		return "", err
	}

	// Return the relative URL path
	return fmt.Sprintf("/uploads/profiles/%s", filename), nil
}

// Define a more comprehensive user creation function in your handlers
func (s *Server) createUserWithRoles(ctx context.Context, params db.CreateUserParams, roles []string) (db.User, error) {
    // Create the user
    createdUser, err := s.db.CreateUser(ctx, params)
    if err != nil {
        return db.User{}, fmt.Errorf("failed to create user: %w", err)
    }

    // Convert createdUser to User type
    user := db.User{
        ID:       createdUser.ID,
        Auth0ID:  createdUser.Auth0ID,
        Name:     createdUser.Name,
        Email:    createdUser.Email,
        // Copy other fields as needed
    }

    // Assign roles if provided
    for _, roleName := range roles {
        role, err := s.db.GetRoleByName(ctx, roleName)
        if err != nil {
            if err == sql.ErrNoRows {
                continue
            }
            log.Error().Err(err).Str("role", roleName).Msg("Failed to get role")
            continue
        }

        err = s.db.AssignUserRole(ctx, db.AssignUserRoleParams{
            UserID:    user.ID,
            RoleID:    role.ID,
            GrantedBy: user.ID, // self-granted
        })
        if err != nil {
            log.Error().Err(err).
                Int32("user_id", user.ID).
                Str("role", roleName).
                Msg("Failed to assign role")
        }
    }

    return user, nil
}

// Update PostUsers handler
func (s *Server) PostUsers(w http.ResponseWriter, r *http.Request) {
    // Define a more flexible request structure
    var req struct {
        Name   string   `json:"name"`
        Email  string   `json:"email"`
        Roles  []string `json:"roles"`
        Auth0ID string  `json:"auth0_id,omitempty"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Generate an Auth0 ID if not provided
    if req.Auth0ID == "" {
        req.Auth0ID = fmt.Sprintf("manual-%d", time.Now().UnixNano())
    }

    // Prepare user creation params
    params := db.CreateUserParams{
        Name:     req.Name,
        Email:    req.Email,
        Auth0ID:  req.Auth0ID,
    }

    // Create user with optional roles
    user, err := s.createUserWithRoles(r.Context(), params, req.Roles)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Prepare response
    resp := User{
        Id:    int(user.ID),
        Name:  user.Name,
        Email: user.Email,
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(resp)
}
