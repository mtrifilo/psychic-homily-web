package main

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/admin"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared/revisiondiff"
)

// Summaries on the revisions this tool records.
const (
	renameRevisionSummary = "Renamed from a reviewed artist triage worklist"
	titleRevisionSummary  = "Title taken from a non-performer removed from the bill (reviewed artist triage worklist)"
)

// auditActionMergeArtists is the audit action the admin merge endpoint records.
// The tool records the same action and adds the merged artist's slug to its
// metadata, which is how a second run recognises a merge it already applied.
const auditActionMergeArtists = "merge_artists"

// mergedSlugKey is the metadata key that carries the merged artist's slug.
const mergedSlugKey = "merged_artist_slug"

// errDryRunRollback ends a planning transaction after the real write has run,
// so the dry run reports exactly what the write would do.
var errDryRunRollback = errors.New("dry run rollback")

// triage runs rows against one database on behalf of one admin.
type triage struct {
	db    *gorm.DB
	actor uint
}

// plan reaches a row's verdict without writing: a row with work to do runs its
// write in a transaction that is rolled back.
func (t *triage) plan(p *rowPlan) {
	subject, err := artistBySlug(t.db, p.Row.ArtistSlug)
	if err != nil {
		p.refuse("load artist %s: %v", p.Row.ArtistSlug, err)
		return
	}
	if subject == nil {
		t.planMissingSubject(p)
		return
	}
	p.SubjectID = subject.ID

	switch p.Row.Action {
	case actionKeep:
		p.Detail = []string{describeArtist(t.db, "artist", subject)}
		p.Status = statusKept
		return
	case actionRename:
		if subject.Name == p.Row.Target {
			p.Detail = []string{describeArtist(t.db, "artist", subject), fmt.Sprintf("already named %q", p.Row.Target)}
			p.Status = statusDone
			return
		}
	}

	detail, err := t.runRow(p, true)
	p.Detail = detail
	if !errors.Is(err, errDryRunRollback) {
		p.refuse("%v", err)
		return
	}
	p.Status = statusWouldApply
}

// planMissingSubject decides a row whose artist slug names no artist. Only a
// merge can tell that an earlier run did the work, from the audit record the
// merge left.
func (t *triage) planMissingSubject(p *rowPlan) {
	if p.Row.Action != actionMerge {
		hint := ""
		if p.Row.Action == actionRemove || p.Row.Action == actionSplit {
			hint = "; if an earlier run of this file applied this row, the artist is gone and the row can be dropped"
		}
		p.refuse("no artist has slug %s%s", p.Row.ArtistSlug, hint)
		return
	}
	into, err := artistBySlug(t.db, p.Row.Target)
	if err != nil {
		p.refuse("load target %s: %v", p.Row.Target, err)
		return
	}
	if into == nil {
		p.refuse("no artist has slug %s, and no artist has target slug %s", p.Row.ArtistSlug, p.Row.Target)
		return
	}
	merged, err := mergeRecorded(t.db, p.Row.ArtistSlug, into.ID)
	if err != nil {
		p.refuse("check the merge record: %v", err)
		return
	}
	if !merged {
		p.refuse("no artist has slug %s", p.Row.ArtistSlug)
		return
	}
	p.TargetID = into.ID
	p.Status = statusDone
	p.Detail = []string{
		fmt.Sprintf("an audit record shows %s was merged into %s", p.Row.ArtistSlug, p.Row.Target),
		describeArtist(t.db, "into", into),
	}
}

// apply writes one accepted row in its own transaction. A failure leaves the
// row unapplied and is reported as a refusal.
func (t *triage) apply(p *rowPlan) {
	detail, err := t.runRow(p, false)
	if err == nil {
		p.Detail = detail
		p.Status = statusApplied
		return
	}
	// An error can come at COMMIT, which does not prove the commit failed. What
	// the row leaves behind decides.
	done, checkErr := t.rowTookEffect(p)
	switch {
	case checkErr != nil:
		p.refuse("%s outcome unknown: %v; could not check the database: %v", p.Row.Action, err, checkErr)
	case done:
		p.Detail = append(detail, fmt.Sprintf("the write reported an error but its result is in the database, so it committed: %v", err))
		p.Status = statusApplied
	default:
		p.refuse("%s failed, nothing changed: %v", p.Row.Action, err)
	}
}

