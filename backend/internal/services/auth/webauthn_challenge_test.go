package auth

import (
	"sync"

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
		suite.ErrorIs(err, errChallengeNotFound)
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
					if operation == "registration" {
						_, err := suite.service.ConsumeRegistrationChallenge(challengeID, user.ID)
						return err
					}
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

// A consumed challenge's row is gone and a second consume answers not found.
func (suite *WebAuthnServiceIntegrationTestSuite) TestConsumedChallengeAnswersNotFound() {
	user := suite.createUser("spent@test.com")
	challengeID, err := suite.service.StoreChallenge(user.ID, &webauthn.SessionData{Challenge: "spent"}, "authentication")
	suite.Require().NoError(err)

	_, _, err = suite.service.ConsumeChallenge(challengeID, "authentication")
	suite.Require().NoError(err)
	suite.Zero(suite.challengeRows(challengeID))

	_, _, err = suite.service.ConsumeChallenge(challengeID, "authentication")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "challenge not found")
}

func (suite *WebAuthnServiceIntegrationTestSuite) challengeRows(challengeID string) int64 {
	var count int64
	suite.Require().NoError(suite.db.Model(&authm.WebAuthnChallenge{}).Where("id = ?", challengeID).Count(&count).Error)
	return count
}
