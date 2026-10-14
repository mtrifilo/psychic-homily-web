package auth

import (
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// raceRounds is how many times each concurrency test stores a challenge and
// races two finishes for it.
const raceRounds = 10

// raceConsume runs consume twice at once, both released together, and
// returns the two results.
func raceConsume(consume func() error) [2]error {
	var results [2]error
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i] = consume()
		}()
	}
	close(start)
	wg.Wait()
	return results
}

// requireOneWinner asserts that exactly one of two racing finishes received
// the challenge and the other was told it does not exist.
func (suite *WebAuthnServiceIntegrationTestSuite) requireOneWinner(results [2]error) {
	succeeded := 0
	for _, err := range results {
		if err == nil {
			succeeded++
			continue
		}
		suite.Contains(err.Error(), "challenge not found")
	}
	suite.Equal(1, succeeded, "exactly one finish receives the challenge")
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestConcurrentFinishesConsumeAChallengeOnce() {
	user := suite.createUser("race@test.com")
	for _, operation := range []string{"registration", "authentication"} {
		suite.Run(operation, func() {
			for range raceRounds {
				challengeID, err := suite.service.StoreChallenge(user.ID, &webauthn.SessionData{Challenge: "race"}, operation)
				suite.Require().NoError(err)

				suite.requireOneWinner(raceConsume(func() error {
					_, _, err := suite.service.ConsumeChallenge(challengeID, operation)
					return err
				}))
			}
		})
	}
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestConcurrentSignupFinishesConsumeAChallengeOnce() {
	for range raceRounds {
		challengeID, err := suite.service.StoreSignupChallenge(
			contracts.SignupChallenge{Email: "race@test.com"}, &webauthn.SessionData{Challenge: "race"}, "signup",
		)
		suite.Require().NoError(err)

		suite.requireOneWinner(raceConsume(func() error {
			_, _, err := suite.service.ConsumeSignupChallenge(challengeID, "signup")
			return err
		}))
	}
}

// A finish that received the challenge spends it whatever its ceremony then
// answers, so a retry with the same id needs a new begin.
func (suite *WebAuthnServiceIntegrationTestSuite) TestConsumedChallengeAnswersNotFound() {
	user := suite.createUser("spent@test.com")
	challengeID, err := suite.service.StoreChallenge(user.ID, &webauthn.SessionData{Challenge: "spent"}, "authentication")
	suite.Require().NoError(err)

	_, _, err = suite.service.ConsumeChallenge(challengeID, "authentication")
	suite.Require().NoError(err)

	_, _, err = suite.service.ConsumeChallenge(challengeID, "authentication")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "challenge not found")
	suite.Zero(suite.challengeRows(challengeID))
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestConsumedSignupChallengeAnswersNotFound() {
	challengeID, err := suite.service.StoreSignupChallenge(
		contracts.SignupChallenge{Email: "spent@test.com"}, &webauthn.SessionData{Challenge: "spent"}, "signup",
	)
	suite.Require().NoError(err)

	_, _, err = suite.service.ConsumeSignupChallenge(challengeID, "signup")
	suite.Require().NoError(err)

	_, _, err = suite.service.ConsumeSignupChallenge(challengeID, "signup")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "challenge not found")
}

// An expired challenge is refused and removed by the same call.
func (suite *WebAuthnServiceIntegrationTestSuite) TestExpiredChallengeIsRemovedWhenRefused() {
	user := suite.createUser("expired@test.com")
	challengeID, err := suite.service.StoreChallenge(user.ID, &webauthn.SessionData{Challenge: "expired"}, "authentication")
	suite.Require().NoError(err)
	suite.Require().NoError(suite.db.Exec("UPDATE webauthn_challenges SET expires_at = ? WHERE id = ?",
		time.Now().Add(-time.Minute), challengeID).Error)

	_, _, err = suite.service.ConsumeChallenge(challengeID, "authentication")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "challenge expired")
	suite.Zero(suite.challengeRows(challengeID))
}

// A challenge asked for under another operation is neither returned nor
// spent.
func (suite *WebAuthnServiceIntegrationTestSuite) TestWrongOperationLeavesChallengeInPlace() {
	user := suite.createUser("operation@test.com")
	challengeID, err := suite.service.StoreChallenge(user.ID, &webauthn.SessionData{Challenge: "op"}, "registration")
	suite.Require().NoError(err)

	_, _, err = suite.service.ConsumeChallenge(challengeID, "authentication")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "challenge not found")
	_, _, err = suite.service.ConsumeSignupChallenge(challengeID, "signup")
	suite.Require().Error(err)

	_, userID, err := suite.service.ConsumeChallenge(challengeID, "registration")
	suite.Require().NoError(err)
	suite.Equal(user.ID, userID)
}

func (suite *WebAuthnServiceIntegrationTestSuite) challengeRows(challengeID string) int64 {
	var count int64
	suite.Require().NoError(suite.db.Model(&authm.WebAuthnChallenge{}).Where("id = ?", challengeID).Count(&count).Error)
	return count
}
