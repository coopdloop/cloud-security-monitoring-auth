// internal/api/handlers.go
package api

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coopdloop/go-backend-sqlc/internal/auth"
	"github.com/coopdloop/go-backend-sqlc/internal/aws"
	"github.com/coopdloop/go-backend-sqlc/internal/db"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/oauth2"
)

// Add generateRandomState helper function
func generateRandomState() string {
	b := make([]byte, 32)
	cryptorand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

type Server struct {
	db   *db.Queries
	auth *auth.Authenticator
	sns  *aws.SNSClient
}

func NewServer(db *db.Queries, auth *auth.Authenticator, sns *aws.SNSClient) *Server {
	return &Server{
		db:   db,
		auth: auth,
		sns:  sns,
	}
}

// SetupMFA initiates MFA setup with Auth0
func (s *Server) SetupMFA(w http.ResponseWriter, r *http.Request) {
	log.Info().Msg("Starting MFA setup")
	user := r.Context().Value("user").(Claims)

	// Get Auth0 Management API token
	token, err := s.auth.GetManagementToken(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("Failed to get management token")
		http.Error(w, "Failed to setup MFA", http.StatusInternalServerError)
		return
	}

	// Create MFA enrollment ticket
	setupURL := fmt.Sprintf(
		"https://%s/api/v2/guardian/enrollments/ticket",
		os.Getenv("AUTH0_DOMAIN"),
	)

	setupPayload := map[string]interface{}{
		"user_id": user.Sub,
		"email":   user.Email,
	}

	jsonPayload, err := json.Marshal(setupPayload)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal payload")
		http.Error(w, "Failed to setup MFA", http.StatusInternalServerError)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), "POST", setupURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create request")
		http.Error(w, "Failed to setup MFA", http.StatusInternalServerError)
		return
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send request")
		http.Error(w, "Failed to setup MFA", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Error().
			Int("status", resp.StatusCode).
			Str("body", string(body)).
			Msg("Auth0 returned non-200 status")
		http.Error(w, "Failed to setup MFA", http.StatusInternalServerError)
		return
	}

	var result struct {
		TicketID  string `json:"ticket_id"`
		QRCodeURL string `json:"qr_code_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Error().Err(err).Msg("Failed to decode response")
		http.Error(w, "Failed to setup MFA", http.StatusInternalServerError)
		return
	}

	// Publish MFA setup event
	mfaEvent := struct {
		UserID    string    `json:"user_id"`
		Timestamp time.Time `json:"timestamp"`
		Action    string    `json:"action"`
		Success   bool      `json:"success"`
	}{
		UserID:    user.Sub,
		Timestamp: time.Now(),
		Action:    "mfa_setup",
		Success:   true,
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.sns.PublishEvent(ctx, "mfa.setup", mfaEvent); err != nil {
			log.Error().Err(err).Msg("Failed to publish MFA setup event")
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"qr_code":   result.QRCodeURL,
		"ticket_id": result.TicketID,
	})
}

// SetupPasswordless initiates passwordless authentication with OTP
func (s *Server) SetupPasswordless(w http.ResponseWriter, r *http.Request) {
	log.Info().Msg("Starting passwordless setup")
	// Get user from context
	user := r.Context().Value("user").(Claims)

	// Call the passwordless/start endpoint
	setupURL := fmt.Sprintf(
		"https://%s/passwordless/start",
		os.Getenv("AUTH0_DOMAIN"),
	)

	setupPayload := map[string]interface{}{
		"client_id":     os.Getenv("AUTH0_CLIENT_ID"),
		"client_secret": os.Getenv("AUTH0_CLIENT_SECRET"),
		"connection":    "email",
		"email":         user.Email,
		"send":          "code",
		"authParams": map[string]interface{}{
			"scope": "openid profile email",
		},
	}

	jsonPayload, err := json.Marshal(setupPayload)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal payload")
		http.Error(w, "Failed to setup passwordless", http.StatusInternalServerError)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), "POST", setupURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create setup request")
		http.Error(w, "Failed to setup passwordless", http.StatusInternalServerError)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	// Clean IP
	clientIP := getClientIP(r)
	if parsedIP := net.ParseIP(clientIP); parsedIP != nil {
		req.Header.Set("auth0-forwarded-for", parsedIP.String())
		log.Debug().Str("forwarded_ip", parsedIP.String()).Msg("Setting forwarded IP")
	} else {
		log.Warn().Str("ip", clientIP).Msg("Invalid IP address format")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send request")
		http.Error(w, "Failed to setup passwordless", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Error().
			Int("status", resp.StatusCode).
			Str("body", string(body)).
			Msg("Auth0 returned non-200 status")
		http.Error(w, "Failed to setup passwordless", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Please check your email for the verification code.",
		"email":   user.Email,
	})
}

// StartPasswordlessLogin initiates the passwordless login flow
func (s *Server) StartPasswordlessLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	setupURL := fmt.Sprintf(
		"https://%s/passwordless/start",
		os.Getenv("AUTH0_DOMAIN"),
	)

	setupPayload := map[string]interface{}{
		"client_id":     os.Getenv("AUTH0_CLIENT_ID"),
		"client_secret": os.Getenv("AUTH0_CLIENT_SECRET"),
		"connection":    "email",
		"email":         req.Email,
		"send":          "code",
		"authParams": map[string]interface{}{
			"scope":         "openid profile email",
			"response_type": "code",
		},
	}

	jsonPayload, err := json.Marshal(setupPayload)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal payload")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	resp, err := http.Post(setupURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Error().Err(err).Msg("Failed to send request")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "Failed to start passwordless login", resp.StatusCode)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// VerifyPasswordlessLogin verifies the code and completes login
func (s *Server) VerifyPasswordlessLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code  string `json:"code"`
		Email string `json:"email"` // Added email field
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error().Err(err).Msg("Failed to decode request")
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Info().
		Str("code", req.Code).
		Str("email", req.Email).
		Msg("Verifying passwordless code")

	tokenURL := fmt.Sprintf("https://%s/oauth/token", os.Getenv("AUTH0_DOMAIN"))
	tokenPayload := map[string]interface{}{
		"grant_type":    "http://auth0.com/oauth/grant-type/passwordless/otp",
		"client_id":     os.Getenv("AUTH0_CLIENT_ID"),
		"client_secret": os.Getenv("AUTH0_CLIENT_SECRET"),
		"username":      req.Email, // Add the email as username
		"otp":           req.Code,
		"realm":         "email",
		"scope":         "openid profile email",
		"audience":      fmt.Sprintf("https://%s/api/v2/", os.Getenv("AUTH0_DOMAIN")),
	}

	jsonPayload, err := json.Marshal(tokenPayload)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal token payload")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Log the request payload (be careful with this in production)
	log.Debug().RawJSON("payload", jsonPayload).Msg("Token request payload")

	resp, err := http.Post(tokenURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Error().Err(err).Msg("Failed to send token request")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read response body")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Log the response for debugging
	log.Debug().
		Int("status_code", resp.StatusCode).
		Str("response_body", string(body)).
		Msg("Auth0 token response")

	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if err := json.Unmarshal(body, &errorResp); err != nil {
			log.Error().Err(err).Msg("Failed to parse error response")
		} else {
			log.Error().
				Str("error", errorResp.Error).
				Str("description", errorResp.ErrorDescription).
				Msg("Auth0 returned error")
		}
		http.Error(w, "Invalid verification code", http.StatusBadRequest)
		return
	}

	// Return the successful response
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// VerifyPasswordlessOTP verifies the OTP code and completes passwordless setup
func (s *Server) VerifyPasswordlessOTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user := r.Context().Value("user").(Claims)

	tokenURL := fmt.Sprintf("https://%s/oauth/token", os.Getenv("AUTH0_DOMAIN"))
	tokenPayload := map[string]interface{}{
		"grant_type":    "http://auth0.com/oauth/grant-type/passwordless/otp",
		"client_id":     os.Getenv("AUTH0_CLIENT_ID"),
		"client_secret": os.Getenv("AUTH0_CLIENT_SECRET"),
		"username":      user.Email,
		"otp":           req.Code,
		"realm":         "email",
		"scope":         "openid profile email",
	}

	jsonPayload, err := json.Marshal(tokenPayload)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal token payload")
		http.Error(w, "Failed to verify code", http.StatusInternalServerError)
		return
	}

	tokenReq, err := http.NewRequestWithContext(r.Context(), "POST", tokenURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create token request")
		http.Error(w, "Failed to verify code", http.StatusInternalServerError)
		return
	}

	tokenReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(tokenReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send token request")
		http.Error(w, "Failed to verify code", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Error().
			Int("status", resp.StatusCode).
			Str("body", string(body)).
			Msg("Auth0 returned non-200 status")
		http.Error(w, "Invalid verification code", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Passwordless authentication enabled successfully",
	})
}

// Login initiates the OAuth2 login flow with Auth0
func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	// Generate random state
	b := make([]byte, 32)
	_, err := cryptorand.Read(b)
	if err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}
	state := base64.RawURLEncoding.EncodeToString(b)

	// Store state in cookie
	cookie := &http.Cookie{
		Name:     "auth_state",
		Value:    state,
		MaxAge:   int(time.Hour.Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	}
	http.SetCookie(w, cookie)

	// Get auth URL from Auth0
	authURL := s.auth.Config.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("audience", fmt.Sprintf("https://%s/api/v2/", os.Getenv("AUTH0_DOMAIN"))),
	)

	// Redirect to Auth0 login
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// Logout handler
func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	// Clear any session cookies here
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

// Callback handles the OAuth2 callback
func (s *Server) Callback(w http.ResponseWriter, r *http.Request) {
	reqID := r.Context().Value("request_id").(string)
	log.Info().
		Str("request_id", reqID).
		Msg("OAuth callback received")

		// Retrieve Authorization code
	code := r.Header.Get("X-Auth-Code")
	if code == "" {
		code = r.URL.Query().Get("code")
	}

	if code == "" {
		log.Error().Msg("No code provided in callback")
		http.Error(w, "No code provided", http.StatusBadRequest)
		return
	}

	// Exchange code for token
	oauth2Token, err := s.auth.Config.Exchange(r.Context(), code)
	if err != nil {
		log.Error().
			Str("request_id", reqID).
			Err(err).
			Msg("Token exchange failed")
		http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
		return
	}

	// Extract the ID Token from OAuth2 token.
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		log.Error().
			Str("request_id", reqID).
			Msg("No id_token in OAuth2 token response")
		http.Error(w, "No id_token field in OAuth2 token", http.StatusInternalServerError)
		return
	}

	// Verify the ID Token
	verifier := s.auth.Provider.Verifier(&oidc.Config{
		ClientID: os.Getenv("AUTH0_CLIENT_ID"),
	})
	idToken, err := verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		log.Error().
			Err(err).
			Str("client_id", os.Getenv("AUTH0_CLIENT_ID")).
			Msg("Failed to verify ID token")
		http.Error(w, "Failed to verify ID token", http.StatusInternalServerError)
		return
	}

	// Parse claims
	var claims Claims
	if err := idToken.Claims(&claims); err != nil {
		log.Error().Err(err).Msg("Failed to parse claims")
		http.Error(w, "Failed to parse claims", http.StatusInternalServerError)
		return
	}

	// Prepare user creation/update parameters
	userParams := db.CreateUserParams{
		Auth0ID: claims.Sub,
		Name:    claims.Name,
		Email:   claims.Email,
		Picture: sql.NullString{
			String: claims.Picture,
			Valid:  claims.Picture != "",
		},
	}

	// Create or update user
	var user db.User
	existingUser, err := s.db.GetUserByAuth0ID(r.Context(), claims.Sub)
	if err != nil {
		// User doesn't exist, create new user
		createdUser, err := s.createUserWithRoles(r.Context(), userParams, []string{"security_admin"})
		if err != nil {
			log.Error().Err(err).Msg("Failed to create user")
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}
		user = createdUser
	} else {
		// Update existing user
		updateParams := db.UpdateUserParams{
			ID:    existingUser.ID,
			Name:  claims.Name,
			Email: claims.Email,
		}
		if err := s.db.UpdateUser(r.Context(), updateParams); err != nil {
			log.Error().Err(err).Msg("Failed to update user")
			http.Error(w, "Failed to update user", http.StatusInternalServerError)
			return
		}
		user = existingUser
	}

	// // Create or update user in database
	// user, err := s.db.GetUserByAuth0ID(r.Context(), claims.Sub)
	// if err != nil {
	// 	// User doesn't exist, create new user
	// 	params := db.CreateUserParams{
	// 		Auth0ID: claims.Sub,
	// 		Name:    claims.Name,
	// 		Email:   claims.Email,
	// 	}
	// 	user, err = s.db.CreateUser(r.Context(), params)
	// 	if err != nil {
	// 		log.Error().Err(err).Msg("Failed to create user")
	// 		http.Error(w, "Failed to create user", http.StatusInternalServerError)
	// 		return
	// 	}
	// } else {
	// 	// Update existing user
	// 	params := db.UpdateUserParams{
	// 		ID:    user.ID,
	// 		Name:  claims.Name,
	// 		Email: claims.Email,
	// 	}
	// 	if err := s.db.UpdateUser(r.Context(), params); err != nil {
	// 		log.Error().Err(err).Msg("Failed to update user")
	// 		http.Error(w, "Failed to update user", http.StatusInternalServerError)
	// 		return
	// 	}
	// }
	//
	// // Assign admin role to new user
	// // First get the admin role ID
	// adminRole, err := s.db.GetRoleByName(r.Context(), "admin")
	// if err != nil {
	// 	log.Error().Err(err).Msg("Failed to get admin role")
	// } else {
	// 	// Assign admin role to user
	// 	err = s.db.AssignUserRole(r.Context(), db.AssignUserRoleParams{
	// 		UserID:    user.ID,
	// 		RoleID:    adminRole.ID,
	// 		GrantedBy: user.ID, // self-granted since it's automatic
	// 	})
	// 	if err != nil {
	// 		log.Error().Err(err).Msg("Failed to assign admin role")
	// 	}
	// }

	// Get IP address and create session
	ipAddr := getIPFromRequest(r)
	ipNet := createInet(ipAddr)
	userAgent := r.UserAgent()

	// if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
	// 	ipAddr = forwardedFor
	// }
	sessionParams := db.CreateSessionParams{
		UserID:    user.ID,
		Device:    parseUserAgent(userAgent),
		IpAddress: ipNet,
		UserAgent: sql.NullString{String: userAgent, Valid: true},
		Location:  sql.NullString{String: "", Valid: false}, // Location lookup TODO
	}

	session, err := s.db.CreateSession(r.Context(), sessionParams)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create session")
		// Don't return error to user, just log it
	}

	// Log the security event
	_, err = s.db.CreateSecurityLog(r.Context(), db.CreateSecurityLogParams{
		UserID:           user.ID,
		EventType:        "login",
		EventDescription: "User logged in successfully",
		IpAddress:        ipNet,
		UserAgent:        sql.NullString{String: userAgent, Valid: true},
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to create security log")
		// Don't return error to user, just log it
	}

	// Return tokens as JSON
	response := struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		SessionID   string `json:"session_id"`
	}{
		AccessToken: oauth2Token.AccessToken,
		IDToken:     rawIDToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(time.Until(oauth2Token.Expiry).Seconds()),
		SessionID:   session.ID.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		log.Error().
			Str("request_id", reqID).
			Err(err).
			Msg("Failed to encode token response")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Publish login event asynchronously
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		loginEvent := struct {
			UserID    int32     `json:"user_id"`
			Email     string    `json:"email"`
			Timestamp time.Time `json:"timestamp"`
			IPAddress string    `json:"ip_address"`
			UserAgent string    `json:"user_agent"`
		}{
			UserID:    user.ID,
			Email:     claims.Email,
			Timestamp: time.Now(),
			IPAddress: ipAddr,
			UserAgent: userAgent,
		}

		if err := s.sns.PublishEvent(ctx, "user.login", loginEvent); err != nil {
			log.Error().Err(err).Msg("Failed to publish login event")
		}
	}()

	log.Info().
		Str("request_id", reqID).
		Msg("Successfully completed OAuth callback")
}

// GetUsers implements ServerInterface
func (s *Server) GetUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.db.ListUsers(r.Context())

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	apiUsers := make([]User, len(users))
	for i, u := range users {
		apiUsers[i] = User{
			Id:    int(u.ID),
			Name:  u.Name,
			Email: u.Email,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiUsers)
}

func (s *Server) GetUser(w http.ResponseWriter, r *http.Request) {
	// Add debug logging
	log.Debug().Msg("GetUser handler starting")

	// Get user from context (for Auth0 ID)
	contextUser, ok := r.Context().Value("user").(Claims)
	if !ok {
		log.Error().Msg("Failed to get user from context")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Fetch user from database using Auth0 ID
	dbUser, err := s.db.GetUserByAuth0ID(r.Context(), contextUser.Sub)
	if err != nil {
		log.Error().
			Err(err).
			Str("auth0_id", contextUser.Sub).
			Msg("Failed to retrieve user from database")
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Fetch user roles
	userRoles, err := s.db.GetUserRoles(r.Context(), dbUser.ID)
	if err != nil {
		log.Error().
			Err(err).
			Int32("user_id", dbUser.ID).
			Msg("Failed to retrieve user roles")
		// Continue without roles
	}

	// Prepare response
	userResponse := struct {
		ID         int32        `json:"id"`
		Name       string       `json:"name"`
		Email      string       `json:"email"`
		Picture    string       `json:"picture"`
		Roles      []string     `json:"roles"`
		MfaEnabled sql.NullBool `json:"mfa_enabled"`
	}{
		ID:         dbUser.ID,
		Name:       dbUser.Name,
		Email:      dbUser.Email,
		Picture:    dbUser.Picture.String,
		MfaEnabled: dbUser.MfaEnabled,
		Roles:      make([]string, 0),
	}

	// Add roles to response
	for _, role := range userRoles {
		userResponse.Roles = append(userResponse.Roles, role.Name)
	}

	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
	w.Header().Set("Content-Type", "application/json")

	// Encode and send response
	if err := json.NewEncoder(w).Encode(userResponse); err != nil {
		log.Error().Err(err).Msg("Failed to encode user response")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Debug().
		Int32("user_id", dbUser.ID).
		Str("name", dbUser.Name).
		Msg("User retrieved successfully")
}

// func (s *Server) GetUser(w http.ResponseWriter, r *http.Request) {
// 	// Add debug logging
// 	log.Debug().Msg("GetUser handler starting")
//
// 	user, ok := r.Context().Value("user").(Claims)
// 	if !ok {
// 		log.Error().Msg("Failed to get user from context")
// 		http.Error(w, "Unauthorized", http.StatusUnauthorized)
// 		return
// 	}
//
// 	log.Debug().
// 		Interface("user", user).
// 		Msg("Got user from context")
//
// 	// Add CORS headers
// 	w.Header().Set("Access-Control-Allow-Origin", "*")
// 	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
// 	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
//
// 	w.Header().Set("Content-Type", "application/json")
// 	if err := json.NewEncoder(w).Encode(user); err != nil {
// 		log.Error().Err(err).Msg("Failed to encode user response")
// 		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
// 		return
// 	}
// }

// // PostUsers implements ServerInterface
//
//	func (s *Server) PostUsers(w http.ResponseWriter, r *http.Request) {
//		var req CreateUserRequest
//		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
//			http.Error(w, err.Error(), http.StatusBadRequest)
//			return
//		}
//
//		user, err := s.db.CreateUser(r.Context(), db.CreateUserParams{
//			Name:  req.Name,
//			Email: req.Email,
//		})
//		if err != nil {
//			http.Error(w, err.Error(), http.StatusInternalServerError)
//			return
//		}
//
//		resp := User{
//			Id:    int(user.ID),
//			Name:  user.Name,
//			Email: user.Email,
//		}
//
//		w.Header().Set("Content-Type", "application/json")
//		w.WriteHeader(http.StatusCreated)
//		json.NewEncoder(w).Encode(resp)
//	}
func (s *Server) DebugToken(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Invalid Authorization header format",
		})
		return
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	// Parse without verification first to check structure
	parser := new(jwt.Parser)
	token, _, err := parser.ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Failed to parse token",
			"details": err.Error(),
		})
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Invalid claims format",
		})
		return
	}

	// Now try to verify with Auth0
	verifier := s.auth.Provider.Verifier(&oidc.Config{
		ClientID: os.Getenv("AUTH0_CLIENT_ID"),
	})

	idToken, err := verifier.Verify(r.Context(), tokenString)

	result := map[string]interface{}{
		"token_header": token.Header,
		"token_claims": claims,
		"verification": map[string]interface{}{
			"success": err == nil,
			"error":   err.Error(),
		},
		"expected": map[string]interface{}{
			"issuer":    "https://" + os.Getenv("AUTH0_DOMAIN") + "/",
			"audience":  os.Getenv("AUTH0_CLIENT_ID"),
			"client_id": os.Getenv("AUTH0_CLIENT_ID"),
		},
	}

	if err == nil {
		result["expiry"] = idToken.Expiry
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// SetUserAdmin sets or removes admin role for a user
func (s *Server) SetUserAdmin(w http.ResponseWriter, r *http.Request) {
	// Only allow POST method
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req struct {
		UserID  int32 `json:"user_id"`
		IsAdmin bool  `json:"is_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get current user (the one making the request)
	currentUser := r.Context().Value("user").(Claims)
	adminUser, err := s.db.GetUserByAuth0ID(r.Context(), currentUser.Sub)
	if err != nil {
		http.Error(w, "Failed to verify permissions", http.StatusInternalServerError)
		return
	}

	// Check if current user has admin role
	userRoles, err := s.db.GetUserRoles(r.Context(), adminUser.ID)
	if err != nil {
		http.Error(w, "Failed to verify permissions", http.StatusInternalServerError)
		return
	}

	hasAdminAccess := false
	for _, role := range userRoles {
		if role.Name == "admin" {
			hasAdminAccess = true
			break
		}
	}

	if !hasAdminAccess {
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	// Get admin role ID
	adminRole, err := s.db.GetRoleByName(r.Context(), "admin")
	if err != nil {
		http.Error(w, "Failed to get admin role", http.StatusInternalServerError)
		return
	}

	if req.IsAdmin {
		// Assign admin role
		err = s.db.AssignUserRole(r.Context(), db.AssignUserRoleParams{
			UserID:    req.UserID,
			RoleID:    adminRole.ID,
			GrantedBy: adminUser.ID,
		})
	} else {
		// Remove admin role
		err = s.db.RemoveUserRole(r.Context(), db.RemoveUserRoleParams{
			UserID: req.UserID,
			RoleID: adminRole.ID,
		})
	}

	if err != nil {
		http.Error(w, "Failed to update admin role", http.StatusInternalServerError)
		return
	}

	// Return success
	w.WriteHeader(http.StatusOK)
}

// Helper function to parse user agent into device string
func parseUserAgent(ua string) string {
	if strings.Contains(ua, "Mobile") {
		return "Mobile Device"
	} else if strings.Contains(ua, "Windows") {
		return "Windows PC"
	} else if strings.Contains(ua, "Macintosh") || strings.Contains(ua, "Mac OS X") {
		return "Mac"
	} else if strings.Contains(ua, "Linux") {
		return "Linux PC"
	}
	return "Unknown Device"
}

// Helper function to get valid IP from request
func getIPFromRequest(r *http.Request) string {
	// Try X-Forwarded-For first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Try X-Real-IP
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return xrip
	}

	// Get from RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// If splitting fails, use RemoteAddr directly
		return r.RemoteAddr
	}
	return ip
}

// Helper function to create valid Inet type
func createInet(ipStr string) pqtype.Inet {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		// Use a fallback IP if parsing fails
		ip = net.ParseIP("0.0.0.0")
	}

	// Use appropriate mask based on IP version
	var mask net.IPMask
	if ip.To4() != nil {
		mask = net.CIDRMask(32, 32) // IPv4
	} else {
		mask = net.CIDRMask(128, 128) // IPv6
	}

	return pqtype.Inet{
		IPNet: net.IPNet{
			IP:   ip,
			Mask: mask,
		},
		Valid: true,
	}
}

// Helper function to extract clean IP address
func getClientIP(r *http.Request) string {
	// Try X-Forwarded-For first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Try X-Real-IP
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}

	// Extract IP from RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// If splitting fails, try to use RemoteAddr directly
		return strings.TrimSpace(r.RemoteAddr)
	}
	return strings.TrimSpace(ip)
}
