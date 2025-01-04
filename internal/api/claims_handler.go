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
// type CustomClaim struct {
// 	Key   string          `json:"key"`
// 	Value json.RawMessage `json:"value"`
// }

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
		// Convert map[string]interface{} to JSON bytes
		valueBytes, err := json.Marshal(claim.Value)
		if err != nil {
			log.Error().Err(err).
				Str("claim_key", claim.Key).
				Msg("Failed to marshal claim value")
			http.Error(w, "Failed to process claims", http.StatusInternalServerError)
			return
		}

		// Create NullRawMessage
		nullRawMessage := pqtype.NullRawMessage{
			RawMessage: valueBytes,
			Valid:      true,
		}

		err = s.db.UpsertCustomClaim(r.Context(), db.UpsertCustomClaimParams{
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
	validClaimKeys := map[string]func(map[string]interface{}) error{
		"department": func(value map[string]interface{}) error {
			name, ok := value["name"].(string)
			if !ok || name == "" {
				return fmt.Errorf("invalid department format or empty name")
			}
			return nil
		},
		"permissions": func(value map[string]interface{}) error {
			perms, ok := value["permissions"].([]interface{})
			if !ok {
				return fmt.Errorf("invalid permissions format")
			}
			for _, p := range perms {
				if _, ok := p.(string); !ok {
					return fmt.Errorf("invalid permission type")
				}
			}
			return nil
		},
		"project_access": func(value map[string]interface{}) error {
			projects, ok := value["projects"].([]interface{})
			if !ok {
				return fmt.Errorf("invalid project access format")
			}
			for _, p := range projects {
				project, ok := p.(map[string]interface{})
				if !ok {
					return fmt.Errorf("invalid project format")
				}
				if _, ok := project["id"].(string); !ok {
					return fmt.Errorf("invalid project ID")
				}
				if _, ok := project["role"].(string); !ok {
					return fmt.Errorf("invalid project role")
				}
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
		if claim.ClaimValue.Valid {
			var value map[string]interface{}
			if err := json.Unmarshal(claim.ClaimValue.RawMessage, &value); err != nil {
				log.Error().Err(err).
					Str("claim_key", claim.ClaimKey).
					Msg("Failed to unmarshal claim value")
				continue
			}

			claims = append(claims, CustomClaim{
				Key:   claim.ClaimKey,
				Value: value,
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
