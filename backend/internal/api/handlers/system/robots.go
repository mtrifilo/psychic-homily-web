package system

import (
	"net/http"

	"psychic-homily-backend/internal/respond"
)

// RobotsTxtRoute is the path RobotsTxtHandler is registered on.
const RobotsTxtRoute = "/robots.txt"

// robotsTxtBody asks every crawler to stay off this host. This server answers
// with JSON documents that no crawler indexes; the catalog is published for
// indexing by the public site's pages, and the public site's own robots.txt
// governs crawler access to them. No agent gets an allow group here, retrieval
// fetchers included: the agents the public site allows reach the catalog
// through its pages.
const robotsTxtBody = "User-agent: *\nDisallow: /\n"

// robotsTxtCacheControl lets a crawler or a shared cache reuse the file for a
// day.
const robotsTxtCacheControl = "public, max-age=86400"

// RobotsTxtHandler serves robotsTxtBody. The answer is the same for every
// caller, whatever its user agent or credentials.
func RobotsTxtHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", robotsTxtCacheControl)
	respond.SafeWrite(r.Context(), w, []byte(robotsTxtBody))
}
