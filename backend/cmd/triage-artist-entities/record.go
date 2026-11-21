package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"gorm.io/gorm"

	notificationm "psychic-homily-backend/internal/models/notification"
)

// recordLine is one line of the --record file, a JSON object on its own line.
//
// Lines of one row share run_id and line: run_id names the run (every line of
// a run carries it), and line is the worklist's line number, the header being
// line 1. Line numbers repeat across runs, so join on both.
//
// outcome is one of:
//
//   - "pending": written and synced inside the row's transaction, after its
//     write and before its commit. It carries the full account of the row:
//     shows, acts, the kept artist of a merge, and before.
//   - "committed": the commit returned, or it returned an error but the
//     database reports the transaction committed (reason then says so).
//   - "not_applied": the commit errored and the database reports the
//     transaction aborted, or the pending line could not be synced and the row
//     was rolled back; nothing of the row is in the database.
//   - "unknown": the commit errored and the database could not say how the
//     transaction ended; read the database before acting on the row.
//
// Follow-up lines (every outcome but pending) carry only the row's identity and
// a reason. A pending line with no line after it for the same run_id and line
// means the process stopped between the sync and the commit's answer: look up
// the pending line's xid with txid_status() to learn how it ended.
//
// before maps a column to the rows that named the artist in it, copied inside
// the transaction before the write, for merge, remove and split:
//
//   - "artists.id" is the artist's own row;
//   - "table.column" is a foreign key column to artists(id), or a bare id
//     column (artist_relationship_votes' pair, notification_log's
//     subject_entity_id for artist show alerts);
//   - "table.entity_id", or "table.<x>_entity_id", is a polymorphic column,
//     holding only its rows whose entity_type is 'artist' (a table with an
//     entity_type column and an entity_id or *_entity_id column);
//   - "notification_filters.artist_ids" holds the whole filter rows whose
//     artist_ids list the artist.
//
// A row can appear under two keys (an artist_relationships row under source
// and target). A merge's bill moves are in before["show_artists.artist_id"];
// shows lists the bills a remove or split rewrote.
//
// The admin paths this tool drives record nothing for a bill change or an
// artist delete, and a merge's audit row keeps only counts, so these lines are
// the durable account a wrong merge, remove or split would be repaired from.
type recordLine struct {
	At      string `json:"at"`
	RunID   string `json:"run_id"`
	Line    int    `json:"line"`
	Action  string `json:"action"`
	Outcome string `json:"outcome"`
	// Xid is the row's transaction id, set on the pending line.
	Xid string `json:"xid,omitempty"`

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

// Outcomes written to the record file; see recordLine.
const (
	recordPending    = "pending"
	recordCommitted  = "committed"
	recordNotApplied = "not_applied"
	recordUnknown    = "unknown"
)

// recordFile is the file a recorder appends to: a writer whose size can be
// read and cut back, so a line that fails partway is removed.
type recordFile interface {
	io.Writer
	Sync() error
	Stat() (os.FileInfo, error)
	Truncate(size int64) error
}

// recorder appends record lines and syncs each to disk before returning. It
// stamps every line with the run's id, actor and database. A line that cannot
// be written whole is cut back off the file, so a failed write never leaves
// half a line for the next line to be appended to.
type recorder struct {
	f   recordFile
	now func() time.Time

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
	info, err := r.f.Stat()
	if err != nil {
		return err
	}
	if _, err := r.f.Write(append(b, '\n')); err != nil {
		if terr := r.f.Truncate(info.Size()); terr != nil {
			return fmt.Errorf("%w; cutting the partial line back off also failed: %w", err, terr)
		}
		return err
	}
	return r.f.Sync()
}

// outcomeLine is the short line that follows a row's pending line.
func outcomeLine(p *rowPlan, outcome, reason string) recordLine {
	return recordLine{Line: p.Row.Line, Action: p.Row.Action, Outcome: outcome,
		ArtistID: p.Record.ArtistID, ArtistSlug: p.Record.ArtistSlug, ArtistName: p.Record.ArtistName, Reason: reason}
}

// openRecord opens the record file for append, creating it with mode 0600 if
// absent. An existing file is never truncated; if it does not end in a
// newline, one is appended so the next line starts on its own. A symlink,
// anything other than a regular file, a file another user owns, and a file
// group or others can read or write are refused: the lines name user ids.
func openRecord(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("--record %s cannot be opened for append: %w", path, err)
	}
	refuse := func(why string) (*os.File, error) {
		_ = f.Close()
		return nil, fmt.Errorf("--record %s %s", path, why)
	}
	info, err := f.Stat()
	if err != nil {
		return refuse(err.Error())
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
	if info.Size() > 0 {
		last := make([]byte, 1)
		if _, err := f.ReadAt(last, info.Size()-1); err != nil {
			return refuse(err.Error())
		}
		if last[0] != '\n' {
			if _, err := f.Write([]byte("\n")); err != nil {
				return refuse(err.Error())
			}
		}
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

// artistReference is one column that holds an artist id, and the condition
// that selects the rows naming the artist in it (@id is the artist's id).
type artistReference struct {
	key   string
	from  string
	where string
}

// artistReferenceColumns lists, from the live schema, the columns that hold an
// artist id: each single-column foreign key to artists(id); each entity_id or
// *_entity_id column of a table that also has an entity_type column; and the
// bare id columns named below. Reading the schema keeps the copy current when a
// migration adds a reference of those shapes. It misses a reference of any
// other shape: a composite foreign key, an id column with another name and no
// entity_type beside it, or an array of ids other than notification filters'
// artist_ids.
func artistReferenceColumns(db *gorm.DB) ([]artistReference, error) {
	var fks []struct{ Table, Column string }
	if err := db.Raw(`
		SELECT cl.relname AS "table", att.attname AS "column"
		FROM pg_constraint con
		JOIN pg_class cl ON cl.oid = con.conrelid
		JOIN pg_namespace ns ON ns.oid = cl.relnamespace
		JOIN pg_attribute att ON att.attrelid = con.conrelid AND att.attnum = con.conkey[1]
		WHERE con.contype = 'f' AND con.confrelid = 'artists'::regclass AND ns.nspname = current_schema()
		  AND array_length(con.conkey, 1) = 1
		ORDER BY 1, 2`).Scan(&fks).Error; err != nil {
		return nil, fmt.Errorf("list the foreign keys to artists: %w", err)
	}
	// Polymorphic columns: entity_id, or a column ending in _entity_id
	// (requests.requested_entity_id, entity_requests.created_entity_id), in a
	// table that also has an entity_type column. notification_log's
	// subject_entity_id is keyed by alert type, not entity_type, and is read
	// below on its own terms.
	var poly []struct{ Table, Column string }
	if err := db.Raw(`
		SELECT c.table_name AS "table", c.column_name AS "column"
		FROM information_schema.columns c
		JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name
		WHERE c.table_schema = current_schema() AND t.table_type = 'BASE TABLE'
		  AND (c.column_name = 'entity_id' OR c.column_name LIKE '%\_entity\_id')
		  AND NOT (c.table_name = 'notification_log' AND c.column_name = 'subject_entity_id')
		  AND EXISTS (SELECT 1 FROM information_schema.columns e
		              WHERE e.table_schema = c.table_schema AND e.table_name = c.table_name AND e.column_name = 'entity_type')
		ORDER BY 1, 2`).Scan(&poly).Error; err != nil {
		return nil, fmt.Errorf("list the entity reference columns: %w", err)
	}

	refs := []artistReference{{key: "artists.id", from: "artists", where: "t.id = @id"}}
	for _, fk := range fks {
		refs = append(refs, artistReference{key: fk.Table + "." + fk.Column, from: quoteIdent(fk.Table),
			where: "t." + quoteIdent(fk.Column) + " = @id"})
	}
	for _, p := range poly {
		refs = append(refs, artistReference{key: p.Table + "." + p.Column, from: quoteIdent(p.Table),
			where: "t." + quoteIdent(p.Column) + " = @id AND t.entity_type = 'artist'"})
	}
	// Bare id columns. Each is read only when the column exists, so a
	// migration that drops one does not break the copy.
	bare := []struct {
		table, column string
		ref           artistReference
	}{
		{"artist_relationship_votes", "source_artist_id", artistReference{key: "artist_relationship_votes.source_artist_id",
			from: "artist_relationship_votes", where: "t.source_artist_id = @id"}},
		{"artist_relationship_votes", "target_artist_id", artistReference{key: "artist_relationship_votes.target_artist_id",
			from: "artist_relationship_votes", where: "t.target_artist_id = @id"}},
		// The alert types whose subject_entity_id is an artist id: a copy of
		// the artist merge's artistSubjectAlertTypes, which is not exported. A
		// type added there must be added here too.
		{"notification_log", "subject_entity_id", artistReference{key: "notification_log.subject_entity_id",
			from: "notification_log", where: "t.subject_entity_id = @id AND t.entity_type = '" +
				notificationm.NotificationEntityArtistShowAlert + "'"}},
		{"notification_filters", "artist_ids", artistReference{key: "notification_filters.artist_ids",
			from: "notification_filters", where: "@id = ANY(t.artist_ids)"}},
	}
	for _, b := range bare {
		var present int64
		if err := db.Raw(`SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`, b.table, b.column).
			Scan(&present).Error; err != nil {
			return nil, fmt.Errorf("check %s.%s: %w", b.table, b.column, err)
		}
		if present > 0 {
			refs = append(refs, b.ref)
		}
	}
	return refs, nil
}

// snapshotArtistReferences copies, as JSON, the artist's own row and every row
// that references it, keyed as recordLine describes, in one query. Columns
// with no rows are left out.
func snapshotArtistReferences(tx *gorm.DB, refs []artistReference, artistID uint) (map[string]json.RawMessage, error) {
	cols := make([]string, len(refs))
	for i, r := range refs {
		cols[i] = fmt.Sprintf("(SELECT json_agg(to_jsonb(t)) FROM %s t WHERE %s)::text AS c%d", r.from, r.where, i)
	}
	row := map[string]interface{}{}
	if err := tx.Raw("SELECT "+strings.Join(cols, ", "), map[string]interface{}{"id": artistID}).Scan(&row).Error; err != nil {
		return nil, fmt.Errorf("copy the rows that reference artist %d for the record: %w", artistID, err)
	}
	out := map[string]json.RawMessage{}
	for i, r := range refs {
		switch v := row[fmt.Sprintf("c%d", i)].(type) {
		case nil:
		case string:
			out[r.key] = json.RawMessage(v)
		default:
			return nil, fmt.Errorf("copy %s for the record: got %T", r.key, v)
		}
	}
	return out, nil
}

// quoteIdent quotes a Postgres identifier read from the catalog.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
