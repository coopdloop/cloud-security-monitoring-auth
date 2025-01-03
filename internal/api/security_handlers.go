// internal/api/security_handlers.go
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/coopdloop/go-backend-sqlc/internal/db"
	"github.com/google/uuid"
	"github.com/oapi-codegen/runtime/types"
	"github.com/rs/zerolog/log"
	"github.com/sqlc-dev/pqtype"
)

// GetSessions implements the OpenAPI spec for listing sessions
func (s *Server) GetSessions(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(Claims)

	// Get user from database by Auth0 ID
	dbUser, err := s.db.GetUserByAuth0ID(r.Context(), user.Sub)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	sessions, err := s.db.ListActiveSessions(r.Context(), dbUser.ID)
	if err != nil {
		http.Error(w, "Failed to fetch sessions", http.StatusInternalServerError)
		return
	}

	// Convert to API response format
	response := make([]Session, len(sessions))
	for i, sess := range sessions {
		var location *string
		if sess.Location.Valid {
			location = &sess.Location.String
		}

		lastActive := time.Now()
		if sess.LastActive.Valid {
			lastActive = sess.LastActive.Time
		}

		sessID, err := uuid.Parse(sess.ID.String())
		if err != nil {
			log.Error().Err(err).Msg("Invalid session ID")
			continue
		}

		response[i] = Session{
			Id:         sessID,
			Device:     sess.Device,
			Location:   location,
			LastActive: lastActive,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// RevokeSession implements the OpenAPI spec for revoking a session
func (s *Server) RevokeSession(w http.ResponseWriter, r *http.Request, sessionId types.UUID) {
	user := r.Context().Value("user").(Claims)

	dbUser, err := s.db.GetUserByAuth0ID(r.Context(), user.Sub)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	sessID, err := uuid.Parse(sessionId.String())
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	err = s.db.RevokeSession(r.Context(), db.RevokeSessionParams{
		ID:     sessID,
		UserID: dbUser.ID,
	})
	if err != nil {
		http.Error(w, "Failed to revoke session", http.StatusInternalServerError)
		return
	}

	// Parse IP address
	ip := net.ParseIP(r.RemoteAddr)
	if ip == nil {
		ip = net.IPv4(0, 0, 0, 0)
	}

	// Log the security event
	_, err = s.db.CreateSecurityLog(r.Context(), db.CreateSecurityLogParams{
		UserID:           dbUser.ID,
		EventType:        string(SuspiciousActivity),
		EventDescription: "Session was manually revoked",
		IpAddress:        pqtype.Inet{IPNet: net.IPNet{IP: ip, Mask: ip.DefaultMask()}},
		UserAgent:        sql.NullString{String: r.UserAgent(), Valid: true},
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to create security log")
	}

    // Publish session revocation event
    sessionEvent := struct {
        SessionID string    `json:"session_id"`
        UserID    int32    `json:"user_id"`
        Timestamp time.Time `json:"timestamp"`
        Action    string    `json:"action"`
        Reason    string    `json:"reason"`
    }{
        SessionID: sessionId.String(),
        UserID:    dbUser.ID,
        Timestamp: time.Now(),
        Action:    "revoke",
        Reason:    "user_initiated",
    }

    go func() {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()

        if err := s.sns.PublishEvent(ctx, "session.revoked", sessionEvent); err != nil {
            log.Error().Err(err).Msg("Failed to publish session revocation event")
        }
    }()


	w.WriteHeader(http.StatusOK)
}

// GetSecurityLog implements the OpenAPI spec for fetching security logs
func (s *Server) GetSecurityLog(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(Claims)

	dbUser, err := s.db.GetUserByAuth0ID(r.Context(), user.Sub)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	logs, err := s.db.GetUserSecurityLogs(r.Context(), db.GetUserSecurityLogsParams{
		UserID: dbUser.ID,
		Limit:  50,
	})
	if err != nil {
		http.Error(w, "Failed to fetch security logs", http.StatusInternalServerError)
		return
	}

	// Convert to API response format
	response := make([]SecurityLog, len(logs))
	for i, log := range logs {
		ip := log.IpAddress.IPNet.IP.String()
		response[i] = SecurityLog{
			Timestamp: log.CreatedAt.Time,
			Event:     log.EventDescription,
			Type:      SecurityLogType(log.EventType),
			IpAddress: ip,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// VerifyMFA implements the OpenAPI spec for MFA verification
func (s *Server) VerifyMFA(w http.ResponseWriter, r *http.Request) {
	var req MFAVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user := r.Context().Value("user").(Claims)

	dbUser, err := s.db.GetUserByAuth0ID(r.Context(), user.Sub)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Verify MFA code with Auth0
	verified, err := s.auth.VerifyMFAToken(r.Context(), user.Sub, req.Code)
	if err != nil {
		http.Error(w, "Failed to verify MFA code", http.StatusInternalServerError)
		return
	}

	if !verified {
		http.Error(w, "Invalid MFA code", http.StatusBadRequest)
		return
	}

	// Update user's MFA status
	err = s.db.UpdateUserMFAStatus(r.Context(), db.UpdateUserMFAStatusParams{
		ID:         dbUser.ID,
		MfaEnabled: sql.NullBool{Bool: true, Valid: true},
	})
	if err != nil {
		http.Error(w, "Failed to update MFA status", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// DisableMFA implements the OpenAPI spec for disabling MFA
func (s *Server) DisableMFA(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(Claims)

	dbUser, err := s.db.GetUserByAuth0ID(r.Context(), user.Sub)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	err = s.db.UpdateUserMFAStatus(r.Context(), db.UpdateUserMFAStatusParams{
		ID:         dbUser.ID,
		MfaEnabled: sql.NullBool{Bool: false, Valid: true},
	})
	if err != nil {
		http.Error(w, "Failed to update MFA status", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

