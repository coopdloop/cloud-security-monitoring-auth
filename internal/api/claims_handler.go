package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/coopdloop/go-backend-sqlc/internal/db"
	"github.com/rs/zerolog/log"
	"github.com/sqlc-dev/pqtype"
)

// Custom claim structure
type CustomClaim struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

// Update user claims handler
func (s *Server) UpdateUserClaims(w http.ResponseWriter, r *http.Request) {
	// Get current user
	currentUser := r.Context().Value("user").(Claims)
	dbUser, err := s.db.GetUserByAuth0ID(r.Context(), currentUser.Sub)
	if err != nil {
		http.Error(w, "User not found", http.StatusUnauthorized)
		return
	}

	// Parse request body
	var claims []CustomClaim
	if err := json.NewDecoder(r.Body).Decode(&claims); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate claims (implement your custom validation logic)
	if err := s.validateCustomClaims(claims); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Upsert claims
	for _, claim := range claims {
		// Convert RawMessage to pqtype.NullRawMessage
		nullRawMessage := pqtype.NullRawMessage{
			RawMessage: []byte(claim.Value),
			Valid:      len(claim.Value) > 0,
		}

		err := s.db.UpsertCustomClaim(r.Context(), db.UpsertCustomClaimParams{
			UserID:     dbUser.ID,
			ClaimKey:   claim.Key,
			ClaimValue: nullRawMessage,
		})
		if err != nil {
			log.Error().Err(err).
				Str("claim_key", claim.Key).
				Msg("Failed to upsert custom claim")
			http.Error(w, "Failed to update claims", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

// Validate custom claims
func (s *Server) validateCustomClaims(claims []CustomClaim) error {
	// Implement custom validation logic
	validClaimKeys := map[string]func(json.RawMessage) error{
		"department": func(value json.RawMessage) error {
			var dept struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(value, &dept); err != nil {
				return fmt.Errorf("invalid department format")
			}
			if dept.Name == "" {
				return fmt.Errorf("department name cannot be empty")
			}
			return nil
		},
		"permissions": func(value json.RawMessage) error {
			var perms []string
			if err := json.Unmarshal(value, &perms); err != nil {
				return fmt.Errorf("invalid permissions format")
			}
			return nil
		},
		"project_access": func(value json.RawMessage) error {
			var projects []struct {
				ID   string `json:"id"`
				Role string `json:"role"`
			}
			if err := json.Unmarshal(value, &projects); err != nil {
				return fmt.Errorf("invalid project access format")
			}
			return nil
		},
	}

	for _, claim := range claims {
		validator, exists := validClaimKeys[claim.Key]
		if !exists {
			return fmt.Errorf("unsupported claim key: %s", claim.Key)
		}

		if err := validator(claim.Value); err != nil {
			return err
		}
	}

	return nil
}

// Get user claims handler
func (s *Server) GetUserClaims(w http.ResponseWriter, r *http.Request) {
	// Get current user
	currentUser := r.Context().Value("user").(Claims)
	dbUser, err := s.db.GetUserByAuth0ID(r.Context(), currentUser.Sub)
	if err != nil {
		http.Error(w, "User not found", http.StatusUnauthorized)
		return
	}

	// Fetch custom claims
	customClaims, err := s.db.GetUserCustomClaims(r.Context(), dbUser.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch custom claims")
		http.Error(w, "Failed to retrieve claims", http.StatusInternalServerError)
		return
	}

	// Convert to response format
	claims := make([]CustomClaim, 0)
	for _, claim := range customClaims {
		// Only add claims with valid values
		if claim.ClaimValue.Valid {
			claims = append(claims, CustomClaim{
				Key:   claim.ClaimKey,
				Value: json.RawMessage(claim.ClaimValue.RawMessage),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(claims); err != nil {
		log.Error().Err(err).Msg("Failed to encode claims")
		http.Error(w, "Failed to process claims", http.StatusInternalServerError)
		return
	}
}
