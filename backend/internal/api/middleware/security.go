package middleware

import (
	"net/http"
	"os"
)

// SecurityHeaders adds security-related HTTP headers to all responses.
// These headers help protect against common web vulnerabilities:
// - XSS (Cross-Site Scripting)
// - Clickjacking
// - MIME type sniffing
// - Protocol downgrade attacks
// - Information leakage via referrer
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// X-Content-Type-Options: Prevents MIME type sniffing
		// Browsers will strictly follow the Content-Type header
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// X-Frame-Options: Prevents clickjacking by disallowing framing
		// DENY = page cannot be displayed in a frame, regardless of origin
		w.Header().Set("X-Frame-Options", "DENY")

		// X-XSS-Protection: Legacy XSS filter (for older browsers)
		// Modern browsers use CSP instead, but this doesn't hurt
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Referrer-Policy: Controls how much referrer info is sent
		// strict-origin-when-cross-origin = full URL for same-origin, only origin for cross-origin
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Permissions-Policy: Restricts browser features
		// Disable features we don't use to reduce attack surface
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=()")

		// Strict-Transport-Security (HSTS): Forces HTTPS
		// Only set in production to avoid issues with local development
		// max-age=31536000 = 1 year, includeSubDomains = apply to all subdomains
		if isProduction() {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// Content-Security-Policy: Controls which resources can be loaded
		// This is set separately for API vs frontend concerns
		// For an API backend, we mainly care about preventing script injection in error responses
		w.Header().Set("Content-Security-Policy", buildCSP())

		// X-Permitted-Cross-Domain-Policies: Restricts Adobe Flash/PDF cross-domain requests
		w.Header().Set("X-Permitted-Cross-Domain-Policies", "none")

		next.ServeHTTP(w, r)
	})
}

// buildCSP constructs a Content-Security-Policy header value
// For a JSON API, this is relatively simple since we don't serve HTML pages
func buildCSP() string {
	// For a REST API that primarily returns JSON:
	// - default-src 'none': Deny everything by default
	// - frame-ancestors 'none': Prevent embedding (similar to X-Frame-Options)
	// - base-uri 'none': Prevent base tag injection
	// - form-action 'none': Prevent form submissions
	//
	// Note: If your API ever returns HTML (error pages, docs), you may need
	// to adjust this. For pure JSON APIs, this strict policy is appropriate.
	return "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"
}

// isProduction checks if we're running in production environment
func isProduction() bool {
	env := os.Getenv("ENVIRONMENT")
	return env == "production" || env == "prod"
}

// SecurityHeadersConfig allows customizing security headers
type SecurityHeadersConfig struct {
	// EnableHSTS enables Strict-Transport-Security even in non-production
	EnableHSTS bool
	// HSTSMaxAge is the max-age value for HSTS in seconds (default: 31536000 = 1 year)
	HSTSMaxAge int
	// HSTSIncludeSubDomains includes subdomains in HSTS
	HSTSIncludeSubDomains bool
	// HSTSPreload enables HSTS preload (only if you've submitted to preload list)
	HSTSPreload bool
	// CustomCSP allows overriding the default Content-Security-Policy
	CustomCSP string
	// FrameOptions allows changing X-Frame-Options (default: DENY)
	FrameOptions string
}

// DefaultSecurityConfig returns the default security headers configuration
func DefaultSecurityConfig() SecurityHeadersConfig {
	return SecurityHeadersConfig{
		EnableHSTS:            false, // Only enabled automatically in production
		HSTSMaxAge:            31536000,
		HSTSIncludeSubDomains: true,
		HSTSPreload:           false,
		CustomCSP:             "",
		FrameOptions:          "DENY",
	}
}

// SecurityHeadersWithConfig creates security headers middleware with custom configuration
func SecurityHeadersWithConfig(cfg SecurityHeadersConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")

			frameOptions := cfg.FrameOptions
			if frameOptions == "" {
				frameOptions = "DENY"
			}
			w.Header().Set("X-Frame-Options", frameOptions)

			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=()")

			// HSTS
			if cfg.EnableHSTS || isProduction() {
				hstsValue := "max-age=" + string(rune(cfg.HSTSMaxAge))
				if cfg.HSTSMaxAge == 0 {
					hstsValue = "max-age=31536000"
				}
				if cfg.HSTSIncludeSubDomains {
					hstsValue += "; includeSubDomains"
				}
				if cfg.HSTSPreload {
					hstsValue += "; preload"
				}
				w.Header().Set("Strict-Transport-Security", hstsValue)
			}

			// CSP
			csp := cfg.CustomCSP
			if csp == "" {
				csp = buildCSP()
			}
			w.Header().Set("Content-Security-Policy", csp)

			w.Header().Set("X-Permitted-Cross-Domain-Policies", "none")

			next.ServeHTTP(w, r)
		})
	}
}
