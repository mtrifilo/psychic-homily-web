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
