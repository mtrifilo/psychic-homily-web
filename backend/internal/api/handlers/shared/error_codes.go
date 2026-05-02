package shared

// HTTP 400 vs 422 convention (PSY-524, strict RFC split)
// =====================================================
//
// This file documents the canonical convention for HTTP 400 vs 422
// responses across the catalog / community / auth / admin handler
// buckets. The convention is enforced by the handlers themselves and by
// MapTagError / MapCollectionError in error_mappers.go.
//
//   - 400 BadRequest      — syntactically malformed only. The request
//     body / params / headers cannot be parsed or violate the protocol
//     itself. The reader / parser / decoder failed.
//   - 422 UnprocessableEntity — request parses fine but the value is
//     semantically rejected. Length cap, missing required field after
//     trim, invalid enum, business-rule violation, format check on a
//     parsed value.
//
// When in doubt: if the request parses, return 422; if parse failed,
// return 400.
//
// Examples — stays 400 (parse-time):
//
//   - strconv.ParseUint(req.ShowID, …) returns err  → "Invalid show ID"
//   - shared.ParseDate(req.Date) returns err        → "Invalid date format, expected YYYY-MM-DD"
//   - base64.StdEncoding.DecodeString(req.Body.Content) returns err  → "Invalid base64 content"
//   - strconv.Atoi(year) returns err                → "Invalid year: 20xx"
//
// Examples — becomes / stays 422 (semantic):
//
//   - if req.Body.Title == ""                       → "Title is required"
//   - if len(*req.Body.Description) > 5000          → "Description must be 5000 characters or fewer"
//   - if !catalogm.IsValidTagEntityType(et)         → "Invalid entity_type"
//   - if !isValidBandcampURL(*req.Body.Bandcamp)    → "Invalid Bandcamp URL format"
//   - if req.Body.ArtistID == 0                     → "artist_id is required"
//   - service returns CodeTagMergeInvalid           → "Cannot merge a tag with itself"
//   - service returns CodeCollectionTagLimitExceeded → "Collections can have at most 10 tags"
//
// Out of scope:
//
//   - 401 / 403 / 404 / 409 / 5xx — unchanged.
//   - The pipeline / engagement / notification / system handler buckets
//     were not part of the PSY-524 sweep; they should be normalized in
//     a follow-up if they drift from this convention.
//
// New handlers MUST follow this convention. Treat the diff for PSY-524
// as the canonical reference until this becomes muscle memory.
//
// This file is intentionally code-free; it exists to host the doc
// comment in a discoverable location next to the error_mappers helpers
// that translate domain errors into HTTP responses.
