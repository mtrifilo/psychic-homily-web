package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

// The suite runs the tool against a real, fully migrated Postgres: the merge
// and delete it drives rely on cascades, triggers and unique indexes only the
// real schema has.
type TriageSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	actor  uint
	venue  uint
	seq    atomic.Int64
	record *memRecord
}

func TestTriageSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(TriageSuite))
}

func (s *TriageSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *TriageSuite) TearDownSuite() { s.testDB.Cleanup() }

func (s *TriageSuite) SetupTest() {
	for _, stmt := range []string{
		"DELETE FROM audit_logs",
		"DELETE FROM entity_edit_audit_logs",
		"DELETE FROM revisions",
		"DELETE FROM user_bookmarks",
		"DELETE FROM entity_tags",
		"DELETE FROM tags",
		"DELETE FROM artist_relationship_votes",
		"DELETE FROM artist_relationships",
		"DELETE FROM artist_aliases",
		"DELETE FROM artist_labels",
		"DELETE FROM labels",
		"DELETE FROM artist_releases",
		"DELETE FROM releases",
		"DELETE FROM image_enrich_queue",
		"DELETE FROM show_artists",
		"DELETE FROM show_venues",
		"DELETE FROM shows",
		"DELETE FROM venues",
		"DELETE FROM artists",
		"DELETE FROM users",
	} {
		s.Require().NoError(s.db.Exec(stmt).Error, stmt)
	}
	s.record = &memRecord{}
	s.actor = s.user()
	s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", s.actor).Update("is_admin", true).Error)
	slug := "launchpad-albuquerque-nm"
	v := &catalogm.Venue{Name: "Launchpad", Slug: &slug, City: "Albuquerque", State: "NM"}
	s.Require().NoError(s.db.Create(v).Error)
	s.venue = v.ID
}

var nonSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

func (s *TriageSuite) artist(name string) *catalogm.Artist {
	slug := strings.Trim(nonSlugChars.ReplaceAllString(strings.ToLower(name), "-"), "-")
	a := &catalogm.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)
	return a
}

// act is one bill entry for show.
type act struct {
	artist  *catalogm.Artist
	setType string
}

// show creates an approved show at the suite's room, on its own day so the
// dedup index never joins two test shows, billing acts in order.
func (s *TriageSuite) show(title string, acts ...act) *catalogm.Show {
	n := s.seq.Add(1)
	slug := fmt.Sprintf("2026-12-%02d-show-%d-at-launchpad", n%28+1, n)
	sh := &catalogm.Show{Title: title, Slug: &slug, EventDate: time.Date(2026, 12, 1, 3, 0, 0, 0, time.UTC).AddDate(0, 0, int(n))}
	s.Require().NoError(s.db.Create(sh).Error)
	s.Require().NoError(s.db.Create(&catalogm.ShowVenue{ShowID: sh.ID, VenueID: s.venue}).Error)
	for i, a := range acts {
		s.Require().NoError(s.db.Create(&catalogm.ShowArtist{ShowID: sh.ID, ArtistID: a.artist.ID, Position: i, SetType: a.setType}).Error)
	}
	return sh
}

func (s *TriageSuite) user() uint {
	email := fmt.Sprintf("fan-%d@test.local", s.seq.Add(1))
	u := &authm.User{Email: &email, IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(u).Error)
	return u.ID
}

func (s *TriageSuite) follow(artist *catalogm.Artist) uint {
	fan := s.user()
	s.Require().NoError(s.db.Exec(`INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at)
		VALUES (?, 'artist', ?, 'follow', NOW())`, fan, artist.ID).Error)
	return fan
}

func (s *TriageSuite) tag(artist *catalogm.Artist) {
	n := s.seq.Add(1)
	var tagID uint
	s.Require().NoError(s.db.Raw(`INSERT INTO tags (name, slug) VALUES (?, ?) RETURNING id`,
		fmt.Sprintf("tag %d", n), fmt.Sprintf("tag-%d", n)).Scan(&tagID).Error)
	s.Require().NoError(s.db.Exec(`INSERT INTO entity_tags (tag_id, entity_type, entity_id, added_by_user_id)
		VALUES (?, 'artist', ?, ?)`, tagID, artist.ID, s.actor).Error)
}

func (s *TriageSuite) release(artist *catalogm.Artist) uint {
	n := s.seq.Add(1)
	slug := fmt.Sprintf("release-%d", n)
	r := &catalogm.Release{Title: fmt.Sprintf("Release %d", n), Slug: &slug}
	s.Require().NoError(s.db.Create(r).Error)
	s.Require().NoError(s.db.Create(&catalogm.ArtistRelease{ArtistID: artist.ID, ReleaseID: r.ID, Role: "main"}).Error)
	return r.ID
}

