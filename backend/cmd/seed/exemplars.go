package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"

	"gorm.io/gorm"
)

// Rich exemplar seed (PSY-665).
//
// The minimum-viable dev seed (cmd/seed/main.go) leaves most optional fields
// NULL/empty, so rich-data render paths (About sections, Listen/Buy grids,
// social-link rows, tag clouds, multi-day festival lineups) are untestable
// locally and agents can't visually verify them during repro. This file adds
// ONE exemplar per entity type with EVERY optional field populated, so the
// next agent's screenshot work exercises every rich path.
//
// Design rules:
//   - Additive: every exemplar uses a NEW, fixed slug suffixed "-exemplar"
//     so it never collides with the YAML/Go-hardcoded dev entities and the
//     existing minimal seed (and its E2E sibling fixtures) are untouched.
//   - Idempotent: each create is guarded by a slug existence check, so
//     re-running the seed neither duplicates rows nor breaks referential
//     integrity.
//   - Canaries preserved: rich exemplars do NOT backfill the empty-state
//     entities. seedEdgeCaseFestival keeps an explicit social: {} festival
//     (the PSY-657 truthy-empty-object canary) alongside the rich one.
//   - Image fields use LOCAL committed placeholders under
//     frontend/public/seed-placeholders/ (rendered via plain <img>, not
//     next/image, so no remote-host allowlist applies). Stable + offline-safe
//     per the PSY-665 decision; entity NAMES stay realistic per the ticket.
//
// Slugs are documented in backend/db/seeds/README.md for the screenshot pass.

// exemplar slug constants — the single source of truth for the README table
// and the idempotency guards below.
const (
	exemplarArtistSlug     = "marissa-nadler-exemplar"
	exemplarVenueSlug      = "the-rhythm-room-exemplar-phoenix-az"
	exemplarReleaseSlug    = "the-path-of-the-clouds-exemplar"
	exemplarLabelSlug      = "sacred-bones-records-exemplar"
	exemplarFestivalSlug   = "marfa-myths-exemplar-2026"
	edgeCaseFestivalSlug   = "desert-daze-exemplar-2026" // PSY-657 social:{} canary
	exemplarShowSlug       = "the-path-tour-exemplar-at-the-rhythm-room-exemplar"
	exemplarCollectionSlug = "psychic-homily-staff-picks-exemplar"
)

// strptr is a tiny helper to take the address of a string literal inline.
func strptr(s string) *string { return &s }

// intptr is a tiny helper to take the address of an int literal inline.
func intptr(i int) *int { return &i }

// fullSocial returns a Social struct with every platform populated, for the
// rich exemplars whose AC requires all social.{...} fields non-empty.
func fullSocial(handle string) catalogm.Social {
	return catalogm.Social{
		Instagram:  strptr("https://instagram.com/" + handle),
		Facebook:   strptr("https://facebook.com/" + handle),
		Twitter:    strptr("https://twitter.com/" + handle),
		YouTube:    strptr("https://youtube.com/@" + handle),
		Spotify:    strptr("https://open.spotify.com/artist/" + handle),
		SoundCloud: strptr("https://soundcloud.com/" + handle),
		Bandcamp:   strptr("https://" + handle + ".bandcamp.com"),
		Website:    strptr("https://" + handle + ".example.com"),
	}
}

// seedRichExemplars is the entry point invoked from main() after users exist
// (entity_tags.added_by_user_id and collections.creator_id are NOT NULL FKs to
// users). Each helper is independently idempotent.
func seedRichExemplars(db *gorm.DB) {
	fmt.Println("Seeding rich exemplars (PSY-665)...")

	// The admin test user owns every tag application and the collection. It is
	// seeded by seedTestUsers, which main() calls before this function.
	var admin authm.User
	if err := db.Where("email = ?", "admin@test.local").First(&admin).Error; err != nil {
		log.Printf("Warning: admin user not found; skipping rich exemplars: %v", err)
		return
	}

	venueID := seedExemplarVenue(db, admin.ID)
	labelID := seedExemplarLabel(db, admin.ID)
	artistID := seedExemplarArtist(db, admin.ID, labelID)
	seedExemplarRelease(db, admin.ID, artistID, labelID)
	seedExemplarFestival(db, admin.ID, artistID, venueID)
	seedEdgeCaseFestival(db) // PSY-657 social:{} canary
	showID := seedExemplarShow(db, admin.ID, artistID, venueID)
	seedExemplarArtistShows(db, artistID, venueID)
	seedExemplarSimilarArtists(db, artistID)
	seedExemplarCollection(db, admin.ID, artistID, venueID, labelID, showID)

	fmt.Println("✅ Rich exemplars seeded (or already present)")
}