// rowTookEffect reports whether an applied row's change is in the database: a
// rename's new name is on the artist, and every other action deleted the
// artist.
func (t *triage) rowTookEffect(p *rowPlan) (bool, error) {
	var a catalogm.Artist
	err := t.db.First(&a, p.SubjectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return p.Row.Action != actionRename, nil
	}
	if err != nil {
		return false, err
	}
	return p.Row.Action == actionRename && a.Name == p.Row.Target, nil
}

// runRow runs a row's write in its own transaction and returns the lines that
// describe it. With rollback set the transaction is always rolled back, and a
// write that succeeded returns errDryRunRollback.
func (t *triage) runRow(p *rowPlan, rollback bool) ([]string, error) {
	var detail []string
	err := t.db.Transaction(func(tx *gorm.DB) error {
		if err := setRowTimeouts(tx); err != nil {
			return err
		}
		d, err := t.execute(tx, p)
		detail = d
		if err == nil && rollback {
			return errDryRunRollback
		}
		return err
	})
	return detail, err
}

// setRowTimeouts bounds how long a row's transaction waits on a lock the site
// holds and how long one statement may run, so a busy row fails instead of
// stalling the run or the site. Both settings last for the transaction only.
func setRowTimeouts(tx *gorm.DB) error {
	if err := tx.Exec("SELECT set_config('lock_timeout', '5s', true), set_config('statement_timeout', '60s', true)").Error; err != nil {
		return fmt.Errorf("set the row's lock and statement timeouts: %w", err)
	}
	return nil
}

// execute performs a row's write inside tx and returns the lines that
// describe it. The plan and the confirmed run both call it, so the preview and
// the write are one code path. It fills the row's footprint as it goes.
func (t *triage) execute(tx *gorm.DB, p *rowPlan) ([]string, error) {
	subject, err := lockArtist(tx, p.SubjectID)
	if err != nil {
		return nil, err
	}
	switch p.Row.Action {
	case actionRename:
		return t.rename(tx, p, subject)
	case actionMerge:
		return t.merge(tx, p, subject)
	case actionRemove:
		return t.remove(tx, p, subject)
	case actionSplit:
		return t.split(tx, p, subject)
	}
	return nil, fmt.Errorf("action %q has no write", p.Row.Action)
}

// rename changes the artist's name through ArtistService.UpdateArtist, the
// admin edit path, and keeps the slug exactly as it was: UpdateArtist derives a
// new slug from the new name, and the tool writes the old one back in the same
// transaction, so the artist's URL keeps answering. The change is recorded the
// way the admin edit records it: a revision and an entity edit audit row, both
// naming the actor.
func (t *triage) rename(tx *gorm.DB, p *rowPlan, subject *catalogm.Artist) ([]string, error) {
	name := p.Row.Target
	detail := []string{describeArtist(tx, "artist", subject)}

	// UpdateArtist refuses the same name; checking first lets the refusal name
	// the other artist's slug.
	other, err := artistByName(tx, name, subject.ID)
	if err != nil {
		return detail, err
	}
	if other != nil {
		return detail, fmt.Errorf("artist %q (slug %s) already has that name; merge into it instead", other.Name, slugOf(other))
	}
	aliasOwner, err := aliasOwnerOf(tx, name, subject.ID)
	if err != nil {
		return detail, err
	}
	if aliasOwner != nil {
		return detail, fmt.Errorf("%q is an alias of %q (slug %s); merge into it instead", name, aliasOwner.Name, slugOf(aliasOwner))
	}

	artists := catalog.NewArtistService(tx)
	before, err := artists.GetArtist(subject.ID)
	if err != nil {
		return detail, fmt.Errorf("read artist: %w", err)
	}
	after, err := artists.UpdateArtist(subject.ID, &contracts.UpdateArtistRequest{Name: &name})
	if err != nil {
		return detail, fmt.Errorf("rename: %w", err)
	}
	if err := tx.Model(&catalogm.Artist{}).Where("id = ?", subject.ID).Update("slug", subject.Slug).Error; err != nil {
		return detail, fmt.Errorf("keep the slug: %w", err)
	}
	reread, err := lockArtist(tx, subject.ID)
	if err != nil {
		return detail, err
	}
	if slugOf(reread) != slugOf(subject) || reread.Name != name {
		return detail, fmt.Errorf("after the rename the artist reads name %q slug %s, want %q and %s",
			reread.Name, slugOf(reread), name, slugOf(subject))
	}

	changes := revisiondiff.Compare(before, after, revisiondiff.ArtistFields)
	if err := admin.NewRevisionService(tx).RecordRevision("artist", subject.ID, t.actor, changes, renameRevisionSummary); err != nil {
		return detail, fmt.Errorf("record rename revision: %w", err)
	}
	admin.NewAuditLogService(tx).LogEntityEdit(t.actor, "artist", subject.ID, nil)

	p.NewNames = []string{strings.ToLower(name)}
	return append(detail,
		fmt.Sprintf("name: %q -> %q", subject.Name, name),
		fmt.Sprintf("slug: %s (unchanged)", slugOf(subject))), nil
}

