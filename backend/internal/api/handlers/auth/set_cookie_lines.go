package auth

import "net/http"

// setCookieLines is a Set-Cookie response header field for a response that
// sets several cookies, or none. Each element is one serialized cookie, so
// Huma writes one Set-Cookie line per cookie added and nothing when none was.
//
// The field must not be a slice of http.Cookie: Huma registers the element
// struct's exported fields as response headers of their own, which writes the
// cookie's Name, Value, Expires and the rest beside the Set-Cookie line, the
// session token among them.
type setCookieLines []string

// add appends each cookie as its Set-Cookie value. http.Cookie.String drops a
// cookie whose name is invalid to "", which Huma does not write.
func (l *setCookieLines) add(cookies ...http.Cookie) {
	for i := range cookies {
		*l = append(*l, cookies[i].String())
	}
}
