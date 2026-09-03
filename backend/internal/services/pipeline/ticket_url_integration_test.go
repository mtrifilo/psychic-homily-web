package pipeline

import (
	"strings"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// =============================================================================
// Ingest writes the vendor URL to its column, never to the description
// =============================================================================

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_WritesTicketURLToItsColumn() {
	event := suite.makeEvent("evt-ticket-001", "Wednesday", "valley-bar", "2026-06-15", []string{"Wednesday"})
	event.TicketURL = strPtr("https://www.ticketweb.com/event/wednesday-1")

	result, err := suite.svc.ImportEvents([]contracts.DiscoveredEvent{event}, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Imported)

	var show catalogm.Show
	suite.Require().NoError(suite.db.Where("source_event_id = ?", "evt-ticket-001").First(&show).Error)
	suite.Require().NotNil(show.TicketURL)
	suite.Equal("https://www.ticketweb.com/event/wednesday-1", *show.TicketURL)
	suite.Nil(show.Description)
}

// The description is the JSON-LD `description` verbatim, so a URL kept there
// would publish a vendor reference no render-side gate can withhold.
func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_DescriptionCarriesNoVendorURL() {
	event := suite.makeEvent("evt-ticket-002", "Hotline TNT", "valley-bar", "2026-06-16", []string{"Hotline TNT"})
	event.DoorsTime = strPtr("doors at 7ish")
	event.TicketURL = strPtr("https://dice.fm/event/hotline-2")

	_, err := suite.svc.ImportEvents([]contracts.DiscoveredEvent{event}, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)

	var show catalogm.Show
	suite.Require().NoError(suite.db.Where("source_event_id = ?", "evt-ticket-002").First(&show).Error)
	suite.Require().NotNil(show.Description)
	suite.Equal("Doors: doors at 7ish", *show.Description)
	suite.NotContains(*show.Description, "dice.fm")
	suite.Require().NotNil(show.TicketURL)
	suite.Equal("https://dice.fm/event/hotline-2", *show.TicketURL)
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_UpdateWritesANewTicketURL() {
	event := suite.makeEvent("evt-ticket-003", "Wombo", "valley-bar", "2026-06-17", []string{"Wombo"})
	event.TicketURL = strPtr("https://dice.fm/event/wombo-old")

	_, err := suite.svc.ImportEvents([]contracts.DiscoveredEvent{event}, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)

	event.TicketURL = strPtr("https://dice.fm/event/wombo-new")
	_, err = suite.svc.ImportEvents([]contracts.DiscoveredEvent{event}, false, true, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)

	var show catalogm.Show
	suite.Require().NoError(suite.db.Where("source_event_id = ?", "evt-ticket-003").First(&show).Error)
	suite.Require().NotNil(show.TicketURL)
	suite.Equal("https://dice.fm/event/wombo-new", *show.TicketURL)
}

// The re-scrape absence rule price uses, applied to the ticket URL: a listing
// that stops naming a ticket link has not said the show stopped selling.
func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_UpdateKeepsATicketURLTheRescrapeStopsStating() {
	event := suite.makeEvent("evt-ticket-004", "Cindy Lee", "valley-bar", "2026-06-18", []string{"Cindy Lee"})
	event.TicketURL = strPtr("https://dice.fm/event/cindy-1")

	_, err := suite.svc.ImportEvents([]contracts.DiscoveredEvent{event}, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)

	event.TicketURL = nil
	_, err = suite.svc.ImportEvents([]contracts.DiscoveredEvent{event}, false, true, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)

	var show catalogm.Show
	suite.Require().NoError(suite.db.Where("source_event_id = ?", "evt-ticket-004").First(&show).Error)
	suite.Require().NotNil(show.TicketURL)
	suite.Equal("https://dice.fm/event/cindy-1", *show.TicketURL)
}

// =============================================================================
// CleanupTicketDescriptions
// =============================================================================

func (suite *DiscoveryIntegrationTestSuite) seedLegacyShow(title, description string, ticketURL *string) *catalogm.Show {
	show := &catalogm.Show{
		Title:       title,
		EventDate:   time.Date(2026, 6, 20, 3, 0, 0, 0, time.UTC),
		Description: &description,
		TicketURL:   ticketURL,
		Status:      catalogm.ShowStatusApproved,
		Source:      catalogm.ShowSourceDiscovery,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	return show
}

func (suite *DiscoveryIntegrationTestSuite) TestCleanupTicketDescriptions_DryRunWritesNothing() {
	seeded := suite.seedLegacyShow("Legacy One", "Doors: 7ish | Tickets: https://dice.fm/event/legacy-1", nil)

	report, err := CleanupTicketDescriptions(suite.db, TicketDescriptionCleanupOptions{DryRun: true, Verbose: true})
	suite.Require().NoError(err)
	suite.Equal(1, report.Scanned)
	suite.Equal(1, report.Stripped)
	suite.Equal(1, report.MovedToColumn)
	suite.Require().Len(report.Rows, 1)
	suite.Equal(seeded.ID, report.Rows[0].ShowID)
	suite.Equal("discovery=1", report.SourceBreakdown())

	var stored catalogm.Show
	suite.Require().NoError(suite.db.First(&stored, seeded.ID).Error)
	suite.Require().NotNil(stored.Description)
	suite.Equal("Doors: 7ish | Tickets: https://dice.fm/event/legacy-1", *stored.Description)
	suite.Nil(stored.TicketURL)
}

func (suite *DiscoveryIntegrationTestSuite) TestCleanupTicketDescriptions_MovesAndStrips() {
	moved := suite.seedLegacyShow("Legacy Two", "Doors: 7ish | Tickets: https://dice.fm/event/legacy-2", nil)
	onlyLine := suite.seedLegacyShow("Legacy Three", "Tickets: https://dice.fm/event/legacy-3", nil)
	alreadySet := suite.seedLegacyShow(
		"Legacy Four",
		"Tickets: https://dice.fm/event/legacy-4-old",
		strPtr("https://dice.fm/event/legacy-4-current"),
	)

	report, err := CleanupTicketDescriptions(suite.db, TicketDescriptionCleanupOptions{})
	suite.Require().NoError(err)
	suite.Equal(3, report.Scanned)
	suite.Equal(3, report.Stripped)
	suite.Equal(2, report.MovedToColumn)

	var withDoors catalogm.Show
	suite.Require().NoError(suite.db.First(&withDoors, moved.ID).Error)
	suite.Require().NotNil(withDoors.Description)
	suite.Equal("Doors: 7ish", *withDoors.Description)
	suite.Require().NotNil(withDoors.TicketURL)
	suite.Equal("https://dice.fm/event/legacy-2", *withDoors.TicketURL)

	var cleared catalogm.Show
	suite.Require().NoError(suite.db.First(&cleared, onlyLine.ID).Error)
	suite.Nil(cleared.Description)
	suite.Require().NotNil(cleared.TicketURL)
	suite.Equal("https://dice.fm/event/legacy-3", *cleared.TicketURL)

	// A populated column outranks the older description line.
	var kept catalogm.Show
	suite.Require().NoError(suite.db.First(&kept, alreadySet.ID).Error)
	suite.Nil(kept.Description)
	suite.Require().NotNil(kept.TicketURL)
	suite.Equal("https://dice.fm/event/legacy-4-current", *kept.TicketURL)
}

func (suite *DiscoveryIntegrationTestSuite) TestCleanupTicketDescriptions_IsIdempotent() {
	suite.seedLegacyShow("Legacy Five", "Doors: 7ish | Tickets: https://dice.fm/event/legacy-5", nil)

	first, err := CleanupTicketDescriptions(suite.db, TicketDescriptionCleanupOptions{})
	suite.Require().NoError(err)
	suite.Equal(1, first.Stripped)

	second, err := CleanupTicketDescriptions(suite.db, TicketDescriptionCleanupOptions{})
	suite.Require().NoError(err)
	suite.Equal(0, second.Scanned)
	suite.Equal(0, second.Stripped)
}

func (suite *DiscoveryIntegrationTestSuite) TestCleanupTicketDescriptions_LeavesProseAlone() {
	prose := suite.seedLegacyShow("Legacy Six", "Tickets: at the door only", nil)

	report, err := CleanupTicketDescriptions(suite.db, TicketDescriptionCleanupOptions{})
	suite.Require().NoError(err)
	suite.Equal(1, report.Scanned)
	suite.Equal(0, report.Stripped)
	suite.Equal(1, report.SkippedNonURL)

	var stored catalogm.Show
	suite.Require().NoError(suite.db.First(&stored, prose.ID).Error)
	suite.Require().NotNil(stored.Description)
	suite.Equal("Tickets: at the door only", *stored.Description)
}

// A URL wider than the column with nowhere else to live stays in the
// description: stripping it would destroy the only copy.
func (suite *DiscoveryIntegrationTestSuite) TestCleanupTicketDescriptions_LeavesAnUnstorableURLInPlace() {
	oversize := "https://a.example/" + strings.Repeat("x", utils.MaxTicketURLLen)
	seeded := suite.seedLegacyShow("Legacy Seven", "Tickets: "+oversize, nil)

	report, err := CleanupTicketDescriptions(suite.db, TicketDescriptionCleanupOptions{})
	suite.Require().NoError(err)
	suite.Equal(1, report.Scanned)
	suite.Equal(0, report.Stripped)
	suite.Equal(1, report.SkippedOversizeURL)

	var stored catalogm.Show
	suite.Require().NoError(suite.db.First(&stored, seeded.ID).Error)
	suite.Require().NotNil(stored.Description)
	suite.Equal("Tickets: "+oversize, *stored.Description)
	suite.Nil(stored.TicketURL)
}