// merge merges the artist into the target through ArtistService.MergeArtists,
// the service behind POST /admin/artists/merge, and records the audit row that
// endpoint records, with the merged artist's slug added.
func (t *triage) merge(tx *gorm.DB, p *rowPlan, subject *catalogm.Artist) ([]string, error) {
	detail := []string{describeArtist(tx, "from", subject)}
	into, err := artistBySlug(tx, p.Row.Target)
	if err != nil {
		return detail, fmt.Errorf("load target %s: %w", p.Row.Target, err)
	}
	if into == nil {
		return detail, fmt.Errorf("no artist has target slug %s", p.Row.Target)
	}
	p.TargetID = into.ID
	detail = append(detail, describeArtist(tx, "into", into))

	handMadeCount, err := countAll(tx, subject.ID, []labeledCount{handMadeRelationships})
	if err != nil {
		return detail, err
	}
	handMade := handMadeCount[0]
	lost := linksTheTargetLacks(subject, into)

	result, err := catalog.NewArtistService(tx).MergeArtists(into.ID, subject.ID)
	if err != nil {
		return detail, err
	}

	admin.NewAuditLogService(tx).LogAction(t.actor, auditActionMergeArtists, "artist", into.ID, map[string]interface{}{
		"merged_artist_id":   subject.ID,
		"merged_artist_name": result.MergedArtistName,
		"shows_moved":        result.ShowsMoved,
		mergedSlugKey:        slugOf(subject),
	})
	recorded, err := mergeRecorded(tx, slugOf(subject), into.ID)
	if err != nil {
		return detail, fmt.Errorf("check the merge's audit row: %w", err)
	}
	if !recorded {
		return detail, fmt.Errorf("the merge's audit row was not written, so a later run could not recognise the merge")
	}

	detail = append(detail, fmt.Sprintf("merge: bills_moved=%d releases_moved=%d labels_moved=%d festivals_moved=%d "+
		"relationships_deleted=%d follows_moved=%d crate_items_moved=%d alert_filters_updated=%d",
		result.ShowsMoved, result.ReleasesMoved, result.LabelsMoved, result.FestivalsMoved,
		result.RelationshipsMoved, result.BookmarksMoved, result.CollectionItemsMoved, result.FiltersUpdated))
	if result.AliasCreated {
		detail = append(detail, fmt.Sprintf("alias: %q is added to %s", subject.Name, slugOf(into)))
	}
	detail = append(detail, fmt.Sprintf("after the merge /artists/%s answers 404; /artists/%s keeps its name and slug",
		slugOf(subject), slugOf(into)))
	if handMade > 0 {
		detail = append(detail, fmt.Sprintf("WARNING: the merge deletes %d hand-made relationship(s) of %s", handMade, slugOf(subject)))
	}
	if len(lost) > 0 {
		detail = append(detail, fmt.Sprintf("WARNING: the merge keeps %s's own fields; %s's %s are not carried over",
			slugOf(into), slugOf(subject), strings.Join(lost, ", ")))
	}
	return detail, nil
}

