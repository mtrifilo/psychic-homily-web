package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"gorm.io/gorm"
)

// recordLine is one line of the --record file.
//
// A confirmed row writes a "pending" line, synced, before its transaction
// commits, and a "committed" line after the commit returns. A pending line with
// no committed line after it means the commit's outcome was not seen (the
// process died, or the commit errored and the database could not be checked):
// read the database. A "not_applied" line says the commit failed and nothing
// of the row is in the database.
//
// The pending line names the row's write by id and slug and carries Before: a
// copy, taken inside the transaction before the write, of the artist's own row
// and of every row that references it (see snapshotArtistReferences). The
// admin paths this tool drives record nothing for a bill change or an artist
// delete, and a merge's audit row keeps only counts, so these lines are the
// durable account a wrong merge, remove or split would be repaired from.
type recordLine struct {
	At      string `json:"at"`
	RunID   string `json:"run_id"`
	Line    int    `json:"line"`
	Action  string `json:"action"`
	Outcome string `json:"outcome"`

	// Who and where: the actor the writes are attributed to and the database
	// they went to (host/dbname, no credentials).
	ActorID     uint   `json:"actor_id"`
	Environment string `json:"environment"`
	Database    string `json:"database"`

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

	// Before maps "table.column" to the rows, as JSON objects, that named the
	// artist in that column before the write.
	Before map[string]json.RawMessage `json:"before,omitempty"`

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

// Outcomes written to the record file.
const (
	recordPending    = "pending"
	recordCommitted  = "committed"
	recordNotApplied = "not_applied"
	recordUnknown    = "unknown"
)

// recorder appends record lines and syncs each to disk before returning. It
// stamps every line with the run's id, actor and database.
type recorder struct {
	w    io.Writer
	sync func() error
	now  func() time.Time

	runID       string
	actorID     uint
	environment string
	database    string
}

func (r *recorder) write(l recordLine) error {
	l.At = r.now().UTC().Format(time.RFC3339Nano)
	l.RunID, l.ActorID, l.Environment, l.Database = r.runID, r.actorID, r.environment, r.database
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	if _, err := r.w.Write(append(b, '\n')); err != nil {
		return err
	}
	return r.sync()
}

// outcomeLine is the short line that follows a row's pending line.
func outcomeLine(p *rowPlan, outcome, reason string) recordLine {
	return recordLine{Line: p.Row.Line, Action: p.Row.Action, Outcome: outcome,
		ArtistID: p.Record.ArtistID, ArtistSlug: p.Record.ArtistSlug, ArtistName: p.Record.ArtistName, Reason: reason}
}

// openRecord opens the record file for append, creating it with mode 0600 if
// absent. An existing file is never truncated. A symlink, anything other than
// a regular file, a file another user owns, and a file group or others can
// read or write are refused: the lines name user ids.
func openRecord(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("--record %s cannot be opened for append: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("--record %s: %w", path, err)
	}
	refuse := func(why string) (*os.File, error) {
		_ = f.Close()
		return nil, fmt.Errorf("--record %s %s", path, why)
	}
	if !info.Mode().IsRegular() {
		return refuse("is not a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return refuse(fmt.Sprintf("has mode %v; it must be readable by its owner only (chmod 600)", info.Mode().Perm()))
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return refuse("belongs to another user")
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

// artistReference is one column that holds an artist id.
type artistReference struct {
	table, column string
	// polymorphic is set for an entity_id column read with entity_type = 'artist'.
	polymorphic bool
	// array is set for a bigint[] column that lists artist ids.
	array bool
}

// artistReferenceColumns lists, from the live schema, every column that holds
// an artist id: each foreign key to artists(id), each entity_id column of a
// table that also has an entity_type column, and the bare id columns no
// constraint marks (artist_relationship_votes' pair, notification_log's
// subject_entity_id, notification_filters.artist_ids). Reading the schema
// rather than a list keeps the record complete when a migration adds a
// reference.
func artistReferenceColumns(tx *gorm.DB) ([]artistReference, error) {
	var fks []struct{ Table, Column string }
	if err := tx.Raw(`
		SELECT cl.relname AS "table", att.attname AS "column"
		FROM pg_constraint con
		JOIN pg_class cl ON cl.oid = con.conrelid
		JOIN pg_namespace ns ON ns.oid = cl.relnamespace
		JOIN LATERAL unnest(con.conkey) AS k(attnum) ON true
		JOIN pg_attribute att ON att.attrelid = con.conrelid AND att.attnum = k.attnum
		WHERE con.contype = 'f' AND con.confrelid = 'artists'::regclass AND ns.nspname = current_schema()
		  AND array_length(con.conkey, 1) = 1
		ORDER BY 1, 2`).Scan(&fks).Error; err != nil {
		return nil, fmt.Errorf("list the foreign keys to artists: %w", err)
	}
	var poly []struct{ Table string }
	if err := tx.Raw(`
		SELECT c.table_name AS "table"
		FROM information_schema.columns c
		JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name
		WHERE c.table_schema = current_schema() AND c.column_name = 'entity_id' AND t.table_type = 'BASE TABLE'
		  AND EXISTS (SELECT 1 FROM information_schema.columns e
		              WHERE e.table_schema = c.table_schema AND e.table_name = c.table_name AND e.column_name = 'entity_type')
		ORDER BY 1`).Scan(&poly).Error; err != nil {
		return nil, fmt.Errorf("list the entity reference tables: %w", err)
	}
	refs := make([]artistReference, 0, len(fks)+len(poly)+4)
	for _, fk := range fks {
		refs = append(refs, artistReference{table: fk.Table, column: fk.Column})
	}
	for _, p := range poly {
		refs = append(refs, artistReference{table: p.Table, column: "entity_id", polymorphic: true})
	}
	return append(refs,
		artistReference{table: "artist_relationship_votes", column: "source_artist_id"},
		artistReference{table: "artist_relationship_votes", column: "target_artist_id"},
		artistReference{table: "notification_log", column: "subject_entity_id"},
		artistReference{table: "notification_filters", column: "artist_ids", array: true},
	), nil
}

// snapshotArtistReferences copies, as JSON, the artist's own row and every row
// that references it, keyed "table.column" ("artists.id" for the artist
// itself). Columns with no rows are left out.
func snapshotArtistReferences(tx *gorm.DB, artistID uint) (map[string]json.RawMessage, error) {
	refs, err := artistReferenceColumns(tx)
	if err != nil {
		return nil, err
	}
	refs = append([]artistReference{{table: "artists", column: "id"}}, refs...)
	out := map[string]json.RawMessage{}
	for _, ref := range refs {
		var present int64
		if err := tx.Raw(`SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`, ref.table, ref.column).
			Scan(&present).Error; err != nil {
			return nil, fmt.Errorf("check %s.%s: %w", ref.table, ref.column, err)
		}
		if present == 0 {
			continue
		}
		table := quoteIdent(ref.table)
		column := quoteIdent(ref.column)
		where := fmt.Sprintf("t.%s = @id", column)
		switch {
		case ref.polymorphic:
			where += " AND t.entity_type = 'artist'"
		case ref.array:
			where = fmt.Sprintf("@id = ANY(t.%s)", column)
		}
		var rows *string
		query := fmt.Sprintf("SELECT json_agg(to_jsonb(t)) FROM %s t WHERE %s", table, where)
		if err := tx.Raw(query, map[string]interface{}{"id": artistID}).Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("copy %s.%s for the record: %w", ref.table, ref.column, err)
		}
		if rows != nil {
			out[ref.table+"."+ref.column] = json.RawMessage(*rows)
		}
	}
	return out, nil
}

// quoteIdent quotes a Postgres identifier read from the catalog.
func quoteIdent(s string) string {
	q := []byte{'"'}
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			q = append(q, '"')
		}
		q = append(q, s[i])
	}
	return string(append(q, '"'))
}
