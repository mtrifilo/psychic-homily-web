// Command openapi-spec prints the API's OpenAPI document to stdout.
//
// This exists so the frontend can generate its API types WITHOUT a running
// server or database (PSY-1550). The alternative — curling a live backend —
// would couple type generation and the CI drift check to a deployed
// environment being up, which is exactly the kind of coupling that makes a
// contract check get disabled the first time it is inconvenient.
//
// It builds the real router through SetupRoutes, so the emitted document is
// the same one the server serves: any route added, removed, or renamed shows
// up here with no separate registration step to forget.
//
// A nil *gorm.DB is deliberate. Route registration only needs handlers to
// exist, not to run, and the routes tests already construct the container this
// way. Nothing here opens a connection.
//
// Usage:
//
//	go run ./cmd/openapi-spec > spec.json
package main

import (
	"fmt"
	"os"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/routes"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services"
)

func main() {
	// Placeholder secrets: never used to sign or verify anything here, but the
	// constructors validate their length, so they cannot be empty.
	cfg := &config.Config{
		Server: config.ServerConfig{Addr: "localhost:8080"},
		JWT: config.JWTConfig{
			SecretKey: "openapi-spec-generation-placeholder-key",
			Expiry:    24,
		},
		OAuth: config.OAuthConfig{
			SecretKey: "openapi-spec-generation-placeholder-key",
		},
	}

	api := routes.SetupRoutes(chi.NewRouter(), services.NewServiceContainer(nil, cfg), cfg)

	spec, err := api.OpenAPI().YAML()
	if err != nil {
		fmt.Fprintf(os.Stderr, "openapi-spec: marshal: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(spec); err != nil {
		fmt.Fprintf(os.Stderr, "openapi-spec: write: %v\n", err)
		os.Exit(1)
	}
}
