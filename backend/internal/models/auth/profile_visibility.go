package auth

// The values users.profile_visibility holds.
const (
	ProfileVisibilityPublic  = "public"
	ProfileVisibilityPrivate = "private"
)

// IsPrivateProfile reports whether the account's profile is private, which
// every public read of the profile shows to its owner alone.
func (u *User) IsPrivateProfile() bool {
	return u.ProfileVisibility == ProfileVisibilityPrivate
}

// ProfileHiddenFrom reports whether the viewer with id viewerID may not see
// the account's profile: it is private and the viewer is not its owner. A
// viewerID of 0 is a logged-out visitor.
func (u *User) ProfileHiddenFrom(viewerID uint) bool {
	isOwner := viewerID != 0 && viewerID == u.ID
	return u.IsPrivateProfile() && !isOwner
}
