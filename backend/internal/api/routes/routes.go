package routes

import (
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/handlers"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services"
)

// SetupRoutes configures all API routes
func SetupRoutes(router *chi.Mux, cfg *config.Config) huma.API {
	api := humachi.New(router, huma.DefaultConfig("Psychic Homily", "1.0.0"))

	// Create services
	authService := services.NewAuthService(cfg)
	jwtService := services.NewJWTService(cfg)

	// Setup domain-specific routes
	setupSystemRoutes(router, api)
	setupAuthRoutes(router, api, authService, jwtService)
	setupApplicationRoutes(router, api, jwtService)

	return api
}

// setupAuthRoutes configures all authentication-related endpoints
func setupAuthRoutes(router *chi.Mux, api huma.API, authService *services.AuthService, jwtService *services.JWTService) {
	authHandler := handlers.NewAuthHandler(authService)
	oauthHTTPHandler := handlers.NewOAuthHTTPHandler(authService)

	// Public OAuth routes
	router.Get("/auth/login/{provider}", oauthHTTPHandler.OAuthLoginHTTPHandler)
	router.Get("/auth/callback/{provider}", oauthHTTPHandler.OAuthCallbackHTTPHandler)

	// Public auth endpoints
	huma.Post(api, "/auth/logout", authHandler.LogoutHandler)

	// Protected auth routes
	router.Group(func(r chi.Router) {
		r.Use(middleware.JWTMiddleware(jwtService))

		r.Get("/auth/profile", func(w http.ResponseWriter, req *http.Request) {
			resp, err := authHandler.GetProfileHandler(req.Context(), &struct{}{})
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp.Body)
		})

		r.Post("/auth/refresh", func(w http.ResponseWriter, req *http.Request) {
			resp, err := authHandler.RefreshTokenHandler(req.Context(), &struct{}{})
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp.Body)
		})
	})
}

// setupApplicationRoutes configures all business logic endpoints
func setupApplicationRoutes(router *chi.Mux, api huma.API, jwtService *services.JWTService) {
	// Public application endpoints
	huma.Post(api, "/show", handlers.ShowSubmissionHandler)

	// Protected application routes
	router.Group(func(r chi.Router) {
		r.Use(middleware.JWTMiddleware(jwtService))

		// Future protected endpoints:
		// huma.Get(api, "/user/shows", handlers.GetUserShowsHandler)
		// huma.Post(api, "/venues", handlers.CreateVenueHandler)
		// huma.Get(api, "/user/favorites", handlers.GetFavoritesHandler)
	})
}

// setupSystemRoutes configures system/infrastructure endpoints
func setupSystemRoutes(router *chi.Mux, api huma.API) {
	// Health check endpoint
	huma.Get(api, "/health", handlers.HealthHandler)
	
	// OpenAPI specification endpoint
	router.Get("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.OpenAPI())
	})
}
