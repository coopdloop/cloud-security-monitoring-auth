package api

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed swagger/*
var swaggerFiles embed.FS

func (s *Server) ServeSwaggerUI(w http.ResponseWriter, r *http.Request) {
	// Serve index.html for the base route
	if r.URL.Path == "/docs" || r.URL.Path == "/docs/" {
		content, err := swaggerFiles.ReadFile("swagger/index.html")
		if err != nil {
			http.Error(w, "Failed to read index.html", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(content)
		return
	}

	// Serve OpenAPI spec
	if r.URL.Path == "/openapi.yaml" {

		w.Header().Set("Content-Type", "application/x-yaml")
		w.Header().Set("Content-Disposition", "inline; filename=openapi.yaml")
		http.ServeFile(w, r, "api/openapi.yaml")
		return
	}

	// Serve static files
	filePath := strings.TrimPrefix(r.URL.Path, "/docs/")

	// Create a sub filesystem
	subFS, err := fs.Sub(swaggerFiles, "swagger")
	if err != nil {
		http.Error(w, "Failed to create sub filesystem", http.StatusInternalServerError)
		return
	}

	// Use http.FileServer with the sub filesystem
	fileServer := http.FileServer(http.FS(subFS))

	// Adjust the URL path to remove the "/docs" prefix
	r.URL.Path = "/" + filePath
	fileServer.ServeHTTP(w, r)
}
