package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/services/catalog"
)

// recordLine is one line of the --record file. A room change writes no
// revision, so these lines are the record of which show left which room.
type recordLine struct {
	At      string `json:"at"`
	Line    int    `json:"line"`
	Outcome string `json:"outcome"`

	ShowID       uint   `json:"show_id,omitempty"`
	ShowSlug     string `json:"show_slug"`
	FromRoomID   uint   `json:"from_room_id,omitempty"`
	FromRoomSlug string `json:"from_room_slug,omitempty"`
	ToRoomID     uint   `json:"to_room_id,omitempty"`
	ToRoomSlug   string `json:"to_room_slug,omitempty"`

	// A merge keeps the named show and deletes the row's show.
	KeptShowID    uint   `json:"kept_show_id,omitempty"`
	KeptShowSlug  string `json:"kept_show_slug,omitempty"`
	DeletedShowID uint   `json:"deleted_show_id,omitempty"`

	Reason string `json:"reason,omitempty"`
}

// Outcomes written to the record file.
const (
	recordMoved      = "moved"
	recordMerged     = "merged"
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

func appliedLine(row worklistRow, res catalog.ShowReattributionResult) recordLine {
	l := recordLine{Line: row.Line, ShowID: res.ShowID, ShowSlug: res.ShowSlug}
	if res.CurrentRoom != nil {
		l.FromRoomID, l.FromRoomSlug = res.CurrentRoom.ID, res.CurrentRoom.Slug
	}
	if res.TargetRoom != nil {
		l.ToRoomID, l.ToRoomSlug = res.TargetRoom.ID, res.TargetRoom.Slug
	}
	if res.Outcome == catalog.ReattributionMerged {
		l.Outcome = recordMerged
		l.KeptShowID, l.KeptShowSlug, l.DeletedShowID = res.MergeIntoID, res.MergeIntoSlug, res.ShowID
	} else {
		l.Outcome = recordMoved
	}
	return l
}

// errRowFailed ends a confirmed row's transaction when the row failed inside
// its savepoint.
var errRowFailed = errors.New("row failed")

// applyConfirmedRow applies one row in its own transaction under timeouts. A
// moved or merged row's record line is written and synced before the commit;
// if it cannot be, the row is rolled back. A non-nil error stops the run: the
// row failed, its record line could not be written, or the commit failed (in
// which case a not_applied line follows the row's line in the record).
func applyConfirmedRow(database *gorm.DB, rec *recorder, row worklistRow,
	timeouts []string) (catalog.ShowReattributionResult, error) {
	var res catalog.ShowReattributionResult
	var recordErr error
	recorded := false
	err := database.Transaction(func(tx *gorm.DB) error {
		if err := setTimeouts(tx, timeouts); err != nil {
			return err
		}
		res = catalog.ReattributeShow(tx, row.Row)
		switch res.Outcome {
		case catalog.ReattributionMoved, catalog.ReattributionMerged:
			if err := rec.write(appliedLine(row, res)); err != nil {
				recordErr = err
				return err
			}
			recorded = true
			return nil
		case catalog.ReattributionFailed:
			return errRowFailed
		default:
			return nil
		}
	})

	switch {
	case err == nil:
		return res, nil
	case recordErr != nil:
		res.Outcome = catalog.ReattributionFailed
		res.Reason = "the record line could not be written, so the row was rolled back: " + recordErr.Error()
		return res, fmt.Errorf("the record file could not be written: %w", recordErr)
	case errors.Is(err, errRowFailed):
		return res, fmt.Errorf("the row failed and was rolled back: %s", res.Reason)
	default:
		// Setup or the commit itself failed; nothing of the row is applied.
		if res.ShowSlug == "" {
			res.ShowSlug = row.Row.ShowSlug
		}
		res.Outcome = catalog.ReattributionFailed
		res.Reason = "the transaction did not commit: " + err.Error()
		if recorded {
			if werr := rec.write(recordLine{
				Line: row.Line, Outcome: recordNotApplied, ShowID: res.ShowID, ShowSlug: res.ShowSlug,
				Reason: "the line above was written before a commit that failed: " + err.Error(),
			}); werr != nil {
				return res, fmt.Errorf("the commit failed (%v) and the record could not say so (%v); "+
					"the record's last line for this row is wrong", err, werr)
			}
		}
		return res, fmt.Errorf("the row did not commit: %w", err)
	}
}
