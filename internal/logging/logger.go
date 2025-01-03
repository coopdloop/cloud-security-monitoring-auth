// internal/logging/logger.go
package logging

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/middleware"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type LogConfig struct {
    Level      string
    Pretty     bool
    WithCaller bool
}

func Init(config LogConfig) {
    // Set default level to info
    level := zerolog.InfoLevel
    if parsedLevel, err := zerolog.ParseLevel(config.Level); err == nil {
        level = parsedLevel
    }
    zerolog.SetGlobalLevel(level)

    // Configure logger output
    var output zerolog.ConsoleWriter
    if config.Pretty {
        output = zerolog.ConsoleWriter{
            Out:        os.Stdout,
            TimeFormat: time.RFC3339,
            NoColor:    false,
        }
    }

    // Set global logger
    log.Logger = zerolog.New(output).
        With().
        Timestamp().
        Caller().
        Logger()
}

// Custom logging middleware for Chi
func LoggerMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()

        // Create a response wrapper to capture the status code
        ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

        // Process request
        next.ServeHTTP(ww, r)

        // Log the request details
        log.Info().
            Str("method", r.Method).
            Str("path", r.URL.Path).
            Str("remote_ip", r.RemoteAddr).
            Str("user_agent", r.UserAgent()).
            Int("status", ww.Status()).
            Str("duration", time.Since(start).String()).
            Int("bytes_written", ww.BytesWritten()).
            Msg("Request processed")
    })
}

// RequestIDMiddleware adds a request ID to the context
func RequestIDMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        id := uuid.New().String()
        ctx := context.WithValue(r.Context(), "request_id", id)
        w.Header().Set("X-Request-ID", id)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
