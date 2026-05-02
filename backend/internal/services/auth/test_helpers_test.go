package auth

import (
	"fmt"
	"time"

	"github.com/markbates/goth"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// nilDBUserService simulates a UserService with no database connection.
// All methods return "database not initialized" — matching the real
// UserService behaviour when constructed with a nil *gorm.DB.
type nilDBUserService struct{}

func (n *nilDBUserService) ListUsers(limit, offset int, filters contracts.AdminUserFilters) ([]*contracts.AdminUserResponse, int64, error) {
	return nil, 0, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) FindOrCreateUser(gothUser goth.User, provider string) (*authm.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) FindOrCreateUserWithConsent(gothUser goth.User, provider string, consent *contracts.OAuthSignupConsent) (*authm.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) AuthenticateUserWithPassword(email, password string) (*authm.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) CreateUserWithPassword(email, password, firstName, lastName string) (*authm.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) CreateUserWithPasswordWithLegal(email, password, firstName, lastName string, acceptance contracts.LegalAcceptance) (*authm.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) GetUserByID(userID uint) (*authm.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) GetUserByEmail(email string) (*authm.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) GetUserByUsername(username string) (*authm.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) UpdateUser(userID uint, updates map[string]any) (*authm.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) HashPassword(password string) (string, error) {
	return "", fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) VerifyPassword(hashedPassword, password string) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) IsAccountLocked(user *authm.User) bool { return false }

func (n *nilDBUserService) GetLockTimeRemaining(user *authm.User) time.Duration { return 0 }

func (n *nilDBUserService) IncrementFailedAttempts(userID uint) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) ResetFailedAttempts(userID uint) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) UpdatePassword(userID uint, currentPassword, newPassword string) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) SetEmailVerified(userID uint, verified bool) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) GetDeletionSummary(userID uint) (*contracts.DeletionSummary, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) SoftDeleteAccount(userID uint, reason *string) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) CreateUserWithoutPassword(email string) (*authm.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) ExportUserData(userID uint) (*contracts.UserDataExport, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) ExportUserDataJSON(userID uint) ([]byte, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) GetOAuthAccounts(userID uint) ([]authm.OAuthAccount, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) GetUserByEmailIncludingDeleted(email string) (*authm.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) IsAccountRecoverable(user *authm.User) bool { return false }

func (n *nilDBUserService) GetDaysUntilPermanentDeletion(user *authm.User) int { return 0 }

func (n *nilDBUserService) RestoreAccount(userID uint) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) GetExpiredDeletedAccounts() ([]authm.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) PermanentlyDeleteUser(userID uint) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) CanUnlinkOAuthAccount(userID uint, provider string) (bool, string, error) {
	return false, "", fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) UnlinkOAuthAccount(userID uint, provider string) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) GetFavoriteCities(userID uint) ([]authm.FavoriteCity, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) SetFavoriteCities(userID uint, cities []authm.FavoriteCity) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) SetShowReminders(userID uint, enabled bool) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) SetDefaultReplyPermission(userID uint, permission string) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) SetNotifyOnCommentSubscription(userID uint, enabled bool) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) SetNotifyOnMention(userID uint, enabled bool) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) SetNotifyOnCollectionDigest(userID uint, enabled bool) error {
	return fmt.Errorf("database not initialized")
}

// newNilDBUserService returns a UserServiceInterface that returns
// "database not initialized" for every DB-dependent method.
func newNilDBUserService() contracts.UserServiceInterface {
	return &nilDBUserService{}
}

// stringPtr returns a pointer to a string. Test helper.
func stringPtr(s string) *string {
	return &s
}
