package shared

import "strings"

// IfNoneMatchCovers reports whether an If-None-Match header covers the current
// ETag, i.e. whether the request should be answered 304 instead of the body.
//
// COMPARISON IS WEAK, per RFC 9110 §13.1.2, which specifies weak comparison for
// If-None-Match specifically (If-Match is the strong one). A `W/` prefix on
// either side is therefore ignored: a cache or proxy is allowed to weaken a
// validator it forwards, and refusing to match a weakened copy of our own tag
// would send the whole payload to a client that already has it.
//
// Also handles the two other shapes the header can take: the `*` wildcard, and
// a comma-separated list of candidate validators.
//
// Shared because conditional GETs are not one surface's concern — the calendar
// feeds and the graph overview both serve large, slow-changing bodies where a
// 304 is the difference between a header exchange and hundreds of KB.
func IfNoneMatchCovers(ifNoneMatch, etag string) bool {
	ifNoneMatch = strings.TrimSpace(ifNoneMatch)
	if ifNoneMatch == "" || etag == "" {
		return false
	}
	if ifNoneMatch == "*" {
		return true
	}
	want := strings.TrimPrefix(etag, "W/")
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		if strings.TrimPrefix(strings.TrimSpace(candidate), "W/") == want {
			return true
		}
	}
	return false
}
