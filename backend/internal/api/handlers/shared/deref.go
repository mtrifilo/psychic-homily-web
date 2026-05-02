package shared

// Deref returns *p if p is non-nil, otherwise the zero value of T.
//
// It exists to collapse the very common handler pattern
//
//	var s string
//	if req.Body.Field != nil {
//	    s = *req.Body.Field
//	}
//
// down to a single expression
//
//	s := shared.Deref(req.Body.Field)
//
// Use Deref only when "treat nil as the zero value" is the intended
// semantics. When the nil case carries a different meaning (e.g. PATCH
// "field absent" vs PATCH "field set to zero value"), keep the explicit
// nil-check so the branching is preserved.
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// DerefOr returns *p if p is non-nil, otherwise fallback.
//
// Prefer Deref when the fallback is the zero value of T (the empty
// string, 0, false, etc.). DerefOr is for cases where the default
// is genuinely non-zero, e.g. boolean preferences that default to
// true:
//
//	notifyEmail := shared.DerefOr(req.Body.NotifyEmail, true)
func DerefOr[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}
