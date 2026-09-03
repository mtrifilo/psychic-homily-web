package middleware

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	engagementsvc "psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/testutil"
)

// CalendarTokenValidator against real rows. The routes-level tests inject a stub
// predicate, so this suite is what pins that the stub matches what
// CalendarService.ValidateCalendarToken actually answers, and that the exemption
// hashes through the service rather than spelling its own idiom.
type CalendarTokenValidatorSuite struct {
	suite.Suite
	db     *gorm.DB
	testDB *testutil.TestDatabase
	svc    *engagementsvc.CalendarService
}

func TestCalendarTokenValidator(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(CalendarTokenValidatorSuite))
}

func (s *CalendarTokenValidatorSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.svc = engagementsvc.NewCalendarService(s.db, nil)
}

func (s *CalendarTokenValidatorSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *CalendarTokenValidatorSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	for _, table := range []string{"calendar_tokens", "user_preferences", "oauth_accounts", "users"} {
		_, err := sqlDB.Exec("DELETE FROM " + table)
		s.Require().NoError(err, "cleanup %s", table)
	}
}

func (s *CalendarTokenValidatorSuite) createUser(email string, active bool) *authm.User {
	s.T().Helper()
	user := &authm.User{Email: &email, IsActive: active, EmailVerified: true}
	s.Require().NoError(s.db.Create(user).Error)
	return user
}

func (s *CalendarTokenValidatorSuite) TestLiveTokenValidates() {
	user := s.createUser("feed-owner@test.com", true)
	created, err := s.svc.CreateToken(user.ID, "https://api.test")
	s.Require().NoError(err)

	validate := CalendarTokenValidator(s.svc)
	s.True(validate(created.Token), "a live feed token must validate")
}

func (s *CalendarTokenValidatorSuite) TestJunkTokenDoesNotValidate() {
	validate := CalendarTokenValidator(s.svc)
	s.False(validate(engagementsvc.CalendarTokenPrefix+"junk"),
		"a token no row hashes to must not validate")
}

// Regenerating a token replaces the row, so the previous URL stops being exempt
// the moment its owner rotates it.
func (s *CalendarTokenValidatorSuite) TestRotatedTokenStopsValidating() {
	user := s.createUser("rotator@test.com", true)
	first, err := s.svc.CreateToken(user.ID, "https://api.test")
	s.Require().NoError(err)
	second, err := s.svc.CreateToken(user.ID, "https://api.test")
	s.Require().NoError(err)

	validate := CalendarTokenValidator(s.svc)
	s.False(validate(first.Token), "the replaced token must stop validating")
	s.True(validate(second.Token), "the current token must validate")
}

func (s *CalendarTokenValidatorSuite) TestDeletedTokenDoesNotValidate() {
	user := s.createUser("deleter@test.com", true)
	created, err := s.svc.CreateToken(user.ID, "https://api.test")
	s.Require().NoError(err)
	s.Require().NoError(s.svc.DeleteToken(user.ID))

	validate := CalendarTokenValidator(s.svc)
	s.False(validate(created.Token), "a deleted feed token must not validate")
}

// A deactivated account's feed is not exempt: the same rule the feed handler
// itself applies, resolved through the same service rather than restated.
func (s *CalendarTokenValidatorSuite) TestInactiveUserTokenDoesNotValidate() {
	user := s.createUser("inactive-owner@test.com", true)
	created, err := s.svc.CreateToken(user.ID, "https://api.test")
	s.Require().NoError(err)
	s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", user.ID).
		Update("is_active", false).Error)

	validate := CalendarTokenValidator(s.svc)
	s.False(validate(created.Token), "an inactive owner's feed token must not validate")
}

// A nil service yields a nil predicate, which the exemption reads as "no usable
// token" and meters every feed request.
func (s *CalendarTokenValidatorSuite) TestNilServiceYieldsNilPredicate() {
	s.Nil(CalendarTokenValidator(nil))
}
