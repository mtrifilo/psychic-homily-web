package user

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/shared"
)

// users_live_account_check refuses an active row with deleted_at set, so a
// filter on is_active alone selects exactly the rows User.CanSignIn accepts.

func (suite *UserServiceIntegrationTestSuite) createLiveAccount(address string) *authm.User {
	suite.T().Helper()
	user := &authm.User{Email: stringPtr(address), IsActive: true}
	suite.Require().NoError(suite.db.Create(user).Error)
	return user
}

func (suite *UserServiceIntegrationTestSuite) reloadAccount(userID uint) authm.User {
	suite.T().Helper()
	var user authm.User
	suite.Require().NoError(suite.db.First(&user, userID).Error)
	return user
}

func (suite *UserServiceIntegrationTestSuite) TestLiveAccountCheck_RefusesDeletedAtOnActiveRow() {
	user := suite.createLiveAccount("live-check-gorm@example.com")

	err := suite.db.Model(&authm.User{}).Where("id = ?", user.ID).Update("deleted_at", time.Now()).Error

	suite.Require().Error(err)
	suite.True(shared.IsCheckConstraintViolation(err), "got %v", err)
	suite.Nil(suite.reloadAccount(user.ID).DeletedAt)
}

// The driver error names the constraint; GORM's translation drops the name, so
// this reads it below GORM.
func (suite *UserServiceIntegrationTestSuite) TestLiveAccountCheck_HandWrittenUpdateNamesConstraint() {
	user := suite.createLiveAccount("live-check-sql@example.com")
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)

	_, err = sqlDB.Exec("UPDATE users SET deleted_at = now() WHERE id = $1", user.ID)

	var pgErr *pgconn.PgError
	suite.Require().True(errors.As(err, &pgErr), "got %v", err)
	suite.Equal("23514", pgErr.Code)
	suite.Equal("users_live_account_check", pgErr.ConstraintName)
}

// Create omits a false IsActive because the column has a default, so a
// soft-deleted row cannot be created in one statement through the model.
func (suite *UserServiceIntegrationTestSuite) TestLiveAccountCheck_CreateWithDeletedAtIsRefused() {
	now := time.Now()
	user := &authm.User{Email: stringPtr("live-check-create@example.com"), IsActive: false, DeletedAt: &now}

	err := suite.db.Create(user).Error

	suite.Require().Error(err)
	suite.True(shared.IsCheckConstraintViolation(err), "got %v", err)
}

func (suite *UserServiceIntegrationTestSuite) TestLiveAccountCheck_RefusesReactivatingDeletedRow() {
	user := suite.createLiveAccount("live-check-reactivate@example.com")
	suite.Require().NoError(suite.userService.SoftDeleteAccount(user.ID, nil))

	err := suite.db.Model(&authm.User{}).Where("id = ?", user.ID).Update("is_active", true).Error

	suite.Require().Error(err)
	suite.True(shared.IsCheckConstraintViolation(err), "got %v", err)
	suite.False(suite.reloadAccount(user.ID).IsActive)
}

func (suite *UserServiceIntegrationTestSuite) TestLiveAccountCheck_SoftDeleteAndRestoreSatisfyIt() {
	user := suite.createLiveAccount("live-check-roundtrip@example.com")

	suite.Require().NoError(suite.userService.SoftDeleteAccount(user.ID, stringPtr("leaving")))
	deleted := suite.reloadAccount(user.ID)
	suite.False(deleted.IsActive)
	suite.NotNil(deleted.DeletedAt)
	suite.False(deleted.CanSignIn())

	suite.Require().NoError(suite.userService.RestoreAccount(user.ID))
	restored := suite.reloadAccount(user.ID)
	suite.True(restored.IsActive)
	suite.Nil(restored.DeletedAt)
	suite.True(restored.CanSignIn())
}