func (s *TriageSuite) relationship(a, b *catalogm.Artist, relType string, autoDerived bool) {
	src, tgt := catalogm.CanonicalOrder(a.ID, b.ID)
	s.Require().NoError(s.db.Exec(`INSERT INTO artist_relationships
		(source_artist_id, target_artist_id, relationship_type, auto_derived, created_at, updated_at)
		VALUES (?, ?, ?, ?, NOW(), NOW())`, src, tgt, relType, autoDerived).Error)
}

func (s *TriageSuite) run(csv string, confirm bool) (string, runSummary) {
	out, outcome := s.runOutcome(csv, confirm)
	return out, outcome.Summary
}

// runOutcome runs a worklist; a confirmed run appends its record lines to
// s.record.
func (s *TriageSuite) runOutcome(csv string, confirm bool) (string, runOutcome) {
	rows, err := parseWorklist(strings.NewReader(csv))
	s.Require().NoError(err)
	var rec *recorder
	if confirm {
		rec = s.recorder()
	}
	var out bytes.Buffer
	outcome := runWorklist(s.db, rows, s.actor, rec, &out)
	return out.String(), outcome
}

// recorder appends to s.record.
func (s *TriageSuite) recorder() *recorder {
	return &recorder{f: s.record, now: time.Now,
		runID: "run-1", actorID: s.actor, environment: "test", database: "test-db"}
}

// recordLines parses the record lines written so far.
func (s *TriageSuite) recordLines() []recordLine {
	var lines []recordLine
	for _, raw := range strings.Split(strings.TrimSpace(s.record.buf.String()), "\n") {
		if raw == "" {
			continue
		}
		var l recordLine
		s.Require().NoError(json.Unmarshal([]byte(raw), &l), raw)
		lines = append(lines, l)
	}
	return lines
}

func (s *TriageSuite) artistByID(id uint) *catalogm.Artist {
	var a catalogm.Artist
	err := s.db.First(&a, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	s.Require().NoError(err)
	return &a
}

func (s *TriageSuite) count(sql string, args ...any) int64 {
	var n int64
	s.Require().NoError(s.db.Raw(sql, args...).Scan(&n).Error)
	return n
}

// bill returns a show's acts as "name:set_type" in bill order.
func (s *TriageSuite) bill(showID uint) []string {
	var rows []struct {
		Name    string
		SetType string
	}
	s.Require().NoError(s.db.Raw(`SELECT a.name, sa.set_type FROM show_artists sa JOIN artists a ON a.id = sa.artist_id
		WHERE sa.show_id = ? ORDER BY sa.position`, showID).Scan(&rows).Error)
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Name + ":" + r.SetType
	}
	return out
}

// memRecord is an in-memory record file. With failWrite set, a write puts the
// first partial bytes in and fails; with failSync set, Sync fails.
type memRecord struct {
	buf       bytes.Buffer
	failWrite bool
	partial   int
	failSync  bool
}

func (m *memRecord) Write(p []byte) (int, error) {
	if m.failWrite {
		n := min(m.partial, len(p))
		m.buf.Write(p[:n])
		return n, errors.New("disk full")
	}
	return m.buf.Write(p)
}

func (m *memRecord) Sync() error {
	if m.failSync {
		return errors.New("fsync failed")
	}
	return nil
}

func (m *memRecord) Stat() (os.FileInfo, error) { return memInfo(m.buf.Len()), nil }

func (m *memRecord) Truncate(n int64) error {
	m.buf.Truncate(int(n))
	return nil
}

// memInfo is the size a memRecord reports; nothing else of it is read.
type memInfo int

func (i memInfo) Name() string       { return "record" }
func (i memInfo) Size() int64        { return int64(i) }
func (i memInfo) Mode() os.FileMode  { return 0o600 }
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return false }
func (i memInfo) Sys() any           { return nil }

// csvOf builds a worklist from rows of up to five fields, padding the rest.
func csvOf(rows ...string) string {
	var b strings.Builder
	b.WriteString("artist_slug,action,target,show_slug,note\n")
	for _, r := range rows {
		b.WriteString(r + strings.Repeat(",", 4-strings.Count(r, ",")) + "\n")
	}
	return b.String()
}

// ── rename ─────────────────────────────────────────────