// applyTags creates entity_tags rows (and any missing tags) for an entity,
// mirroring TagService.AddTagToEntity's usage_count bump. Tags are created
// once and reused across entities so the tag graph is shared, matching how
// real tagging works. Idempotent via the (tag_id, entity_type, entity_id)
// unique index.
func applyTags(db *gorm.DB, entityType string, entityID, userID uint, tags []struct{ Name, Slug, Category string }) {
	for _, t := range tags {
		var tag catalogm.Tag
		err := db.Where("slug = ?", t.Slug).First(&tag).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			tag = catalogm.Tag{
				Name:            t.Name,
				Slug:            t.Slug,
				Category:        t.Category,
				IsOfficial:      true,
				CreatedByUserID: &userID,
			}
			if err := db.Create(&tag).Error; err != nil {
				log.Printf("Warning: failed to create tag %s: %v", t.Slug, err)
				continue
			}
		} else if err != nil {
			log.Printf("Warning: failed to look up tag %s: %v", t.Slug, err)
			continue
		}

		et := catalogm.EntityTag{
			TagID:         tag.ID,
			EntityType:    entityType,
			EntityID:      entityID,
			AddedByUserID: userID,
		}
		res := db.Where("tag_id = ? AND entity_type = ? AND entity_id = ?", tag.ID, entityType, entityID).
			FirstOrCreate(&et)
		if res.Error != nil {
			log.Printf("Warning: failed to tag %s/%d with %s: %v", entityType, entityID, t.Slug, res.Error)
			continue
		}
		// Only bump usage_count when a new entity_tag row was actually created,
		// so re-running the seed doesn't inflate the count.
		if res.RowsAffected == 1 {
			db.Model(&catalogm.Tag{}).Where("id = ?", tag.ID).
				Update("usage_count", gorm.Expr("usage_count + 1"))
		}
	}
}

// seedExemplarVenue creates a venue with image_url, full social, and 5+ tags.
// Upcoming + past shows are attached by seedExemplarArtistShows + the festival.
// Returns the venue ID for downstream linking.
func seedExemplarVenue(db *gorm.DB, userID uint) uint {
	var existing catalogm.Venue
	if db.Where("slug = ?", exemplarVenueSlug).First(&existing).Error == nil {
		return existing.ID
	}

	venue := &catalogm.Venue{
		Name:        "The Rhythm Room (Exemplar)",
		Slug:        strptr(exemplarVenueSlug),
		Address:     strptr("1019 E Indian School Rd"),
		City:        "Phoenix",
		State:       "AZ",
		Country:     strptr("USA"),
		Zipcode:     strptr("85014"),
		Description: strptr("A long-running Phoenix blues and roots room that also hosts touring indie and experimental acts. Intimate capacity, full bar, and one of the better-sounding small stages in the Valley.\n\nSeeded as the PSY-665 rich venue exemplar: every optional field is populated so the venue detail page exercises the image header, social-link row, tag cloud, and the upcoming/past show split."),
		ImageURL:    strptr("/seed-placeholders/venue.svg"),
		Social:      fullSocial("rhythmroomphx"),
		Verified:    true,
	}
	if err := db.Create(venue).Error; err != nil {
		log.Printf("Warning: failed to create exemplar venue: %v", err)
		return 0
	}

	applyTags(db, catalogm.TagEntityVenue, venue.ID, userID, []struct{ Name, Slug, Category string }{
		{"Blues", "blues", catalogm.TagCategoryGenre},
		{"Roots", "roots", catalogm.TagCategoryGenre},
		{"All Ages Sometimes", "all-ages-sometimes", catalogm.TagCategoryOther},
		{"Phoenix", "phoenix", catalogm.TagCategoryLocale},
		{"Full Bar", "full-bar", catalogm.TagCategoryOther},
		{"Historic Venue", "historic-venue", catalogm.TagCategoryOther},
	})

	fmt.Printf("  ✅ venue exemplar: %s\n", exemplarVenueSlug)
	return venue.ID
}