// remove takes a non-performer off every bill it is on, through
// ShowService.UpdateShowWithRelations, and deletes it through
// ArtistService.DeleteArtist. With a target of "title" its name is written to
// the title of each show first, and a show that already has a title refuses
// the row.
func (t *triage) remove(tx *gorm.DB, p *rowPlan, subject *catalogm.Artist) ([]string, error) {
	detail := []string{describeArtist(tx, "artist", subject)}
	if err := refuseRealAct(tx, subject); err != nil {
		return detail, err
	}
	shows, err := billedShows(tx, p, subject)
	if err != nil {
		return detail, err
	}
	writeTitle := strings.EqualFold(p.Row.Target, removeTargetTitle)
	title := strings.TrimSpace(subject.Name)
	if writeTitle && len(shows) == 0 {
		return detail, fmt.Errorf("%s is on no bill, so there is no show title to write", slugOf(subject))
	}
	if writeTitle && len(title) > maxShowTitleLength {
		return detail, fmt.Errorf("the name is longer than the %d characters a show title holds", maxShowTitleLength)
	}

	showSvc := catalog.NewShowService(tx)
	for _, sh := range shows {
		bill, err := billOf(tx, sh.ID)
		if err != nil {
			return detail, err
		}
		var kept []billEntry
		for _, e := range bill {
			if e.ArtistID != subject.ID {
				kept = append(kept, e)
			}
		}
		if len(kept) == 0 {
			return detail, fmt.Errorf("it is the only act on show %s; taking it off would leave the show with no acts, "+
				"so decide that show first", slugText(sh.Slug))
		}
		req := &contracts.UpdateShowRequest{}
		var before *contracts.ShowResponse
		if writeTitle {
			if strings.TrimSpace(sh.Title) != "" {
				return detail, fmt.Errorf("show %s already has the title %q; it is not overwritten", slugText(sh.Slug), sh.Title)
			}
			if before, err = showSvc.GetShow(sh.ID); err != nil {
				return detail, fmt.Errorf("read show %s: %w", slugText(sh.Slug), err)
			}
			req.Title = &title
		}
		newBill := make([]contracts.CreateShowArtist, len(kept))
		for i, e := range kept {
			newBill[i] = keptAct(e)
		}
		after, line, err := rewriteBill(tx, p, sh, bill, req, newBill)
		if err != nil {
			return detail, err
		}
		detail = append(detail, line)
		if writeTitle {
			changes := revisiondiff.Compare(before, after.show, revisiondiff.ShowFields)
			if err := admin.NewRevisionService(tx).RecordRevision("show", sh.ID, t.actor, changes, titleRevisionSummary); err != nil {
				return detail, fmt.Errorf("record title revision: %w", err)
			}
			detail = append(detail, fmt.Sprintf("  title: (empty) -> %q", title))
		}
		if headlineSlot(bill)[subject.ID] {
			detail = append(detail, "  WARNING: "+headlineSlotWarning(after.bill))
		}
	}

	deleted, err := deleteArtist(tx, t.actor, subject)
	return append(detail, deleted...), err
}

// split replaces the joined entity on every bill it is on with its acts, in
// order, through ShowService.UpdateShowWithRelations, whose find-or-create
// (FindOrCreateArtistTx) links an existing artist of the same name or creates
// one; each act takes the joined entity's role. The joined entity is then
// deleted through ArtistService.DeleteArtist.
func (t *triage) split(tx *gorm.DB, p *rowPlan, subject *catalogm.Artist) ([]string, error) {
	detail := []string{describeArtist(tx, "artist", subject)}
	acts, err := splitActNames(p.Row.Target, subject.Name)
	if err != nil {
		return detail, err
	}
	if err := refuseRealAct(tx, subject); err != nil {
		return detail, err
	}
	shows, err := billedShows(tx, p, subject)
	if err != nil {
		return detail, err
	}
	if len(shows) == 0 {
		return detail, fmt.Errorf("%s is on no bill, so there is nothing to split; remove deletes it", slugOf(subject))
	}

	found := make([]*catalogm.Artist, len(acts))
	for i, name := range acts {
		a, err := artistByName(tx, name, 0)
		if err != nil {
			return detail, err
		}
		found[i] = a
		if a != nil {
			p.FoundActIDs = append(p.FoundActIDs, a.ID)
			detail = append(detail, fmt.Sprintf("act %q: links the existing %s", name, describeArtist(tx, "artist", a)))
			continue
		}
		p.NewNames = append(p.NewNames, strings.ToLower(name))
		line := fmt.Sprintf("act %q: creates a new artist", name)
		owner, err := aliasOwnerOf(tx, name, 0)
		if err != nil {
			return detail, err
		}
		if owner != nil {
			line += fmt.Sprintf("; WARNING: %q is an alias of %q (slug %s), which is not linked", name, owner.Name, slugOf(owner))
		}
		detail = append(detail, line)
	}

	for _, sh := range shows {
		bill, err := billOf(tx, sh.ID)
		if err != nil {
			return detail, err
		}
		var newBill []contracts.CreateShowArtist
		for _, e := range bill {
			if e.ArtistID != subject.ID {
				for _, a := range found {
					if a != nil && a.ID == e.ArtistID {
						return detail, fmt.Errorf("act %q is already on the bill of show %s", a.Name, slugText(sh.Slug))
					}
				}
				newBill = append(newBill, keptAct(e))
				continue
			}
			for i, name := range acts {
				act := contracts.CreateShowArtist{Name: name}
				if found[i] != nil {
					id := found[i].ID
					act = contracts.CreateShowArtist{ID: &id}
				}
				setRole(&act, e.SetType)
				newBill = append(newBill, act)
			}
		}
		_, line, err := rewriteBill(tx, p, sh, bill, &contracts.UpdateShowRequest{}, newBill)
		if err != nil {
			return detail, err
		}
		detail = append(detail, line)
	}

	deleted, err := deleteArtist(tx, t.actor, subject)
	return append(detail, deleted...), err
}