// A rename changes the name and nothing else: the slug stays, so the URL keeps
// answering, and the artist's bill, release, follow and tag stay on it. It is
// recorded against the actor as the admin edit records it.
func (s *TriageSuite) TestRenameKeepsTheSlugAndEveryReference() {
	a := s.artist("Riot Act (Pearl Jam Tribute)")
	sh := s.show("", act{s.artist("Headliner"), "headliner"}, act{a, "opener"})
	releaseID := s.release(a)
	fan := s.follow(a)
	s.tag(a)
	csv := csvOf(*a.Slug + ",rename,Riot Act,,tribute descriptor")

	out, summary := s.run(csv, false)

	s.False(summary.Refused(), out)
	s.Equal(1, summary[statusWouldApply])
	s.Contains(out, `name: "Riot Act (Pearl Jam Tribute)" -> "Riot Act"`)
	s.Contains(out, "slug: riot-act-pearl-jam-tribute (unchanged)")
	s.Equal("Riot Act (Pearl Jam Tribute)", s.artistByID(a.ID).Name, "the dry run renames nothing")
	s.Equal(int64(0), s.count(`SELECT COUNT(*) FROM revisions`), "the dry run records nothing")

	out, summary = s.run(csv, true)

	s.False(summary.Refused(), out)
	s.Equal(1, summary[statusApplied])
	got := s.artistByID(a.ID)
	s.Equal("Riot Act", got.Name)
	s.Equal("riot-act-pearl-jam-tribute", *got.Slug, "the slug stays")
	s.Equal([]string{"Headliner:headliner", "Riot Act:opener"}, s.bill(sh.ID))
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM artist_releases WHERE artist_id = ? AND release_id = ?`, a.ID, releaseID))
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM user_bookmarks WHERE entity_type = 'artist' AND entity_id = ? AND user_id = ?`, a.ID, fan))
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM entity_tags WHERE entity_type = 'artist' AND entity_id = ?`, a.ID))
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM revisions WHERE entity_type = 'artist' AND entity_id = ? AND user_id = ?
		AND field_changes @> '[{"field": "name", "old_value": "Riot Act (Pearl Jam Tribute)", "new_value": "Riot Act"}]'`, a.ID, s.actor))
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM entity_edit_audit_logs WHERE entity_type = 'artist' AND entity_id = ? AND actor_id = ?`, a.ID, s.actor))
}

func (s *TriageSuite) TestRenameOntoANameOrAliasAnotherArtistHasIsRefused() {
	existing := s.artist("Lone Stranger")
	aliased := s.artist("The Mavericks")
	s.Require().NoError(s.db.Exec(`INSERT INTO artist_aliases (artist_id, alias) VALUES (?, 'Mavericks')`, aliased.ID).Error)
	a := s.artist("Lone Stranger & Friends")
	b := s.artist("The Mavericks & Friends")
	csv := csvOf(*a.Slug+",rename,lone stranger", *b.Slug+",rename,Mavericks")

	out, summary := s.run(csv, true)

	s.Equal(2, summary[statusRefused], out)
	s.Contains(out, `artist "Lone Stranger" (slug `+*existing.Slug+`) already has that name; merge into it instead`)
	s.Contains(out, `"Mavericks" is an alias of "The Mavericks"`)
	s.Equal("Lone Stranger & Friends", s.artistByID(a.ID).Name)
	s.Equal("The Mavericks & Friends", s.artistByID(b.ID).Name)
}

// ── merge ──────────────────────────────────────────────

// A merge moves the bill, release, follow and tag onto the artist kept, adds
// the merged name as an alias, deletes the merged artist and records the merge
// against the actor. The dry run before it changes nothing.
func (s *TriageSuite) TestMergeMovesEveryReferenceOntoTheTarget() {
	keep := s.artist("Harry Katz & The Pistachios")
	dup := s.artist("Harry Katz and the Pistachios")
	other := s.artist("Opener")
	sh := s.show("", act{keep, "headliner"}, act{dup, "performer"}, act{other, "opener"})
	sh2 := s.show("", act{dup, "headliner"})
	releaseID := s.release(dup)
	fan := s.follow(dup)
	s.tag(dup)
	csv := csvOf(*dup.Slug + ",merge," + *keep.Slug + ",,decorated copy of the headliner")

	out, summary := s.run(csv, false)

	s.False(summary.Refused(), out)
	s.Contains(out, `from: "Harry Katz and the Pistachios"`)
	s.Contains(out, `into: "Harry Katz & The Pistachios"`)
	s.Contains(out, "/artists/"+*dup.Slug+" answers 404")
	s.NotNil(s.artistByID(dup.ID), "the dry run deletes nothing")
	s.Equal(int64(0), s.count(`SELECT COUNT(*) FROM audit_logs`))

	out, summary = s.run(csv, true)

	s.False(summary.Refused(), out)
	s.Equal(1, summary[statusApplied])
	s.Nil(s.artistByID(dup.ID))
	s.Equal([]string{"Harry Katz & The Pistachios:headliner", "Opener:opener"}, s.bill(sh.ID), "the duplicate on the same bill is dropped")
	s.Equal([]string{"Harry Katz & The Pistachios:headliner"}, s.bill(sh2.ID))
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM artist_releases WHERE artist_id = ? AND release_id = ?`, keep.ID, releaseID))
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM user_bookmarks WHERE entity_type = 'artist' AND entity_id = ? AND user_id = ?`, keep.ID, fan))
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM entity_tags WHERE entity_type = 'artist' AND entity_id = ?`, keep.ID))
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM artist_aliases WHERE artist_id = ? AND alias = 'Harry Katz and the Pistachios'`, keep.ID))
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM audit_logs WHERE action = 'merge_artists' AND entity_id = ? AND actor_id = ?
		AND metadata->>'merged_artist_slug' = ?`, keep.ID, s.actor, *dup.Slug))
	s.Equal("harry-katz-the-pistachios", *s.artistByID(keep.ID).Slug, "the target keeps its slug")
}

