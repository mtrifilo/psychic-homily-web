package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"psychic-homily-backend/internal/services/catalog"
)

// The reviewed worklist is a CSV with a header row. Columns are found by name.
const (
	colShowSlug  = "show_slug"
	colRoomSlug  = "correct_room_slug"
	colMergeInto = "merge_into_show_slug"
	colNote      = "note"
)

// evidenceColumns are the columns a worklist may carry for its reviewer, which
// this tool reads past. They are the evidence columns of the reviewed
// worklist format. Every header must be one of these or one of the columns
// above, so a misspelt merge column refuses the file instead of turning every
// merge into a plain move.
var evidenceColumns = []string{
	"section", "current_room", "room_on_page", "page_read", "date_time_on_page",
	"verdict", "copy_at_target", "not_confirmed",
}

func knownColumn(name string) bool {
	switch name {
	case colShowSlug, colRoomSlug, colMergeInto, colNote:
		return true
	}
	for _, c := range evidenceColumns {
		if name == c {
			return true
		}
	}
	return false
}

// worklistRow is one data row of the worklist. Line is the file line the row
// starts on, so a reviewer can find it.
type worklistRow struct {
	Line int
	Row  catalog.ShowReattribution
	Note string
	// Refusal is set for a row refused before it reaches the database.
	Refusal string
}

// parseWorklist reads the whole worklist. An error means the FILE is unusable
// (no header, a required column missing, malformed CSV) and nothing should run.
// Problems with a single row are recorded on that row instead.
func parseWorklist(r io.Reader) ([]worklistRow, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("worklist is empty")
	}
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	index := map[string]int{}
	for i, name := range header {
		name = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "\ufeff")))
		if _, dup := index[name]; dup {
			return nil, fmt.Errorf("header names column %q twice", name)
		}
		if !knownColumn(name) {
			return nil, fmt.Errorf("header has an unknown column %q; the columns are %s, %s, %s, %s and the evidence columns %s",
				name, colShowSlug, colRoomSlug, colMergeInto, colNote, strings.Join(evidenceColumns, ", "))
		}
		index[name] = i
	}
	for _, required := range []string{colShowSlug, colRoomSlug} {
		if _, ok := index[required]; !ok {
			return nil, fmt.Errorf("header has no %q column", required)
		}
	}

	field := func(record []string, col string) string {
		i, ok := index[col]
		if !ok || i >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[i])
	}

	var rows []worklistRow
	seen := map[string]int{}
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read worklist: %w", err)
		}
		line, _ := reader.FieldPos(0)
		row := worklistRow{
			Line: line,
			Row: catalog.ShowReattribution{
				ShowSlug:      field(record, colShowSlug),
				RoomSlug:      field(record, colRoomSlug),
				MergeIntoSlug: field(record, colMergeInto),
			},
			Note: field(record, colNote),
		}
		switch {
		case row.Row.ShowSlug == "" && row.Row.RoomSlug == "" && row.Row.MergeIntoSlug == "":
			continue
		case row.Row.ShowSlug == "":
			row.Refusal = "no show slug"
		case row.Row.RoomSlug == "":
			row.Refusal = "no target room given"
		}
		if row.Row.ShowSlug != "" {
			// A show named twice is refused on every row after the first, so
			// the outcome never depends on which of two conflicting rows ran
			// last.
			if first, dup := seen[row.Row.ShowSlug]; dup {
				row.Refusal = fmt.Sprintf("show already named on line %d", first)
			} else {
				seen[row.Row.ShowSlug] = line
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
