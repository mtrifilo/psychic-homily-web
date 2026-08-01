package engagement

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"

	ics "github.com/arran4/golang-ical"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"
)

// A PUBLIC single-show iCalendar download — the one-shot "Add to calendar"
// export for one event, as opposed to the venue feed's live subscription.
//
// It lives next to venue_calendar.go so the two surfaces share event identity
// (showEventUID), revisioning (showSequence), naming (applyEventSummaryAndStatus),
// venue-local time anchoring (setVenueLocalEventTimes) and text sanitization
// (sanitizeICSText). The same show exported here and reached through the venue
// feed MUST be the same event to a calendar client, or an attendee who used
// both ends up with duplicates.
//
// Deliberately NOT shared with the venue feed: caching. The feed is polled by
// calendar clients on a schedule; this document is fetched once per human
// click. ETag revalidation is kept (the bytes are deterministic for identical
// DB state) but an in-process cache would be dead weight.

// showEventCalendarProductID identifies this generator per RFC 5545 3.7.3.
const showEventCalendarProductID = "-//Psychic Homily//Show Calendar//EN"

// ShowCalendarService renders a single show as a downloadable VEVENT.
//
// It reads the show through ShowServiceInterface rather than the shows table
// directly: buildShowResponse is where unverified-venue address redaction
// lives, and this surface must inherit it — a calendar file is copied onto
// the attendee's device, which is a worse place to leak a house address than
// a web page.
type ShowCalendarService struct {
	showSvc contracts.ShowServiceInterface
}

// NewShowCalendarService creates the public per-show calendar service.
func NewShowCalendarService(showSvc contracts.ShowServiceInterface) *ShowCalendarService {
	return &ShowCalendarService{showSvc: showSvc}
}

// GenerateShowEvent renders the show identified by idOrSlug as a single-VEVENT
// RFC 5545 document.
func (s *ShowCalendarService) GenerateShowEvent(idOrSlug string, frontendURL string) (*contracts.ShowCalendarEvent, error) {
	if s.showSvc == nil {
		return nil, fmt.Errorf("show service not initialized")
	}

	show, err := s.resolveShow(idOrSlug)
	if err != nil {
		return nil, err
	}

	// This endpoint is anonymous-only. The public show handler gates
	// non-approved shows behind submitter/admin identity; this surface has no
	// identity, so anything not approved is served exactly like a show that
	// does not exist — a distinguishable error would leak that a hidden show
	// ID is real.
	if show.Status != "approved" {
		return nil, apperrors.ErrShowNotFound(show.ID)
	}

	payload := buildShowEventCalendar(show, frontendURL)

	sum := sha256.Sum256(payload)
	return &contracts.ShowCalendarEvent{
		ShowSlug: show.Slug,
		ICS:      payload,
		ETag:     `W/"` + hex.EncodeToString(sum[:16]) + `"`,
	}, nil
}

// resolveShow accepts either a numeric ID or a slug, matching the {show_id}
// convention every other show sub-resource uses.
func (s *ShowCalendarService) resolveShow(idOrSlug string) (*contracts.ShowResponse, error) {
	if id, parseErr := strconv.ParseUint(idOrSlug, 10, 32); parseErr == nil {
		return s.showSvc.GetShow(uint(id))
	}
	return s.showSvc.GetShowBySlug(idOrSlug)
}

// buildShowEventCalendar renders the single-VEVENT calendar. Every value that
// originates in community-editable data passes through sanitizeICSText — see
// its doc comment for why that is a correctness requirement.
func buildShowEventCalendar(show *contracts.ShowResponse, frontendURL string) []byte {
	cal := ics.NewCalendar()
	cal.SetMethod(ics.MethodPublish)
	cal.SetProductId(showEventCalendarProductID)

	event := cal.AddEvent(showEventUID(show.ID))

	// DTSTAMP / SEQUENCE discipline as in the venue feed's buildCalendar,
	// which documents the reasons in full.
	event.SetDtStampTime(show.UpdatedAt.UTC())
	event.SetCreatedTime(show.CreatedAt)
	event.SetModifiedAt(show.UpdatedAt)
	event.SetSequence(showSequence(show.CreatedAt, show.UpdatedAt))

	artistNames := make([]string, 0, len(show.Artists))
	for _, artist := range show.Artists {
		artistNames = append(artistNames, artist.Name)
	}

	venueName := ""
	location := ""
	if len(show.Venues) > 0 {
		venue := &show.Venues[0]
		venueName = venue.Name
		location = sanitizeICSText(formatEventLocation(venue.Name, venue.Address, venue.City, venue.State))
		setVenueLocalEventTimes(event, show.EventDate, defaultShowDuration, venue.Timezone, venue.State)
	} else {
		setVenueLocalEventTimes(event, show.EventDate, defaultShowDuration, nil, "")
	}

	applyEventSummaryAndStatus(event, show.Title, artistNames, venueName, show.IsCancelled, show.IsSoldOut)

	if location != "" {
		event.SetLocation(location)
	}

	showURL := showPageURL(frontendURL, show.Slug, show.ID)
	event.SetDescription(buildEventDescription(
		location, artistNames, show.Price, show.AgeRequirement, show.IsCancelled, showURL))
	if showURL != "" {
		event.SetURL(showURL)
	}

	// RFC 5545 3.1 mandates CRLF; golang-ical defaults to bare LF.
	return []byte(cal.Serialize(ics.WithNewLine("\r\n")))
}