// The dry run warns when the merged artist has links the target lacks or
// hand-made relationships, which the merge does not carry over.
func (s *TriageSuite) TestMergeWarnsAboutWhatItDoesNotCarry() {
	keep := s.artist("Lone Stranger")
	dup := s.artist("Lone Stranger & Friends")
	spotify := "https://open.spotify.com/artist/x"
	s.Require().NoError(s.db.Model(&catalogm.Artist{}).Where("id = ?", dup.ID).Update("spotify", spotify).Error)
	s.relationship(dup, s.artist("Side Band"), "side_project", false)

	out, summary := s.run(csvOf(*dup.Slug+",merge,"+*keep.Slug), false)

	s.False(summary.Refused(), out)
	s.Contains(out, "WARNING: the merge deletes 1 hand-made relationship(s)")
	s.Contains(out, "WARNING: the merge keeps lone-stranger's own fields; lone-stranger-friends's spotify are not carried over")
}

func (s *TriageSuite) TestMergeIntoAnUnknownArtistIsRefused() {
	dup := s.artist("Lone Stranger & Friends")

	out, summary := s.run(csvOf(*dup.Slug+",merge,no-such-artist", "no-such-slug,merge,"+*dup.Slug), true)

	s.Equal(2, summary[statusRefused], out)
	s.Contains(out, "no artist has target slug no-such-artist")
	s.Contains(out, "no artist has slug no-such-slug")
	s.NotNil(s.artistByID(dup.ID))
}

// ── remove ─────────────────────────────────────────────

// A remove takes a non-performer off its bill and deletes it. The other acts
// keep their roles, its follow and tag go with it, and its derived graph edge
// is deleted so the database lets it go.
func (s *TriageSuite) TestRemoveTakesItOffTheBillAndDeletesIt() {
	head := s.artist("Ouija Macc")
	junk := s.artist("LIVE AT LAUNCHPAD")
	support := s.artist("Mikey Rotten")
	sh := s.show("Ouija Macc", act{head, "headliner"}, act{junk, "performer"}, act{support, "opener"})
	s.follow(junk)
	s.tag(junk)
	s.relationship(junk, head, "shared_bills", true)
	s.relationship(junk, support, "shared_bills", true)
	csv := csvOf(*junk.Slug + ",remove,,,feed subtitle")

	out, summary := s.run(csv, false)

	s.False(summary.Refused(), out)
	s.Contains(out, `bill ["Ouija Macc" [headliner], "LIVE AT LAUNCHPAD" [performer], "Mikey Rotten" [opener]] -> ["Ouija Macc" [headliner], "Mikey Rotten" [opener]]`)
	s.Contains(out, "drops, among others, 1 follow/save row(s), 1 tag(s), 0 crate item(s), 0 alias(es)")
	s.Contains(out, "2 derived graph edge(s) deleted")
	s.Contains(out, "/artists/live-at-launchpad answers 404")
	s.NotNil(s.artistByID(junk.ID), "the dry run deletes nothing")
	s.Len(s.bill(sh.ID), 3)

	out, summary = s.run(csv, true)

	s.False(summary.Refused(), out)
	s.Equal(1, summary[statusApplied])
	s.Nil(s.artistByID(junk.ID))
	s.Equal([]string{"Ouija Macc:headliner", "Mikey Rotten:opener"}, s.bill(sh.ID))
	s.Equal(int64(0), s.count(`SELECT COUNT(*) FROM user_bookmarks WHERE entity_type = 'artist' AND entity_id = ?`, junk.ID))
	s.Equal(int64(0), s.count(`SELECT COUNT(*) FROM entity_tags WHERE entity_type = 'artist' AND entity_id = ?`, junk.ID))
	s.Equal(int64(0), s.count(`SELECT COUNT(*) FROM artist_relationships WHERE source_artist_id = ? OR target_artist_id = ?`, junk.ID, junk.ID))
	s.Equal("Ouija Macc", s.artistByID(head.ID).Name)
	var title string
	s.Require().NoError(s.db.Raw(`SELECT title FROM shows WHERE id = ?`, sh.ID).Scan(&title).Error)
	s.Equal("Ouija Macc", title, "a remove without the title target leaves the title alone")
}

