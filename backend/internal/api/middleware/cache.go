package middleware

import (
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// CacheControl returns a Chi middleware that sets Cache-Control headers on GET responses.
// maxAge is the number of seconds the response can be served from cache.
// stale-while-revalidate is set to maxAge*6, allowing browsers to serve stale content
// while revalidating in the background.
func CacheControl(maxAge int) func(next http.Handler) http.Handler {
	swr := maxAge * 6
	headerValue := fmt.Sprintf("public, max-age=%d, stale-while-revalidate=%d", maxAge, swr)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				w.Header().Set("Cache-Control", headerValue)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// HumaCacheControl returns a Huma middleware that sets Cache-Control headers on GET responses.
func HumaCacheControl(maxAge int) func(ctx huma.Context, next func(huma.Context)) {
	swr := maxAge * 6
	headerValue := fmt.Sprintf("public, max-age=%d, stale-while-revalidate=%d", maxAge, swr)

	return func(ctx huma.Context, next func(huma.Context)) {
		if ctx.Method() == http.MethodGet {
			ctx.SetHeader("Cache-Control", headerValue)
		}
		next(ctx)
	}
}
