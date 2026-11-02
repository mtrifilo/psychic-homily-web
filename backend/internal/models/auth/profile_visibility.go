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

// IsOwnedBy reports whether the viewer with id viewerID is this account. A
// viewerID of 0 is a logged-out visitor, who owns no account.
func (u *User) IsOwnedBy(viewerID uint) bool {
	return viewerID != 0 && viewerID == u.ID
}

// ProfileHiddenFrom reports whether the viewer with id viewerID may not see
// the account's profile: it is private and the viewer is not its owner.
func (u *User) ProfileHiddenFrom(viewerID uint) bool {
	return u.IsPrivateProfile() && !u.IsOwnedBy(viewerID)
}
