package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"

	"github.com/coopdloop/go-backend-sqlc/internal/api"
	"github.com/coopdloop/go-backend-sqlc/internal/auth"
	"github.com/coopdloop/go-backend-sqlc/internal/aws"
	"github.com/coopdloop/go-backend-sqlc/internal/db"
	"github.com/coopdloop/go-backend-sqlc/internal/logging"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
)

func loadEnv() error {
	// Try to load dev.env first, fallback to default.env
	if err := godotenv.Load("dev.env"); err != nil {
		// If dev.env fails, try default.env
		log.Printf("Warning: No dev.env file found")
	}

	required := []string{
		"AUTH0_DOMAIN",
		"AUTH0_CLIENT_ID",
		"AUTH0_CLIENT_SECRET",
		"AUTH0_CALLBACK_URL",
		"DATABASE_URL",
	}

	for _, v := range required {
		if os.Getenv(v) == "" {
			return fmt.Errorf("missing required env var: %s", v)
		}
	}
	return nil
}

func main() {
	// Initialize logger
	logging.Init(logging.LogConfig{
		Level:      "debug", // or get from env
		Pretty:     true,    // nice formatting for development
		WithCaller: true,
	})

	// Environment variables
	if err := loadEnv(); err != nil {
		log.Error().Err(err).Msg("Failed to load env variables...")
		// log.Fatal().Msg(err)
	}

	// Database setup
	dbConn, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Error().Err(err).Msg("Failed to setup database connection")
		// log.Fatal(err)
	}
	defer dbConn.Close()

	// Auth setup
	authenticator, err := auth.NewAuthenticator()
	if err != nil {
		log.Error().Err(err).Msg("Failed to setup Authenticator")
		// log.Fatal(err)
	}

	// Initialize SNS client
	snsClient, err := aws.NewSNSClient(context.Background())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize SNS client")
	}

	queries := db.New(dbConn)
	server := api.NewServer(queries, authenticator, snsClient)

	// Create Chi router
	r := chi.NewRouter()

	// logging middlewares
	r.Use(logging.RequestIDMiddleware)
	r.Use(logging.LoggerMiddleware)

	// CORS middleware
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:8080", "http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	// Auth routes
	r.Get("/login", server.Login)
	r.Get("/callback", server.Callback)
	r.Get("/logout", server.Logout)

	// Public routes
	r.Post("/passwordless/start", server.StartPasswordlessLogin)
	r.Post("/passwordless/verify", server.VerifyPasswordlessLogin)
	r.Get("/api/debug/token", server.DebugToken)
	r.Get("/token-info", server.GetTokenInfo)

	// API routes (protected)
	r.Group(func(r chi.Router) {
		r.Use(server.AuthMiddleware)
		r.Get("/api/user", server.GetUser)
		r.Put("/api/user/profile", server.UpdateUserProfile)
		r.Post("/api/setup-mfa", server.SetupMFA)
		r.Post("/api/setup-passwordless", server.SetupPasswordless)
		r.Post("/api/verify-passwordless", server.VerifyPasswordlessOTP)
		// r.Mount("/api", api.HandlerFromMux(server, r))
		// Additional security routes under /api
		r.Get("/api/sessions", server.GetSessions)
		// r.Delete("/api/sessions/{sessionId}", server.RevokeSession)
		r.Get("/api/security-log", server.GetSecurityLog)
		r.Post("/api/verify-mfa", server.VerifyMFA)
		r.Post("/api/disable-mfa", server.DisableMFA)
	})

	// role-protected routes separate
	r.Group(func(r chi.Router) {
		r.Use(server.AuthMiddleware)
		r.Use(server.RequireRole("admin", "security_admin"))
		r.Post("/api/users/set-admin", server.SetUserAdmin)
		// Role management routes
		r.Get("/api/roles", server.GetAllRoles)
		r.Get("/api/users/roles", server.GetUsersWithRoles)
		r.Get("/api/users/roles/{userID}", server.GetUserRoles)
		r.Put("/api/users/roles", server.UpdateUserRoles)
		r.Get("/api/users/claims", server.GetUserClaims)
	})

	// Serve static files
	fileServer := http.FileServer(http.Dir("frontend/public"))
	r.Handle("/static/*", http.StripPrefix("/static", fileServer))

	// open api
	r.Get("/docs", server.ServeSwaggerUI)
	r.Get("/docs/*", server.ServeSwaggerUI)
	r.Get("/openapi.yaml", server.ServeSwaggerUI)

	// Handle other routes
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		// List of API endpoints that should not serve the index.html
		apiPaths := map[string]bool{
			"/callback": true,
			"/login":    true,
			"/logout":   true,
		}

		// If it's not an API path, serve index.html or the static file
		if !apiPaths[r.URL.Path] {
			if _, err := os.Stat("frontend/public" + r.URL.Path); os.IsNotExist(err) {
				http.ServeFile(w, r, "frontend/public/index.html")
				return
			}
			fileServer.ServeHTTP(w, r)
		}
	})

	log.Printf("Starting server on :3000")
	if err := http.ListenAndServe(":3000", r); err != nil {
		log.Error().Err(err).Msg("Failed to start server...")
		// log.Fatal(err)
	}
}
