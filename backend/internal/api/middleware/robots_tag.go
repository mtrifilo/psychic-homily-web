package middleware

import "net/http"

// RobotsTagHeader is the response header that carries indexing directives for
// a resource that is not an HTML page.
const RobotsTagHeader = "X-Robots-Tag"

// RobotsTagNoIndex is the directive every response from this server carries:
// keep the URL out of the index and do not follow links found in it.
const RobotsTagNoIndex = "noindex, nofollow"

// RobotsNoIndex sets X-Robots-Tag: noindex, nofollow on every response written
// through it, error and not-found answers included. It sets the header before
// calling next, so a handler that sets its own value replaces it.
func RobotsNoIndex(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(RobotsTagHeader, RobotsTagNoIndex)
		next.ServeHTTP(w, r)
	})
}