// deleteArtist deletes an artist that is on no bill through
// ArtistService.DeleteArtist, as an admin. The artist's derived graph edges are
// deleted first through ArtistRelationshipService.DeleteRelationship, because
// artist_relationships references artists with no ON DELETE clause and the
// database refuses the delete while any edge remains; refuseRealAct has already
// refused an artist with a hand-made edge or a vote. It returns lines saying
// what the delete takes with it.
func deleteArtist(tx *gorm.DB, actor uint, subject *catalogm.Artist) ([]string, error) {
	collateral, err := deleteCollateral(tx, subject.ID)
	if err != nil {
		return nil, err
	}
	var edges []catalogm.ArtistRelationship
	if err := tx.Where("(source_artist_id = ? OR target_artist_id = ?) AND auto_derived", subject.ID, subject.ID).
		Find(&edges).Error; err != nil {
		return nil, fmt.Errorf("load derived edges: %w", err)
	}
	relationships := catalog.NewArtistRelationshipService(tx)
	for _, e := range edges {
		if err := relationships.DeleteRelationship(e.SourceArtistID, e.TargetArtistID, e.RelationshipType); err != nil {
			return nil, fmt.Errorf("delete derived edge %d-%d %s: %w", e.SourceArtistID, e.TargetArtistID, e.RelationshipType, err)
		}
	}
	if err := catalog.NewArtistService(tx).DeleteArtist(subject.ID, contracts.EntityDeleteActor{UserID: actor, IsAdmin: true}); err != nil {
		return nil, fmt.Errorf("delete artist: %w", err)
	}
	return []string{
		fmt.Sprintf("delete: %s; %d derived graph edge(s) deleted", collateral, len(edges)),
		fmt.Sprintf("after the delete /artists/%s answers 404", slugOf(subject)),
	}, nil
}

// rewrittenShow is a show as a bill rewrite left it.
type rewrittenShow struct {
	show *contracts.ShowResponse
	bill []billEntry
}

// rewriteBill writes newBill (and req's scalar fields) to the show through
// ShowService.UpdateShowWithRelations, records the show in the row's
// footprint, and returns the show as written with a line comparing its bill
// before and after.
func rewriteBill(tx *gorm.DB, p *rowPlan, sh billedShow, before []billEntry,
	req *contracts.UpdateShowRequest, newBill []contracts.CreateShowArtist) (rewrittenShow, string, error) {
	slug := slugText(sh.Slug)
	show, _, err := catalog.NewShowService(tx).UpdateShowWithRelations(sh.ID, req, nil, newBill, true)
	if err != nil {
		return rewrittenShow{}, "", fmt.Errorf("update show %s: %w", slug, err)
	}
	p.BillShows = append(p.BillShows, showRef{ID: sh.ID, Slug: slug})
	after, err := billOf(tx, sh.ID)
	if err != nil {
		return rewrittenShow{}, "", err
	}
	line := fmt.Sprintf("show %s (%s): bill %s -> %s", slug, sh.Status, describeBill(before), describeBill(after))
	return rewrittenShow{show: show, bill: after}, line, nil
}

// keptAct is the request for an act that stays on a rewritten bill, carrying
// its stored role so the rewrite changes no other act's role.
func keptAct(e billEntry) contracts.CreateShowArtist {
	id := e.ArtistID
	a := contracts.CreateShowArtist{ID: &id}
	setRole(&a, e.SetType)
	return a
}

// setRole states an act's role explicitly so the bill rewrite never infers one
// from position: a stored role outside the vocabulary is written as a
// non-headliner.
func setRole(a *contracts.CreateShowArtist, setType string) {
	if contracts.IsValidSetType(setType) {
		st := setType
		a.SetType = &st
		isHeadliner := st == contracts.SetTypeHeadliner
		a.IsHeadliner = &isHeadliner
		return
	}
	notHeadliner := false
	a.IsHeadliner = &notHeadliner
}

