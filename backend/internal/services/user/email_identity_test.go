package user

import (
	"errors"
	"time"

	"github.com/markbates/goth"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// The address these tests register with. Mixed case in both the local part and
// the domain, so a lookup that folds only one of the two still fails.
const mixedCaseAddress = "Sym.Case@Example.com"

// TestGetUserByEmail_CaseInsensitive covers the password-login lookup:
// AuthenticateUserWithPassword, SendMagicLinkHandler and the passkey handlers
// all reach the users row through GetUserByEmail.
func (suite *UserServiceIntegrationTestSuite) TestGetUserByEmail_CaseInsensitive() {
	user := &authm.User{
		Email:         stringPtr(mixedCaseAddress),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(user).Error)

	for _, typed := range []string{
		mixedCaseAddress,
		"sym.case@example.com",
		"SYM.CASE@EXAMPLE.COM",
		"sYm.CaSe@eXaMpLe.CoM",
	} {
		found, err := suite.userService.GetUserByEmail(typed)
		suite.Require().NoError(err, typed)
		suite.Require().NotNil(found, typed)
		suite.Equal(user.ID, found.ID, typed)
		// The row keeps the bytes its owner typed; only the comparison folds.
		suite.Equal(mixedCaseAddress, *found.Email, typed)
	}

	var stored string
	suite.Require().NoError(
		suite.db.Raw("SELECT email FROM users WHERE id = ?", user.ID).Scan(&stored).Error)
	suite.Equal(mixedCaseAddress, stored)
}

// TestGetUserByEmailIncludingDeleted_CaseInsensitive covers the account-recovery
// lookup, which is this codebase's "I cannot get in" path: there is no
// password-reset endpoint, so /auth/recover-account, its request and its
// confirm arm are the flows that resolve an address to a soft-deleted row.
func (suite *UserServiceIntegrationTestSuite) TestGetUserByEmailIncludingDeleted_CaseInsensitive() {
	deletedAt := time.Now().UTC().Add(-24 * time.Hour)
	user := &authm.User{
		Email:         stringPtr("Deleted.Case@Example.com"),
		IsActive:      false,
		EmailVerified: true,
		DeletedAt:     &deletedAt,
	}
	suite.Require().NoError(suite.db.Create(user).Error)

	found, err := suite.userService.GetUserByEmailIncludingDeleted("deleted.case@example.com")
	suite.Require().NoError(err)
	suite.Require().NotNil(found)
	suite.Equal(user.ID, found.ID)
	suite.Equal("Deleted.Case@Example.com", *found.Email)
}

// TestAuthenticateUserWithPassword_CaseInsensitive proves the login path end to
// end: registered in one casing, authenticated in another.
func (suite *UserServiceIntegrationTestSuite) TestAuthenticateUserWithPassword_CaseInsensitive() {
	const password = "StrongPass123!"
	created, err := suite.userService.CreateUserWithPassword(
		"Login.Case@Example.com", password, "Login", "Case",
	)
	suite.Require().NoError(err)

	authenticated, err := suite.userService.AuthenticateUserWithPassword(
		"login.case@example.com", password,
	)
	suite.Require().NoError(err)
	suite.Require().NotNil(authenticated)
	suite.Equal(created.ID, authenticated.ID)
}

// TestCreateUserWithPassword_DuplicateByCase pins the registration refusal: a
// case variant of a registered address is the same identity, refused with the
// same error the byte-identical duplicate gets, and writes no row.
func (suite *UserServiceIntegrationTestSuite) TestCreateUserWithPassword_DuplicateByCase() {
	_, err := suite.userService.CreateUserWithPassword(
		"Case.Dupe@Example.com", "Pass123!", "First", "Last",
	)
	suite.Require().NoError(err)

	user, err := suite.userService.CreateUserWithPassword(
		"CASE.DUPE@example.com", "Pass456!", "Other", "Name",
	)
	suite.Require().Error(err)
	suite.Nil(user)

	var authErr *apperrors.AuthError
	suite.Require().True(errors.As(err, &authErr))
	suite.Equal(apperrors.CodeUserExists, authErr.Code)

	var rows int64
	suite.Require().NoError(
		suite.db.Model(&authm.User{}).
			Where(authm.EmailIdentityWhere, "case.dupe@example.com").
			Count(&rows).Error)
	suite.Equal(int64(1), rows)
}

// TestFindOrCreateUser_LinksCaseVariantEmail covers the goth OAuth callback: a
// provider that returns the address in different casing than the row stores
// links to the existing account instead of minting a second one.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_LinksCaseVariantEmail() {
	existing := &authm.User{
		Email:         stringPtr("Goth.Case@Example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(existing).Error)

	linked, err := suite.userService.FindOrCreateUser(goth.User{
		UserID: "goth-case-subject",
		Email:  "goth.case@example.com",
	}, "google")

	suite.Require().NoError(err)
	suite.Require().NotNil(linked)
	suite.Equal(existing.ID, linked.ID)
	suite.Equal("Goth.Case@Example.com", *linked.Email)

	var rows int64
	suite.Require().NoError(
		suite.db.Model(&authm.User{}).
			Where(authm.EmailIdentityWhere, "goth.case@example.com").
			Count(&rows).Error)
	suite.Equal(int64(1), rows)
}

// TestUsersLowerEmailUniqueIndex_RefusesCaseVariantRow asserts the schema, not
// the service: an insert that goes around every application check still cannot
// create a second row for one mailbox. Without this the case-insensitive
// lookups above would be a convention rather than an identity, and a
// check-then-insert race could still admit the duplicate.
func (suite *UserServiceIntegrationTestSuite) TestUsersLowerEmailUniqueIndex_RefusesCaseVariantRow() {
	first := &authm.User{Email: stringPtr("Index.Case@Example.com"), IsActive: true}
	suite.Require().NoError(suite.db.Create(first).Error)

	second := &authm.User{Email: stringPtr("index.case@EXAMPLE.com"), IsActive: true}
	err := suite.db.Create(second).Error
	suite.Require().Error(err)
	suite.Require().ErrorIs(err, gorm.ErrDuplicatedKey)
}

// TestUsersLowerEmailUniqueIndex_AllowsManyNullEmails pins the OAuth-only case:
// lower(NULL) is NULL and a unique index permits any number of NULLs, so
// addressless accounts are not serialized behind one another.
func (suite *UserServiceIntegrationTestSuite) TestUsersLowerEmailUniqueIndex_AllowsManyNullEmails() {
	for range 3 {
		suite.Require().NoError(suite.db.Create(&authm.User{IsActive: true}).Error)
	}
}
