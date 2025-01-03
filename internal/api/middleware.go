package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	// "github.com/coreos/go-oidc/v3/oidc" // Make sure we use v3
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

func (s *Server) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get request ID from context
		// reqID := r.Context().Value("request_id").(string)

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			log.Debug().
				Str("path", r.URL.Path).
				Msg("No Authorization header")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		// Extract
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			log.Debug().Msg("Invalid Authorization header format")
			http.Error(w, "Invalid token format", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]
		log.Debug().
			Str("path", r.URL.Path).
			Str("token_prefix", tokenString[:10]+"...").
			Msg("Verifying token")

		verifier := s.auth.Provider.Verifier(&oidc.Config{
			ClientID: os.Getenv("AUTH0_CLIENT_ID"),
		})

		// Parse and verify token
		idToken, err := verifier.Verify(r.Context(), tokenString)
		if err != nil {
			log.Error().
				Err(err).
				Str("path", r.URL.Path).
				Str("token_prefix", tokenString[:10]+"...").
				Msg("Token verification failed")
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Parse claims
		var claims Claims
		if err := idToken.Claims(&claims); err != nil {
			log.Error().
				Err(err).
				Str("path", r.URL.Path).
				Msg("Failed to parse token claims")
			http.Error(w, "Invalid token claims", http.StatusUnauthorized)
			return
		}


		// Add claims to context
		ctx := context.WithValue(r.Context(), "user", claims)

		// Add request ID if not present
		if reqID := ctx.Value("request_id"); reqID == nil {
			ctx = context.WithValue(ctx, "request_id", uuid.New().String())
		}

		// Log successful authentication
		log.Info().
			Str("email", claims.Email).
			Str("user_id", claims.Sub).
			Str("path", r.URL.Path).
			Str("request_id", ctx.Value("request_id").(string)).
			Msg("User authenticated successfully")

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := r.Context().Value("user").(Claims)

			// Get user from database
			dbUser, err := s.db.GetUserByAuth0ID(r.Context(), user.Sub)
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Get user's roles
			userRoles, err := s.db.GetUserRoles(r.Context(), dbUser.ID)
			if err != nil {
				http.Error(w, "Failed to verify roles", http.StatusInternalServerError)
				return
			}

			// Check if user has any of the required roles
			hasRole := false
			for _, userRole := range userRoles {
				for _, requiredRole := range roles {
					if userRole.Name == requiredRole {
						hasRole = true
						break
					}
				}
				if hasRole {
					break
				}
			}

			if !hasRole {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Add a simple endpoint to echo back the token info
func (s *Server) GetTokenInfo(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(w, "Invalid token format", http.StatusBadRequest)
		return
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	// Split the token and check each part
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":       "Invalid token structure",
			"parts_count": len(parts),
		})
		return
	}

	// Try to decode each part
	decodedParts := make([]string, 3)
	for i, part := range parts {
		decoded, err := base64.RawURLEncoding.DecodeString(part)
		if err != nil {
			decodedParts[i] = fmt.Sprintf("Decoding error: %v", err)
		} else {
			decodedParts[i] = string(decoded)
		}
	}

	result := map[string]interface{}{
		"structure": map[string]interface{}{
			"header_length":    len(parts[0]),
			"payload_length":   len(parts[1]),
			"signature_length": len(parts[2]),
		},
		"decoded": map[string]interface{}{
			"header":    decodedParts[0],
			"payload":   decodedParts[1],
			"signature": "...", // Don't log the actual signature
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)

	// // Create parser with validation
	// parser := jwt.NewParser(
	// 	jwt.WithValidMethods([]string{"RS256"}), // Auth0 uses RS256
	// 	jwt.WithIssuer(fmt.Sprintf("https://%s/", os.Getenv("AUTH0_DOMAIN"))),
	// 	jwt.WithAudience(os.Getenv("AUTH0_CLIENT_ID")),
	// )
	//
	// claims := jwt.MapClaims{}
	//
	// token, _, err := parser.ParseUnverified(tokenString, claims)
	//
	// result := map[string]interface{}{
	// 	"header": token.Header,
	// 	"claims": claims,
	// 	"error":  err,
	// }
	//
	// w.Header().Set("Content-Type", "application/json")
	// json.NewEncoder(w).Encode(result)
}
