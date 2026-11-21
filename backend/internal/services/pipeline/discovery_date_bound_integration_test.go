package pipeline

import (
	"strings"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// A scraped event dated past the far-future bound is not imported; one inside
// it is.
func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_RefusesAnEventPastTheDateBound() {
	far := suite.makeEvent("evt-bound-far", "Testing Ace", "valley-bar", "2083-08-15", []string{"Testing Ace"})
	near := suite.makeEvent("evt-bound-near", "Wednesday", "valley-bar",
		time.Now().AddDate(0, 17, 0).Format("2006-01-02"), []string{"Wednesday"})

	result, err := suite.svc.ImportEvents([]contracts.DiscoveredEvent{far, near}, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Imported)
	suite.Equal(1, result.Rejected)
	suite.Equal(0, result.Errors, "a refusal is not a failed import")
	suite.True(strings.Contains(strings.Join(result.Messages, "\n"), "Shows can be listed up to 18 months ahead"), "messages: %v", result.Messages)

	var n int64
	suite.Require().NoError(suite.db.Model(&catalogm.Show{}).Where("source_event_id = ?", "evt-bound-far").Count(&n).Error)
	suite.Equal(int64(0), n)
}
