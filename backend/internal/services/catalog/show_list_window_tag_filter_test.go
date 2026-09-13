package catalog

import (
	"fmt"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// The month histogram under a TAG filter, which is the one filter that reaches a
// show transitively through its bill rather than through a column on the row.
//
// It lives in the tag-filter suite rather than beside the other calendar tests
// because this suite already seeds the tag set and truncates entity_tags between
// tests; the invariant under assertion is the same one the sibling file states,
// that the strip's bars sum to the list they label.

// seedShowWithTaggedArtist creates one approved upcoming show at monthsOut whose
// single billed artist carries `slug`.
func (s *TagFilterIntegrationTestSuite) seedShowWithTaggedArtist(name, slug string, monthsOut int) uint {
	artistSlug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	artist := &catalogm.Artist{Name: name, Slug: &artistSlug}
	s.Require().NoError(s.db.Create(artist).Error)

	zone := "America/Phoenix"
	timezone := zone
	venue := &catalogm.Venue{Name: fmt.Sprintf("V-%s", artistSlug), City: "Phoenix", State: "AZ", Timezone: &timezone}
	s.Require().NoError(s.db.Create(venue).Error)

	loc, err := time.LoadLocation(zone)
	s.Require().NoError(err)
	local := time.Now().In(loc).AddDate(0, monthsOut, 0)
	// The 14th at 20:00 venue-local: comfortably inside its month in every zone,
	// so the fixture pins the tag filter rather than a boundary.
	at := time.Date(local.Year(), local.Month(), 14, 20, 0, 0, 0, loc)
	if !at.After(time.Now()) {
		at = at.AddDate(0, 1, 0)
	}

	city, state := "Phoenix", "AZ"
	show := &catalogm.Show{
		Title:       fmt.Sprintf("Show-%s", artistSlug),
		EventDate:   at,
		City:        &city,
		State:       &state,
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &s.user.ID,
	}
	s.Require().NoError(s.db.Create(show).Error)
	s.Require().NoError(s.db.Create(&catalogm.ShowArtist{ShowID: show.ID, ArtistID: artist.ID, Position: 0}).Error)
	s.Require().NoError(s.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID}).Error)

	s.tag("artist", artist.ID, slug)
	return show.ID
}

// tag_match=any is the mode the site sends, and it is the one that can
// double-count: a bill matching two of the requested tags must still be one row
// in the list and one unit in its month's bar.
func (s *TagFilterIntegrationTestSuite) TestUpcomingShowMonths_SumsToTheListTotalUnderTagMatchAny() {
	s.seedShowWithTaggedArtist("Postpunk One", "post-punk", 1)
	s.seedShowWithTaggedArtist("Shoegaze One", "shoegaze", 2)
	s.seedShowWithTaggedArtist("Electronic One", "electronic", 3)

	// A bill carrying BOTH requested tags, which an OR filter joining rather
	// than sub-querying would count twice.
	bothID := s.seedShowWithTaggedArtist("Both Tags", "post-punk", 4)
	var showArtist catalogm.ShowArtist
	s.Require().NoError(s.db.Where("show_id = ?", bothID).First(&showArtist).Error)
	s.tag("artist", showArtist.ArtistID, "shoegaze")

	filters := &contracts.UpcomingShowsFilter{
		TagSlugs:    []string{"post-punk", "shoegaze"},
		TagMatchAny: true,
	}

	months, err := s.showService.GetUpcomingShowMonths(false, filters)
	s.Require().NoError(err)

	var sum int64
	for _, bucket := range months {
		sum += bucket.Count
	}

	_, total, err := s.showService.GetUpcomingShowsPage(
		contracts.ShowCalendarQuery{Limit: 50}, false, filters)
	s.Require().NoError(err)

	s.Require().Equal(int64(3), total, "post-punk OR shoegaze must match three shows, the double-tagged bill once")
	s.Require().Equal(total, sum)

	// And each bar equals the window it names, under the same filter.
	for _, bucket := range months {
		_, windowTotal, err := s.showService.GetUpcomingShowsPage(contracts.ShowCalendarQuery{
			ShowCalendarWindow: contracts.ShowCalendarWindow{Year: bucket.Year, Month: bucket.Month},
			Limit:              50,
		}, false, filters)
		s.Require().NoError(err)
		s.Require().Equal(bucket.Count, windowTotal, "bar %d-%02d disagrees with its own window", bucket.Year, bucket.Month)
	}
}