// seedExemplarLabel creates a label with description, image_url, full social,
// founded_year, city/state/country, and 5+ tags. Associated artists + releases
// are linked by the artist/release helpers. Returns the label ID.
func seedExemplarLabel(db *gorm.DB, userID uint) uint {
	var existing catalogm.Label
	if db.Where("slug = ?", exemplarLabelSlug).First(&existing).Error == nil {
		return existing.ID
	}

	label := &catalogm.Label{
		Name:        "Sacred Bones Records (Exemplar)",
		Slug:        strptr(exemplarLabelSlug),
		City:        strptr("Brooklyn"),
		State:       strptr("NY"),
		Country:     strptr("USA"),
		FoundedYear: intptr(2007),
		Status:      catalogm.LabelStatusActive,
		Description: strptr("An independent label known for dark, cinematic, and outsider music spanning post-punk, drone, folk, and horror soundtracks. Home to a tightly curated roster with a strong visual identity.\n\nSeeded as the PSY-665 rich label exemplar: description, image, all social links, founding metadata, location, tags, associated artists, and a release catalog are all populated so the label detail page renders every section."),
		ImageURL:    strptr("/seed-placeholders/label.svg"),
		Social:      fullSocial("sacredbonesrecords"),
	}
	if err := db.Create(label).Error; err != nil {
		log.Printf("Warning: failed to create exemplar label: %v", err)
		return 0
	}

	applyTags(db, catalogm.TagEntityLabel, label.ID, userID, []struct{ Name, Slug, Category string }{
		{"Post-Punk", "post-punk", catalogm.TagCategoryGenre},
		{"Drone", "drone", catalogm.TagCategoryGenre},
		{"Dark Folk", "dark-folk", catalogm.TagCategoryGenre},
		{"Brooklyn", "brooklyn", catalogm.TagCategoryLocale},
		{"Independent", "independent", catalogm.TagCategoryOther},
		{"Soundtracks", "soundtracks", catalogm.TagCategoryGenre},
	})

	// 3+ associated artists for the label page roster. Reuse artists already
	// present in the dev seed (data/bands.yaml) so the label links to real,
	// browsable artist pages rather than orphan rows.
	for _, name := range []string{"Marissa Nadler", "Chat Pile", "Mount Eerie"} {
		artist, err := findOrCreateArtist(db, name)
		if err != nil {
			log.Printf("Warning: %v", err)
			continue
		}
		al := catalogm.ArtistLabel{ArtistID: artist.ID, LabelID: label.ID}
		db.Where("artist_id = ? AND label_id = ?", artist.ID, label.ID).FirstOrCreate(&al)
	}

	fmt.Printf("  ✅ label exemplar: %s\n", exemplarLabelSlug)
	return label.ID
}

// seedExemplarArtist creates an artist with bio, image_url, full social, 5+
// tags, 2+ aliases, and a label link. Releases, shows, festival appearance,
// and similar-artist edges are attached by sibling helpers. Returns artist ID.
func seedExemplarArtist(db *gorm.DB, userID, labelID uint) uint {
	var existing catalogm.Artist
	if db.Where("slug = ?", exemplarArtistSlug).First(&existing).Error == nil {
		return existing.ID
	}

	artist := &catalogm.Artist{
		Name:        "Marissa Nadler (Exemplar)",
		Slug:        strptr(exemplarArtistSlug),
		City:        strptr("Boston"),
		State:       strptr("MA"),
		Country:     strptr("USA"),
		Description: strptr("A singer-songwriter and guitarist whose gothic, dream-folk songwriting pairs fingerpicked guitar with reverb-soaked vocals. Across a long discography she has moved between hushed solo records and fuller, collaborative productions.\n\nSeeded as the PSY-665 rich artist exemplar: bio, image, all social links, multiple aliases, tags across categories, releases, label links, tracked shows, similar artists, and a festival appearance are all populated."),
		ImageURL:    strptr("/seed-placeholders/artist.svg"),
		Social:      fullSocial("marissanadler"),
	}
	if err := db.Create(artist).Error; err != nil {
		log.Printf("Warning: failed to create exemplar artist: %v", err)
		return 0
	}

	// 2+ aliases.
	for _, alias := range []string{"M. Nadler", "Marissa Elizabeth Nadler"} {
		a := catalogm.ArtistAlias{ArtistID: artist.ID, Alias: alias}
		db.Where("artist_id = ? AND alias = ?", artist.ID, alias).FirstOrCreate(&a)
	}

	// 5+ tags across multiple categories (genre / locale / other).
	applyTags(db, catalogm.TagEntityArtist, artist.ID, userID, []struct{ Name, Slug, Category string }{
		{"Dream Folk", "dream-folk", catalogm.TagCategoryGenre},
		{"Gothic", "gothic", catalogm.TagCategoryGenre},
		{"Singer-Songwriter", "singer-songwriter", catalogm.TagCategoryGenre},
		{"Boston", "boston", catalogm.TagCategoryLocale},
		{"Fingerpicking", "fingerpicking", catalogm.TagCategoryOther},
		{"Reverb", "reverb", catalogm.TagCategoryOther},
	})

	// Link to the exemplar label (1+ label).
	if labelID != 0 {
		al := catalogm.ArtistLabel{ArtistID: artist.ID, LabelID: labelID}
		db.Where("artist_id = ? AND label_id = ?", artist.ID, labelID).FirstOrCreate(&al)
	}

	fmt.Printf("  ✅ artist exemplar: %s\n", exemplarArtistSlug)
	return artist.ID
}

