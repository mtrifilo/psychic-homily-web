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
	setupAuthRoutes(router, api, authService, jwtService, cfg)
	setupShowRoutes(router, api, jwtService)

	return api
}

// setupAuthRoutes configures all authentication-related endpoints
func setupAuthRoutes(router *chi.Mux, api huma.API, authService *services.AuthService,
	jwtService *services.JWTService, cfg *config.Config) {
	userService := services.NewUserService()
	authHandler := handlers.NewAuthHandler(authService, jwtService, userService, cfg)
	oauthHTTPHandler := handlers.NewOAuthHTTPHandler(authService)

	// Public OAuth routes
	router.Get("/auth/login/{provider}", oauthHTTPHandler.OAuthLoginHTTPHandler)
	router.Get("/auth/callback/{provider}", oauthHTTPHandler.OAuthCallbackHTTPHandler)

	// Public auth endpoints
	huma.Post(api, "/auth/login", authHandler.LoginHandler)
	huma.Post(api, "/auth/logout", authHandler.LogoutHandler)
	huma.Post(api, "/auth/register", authHandler.RegisterHandler)

	api.UseMiddleware(middleware.HumaJWTMiddleware(jwtService))

	// Protected auth routes - these will use the middleware
	huma.Get(api, "/auth/profile", authHandler.GetProfileHandler)
	huma.Post(api, "/auth/refresh", authHandler.RefreshTokenHandler)
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

// SetupShowRoutes configures all show-related endpoints
func setupShowRoutes(router *chi.Mux, api huma.API, jwtService *services.JWTService) {
	showHandler := handlers.NewShowHandler()

	// Public show endpoints
	huma.Get(api, "/shows", showHandler.GetShowsHandler)
	huma.Get(api, "/shows/{show_id}", showHandler.GetShowHandler)

	// Protected show endpoints - these will use the middleware already applied to the API
	huma.Post(api, "/shows", showHandler.CreateShowHandler)
	huma.Put(api, "/shows/{show_id}", showHandler.UpdateShowHandler)
	huma.Delete(api, "/shows/{show_id}", showHandler.DeleteShowHandler)
	huma.Post(api, "/shows/ai-process", showHandler.AIProcessShowHandler)
}