// With the title target the name is written to the show's title where it is
// empty, recorded as a show revision against the actor; a show that already has
// a title refuses the row and nothing changes.
func (s *TriageSuite) TestRemoveWritesTheTitleOnlyWhereItIsEmpty() {
	untitled := s.artist("A Night in Stars Hollow")
	titled := s.artist("Kings of Leon Tribute Night")
	act1 := s.artist("Band One")
	act2 := s.artist("Band Two")
	shA := s.show("", act{untitled, "headliner"}, act{act1, "performer"})
	shB := s.show("Already Titled", act{titled, "headliner"}, act{act2, "performer"})
	csv := csvOf(*untitled.Slug+",remove,title", *titled.Slug+",remove,Title")

	out, summary := s.run(csv, true)

	s.Equal(1, summary[statusApplied], out)
	s.Equal(1, summary[statusRefused], out)
	s.Contains(out, `title: (empty) -> "A Night in Stars Hollow"`)
	s.Contains(out, `already has the title "Already Titled"; it is not overwritten`)
	var titles []string
	s.Require().NoError(s.db.Raw(`SELECT title FROM shows WHERE id IN (?, ?) ORDER BY id`, shA.ID, shB.ID).Scan(&titles).Error)
	s.Equal([]string{"A Night in Stars Hollow", "Already Titled"}, titles)
	s.Nil(s.artistByID(untitled.ID))
	s.NotNil(s.artistByID(titled.ID), "the refused row deletes nothing")
	s.Equal([]string{"Kings of Leon Tribute Night:headliner", "Band Two:performer"}, s.bill(shB.ID))
	s.Equal(int64(1), s.count(`SELECT COUNT(*) FROM revisions WHERE entity_type = 'show' AND entity_id = ? AND user_id = ?
		AND field_changes @> '[{"field": "title", "new_value": "A Night in Stars Hollow"}]'`, shA.ID, s.actor))
	s.Contains(out, `WARNING: the artist held the headline slot; it is now held by "Band One"`)
}

// An artist with any mark of a real act is refused, and nothing is written.
func (s *TriageSuite) TestRemoveOfARealActIsRefused() {
	withRelease := s.artist("Ganja White Night")
	s.release(withRelease)
	withLink := s.artist("Late Night Radio")
	website := "https://latenightradio.example"
	s.Require().NoError(s.db.Model(&catalogm.Artist{}).Where("id = ?", withLink.ID).Update("website", website).Error)
	withLabel := s.artist("Our Last Night")
	var labelID uint
	s.Require().NoError(s.db.Raw(`INSERT INTO labels (name, slug) VALUES ('Label', 'label') RETURNING id`).Scan(&labelID).Error)
	s.Require().NoError(s.db.Exec(`INSERT INTO artist_labels (artist_id, label_id) VALUES (?, ?)`, withLabel.ID, labelID).Error)
	withEdge := s.artist("Sunshine & The Night Walkers")
	s.relationship(withEdge, s.artist("Member Band"), "member_of", false)
	withVote := s.artist("Night Voter")
	partner := s.artist("Partner Band")
	s.relationship(withVote, partner, "shared_bills", true)
	src, tgt := catalogm.CanonicalOrder(withVote.ID, partner.ID)
	s.Require().NoError(s.db.Exec(`INSERT INTO artist_relationship_votes
		(source_artist_id, target_artist_id, relationship_type, user_id, direction, created_at)
		VALUES (?, ?, 'shared_bills', ?, 1, NOW())`, src, tgt, s.actor).Error)
	before := s.count(`SELECT COUNT(*) FROM artists`)

	out, summary := s.run(csvOf(
		*withRelease.Slug+",remove,",
		*withLink.Slug+",remove,",
		*withLabel.Slug+",remove,",
		*withEdge.Slug+",split,Sunshine | The Night Walkers",
		*withVote.Slug+",remove,",
	), true)

	s.Equal(5, summary[statusRefused], out)
	s.Contains(out, `"Ganja White Night" has 1 release credit(s): it looks like a real act`)
	s.Contains(out, `"Late Night Radio" has links (website)`)
	s.Contains(out, `"Our Last Night" has 1 label(s)`)
	s.Contains(out, `"Sunshine & The Night Walkers" has 1 hand-made relationship(s)`)
	s.Contains(out, `"Night Voter" has 1 relationship vote(s)`)
	s.Equal(before, s.count(`SELECT COUNT(*) FROM artists`))
}

// The tool never leaves a show with no acts.
func (s *TriageSuite) TestRemoveOfTheOnlyActIsRefused() {
	party := s.artist("2000's Night")
	sh := s.show("", act{party, "headliner"})

	out, summary := s.run(csvOf(*party.Slug+",remove,title"), true)

	s.True(summary.Refused(), out)
	s.Contains(out, "it is the only act on show "+*sh.Slug)
	s.NotNil(s.artistByID(party.ID))
	s.Equal([]string{"2000's Night:headliner"}, s.bill(sh.ID))
}

// An artist on no bill is simply deleted.
func (s *TriageSuite) TestRemoveOfAnArtistOnNoBillDeletesIt() {
	orphan := s.artist("Remembering Ray Gutierrez Memorial Concert")

	out, summary := s.run(csvOf(*orphan.Slug+",remove,"), true)

	s.False(summary.Refused(), out)
	s.Nil(s.artistByID(orphan.ID))
}

