package shared

// InvalidReplyPermissionMessage is the canonical 400 response detail
// when a reply-permission field is present but unrecognized. Distinct
// from the "<field> is required" detail (sent when the field is empty
// or missing) so clients can tell which failure they hit.
//
// Used by:
//   - PUT /comments/{id}/reply-permission (per-comment override; PSY-296,
//     sharper validation in PSY-592)
//   - PATCH /auth/preferences/default-reply-permission (per-user default;
//     PSY-296, unified to match the per-comment handler in PSY-621)
//
// Keep in lockstep with `engagementm.IsValidReplyPermission`. If new
// enum values are added, update both this constant and the validator
// together.
const InvalidReplyPermissionMessage = "permission must be one of: anyone, followers, author_only"
