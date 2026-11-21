package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// dryDigest dry-runs csv and returns its plan digest.
func (s *TriageSuite) dryDigest(csv string) string {
	rows, err := parseWorklist(strings.NewReader(csv))
	s.Require().NoError(err)
	return runWorklist(s.db, rows, s.actor, nil, io.Discard, planBinding{target: "test"}).Digest
}

// confirmWith runs csv confirmed against a given digest.
func (s *TriageSuite) confirmWith(csv, digest string) (string, runOutcome) {
	rows, err := parseWorklist(strings.NewReader(csv))
	s.Require().NoError(err)
	var out bytes.Buffer
	outcome := runWorklist(s.db, rows, s.actor, s.recorder(), &out, planBinding{target: "test", expect: digest})
	return out.String(), outcome
}

// An unchanged database gives the same digest twice, and the digest names the
// database: the same plan against another target has another digest.
func (s *TriageSuite) TestAnUnchangedPlanHasTheSameDigest() {
	junk := s.artist("Digest Night")
	s.show("", act{s.artist("Digest Head"), "headliner"}, act{junk, "performer"})
	csv := csvOf(*junk.Slug + ",remove,")

	first, second := s.dryDigest(csv), s.dryDigest(csv)

	s.Equal(first, second)
	s.Len(first, 16)
	rows, err := parseWorklist(strings.NewReader(csv))
	s.Require().NoError(err)
	other := runWorklist(s.db, rows, s.actor, nil, io.Discard, planBinding{target: "another"}).Digest
	s.NotEqual(first, other)
}

// A bill added between the reviewed dry run and the confirmed run changes the
// plan: the confirmed run writes nothing.
func (s *TriageSuite) TestABillAddedAfterTheDryRunRefusesTheConfirm() {
	junk := s.artist("Spreading Night")
	s.show("", act{s.artist("First Head"), "headliner"}, act{junk, "performer"})
	csv := csvOf(*junk.Slug + ",remove,")
	digest := s.dryDigest(csv)
	added := s.show("", act{s.artist("Second Head"), "headliner"}, act{junk, "performer"})

	out, outcome := s.confirmWith(csv, digest)

	s.Require().Error(outcome.Stopped)
	s.Contains(out, "[ERROR] the plan now has digest")
	s.Contains(out, "not the reviewed "+digest+"; nothing written")
	s.NotNil(s.artistByID(junk.ID))
	s.Equal([]string{"Second Head:headliner", "Spreading Night:performer"}, s.bill(added.ID))
	s.Empty(s.recordLines())
	s.True(outcome.Failed())
}

// An artist created between the dry run and the confirmed run with a split
// act's name would be linked instead of created: the confirmed run refuses.
func (s *TriageSuite) TestAnArtistCreatedForASplitActAfterTheDryRunRefusesTheConfirm() {
	joined := s.artist("Fresh One and Fresh Two")
	s.show("", act{s.artist("Split Head"), "headliner"}, act{joined, "performer"})
	csv := csvOf(*joined.Slug + ",split,Fresh One | Fresh Two")
	digest := s.dryDigest(csv)
	s.artist("Fresh Two")

	out, outcome := s.confirmWith(csv, digest)

	s.Require().Error(outcome.Stopped, out)
	s.Contains(out, "nothing written")
	s.NotNil(s.artistByID(joined.ID))
	s.Equal(int64(0), s.count(`SELECT COUNT(*) FROM artists WHERE name = 'Fresh One'`))
}

// The digest the dry run printed lets the confirmed run through when nothing
// changed, and each row prints its plan fingerprint.
func (s *TriageSuite) TestTheReviewedDigestAppliesThePlan() {
	junk := s.artist("Applied Night")
	s.show("", act{s.artist("Applied Head"), "headliner"}, act{junk, "performer"})
	csv := csvOf(*junk.Slug + ",remove,")
	digest := s.dryDigest(csv)

	out, outcome := s.confirmWith(csv, digest)

	s.NoError(outcome.Stopped, out)
	s.Contains(out, "  plan: ")
	s.Contains(out, "Plan digest:     "+digest)
	s.Nil(s.artistByID(junk.ID))
}

// A follow, a comment or a description marks a real act: remove and split are
// refused for each.
func (s *TriageSuite) TestFollowsCommentsAndDescriptionsMakeARealAct() {
	followed := s.artist("Followed Night")
	s.follow(followed)
	commented := s.artist("Commented Night")
	s.Require().NoError(s.db.Exec(`INSERT INTO comments (entity_type, entity_id, user_id, body, body_html, created_at, updated_at)
		VALUES ('artist', ?, ?, 'great set', '<p>great set</p>', NOW(), NOW())`, commented.ID, s.actor).Error)
	described := s.artist("Described Night")
	s.Require().NoError(s.db.Exec(`UPDATE artists SET description = 'A band from Tucson.' WHERE id = ?`, described.ID).Error)
	splitFollowed := s.artist("Followed One and Followed Two")
	s.show("", act{s.artist("Head"), "headliner"}, act{splitFollowed, "performer"})
	s.follow(splitFollowed)

	out, summary := s.run(csvOf(
		*followed.Slug+",remove,",
		*commented.Slug+",remove,",
		*described.Slug+",remove,",
		*splitFollowed.Slug+",split,Followed One | Followed Two",
	), true)

	s.Equal(4, summary[statusRefused], out)
	s.Contains(out, `"Followed Night" has 1 follow(s): it looks like a real act`)
	s.Contains(out, `"Commented Night" has 1 comment(s)`)
	s.Contains(out, `"Described Night" has 1 description`)
	s.Contains(out, `"Followed One and Followed Two" has 1 follow(s)`)
	for _, a := range []uint{followed.ID, commented.ID, described.ID, splitFollowed.ID} {
		s.NotNil(s.artistByID(a))
	}
}

// A keep or rename row whose slug now names another artist, or whose artist has
// another name, is refused, not reported as kept or already done.
func (s *TriageSuite) TestStaleKeepAndRenameRowsAreRefusedBeforeTheirShortcuts() {
	kept := s.artist("Kept Night")
	done := s.artist("Done Name")
	csv := fmt.Sprintf("artist_slug,action,target,name,artist_id\n%s,keep,,,%d\n%s,rename,Done Name,Other Name,\n",
		*kept.Slug, kept.ID+1000, *done.Slug)

	out, summary := s.run(csv, false)

	s.Equal(2, summary[statusRefused], out)
	s.Equal(0, summary[statusKept], out)
	s.Equal(0, summary[statusDone], out)
	s.Contains(out, fmt.Sprintf("now names artist %d, not %d as reviewed", kept.ID, kept.ID+1000))
	s.Contains(out, `is now named "Done Name", not "Other Name" as reviewed`)
}

// The record copies the polymorphic columns that are not named entity_id: a
// fulfilled request and an entity request that created the artist.
func (s *TriageSuite) TestTheRecordCopyCoversRequestColumns() {
	refs, err := artistReferenceColumns(s.db)
	s.Require().NoError(err)
	where := map[string]string{}
	for _, r := range refs {
		where[r.key] = r.where
	}
	s.Contains(where["requests.requested_entity_id"], "entity_type = 'artist'")
	s.Contains(where["entity_requests.created_entity_id"], "entity_type = 'artist'")
	s.NotContains(where["notification_log.subject_entity_id"], "entity_type = 'artist'",
		"subject_entity_id is read by alert type, not as an artist entity reference")
}
