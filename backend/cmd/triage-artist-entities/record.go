package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"gorm.io/gorm"
)

// recordLine is one line of the --record file: what one applied row changed
// and deleted, by id and slug. The admin paths this tool drives record nothing
// for a bill change or an artist delete, so for remove and split these lines
// are the only durable account of the bills as they were and of the rows the
// delete took with it.
type recordLine struct {
	At      string `json:"at"`
	Line    int    `json:"line"`
	Action  string `json:"action"`
	Outcome string `json:"outcome"`

	ArtistID   uint   `json:"artist_id"`
	ArtistSlug string `json:"artist_slug"`
	ArtistName string `json:"artist_name"`
	// ArtistDeleted is set by merge, remove and split: the row's artist is gone.
	ArtistDeleted bool `json:"artist_deleted,omitempty"`

	NewName        string `json:"new_name,omitempty"`
	KeptArtistID   uint   `json:"kept_artist_id,omitempty"`
	KeptArtistSlug string `json:"kept_artist_slug,omitempty"`

	Shows       []recordShow   `json:"shows,omitempty"`
	ActsLinked  []recordArtist `json:"acts_linked,omitempty"`
	ActsCreated []recordArtist `json:"acts_created,omitempty"`

	EdgesDeleted   []recordEdge     `json:"derived_edges_deleted,omitempty"`
	FollowsDropped []recordFollow   `json:"follows_dropped,omitempty"`
	TagsDropped    []recordTag      `json:"tags_dropped,omitempty"`
	CrateItems     []recordCrateRef `json:"crate_items_dropped,omitempty"`

	Reason string `json:"reason,omitempty"`
}

// recordShow is a show whose bill a row rewrote, with the bill before and after.
type recordShow struct {
	ShowID     uint        `json:"show_id"`
	ShowSlug   string      `json:"show_slug"`
	TitleSet   string      `json:"title_set,omitempty"`
	BillBefore []recordAct `json:"bill_before"`
	BillAfter  []recordAct `json:"bill_after"`
}

type recordAct struct {
	ArtistID uint   `json:"artist_id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
	SetType  string `json:"set_type"`
}

type recordArtist struct {
	ID   uint   `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type recordEdge struct {
	SourceArtistID   uint   `json:"source_artist_id"`
	TargetArtistID   uint   `json:"target_artist_id"`
	RelationshipType string `json:"relationship_type"`
}

type recordFollow struct {
	UserID uint   `json:"user_id"`
	Action string `json:"action"`
}

type recordTag struct {
	TagID         uint `json:"tag_id"`
	AddedByUserID uint `json:"added_by_user_id"`
}

type recordCrateRef struct {
	CollectionID uint `json:"collection_id"`
}

// Outcomes written to the record file.
const (
	recordApplied    = "applied"
	recordNotApplied = "not_applied"
)

// recorder appends record lines and syncs each to disk before returning.
type recorder struct {
	w    io.Writer
	sync func() error
	now  func() time.Time
}

func (r *recorder) write(l recordLine) error {
	l.At = r.now().UTC().Format(time.RFC3339Nano)
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	if _, err := r.w.Write(append(b, '\n')); err != nil {
		return err
	}
	return r.sync()
}

// openRecord opens the record file for append, creating it if absent. An
// existing file is never truncated.
func openRecord(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("--record %s cannot be opened for append: %w", path, err)
	}
	return f, nil
}

func recordBill(bill []billEntry) []recordAct {
	acts := make([]recordAct, len(bill))
	for i, e := range bill {
		acts[i] = recordAct{ArtistID: e.ArtistID, Name: e.Name, Position: e.Position, SetType: e.SetType}
	}
	return acts
}

// recordDroppedRows reads, before the delete, the follows, tags and crate items
// DeleteArtist drops with the artist.
func recordDroppedRows(tx *gorm.DB, artistID uint, l *recordLine) error {
	if err := tx.Raw(`SELECT user_id, action FROM user_bookmarks WHERE entity_type = 'artist' AND entity_id = ? ORDER BY user_id, action`,
		artistID).Scan(&l.FollowsDropped).Error; err != nil {
		return fmt.Errorf("read the follows the delete drops: %w", err)
	}
	if err := tx.Raw(`SELECT tag_id, added_by_user_id FROM entity_tags WHERE entity_type = 'artist' AND entity_id = ? ORDER BY tag_id`,
		artistID).Scan(&l.TagsDropped).Error; err != nil {
		return fmt.Errorf("read the tags the delete drops: %w", err)
	}
	if err := tx.Raw(`SELECT collection_id FROM collection_items WHERE entity_type = 'artist' AND entity_id = ? ORDER BY collection_id`,
		artistID).Scan(&l.CrateItems).Error; err != nil {
		return fmt.Errorf("read the crate items the delete drops: %w", err)
	}
	return nil
}
