package shared

// PSY-426: Generic Huma request/response wrappers.
//
// Huma requires every handler signature to take/return concrete struct types
// (it reflects on the type to derive the OpenAPI operation). The conventional
// per-handler shape is:
//
//	type GetXResponse struct { Body *contracts.X }
//
// which adds a one-off named type per endpoint with no information beyond the
// body element. Generic wrappers below collapse that boilerplate when the
// handler's body shape is just a contract type (or pointer to one).
//
// Usage:
//
//	func (h *Handler) GetThingHandler(ctx context.Context, req *Req) (
//	    *shared.BodyResponse[*contracts.Thing], error,
//	) { ... }
//
// Notes on Huma OpenAPI fidelity:
//   - Huma names component schemas after Go type names. A generic instantiation
//     `BodyResponse[*contracts.Thing]` produces a schema like
//     `BodyResponseThing` (the inner Body schema; Huma walks one level into the
//     wrapper). The original `GetThingResponseBody` schema is renamed; nothing
//     else changes (paths, operationIds — derived from the URL by huma.Get —
//     are unaffected).
//   - Wire format (the JSON body itself) is byte-for-byte identical: the
//     wrapper marshals to whatever the inner type marshals to.

// BodyResponse is the canonical Huma response wrapper when the body is a
// single contract type. Replaces the per-handler
// `type GetXResponse struct { Body *contracts.X }` boilerplate.
//
// Use the pointer instantiation (`BodyResponse[*contracts.Thing]`) when the
// existing handler returned a pointer body — preserves nil semantics and the
// JSON output is identical.
type BodyResponse[T any] struct {
	Body T
}

// IDPathRequest is the canonical path-id input. Use only when the handler's
// only path input is a numeric id and the doc text is the conventional
// "Numeric ID". Handlers with custom doc text (e.g. "Festival ID or slug")
// or extra path/query fields should keep their bespoke request struct.
type IDPathRequest struct {
	ID string `path:"id" validate:"required" doc:"Numeric ID"`
}

// SlugPathRequest is the canonical path-slug input. Use only when the
// handler's only path input is a slug and the doc text is the conventional
// "URL slug". Handlers with custom doc text (e.g. "Scene slug (e.g. phoenix-az)")
// should keep their bespoke request struct.
type SlugPathRequest struct {
	Slug string `path:"slug" validate:"required" doc:"URL slug"`
}