// seedExemplarRelease creates a release with description (200+ chars with a
// paragraph break), cover_art_url, 4+ external links across distinct
// platforms, a label link with catalog number, 3+ artists with role
// assignments, and 5+ tags.
func seedExemplarRelease(db *gorm.DB, userID, mainArtistID, labelID uint) {
	var existing catalogm.Release
	if db.Where("slug = ?", exemplarReleaseSlug).First(&existing).Error == nil {
		return
	}
	if mainArtistID == 0 {
		log.Printf("Warning: no main artist for exemplar release; skipping")
		return
	}

	release := &catalogm.Release{
		Title:       "The Path of the Clouds (Exemplar)",
		Slug:        strptr(exemplarReleaseSlug),
		ReleaseType: catalogm.ReleaseTypeLP,
		ReleaseYear: intptr(2021),
		ReleaseDate: strptr("2021-10-29"),
		CoverArtURL: strptr("/seed-placeholders/release.svg"),
		Description: strptr("A widescreen, atmospheric record built around true-crime narratives and disappearances, with the songwriter's signature reverb-laden guitar and layered vocal harmonies pushed toward something cinematic and full-band.\n\nSeeded as the PSY-665 rich release exemplar: long description with a paragraph break, cover art, external Listen/Buy links across four platforms, a label link with a catalog number, multiple credited artists with distinct roles, and a full tag set so the release detail page renders the About section and the Listen/Buy grid."),
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(release).Error; err != nil {
			return fmt.Errorf("create release: %w", err)
		}

		// 3+ artists with role assignments: main + a featured collaborator + a
		// producer. Reuse seeded artists for the supporting roles so they link
		// to real pages.
		roles := []struct {
			Name string
			Role catalogm.ArtistReleaseRole
			Pos  int
		}{
			{"Marissa Nadler (Exemplar)", catalogm.ArtistReleaseRoleMain, 0},
			{"Mount Eerie", catalogm.ArtistReleaseRoleFeatured, 1},
			{"Bill Orcutt", catalogm.ArtistReleaseRoleProducer, 2},
		}
		for _, r := range roles {
			artist, err := findOrCreateArtist(tx, r.Name)
			if err != nil {
				return err
			}
			ar := catalogm.ArtistRelease{
				ArtistID:  artist.ID,
				ReleaseID: release.ID,
				Role:      r.Role,
				Position:  r.Pos,
			}
			if err := tx.Create(&ar).Error; err != nil {
				return fmt.Errorf("link artist %s: %w", r.Name, err)
			}
		}

		// 1+ label with catalog number.
		if labelID != 0 {
			rl := catalogm.ReleaseLabel{
				ReleaseID:     release.ID,
				LabelID:       labelID,
				CatalogNumber: strptr("SBR-EXEMPLAR-001"),
			}
			if err := tx.Create(&rl).Error; err != nil {
				return fmt.Errorf("link label: %w", err)
			}
		}

		// 4+ external links across distinct platforms.
		links := []struct{ Platform, URL string }{
			{"bandcamp", "https://marissanadler.bandcamp.com/album/the-path-of-the-clouds"},
			{"spotify", "https://open.spotify.com/album/exemplar-path-of-the-clouds"},
			{"apple_music", "https://music.apple.com/us/album/exemplar-path-of-the-clouds"},
			{"youtube_music", "https://music.youtube.com/playlist?list=EXEMPLAR-PATH-OF-CLOUDS"},
			{"discogs", "https://www.discogs.com/release/exemplar-path-of-the-clouds"},
		}
		for _, l := range links {
			el := catalogm.ReleaseExternalLink{
				ReleaseID: release.ID,
				Platform:  l.Platform,
				URL:       l.URL,
			}
			if err := tx.Create(&el).Error; err != nil {
				return fmt.Errorf("create external link %s: %w", l.Platform, err)
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("Warning: failed to create exemplar release: %v", err)
		return
	}

	// 5+ tags (applied outside the transaction; applyTags is independently
	// idempotent and a tag failure shouldn't roll back the release).
	applyTags(db, catalogm.TagEntityRelease, release.ID, userID, []struct{ Name, Slug, Category string }{
		{"Dream Folk", "dream-folk", catalogm.TagCategoryGenre},
		{"Gothic", "gothic", catalogm.TagCategoryGenre},
		{"True Crime", "true-crime", catalogm.TagCategoryOther},
		{"2021", "2021", catalogm.TagCategoryOther},
		{"Cinematic", "cinematic", catalogm.TagCategoryGenre},
		{"Boston", "boston", catalogm.TagCategoryLocale},
	})

	fmt.Printf("  ✅ release exemplar: %s\n", exemplarReleaseSlug)
}

// seedExemplarFestival creates a festival with description, flyer_url, full
// social (jsonb), website, ticket_url, 2+ venues with is_primary flags, a
// multi-day lineup (3 days, 6+ artists/day) covering all billing tiers, and
// 5+ tags.
func seedExemplarFestival(db *gorm.DB, userID, exemplarArtistID, primaryVenueID uint) {
	var existing catalogm.Festival
	if db.Where("slug = ?", exemplarFestivalSlug).First(&existing).Error == nil {
		return
	}

	social := json.RawMessage(`{"instagram":"https://instagram.com/marfamyths","facebook":"https://facebook.com/marfamyths","twitter":"https://twitter.com/marfamyths","youtube":"https://youtube.com/@marfamyths","spotify":"https://open.spotify.com/user/marfamyths","soundcloud":"https://soundcloud.com/marfamyths","bandcamp":"https://marfamyths.bandcamp.com","website":"https://marfamyths.example.com"}`)

	festival := &catalogm.Festival{
		Name:         "Marfa Myths (Exemplar) 2026",
		Slug:         exemplarFestivalSlug,
		SeriesSlug:   "marfa-myths-exemplar",
		EditionYear:  2026,
		Description:  strptr("A multi-day desert festival pairing experimental, folk, and psych acts with site-specific installations across a small West Texas town. Known for collaborative performances and an intimate, sprawling-yet-walkable footprint.\n\nSeeded as the PSY-665 rich festival exemplar: description, flyer, all social links, website, ticket URL, multiple venues with primary flags, and a three-day lineup spanning every billing tier so the festival detail page renders the full lineup grid and metadata."),
		LocationName: strptr("Downtown Marfa"),
		City:         strptr("Marfa"),
		State:        strptr("TX"),
		Country:      strptr("USA"),
		StartDate:    "2026-09-25",
		EndDate:      "2026-09-27",
		Website:      strptr("https://marfamyths.example.com"),
		TicketURL:    strptr("https://tickets.example.com/marfa-myths-exemplar-2026"),
		FlyerURL:     strptr("/seed-placeholders/festival.svg"),
		Status:       catalogm.FestivalStatusConfirmed,
		Social:       &social,
	}
	if err := db.Create(festival).Error; err != nil {
		log.Printf("Warning: failed to create exemplar festival: %v", err)
		return
	}

	// 2+ venues with is_primary flags. Primary = the exemplar venue; secondary
	// = a second seeded venue so the festival spans more than one room.
	if primaryVenueID != 0 {
		fv := catalogm.FestivalVenue{FestivalID: festival.ID, VenueID: primaryVenueID, IsPrimary: true}
		db.Where("festival_id = ? AND venue_id = ?", festival.ID, primaryVenueID).FirstOrCreate(&fv)
	}
	var secondVenue catalogm.Venue
	if db.Where("slug <> ?", exemplarVenueSlug).Order("id ASC").First(&secondVenue).Error == nil && secondVenue.ID != primaryVenueID {
		fv := catalogm.FestivalVenue{FestivalID: festival.ID, VenueID: secondVenue.ID, IsPrimary: false}
		db.Where("festival_id = ? AND venue_id = ?", festival.ID, secondVenue.ID).FirstOrCreate(&fv)
	}

	// Multi-day lineup: 3 days, 6 artists/day (18 slots), every billing tier
	// represented each day (headliner, sub_headliner, mid_card, undercard,
	// local, dj). The exemplar artist headlines day 1 (gives it its festival
	// appearance). The rest reuse seeded artists.
	tiers := []catalogm.BillingTier{
		catalogm.BillingTierHeadliner,
		catalogm.BillingTierSubHeadliner,
		catalogm.BillingTierMidCard,
		catalogm.BillingTierUndercard,
		catalogm.BillingTierLocal,
		catalogm.BillingTierDJ,
	}
	days := []string{"2026-09-25", "2026-09-26", "2026-09-27"}
	// Pool of artist names for the lineup. Day 1 slot 0 is overridden to the
	// exemplar artist below so it earns its festival appearance.
	lineupNames := []string{
		"Mount Eerie", "Chat Pile", "Bill Orcutt", "Cat Power", "Soccer Mommy", "HEALTH",
		"Mogwai", "Pixies", "Alvvays", "Carpenter Brut", "LCD Soundsystem", "Jeff Tweedy",
		"Cursive", "Pile", "Baths", "Dune Rats", "Playboy Manbaby", "Fashion Club (LA)",
	}
	idx := 0
	for dayNum, day := range days {
		for slot, tier := range tiers {
			name := lineupNames[idx%len(lineupNames)]
			idx++
			if dayNum == 0 && slot == 0 && exemplarArtistID != 0 {
				// Headliner of day 1 = the exemplar artist.
				fa := catalogm.FestivalArtist{
					FestivalID:  festival.ID,
					ArtistID:    exemplarArtistID,
					BillingTier: tier,
					Position:    slot,
					DayDate:     strptr(day),
					Stage:       strptr("Main Stage"),
				}
				db.Where("festival_id = ? AND artist_id = ? AND day_date = ?", festival.ID, exemplarArtistID, day).
					FirstOrCreate(&fa)
				continue
			}
			artist, err := findOrCreateArtist(db, name)
			if err != nil {
				log.Printf("Warning: %v", err)
				continue
			}
			stage := "Main Stage"
			if slot >= 3 {
				stage = "Side Stage"
			}
			fa := catalogm.FestivalArtist{
				FestivalID:  festival.ID,
				ArtistID:    artist.ID,
				BillingTier: tier,
				Position:    slot,
				DayDate:     strptr(day),
				Stage:       strptr(stage),
			}
			db.Where("festival_id = ? AND artist_id = ? AND day_date = ?", festival.ID, artist.ID, day).
				FirstOrCreate(&fa)
		}
	}

	applyTags(db, catalogm.TagEntityFestival, festival.ID, userID, []struct{ Name, Slug, Category string }{
		{"Experimental", "experimental", catalogm.TagCategoryGenre},
		{"Psych", "psych", catalogm.TagCategoryGenre},
		{"Desert Festival", "desert-festival", catalogm.TagCategoryOther},
		{"Texas", "texas", catalogm.TagCategoryLocale},
		{"Multi-Day", "multi-day", catalogm.TagCategoryOther},
		{"Installations", "installations", catalogm.TagCategoryOther},
	})

	fmt.Printf("  ✅ festival exemplar: %s\n", exemplarFestivalSlug)
}

// seedEdgeCaseFestival preserves the PSY-657 canary: a festival whose social
// column is an explicit empty JSON object ({}). This is the truthy-but-empty
// shape that surfaced the PSY-657 bug, so it must stay in the dev DB alongside
// the rich exemplar to keep the hide-when-empty render path testable.
func seedEdgeCaseFestival(db *gorm.DB) {
	var existing catalogm.Festival
	if db.Where("slug = ?", edgeCaseFestivalSlug).First(&existing).Error == nil {
		return
	}

	empty := json.RawMessage(`{}`)
	festival := &catalogm.Festival{
		Name:        "Desert Daze (Exemplar) 2026",
		Slug:        edgeCaseFestivalSlug,
		SeriesSlug:  "desert-daze-exemplar",
		EditionYear: 2026,
		Description: strptr("PSY-657 canary: this festival intentionally has social = {} (an empty JSON object, which is truthy) and no venues. It exists so the hide-when-empty render paths stay testable in the dev DB; do NOT backfill its social links or venues."),
		City:        strptr("Lake Perris"),
		State:       strptr("CA"),
		Country:     strptr("USA"),
		StartDate:   "2026-10-09",
		EndDate:     "2026-10-11",
		Status:      catalogm.FestivalStatusAnnounced,
		Social:      &empty, // truthy-empty canary
	}
	if err := db.Create(festival).Error; err != nil {
		log.Printf("Warning: failed to create edge-case festival: %v", err)
		return
	}
	fmt.Printf("  ✅ edge-case festival (social:{} canary): %s\n", edgeCaseFestivalSlug)
}

// seedExemplarShow creates a show with description, image_url, age_requirement,
// ticket_url, 5+ tags, and a 5-artist bill with set_type variety (headliner,
// support, opener, dj, host). Returns the show ID.
func seedExemplarShow(db *gorm.DB, userID, headlinerID, venueID uint) uint {
	var existing catalogm.Show
	if db.Where("slug = ?", exemplarShowSlug).First(&existing).Error == nil {
		return existing.ID
	}
	if headlinerID == 0 {
		log.Printf("Warning: no headliner for exemplar show; skipping")
		return 0
	}

	eventDate := time.Now().Add(21 * 24 * time.Hour).UTC()
	price := 22.0
	show := &catalogm.Show{
		Title:          "Marissa Nadler (Exemplar) + guests at The Rhythm Room",
		Slug:           strptr(exemplarShowSlug),
		EventDate:      eventDate,
		City:           strptr("Phoenix"),
		State:          strptr("AZ"),
		Price:          &price,
		AgeRequirement: strptr("21+"),
		Description:    strptr("An evening of dream-folk and experimental sounds with a five-act bill spanning headliner through host. Doors at 7, music at 8.\n\nSeeded as the PSY-665 rich show exemplar: description, flyer image, age requirement, ticket URL, tags, and a multi-artist bill with full set_type variety so the show detail page renders the lineup with role labels and every metadata field."),
		Status:         catalogm.ShowStatusApproved,
		Source:         catalogm.ShowSourceUser,
		SubmittedBy:    &userID,
		TicketURL:      strptr("https://tickets.example.com/marissa-nadler-exemplar-rhythm-room"),
		ImageURL:       strptr("/seed-placeholders/show.svg"),
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(show).Error; err != nil {
			return fmt.Errorf("create show: %w", err)
		}

		if venueID != 0 {
			sv := catalogm.ShowVenue{ShowID: show.ID, VenueID: venueID}
			if err := tx.Create(&sv).Error; err != nil {
				return fmt.Errorf("link venue: %w", err)
			}
		}

		// 4+ artists with set_type variety. Denormalized event_date + venue_id
		// are intentionally left NULL on these show_artists rows: the PSY-576
		// partial unique index on (artist_id, venue_id, event_date) only covers
		// rows where BOTH are non-NULL, so leaving them NULL keeps the seed
		// graceful (matches the E2E setup-db.sh PSY-636 carveout) and avoids any
		// dedup-key collision with the artist's tracked shows below.
		bill := []struct {
			Name    string
			SetType string
			Pos     int
		}{
			{"Marissa Nadler (Exemplar)", "headliner", 0},
			{"Mount Eerie", "support", 1},
			{"Chat Pile", "opener", 2},
			{"Bill Orcutt", "dj", 3},
			{"Cat Power", "host", 4},
		}
		for _, b := range bill {
			var artist *catalogm.Artist
			var err error
			if b.Pos == 0 {
				artist = &catalogm.Artist{ID: headlinerID}
			} else {
				artist, err = findOrCreateArtist(tx, b.Name)
				if err != nil {
					return err
				}
			}
			sa := catalogm.ShowArtist{
				ShowID:   show.ID,
				ArtistID: artist.ID,
				Position: b.Pos,
				SetType:  b.SetType,
			}
			if err := tx.Create(&sa).Error; err != nil {
				return fmt.Errorf("link artist %s: %w", b.Name, err)
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("Warning: failed to create exemplar show: %v", err)
		return 0
	}

	applyTags(db, catalogm.TagEntityShow, show.ID, userID, []struct{ Name, Slug, Category string }{
		{"Dream Folk", "dream-folk", catalogm.TagCategoryGenre},
		{"Experimental", "experimental", catalogm.TagCategoryGenre},
		{"21 And Over", "21-and-over", catalogm.TagCategoryOther},
		{"Phoenix", "phoenix", catalogm.TagCategoryLocale},
		{"Touring", "touring", catalogm.TagCategoryOther},
		{"Multi-Act Bill", "multi-act-bill", catalogm.TagCategoryOther},
	})

	fmt.Printf("  ✅ show exemplar: %s\n", exemplarShowSlug)
	return show.ID
}

// seedExemplarArtistShows attaches 3+ upcoming shows + 3+ past shows to the
// exemplar venue (satisfying the venue's "3+ upcoming + 3+ past shows" AC) and
// gives the exemplar artist its "3+ shows tracked" coverage. These are
// lightweight shows distinct from the rich exemplar show above.
//
// Each show gets a distinct event_date AND the show_artists denorm cols are
// left NULL, so the PSY-576 partial unique index never trips (it excludes NULL
// denorm rows). Idempotent via a fixed slug per show.
func seedExemplarArtistShows(db *gorm.DB, artistID, venueID uint) {
	if artistID == 0 || venueID == 0 {
		return
	}

	type tracked struct {
		slug      string
		title     string
		dayOffset int // negative = past, positive = upcoming
	}
	shows := []tracked{
		{"exemplar-upcoming-1", "Marissa Nadler (Exemplar) — Spring Run", 35},
		{"exemplar-upcoming-2", "Marissa Nadler (Exemplar) — Early Show", 49},
		{"exemplar-upcoming-3", "Marissa Nadler (Exemplar) — Late Show", 63},
		{"exemplar-past-1", "Marissa Nadler (Exemplar) — Last Winter", -28},
		{"exemplar-past-2", "Marissa Nadler (Exemplar) — Fall Tour", -56},
		{"exemplar-past-3", "Marissa Nadler (Exemplar) — Anniversary", -84},
	}

	for _, s := range shows {
		var existing catalogm.Show
		if db.Where("slug = ?", s.slug).First(&existing).Error == nil {
			continue
		}
		eventDate := time.Now().Add(time.Duration(s.dayOffset) * 24 * time.Hour).UTC()
		show := &catalogm.Show{
			Title:     s.title,
			Slug:      strptr(s.slug),
			EventDate: eventDate,
			City:      strptr("Phoenix"),
			State:     strptr("AZ"),
			Status:    catalogm.ShowStatusApproved,
			Source:    catalogm.ShowSourceUser,
		}
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(show).Error; err != nil {
				return err
			}
			if err := tx.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error; err != nil {
				return err
			}
			return tx.Create(&catalogm.ShowArtist{
				ShowID:   show.ID,
				ArtistID: artistID,
				Position: 0,
				SetType:  "headliner",
			}).Error
		})
		if err != nil {
			log.Printf("Warning: failed to create tracked show %s: %v", s.slug, err)
		}
	}
	fmt.Printf("  ✅ tracked shows (3 upcoming + 3 past) for venue/artist exemplars\n")
}

// seedExemplarSimilarArtists creates 3+ similar-artist edges from the exemplar
// artist. Edges use the canonical-ordering CHECK (source_artist_id <
// target_artist_id) via CanonicalOrder, and the composite PK
// (source, target, relationship_type) makes FirstOrCreate idempotent.
func seedExemplarSimilarArtists(db *gorm.DB, artistID uint) {
	if artistID == 0 {
		return
	}
	for _, name := range []string{"Mount Eerie", "Cat Power", "Soccer Mommy", "Pile"} {
		other, err := findOrCreateArtist(db, name)
		if err != nil {
			log.Printf("Warning: %v", err)
			continue
		}
		if other.ID == artistID {
			continue
		}
		src, tgt := catalogm.CanonicalOrder(artistID, other.ID)
		rel := catalogm.ArtistRelationship{
			SourceArtistID:   src,
			TargetArtistID:   tgt,
			RelationshipType: catalogm.RelationshipTypeSimilar,
			Score:            0.85,
			AutoDerived:      false,
		}
		db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			src, tgt, catalogm.RelationshipTypeSimilar).FirstOrCreate(&rel)
	}
	fmt.Printf("  ✅ similar-artist edges for artist exemplar\n")
}

// seedExemplarCollection creates a collection with description, cover image,
// 5+ tags, and items spanning multiple entity types (artist, release,
// festival, show, venue, label). Idempotent via the collection's unique slug;
// items are guarded by (collection_id, entity_type, entity_id) lookups.
func seedExemplarCollection(db *gorm.DB, userID, artistID, venueID, labelID, showID uint) {
	var coll communitym.Collection
	err := db.Where("slug = ?", exemplarCollectionSlug).First(&coll).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		coll = communitym.Collection{
			Title:         "Psychic Homily Staff Picks (Exemplar)",
			Slug:          exemplarCollectionSlug,
			Description:   "A hand-curated cross-section of the knowledge graph: a marquee artist, a standout release, a festival worth traveling for, a show to catch, a room we love, and a label doing the work.\n\nSeeded as the PSY-665 rich collection exemplar: description, cover image, tags, and items spanning every entity type so the collection detail page renders mixed-type item cards.",
			CreatorID:     userID,
			Collaborative: false,
			CoverImageURL: strptr("/seed-placeholders/collection.svg"),
			IsPublic:      true,
			IsFeatured:    true,
			DisplayMode:   communitym.CollectionDisplayModeRanked,
		}
		if err := db.Create(&coll).Error; err != nil {
			log.Printf("Warning: failed to create exemplar collection: %v", err)
			return
		}
	} else if err != nil {
		log.Printf("Warning: failed to look up exemplar collection: %v", err)
		return
	}

	// Items spanning multiple entity types. Resolve a release ID for the
	// release item.
	var release catalogm.Release
	db.Where("slug = ?", exemplarReleaseSlug).First(&release)
	var festival catalogm.Festival
	db.Where("slug = ?", exemplarFestivalSlug).First(&festival)

	items := []struct {
		EntityType string
		EntityID   uint
		Notes      string
	}{
		{communitym.CollectionEntityArtist, artistID, "The anchor of the whole graph — start here."},
		{communitym.CollectionEntityRelease, release.ID, "The record that pulls the room quiet."},
		{communitym.CollectionEntityFestival, festival.ID, "Three days, every billing tier, worth the drive."},
		{communitym.CollectionEntityShow, showID, "A five-act bill in an intimate room."},
		{communitym.CollectionEntityVenue, venueID, "One of the better-sounding small stages in town."},
		{communitym.CollectionEntityLabel, labelID, "The label tying several of these together."},
	}
	pos := 0
	for _, it := range items {
		if it.EntityID == 0 {
			continue // skip items whose entity failed to seed
		}
		var existing communitym.CollectionItem
		if db.Where("collection_id = ? AND entity_type = ? AND entity_id = ?",
			coll.ID, it.EntityType, it.EntityID).First(&existing).Error == nil {
			pos++
			continue
		}
		ci := communitym.CollectionItem{
			CollectionID:  coll.ID,
			EntityType:    it.EntityType,
			EntityID:      it.EntityID,
			Position:      pos,
			AddedByUserID: userID,
			Notes:         strptr(it.Notes),
		}
		if err := db.Create(&ci).Error; err != nil {
			log.Printf("Warning: failed to add collection item %s/%d: %v", it.EntityType, it.EntityID, err)
		}
		pos++
	}

	applyTags(db, catalogm.TagEntityCollection, coll.ID, userID, []struct{ Name, Slug, Category string }{
		{"Staff Picks", "staff-picks", catalogm.TagCategoryOther},
		{"Dream Folk", "dream-folk", catalogm.TagCategoryGenre},
		{"Experimental", "experimental", catalogm.TagCategoryGenre},
		{"Cross-Type", "cross-type", catalogm.TagCategoryOther},
		{"Curated", "curated", catalogm.TagCategoryOther},
		{"Phoenix", "phoenix", catalogm.TagCategoryLocale},
	})

	fmt.Printf("  ✅ collection exemplar: %s\n", exemplarCollectionSlug)
}