// headlineSlot reads a bill the way headlineSlotSQL does: on a bill that states
// any role other than performer, the headline slot is the acts billed as
// headliner; on a bill that states none, it is the act at position 0.
func headlineSlot(bill []billEntry) map[uint]bool {
	curated := false
	for _, e := range bill {
		if e.SetType != "" && e.SetType != contracts.SetTypePerformer {
			curated = true
		}
	}
	slot := map[uint]bool{}
	for _, e := range bill {
		if (curated && e.SetType == contracts.SetTypeHeadliner) || (!curated && e.Position == 0) {
			slot[e.ArtistID] = true
		}
	}
	return slot
}

// headlineSlotWarning says who holds the headline slot once the artist that
// held it is off the bill.
func headlineSlotWarning(after []billEntry) string {
	var names []string
	slot := headlineSlot(after)
	for _, e := range after {
		if slot[e.ArtistID] {
			names = append(names, fmt.Sprintf("%q", e.Name))
		}
	}
	if len(names) == 0 {
		return "the artist held the headline slot; the bill is left with no headliner"
	}
	return fmt.Sprintf("the artist held the headline slot; it is now held by %s", strings.Join(names, ", "))
}

// lockArtist reads an artist under a row lock.
func lockArtist(tx *gorm.DB, id uint) (*catalogm.Artist, error) {
	var a catalogm.Artist
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("artist %d no longer exists", id)
		}
		return nil, fmt.Errorf("lock artist %d: %w", id, err)
	}
	return &a, nil
}

