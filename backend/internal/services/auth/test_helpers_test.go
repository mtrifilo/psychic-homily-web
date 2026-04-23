package auth

import (
	"fmt"
	"time"

	"github.com/markbates/goth"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// nilDBUserService simulates a UserService with no database connection.
// All methods return "database not initialized" — matching the real
// UserService behaviour when constructed with a nil *gorm.DB.
type nilDBUserService struct{}

func (n *nilDBUserService) ListUsers(limit, offset int, filters contracts.AdminUserFilters) ([]*contracts.AdminUserResponse, int64, error) {
	return nil, 0, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) FindOrCreateUser(gothUser goth.User, provider string) (*models.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) FindOrCreateUserWithConsent(gothUser goth.User, provider string, consent *contracts.OAuthSignupConsent) (*models.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) AuthenticateUserWithPassword(email, password string) (*models.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) CreateUserWithPassword(email, password, firstName, lastName string) (*models.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) CreateUserWithPasswordWithLegal(email, password, firstName, lastName string, acceptance contracts.LegalAcceptance) (*models.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) GetUserByID(userID uint) (*models.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) GetUserByEmail(email string) (*models.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) GetUserByUsername(username string) (*models.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) UpdateUser(userID uint, updates map[string]any) (*models.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) HashPassword(password string) (string, error) {
	return "", fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) VerifyPassword(hashedPassword, password string) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) IsAccountLocked(user *models.User) bool { return false }

func (n *nilDBUserService) GetLockTimeRemaining(user *models.User) time.Duration { return 0 }

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

func (n *nilDBUserService) CreateUserWithoutPassword(email string) (*models.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) ExportUserData(userID uint) (*contracts.UserDataExport, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) ExportUserDataJSON(userID uint) ([]byte, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) GetOAuthAccounts(userID uint) ([]models.OAuthAccount, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) GetUserByEmailIncludingDeleted(email string) (*models.User, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) IsAccountRecoverable(user *models.User) bool { return false }

func (n *nilDBUserService) GetDaysUntilPermanentDeletion(user *models.User) int { return 0 }

func (n *nilDBUserService) RestoreAccount(userID uint) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) GetExpiredDeletedAccounts() ([]models.User, error) {
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

func (n *nilDBUserService) GetFavoriteCities(userID uint) ([]models.FavoriteCity, error) {
	return nil, fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) SetFavoriteCities(userID uint, cities []models.FavoriteCity) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) SetShowReminders(userID uint, enabled bool) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) SetNotifyOnCommentSubscription(userID uint, enabled bool) error {
	return fmt.Errorf("database not initialized")
}

func (n *nilDBUserService) SetNotifyOnMention(userID uint, enabled bool) error {
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