// Taking the first act off a bill that states no roles moves the headline slot
// to the next act, and the dry run says so.
func (s *TriageSuite) TestRemoveFromAnUncuratedBillNamesTheNewHeadliner() {
	junk := s.artist("Presents Night")
	next := s.artist("Real Band")
	sh := s.show("", act{junk, "performer"}, act{next, "performer"})

	out, summary := s.run(csvOf(*junk.Slug+",remove,"), true)

	s.False(summary.Refused(), out)
	s.Contains(out, `the artist held the headline slot; it is now held by "Real Band"`)
	s.Equal([]string{"Real Band:performer"}, s.bill(sh.ID))
}

// A show_slug must name the only show the artist is billed on.
func (s *TriageSuite) TestShowSlugMustNameTheOnlyBill() {
	junk := s.artist("Jazz Night")
	a := s.show("", act{s.artist("Band A"), "headliner"}, act{junk, "performer"})
	b := s.show("", act{s.artist("Band B"), "headliner"}, act{junk, "performer"})

	out, summary := s.run(csvOf(*junk.Slug+",remove,,"+*a.Slug), true)

	s.True(summary.Refused(), out)
	s.Contains(out, "is billed on 2 show(s): "+*a.Slug+", "+*b.Slug)
	s.NotNil(s.artistByID(junk.ID))

	out, summary = s.run(csvOf(*junk.Slug+",remove,"), true)

	s.False(summary.Refused(), out)
	s.Equal([]string{"Band A:headliner"}, s.bill(a.ID))
	s.Equal([]string{"Band B:headliner"}, s.bill(b.ID))
}

// ── split ──────────────────────────────────────────────

// A split puts the acts in the joined entity's place, in order and with its
// role, links an existing artist of the same name, creates the others and
// deletes the joined entity.
func (s *TriageSuite) TestSplitReplacesTheBillRowInOrder() {
	head := s.artist("Ouija Macc")
	joined := s.artist("Darby O'Trill and Mikey Rotten")
	existing := s.artist("Darby O'Trill")
	last := s.artist("Closer")
	sh := s.show("", act{head, "headliner"}, act{joined, "direct_support"}, act{last, "opener"})
	fan := s.follow(joined)
	csv := csvOf(*joined.Slug + ",split,darby o'trill | Mikey Rotten,,feed bills three acts")

	out, summary := s.run(csv, false)

	s.False(summary.Refused(), out)
	s.Contains(out, `act "darby o'trill": links the existing artist: "Darby O'Trill" id=`)
	s.Contains(out, `act "Mikey Rotten": creates a new artist`)
	s.Equal(int64(0), s.count(`SELECT COUNT(*) FROM artists WHERE name = 'Mikey Rotten'`), "the dry run creates nothing")

	out, summary = s.run(csv, true)

	s.False(summary.Refused(), out)
	s.Equal([]string{"Ouija Macc:headliner", "Darby O'Trill:direct_support", "Mikey Rotten:direct_support", "Closer:opener"}, s.bill(sh.ID))
	s.Nil(s.artistByID(joined.ID))
	s.NotNil(s.artistByID(existing.ID))
	var created catalogm.Artist
	s.Require().NoError(s.db.Where("name = ?", "Mikey Rotten").First(&created).Error)
	s.Equal("mikey-rotten", *created.Slug)
	s.Equal(int64(0), s.count(`SELECT COUNT(*) FROM user_bookmarks WHERE user_id = ?`, fan), "the joined entity's follow goes with it")
}

func (s *TriageSuite) TestSplitRefusals() {
	head := s.artist("Darby O'Trill")
	joined := s.artist("Darby O'Trill and Mikey Rotten")
	sh := s.show("", act{head, "headliner"}, act{joined, "performer"})
	orphan := s.artist("Scrape and We Follow The Earth")

	out, summary := s.run(csvOf(
		*joined.Slug+",split,Darby O'Trill | Mikey Rotten",
		*orphan.Slug+",split,Scrape | We Follow The Earth",
	), true)

	s.Equal(2, summary[statusRefused], out)
	s.Contains(out, `act "Darby O'Trill" is already on the bill of show `+*sh.Slug)
	s.Contains(out, "is on no bill, so there is nothing to split")
	s.NotNil(s.artistByID(joined.ID))
	s.Equal(int64(0), s.count(`SELECT COUNT(*) FROM artists WHERE name IN ('Mikey Rotten', 'Scrape')`))
}

// ── keep, idempotence, order, entanglement, actor ──────

func (s *TriageSuite) TestKeepChangesNothing() {
	a := s.artist("Ganja White Night")

	out, summary := s.run(csvOf(*a.Slug+",keep,,,real duo"), true)

	s.False(summary.Refused(), out)
	s.Equal(1, summary[statusKept])
	s.Equal("Ganja White Night", s.artistByID(a.ID).Name)
}