// firstArtistOrNil returns the first artist the query finds, or nil when it
// finds none.
func firstArtistOrNil(q *gorm.DB) (*catalogm.Artist, error) {
	var a catalogm.Artist
	err := q.First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// artistBySlug returns the artist with this slug, or nil when none has it.
func artistBySlug(db *gorm.DB, slug string) (*catalogm.Artist, error) {
	return firstArtistOrNil(db.Where("slug = ?", slug))
}

// artistByName returns the artist other than exceptID whose name matches name
// without regard to case (the rule the unique index artists_lower_name_uniq and
// FindOrCreateArtistTx apply), or nil.
func artistByName(db *gorm.DB, name string, exceptID uint) (*catalogm.Artist, error) {
	a, err := firstArtistOrNil(db.Where("LOWER(name) = LOWER(?) AND id <> ?", name, exceptID))
	if err != nil {
		return nil, fmt.Errorf("look up artist named %q: %w", name, err)
	}
	return a, nil
}

// aliasOwnerOf returns the artist other than exceptID that carries name as an
// alias, or nil.
func aliasOwnerOf(db *gorm.DB, name string, exceptID uint) (*catalogm.Artist, error) {
	a, err := firstArtistOrNil(db.Order("id").
		Where("id <> ? AND id IN (SELECT artist_id FROM artist_aliases WHERE LOWER(alias) = LOWER(?))", exceptID, name))
	if err != nil {
		return nil, fmt.Errorf("look up alias %q: %w", name, err)
	}
	return a, nil
}

// mergeRecorded reports whether an audit row records that the artist with this
// slug was merged into intoID.
func mergeRecorded(db *gorm.DB, slug string, intoID uint) (bool, error) {
	var n int64
	err := db.Table("audit_logs").
		Where("action = ? AND entity_type = 'artist' AND entity_id = ? AND metadata->>? = ?",
			auditActionMergeArtists, intoID, mergedSlugKey, slug).
		Count(&n).Error
	return n > 0, err
}

// artistLink is one of the artist's link columns, named the way the worklist
// names it.
type artistLink struct {
	name  string
	value *string
}

// artistLinks are the artist's link columns in a fixed order; any non-blank
// one marks the artist as a real act.
func artistLinks(a *catalogm.Artist) []artistLink {
	return []artistLink{
		{"bandcamp", a.Social.Bandcamp},
		{"bandcamp_embed_url", a.BandcampEmbedURL},
		{"facebook", a.Social.Facebook},
		{"instagram", a.Social.Instagram},
		{"soundcloud", a.Social.SoundCloud},
		{"spotify", a.Social.Spotify},
		{"twitter", a.Social.Twitter},
		{"website", a.Social.Website},
		{"youtube", a.Social.YouTube},
	}
}

func (l artistLink) set() bool { return l.value != nil && strings.TrimSpace(*l.value) != "" }

func setLinks(a *catalogm.Artist) []string {
	var names []string
	for _, l := range artistLinks(a) {
		if l.set() {
			names = append(names, l.name)
		}
	}
	return names
}

// linksTheTargetLacks lists the merged artist's links whose column is blank on
// the target: MergeArtists keeps the target's fields and drops these.
func linksTheTargetLacks(from, into *catalogm.Artist) []string {
	intoLinks := artistLinks(into)
	var lost []string
	for i, l := range artistLinks(from) {
		if l.set() && !intoLinks[i].set() {
			lost = append(lost, l.name)
		}
	}
	return lost
}

// refuseRealAct refuses to delete an artist that carries what a real act has:
// a release credit, a link, a label, a festival lineup, a matched radio play,
// a hand-made relationship, or a vote on a relationship. Deleting it would drop
// those rows or, for radio plays, strand them without an artist.
func refuseRealAct(tx *gorm.DB, a *catalogm.Artist) error {
	var found []string
	if links := setLinks(a); len(links) > 0 {
		found = append(found, "links ("+strings.Join(links, ", ")+")")
	}
	counts, err := countAll(tx, a.ID, realActCounts)
	if err != nil {
		return err
	}
	for i, n := range counts {
		if n > 0 {
			found = append(found, fmt.Sprintf("%d %s", n, realActCounts[i].what))
		}
	}
	if len(found) == 0 {
		return nil
	}
	return fmt.Errorf("%q has %s: it looks like a real act, so it is not deleted; rename or merge it instead",
		a.Name, strings.Join(found, ", "))
}

// labeledCount is one COUNT(*) query reading the artist id as @id.
type labeledCount struct {
	what  string
	query string
}

// handMadeRelationships counts the artist's relationships a person made.
var handMadeRelationships = labeledCount{"hand-made relationship(s)",
	"SELECT COUNT(*) FROM artist_relationships WHERE (source_artist_id = @id OR target_artist_id = @id) AND NOT auto_derived"}

// realActCounts are the rows refuseRealAct refuses a delete over.
var realActCounts = []labeledCount{
	{"release credit(s)", "SELECT COUNT(*) FROM artist_releases WHERE artist_id = @id"},
	{"label(s)", "SELECT COUNT(*) FROM artist_labels WHERE artist_id = @id"},
	{"festival lineup(s)", "SELECT COUNT(*) FROM festival_artists WHERE artist_id = @id"},
	{"matched radio play(s)", "SELECT COUNT(*) FROM radio_plays WHERE artist_id = @id"},
	handMadeRelationships,
	{"relationship vote(s)", "SELECT COUNT(*) FROM artist_relationship_votes WHERE source_artist_id = @id OR target_artist_id = @id"},
}

// collateralCounts are the rows deleteCollateral reports.
var collateralCounts = []labeledCount{
	{"follow/save row(s)", "SELECT COUNT(*) FROM user_bookmarks WHERE entity_type = 'artist' AND entity_id = @id"},
	{"tag(s)", "SELECT COUNT(*) FROM entity_tags WHERE entity_type = 'artist' AND entity_id = @id"},
	{"crate item(s)", "SELECT COUNT(*) FROM collection_items WHERE entity_type = 'artist' AND entity_id = @id"},
	{"alias(es)", "SELECT COUNT(*) FROM artist_aliases WHERE artist_id = @id"},
}

// summaryCounts are the counts describeArtist prints.
var summaryCounts = []labeledCount{
	{"bills", "SELECT COUNT(*) FROM show_artists WHERE artist_id = @id"},
	{"releases", "SELECT COUNT(*) FROM artist_releases WHERE artist_id = @id"},
	{"follows", "SELECT COUNT(*) FROM user_bookmarks WHERE entity_type = 'artist' AND entity_id = @id"},
}

// countAll runs every count for one artist in a single query and returns them
// in order.
func countAll(db *gorm.DB, id uint, counts []labeledCount) ([]int64, error) {
	cols := make([]string, len(counts))
	for i, c := range counts {
		cols[i] = fmt.Sprintf("(%s) AS c%d", c.query, i)
	}
	row := map[string]interface{}{}
	if err := db.Raw("SELECT "+strings.Join(cols, ", "), map[string]interface{}{"id": id}).Scan(&row).Error; err != nil {
		return nil, fmt.Errorf("count rows for artist %d: %w", id, err)
	}
	out := make([]int64, len(counts))
	for i, c := range counts {
		n, ok := row[fmt.Sprintf("c%d", i)].(int64)
		if !ok {
			return nil, fmt.Errorf("count %s for artist %d: got %T", c.what, id, row[fmt.Sprintf("c%d", i)])
		}
		out[i] = n
	}
	return out, nil
}

// deleteCollateral describes rows DeleteArtist drops with the artist: follows
// and saves, tags, crate items and aliases among them (its sweep also clears
// comment subscriptions, tag votes and queued work). Comments, revisions and
// audit rows stay as history.
func deleteCollateral(tx *gorm.DB, id uint) (string, error) {
	counts, err := countAll(tx, id, collateralCounts)
	if err != nil {
		return "", err
	}
	parts := make([]string, len(counts))
	for i, n := range counts {
		parts[i] = fmt.Sprintf("%d %s", n, collateralCounts[i].what)
	}
	return "drops, among others, " + strings.Join(parts, ", ") + "; comments and revisions stay as history", nil
}

// billedShow is one show an artist is billed on.
type billedShow struct {
	ID     uint
	Slug   *string
	Title  string
	Status string
}

// billedShows returns every show the artist is billed on, each locked, in date
// order. A row naming a show_slug must name the only show the artist is on.
func billedShows(tx *gorm.DB, p *rowPlan, a *catalogm.Artist) ([]billedShow, error) {
	var shows []billedShow
	err := tx.Raw(`SELECT s.id, s.slug, s.title, s.status FROM shows s
		JOIN show_artists sa ON sa.show_id = s.id
		WHERE sa.artist_id = ? ORDER BY s.event_date, s.id FOR UPDATE OF s`, a.ID).Scan(&shows).Error
	if err != nil {
		return nil, fmt.Errorf("load the shows %s is billed on: %w", slugOf(a), err)
	}
	if p.Row.ShowSlug != "" {
		if len(shows) != 1 || slugText(shows[0].Slug) != p.Row.ShowSlug {
			slugs := make([]string, len(shows))
			for i, s := range shows {
				slugs[i] = slugText(s.Slug)
			}
			return nil, fmt.Errorf("the row names show %s, but %s is billed on %d show(s): %s; a row with a show_slug "+
				"must name the only show the artist is on, or leave show_slug empty to act on all of them",
				p.Row.ShowSlug, slugOf(a), len(shows), strings.Join(slugs, ", "))
		}
	}
	return shows, nil
}

// billEntry is one act on a bill.
type billEntry struct {
	ArtistID uint
	Position int
	SetType  string
	Name     string
}

func billOf(tx *gorm.DB, showID uint) ([]billEntry, error) {
	var bill []billEntry
	err := tx.Raw(`SELECT sa.artist_id, sa.position, COALESCE(sa.set_type, '') AS set_type, a.name
		FROM show_artists sa JOIN artists a ON a.id = sa.artist_id
		WHERE sa.show_id = ? ORDER BY sa.position, sa.artist_id`, showID).Scan(&bill).Error
	if err != nil {
		return nil, fmt.Errorf("load the bill of show %d: %w", showID, err)
	}
	return bill, nil
}

func describeBill(bill []billEntry) string {
	parts := make([]string, len(bill))
	for i, e := range bill {
		role := e.SetType
		if role == "" {
			role = "no role"
		}
		parts[i] = fmt.Sprintf("%q [%s]", e.Name, role)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// describeArtist is one line naming an artist the way a reviewer checks it.
func describeArtist(db *gorm.DB, label string, a *catalogm.Artist) string {
	stats := []string{"?", "?", "?"}
	if counts, err := countAll(db, a.ID, summaryCounts); err == nil {
		for i, n := range counts {
			stats[i] = fmt.Sprintf("%d", n)
		}
	}
	links := strings.Join(setLinks(a), "+")
	if links == "" {
		links = "none"
	}
	return fmt.Sprintf("%s: %q id=%d slug=%s bills=%s releases=%s follows=%s links=%s", label, a.Name, a.ID, slugOf(a),
		stats[0], stats[1], stats[2], links)
}

func slugOf(a *catalogm.Artist) string { return slugText(a.Slug) }

// slugText prints a nullable slug.
func slugText(s *string) string {
	if s == nil || *s == "" {
		return "(none)"
	}
	return *s
}