// A second run over the same file changes nothing: a rename and a merge are
// reported as done, a keep is kept, and a removed or split artist is reported
// as gone with a note that an earlier run may have applied the row.
func (s *TriageSuite) TestSecondRunChangesNothing() {
	renamed := s.artist("Riot Act (Pearl Jam Tribute)")
	keep := s.artist("Harry Katz & The Pistachios")
	dup := s.artist("Harry Katz and the Pistachios")
	junk := s.artist("LIVE AT LAUNCHPAD")
	joined := s.artist("Darby O'Trill and Mikey Rotten")
	kept := s.artist("Our Last Night")
	s.show("", act{keep, "headliner"}, act{dup, "performer"})
	s.show("", act{s.artist("Ouija Macc"), "headliner"}, act{junk, "performer"}, act{joined, "performer"})
	s.show("", act{renamed, "headliner"})
	csv := csvOf(
		*renamed.Slug+",rename,Riot Act",
		*dup.Slug+",merge,"+*keep.Slug,
		*junk.Slug+",remove,",
		*kept.Slug+",keep,",
	)
	splitCSV := csvOf(*joined.Slug + ",split,Darby O'Trill | Mikey Rotten")

	out, summary := s.run(csv, true)
	s.Require().False(summary.Refused(), out)
	s.Require().Equal(3, summary[statusApplied], out)
	out, summary = s.run(splitCSV, true)
	s.Require().False(summary.Refused(), out)
	artists := s.count(`SELECT COUNT(*) FROM artists`)
	billRows := s.count(`SELECT COUNT(*) FROM show_artists`)
	audits := s.count(`SELECT COUNT(*) FROM audit_logs`)
	revisions := s.count(`SELECT COUNT(*) FROM revisions`)

	out, summary = s.run(csv, true)

	s.Equal(0, summary[statusApplied], out)
	s.Equal(2, summary[statusDone], out)
	s.Equal(1, summary[statusKept], out)
	s.Equal(1, summary[statusRefused], out)
	s.Contains(out, "an audit record shows "+*dup.Slug+" was merged into "+*keep.Slug)
	s.Contains(out, "no artist has slug live-at-launchpad; if an earlier run of this file applied this row")

	out, summary = s.run(splitCSV, true)

	s.Equal(1, summary[statusRefused], out)
	s.Contains(out, "no artist has slug "+*joined.Slug+"; if an earlier run")
	s.Equal(artists, s.count(`SELECT COUNT(*) FROM artists`))
	s.Equal(billRows, s.count(`SELECT COUNT(*) FROM show_artists`))
	s.Equal(audits, s.count(`SELECT COUNT(*) FROM audit_logs`))
	s.Equal(revisions, s.count(`SELECT COUNT(*) FROM revisions`))
}

// A missing artist counts as merged only when an audit row names both its slug
// and this target.
func (s *TriageSuite) TestMissingArtistWithoutAMergeRecordIsRefused() {
	keep := s.artist("Lone Stranger")
	other := s.artist("Other Act")
	dup := s.artist("Lone Stranger & Friends")
	dupSlug := *dup.Slug
	_, summary := s.run(csvOf(dupSlug+",merge,"+*other.Slug), true)
	s.Require().False(summary.Refused())

	out, summary := s.run(csvOf(dupSlug+",merge,"+*keep.Slug), false)

	s.True(summary.Refused(), "merged into a different artist than this row names")
	s.Contains(out, "no artist has slug "+dupSlug)
}

// Rows whose writes depend on each other are refused, and the verdicts are the
// same whichever order the file lists them in.
func (s *TriageSuite) TestEntangledRowsAreRefusedInEitherOrder() {
	chainA := s.artist("Chain A")
	chainB := s.artist("Chain B")
	chainC := s.artist("Chain C")
	twinA := s.artist("Twin Copy One")
	twinB := s.artist("Twin Copy Two")
	twinTarget := s.artist("Twin")
	renameX := s.artist("Rename X Night")
	renameY := s.artist("Rename Y Night")
	splitJoined := s.artist("Linked Act and New Act")
	linkedAct := s.artist("Linked Act")
	billA := s.artist("Bill Junk One")
	billB := s.artist("Bill Junk Two")
	removedTarget := s.artist("Removed Target Night")
	intoRemoved := s.artist("Into Removed")
	free := s.artist("Free Row Night")
	head := s.artist("Head")
	s.show("", act{head, "headliner"}, act{splitJoined, "performer"})
	s.show("", act{s.artist("Head Two"), "headliner"}, act{billA, "performer"}, act{billB, "performer"})
	s.show("", act{s.artist("Head Three"), "headliner"}, act{removedTarget, "performer"})
	s.show("", act{s.artist("Head Four"), "headliner"}, act{free, "performer"})

	rows := []string{
		*chainA.Slug + ",merge," + *chainB.Slug,     // target merged away below
		*chainB.Slug + ",merge," + *chainC.Slug,     // merged away and a target above
		*twinA.Slug + ",merge," + *twinTarget.Slug,  // two merges into one target
		*twinB.Slug + ",merge," + *twinTarget.Slug,  // two merges into one target
		*renameX.Slug + ",rename,Same New Name",     // two renames to one name
		*renameY.Slug + ",rename,same new name",     // two renames to one name
		*splitJoined.Slug + ",split,Linked Act | X", // links an act renamed below
		*linkedAct.Slug + ",rename,Linked Act Renamed",
		*billA.Slug + ",remove,", // two rows rewrite one bill
		*billB.Slug + ",remove,", // two rows rewrite one bill
		*intoRemoved.Slug + ",merge," + *removedTarget.Slug,
		*removedTarget.Slug + ",remove,",
		*free.Slug + ",remove,", // independent of every other row
	}
	reversed := make([]string, len(rows))
	for i, r := range rows {
		reversed[len(rows)-1-i] = r
	}
	before := s.count(`SELECT COUNT(*) FROM artists`)

	verdicts := map[string]map[string]rowStatus{}
	for name, file := range map[string][]string{"file order": rows, "reversed": reversed} {
		out, summary := s.run(csvOf(file...), false)
		s.Equal(len(rows)-1, summary[statusRefused], "%s: %s", name, out)
		s.Equal(1, summary[statusWouldApply], "%s: %s", name, out)
		verdicts[name] = map[string]rowStatus{}
		parsed, err := parseWorklist(strings.NewReader(csvOf(file...)))
		s.Require().NoError(err)
		for _, r := range parsed {
			verdicts[name][r.ArtistSlug] = statusOfLine(out, r.Line)
		}
		for _, want := range []string{
			"target chain-b is also changed on line",
			"artist chain-b receives the merge on line",
			"target twin receives merges on lines",
			`would each give an artist the name "same new name"`,
			"an act it puts on the bill is changed on line",
			"is an act the split on line",
			"each rewrite the bill of show",
			"target removed-target-night is also changed on line",
		} {
			s.Contains(out, want, name)
		}
	}
	s.Equal(verdicts["file order"], verdicts["reversed"])
	s.Equal(statusWouldApply, verdicts["file order"][*free.Slug])
	s.Equal(before, s.count(`SELECT COUNT(*) FROM artists`), "dry runs write nothing")

	out, summary := s.run(csvOf(rows...), true)

	s.Equal(1, summary[statusApplied], out)
	s.Nil(s.artistByID(free.ID))
	s.Equal(before-1, s.count(`SELECT COUNT(*) FROM artists`), "no refused row changed an artist")
}

// statusOfLine reads the status printed for one line of the report.
func statusOfLine(out string, line int) rowStatus {
	block := out[strings.Index(out, fmt.Sprintf("[line %d] ", line)):]
	if end := strings.Index(block, "\n\n"); end >= 0 {
		block = block[:end]
	}
	for _, st := range []rowStatus{statusRefused, statusWouldApply, statusApplied, statusDone, statusKept} {
		if strings.Contains(block, "  "+string(st)) {
			return st
		}
	}
	return ""
}

func (s *TriageSuite) TestAnArtistOnTwoRowsIsRefused() {
	a := s.artist("Twice Listed Night")

	out, summary := s.run(csvOf(*a.Slug+",keep,", *a.Slug+",rename,Twice Listed"), true)

	s.Equal(2, summary[statusRefused], out)
	s.Contains(out, "appears on 2 rows (lines 2, 3)")
	s.Equal("Twice Listed Night", s.artistByID(a.ID).Name)
}

// A merge already done by an earlier run blocks a later row that would change
// the artist it went into.
func (s *TriageSuite) TestChangingTheTargetOfADoneMergeIsRefused() {
	keep := s.artist("Lone Stranger")
	dup := s.artist("Lone Stranger & Friends")
	mergeRow := *dup.Slug + ",merge," + *keep.Slug
	_, summary := s.run(csvOf(mergeRow), true)
	s.Require().False(summary.Refused())

	out, summary := s.run(csvOf(mergeRow, *keep.Slug+",rename,Lone Stranger Band"), true)

	s.Equal(1, summary[statusDone], out)
	s.Equal(1, summary[statusRefused], out)
	s.Contains(out, "artist lone-stranger already received the merge on line 2")
	s.Equal("Lone Stranger", s.artistByID(keep.ID).Name)
}

func (s *TriageSuite) TestTheActorMustBeAnActiveAdmin() {
	s.NoError(requireAdminActor(s.db, s.actor))
	s.ErrorContains(requireAdminActor(s.db, 0), "--actor is required")
	s.ErrorContains(requireAdminActor(s.db, s.actor+1000), "no such user")
	fan := s.user()
	s.ErrorContains(requireAdminActor(s.db, fan), "not an active admin")
	s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", s.actor).Update("is_active", false).Error)
	s.ErrorContains(requireAdminActor(s.db, s.actor), "not an active admin")
}
