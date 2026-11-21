package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
)

// The five actions a reviewed worklist row may carry.
const (
	actionRename = "rename"
	actionMerge  = "merge"
	actionRemove = "remove"
	actionSplit  = "split"
	actionKeep   = "keep"
)

// The columns a worklist reads. Their names are deliberately not the suggestion
// file's (suggested_action, suggested_target): a list of suggestions cannot be
// applied until a person has reviewed it and renamed those columns.
const (
	colArtistSlug = "artist_slug"
	colAction     = "action"
	colTarget     = "target"
	colShowSlug   = "show_slug"
	colNote       = "note"
)

// suggestionColumnPrefix marks a column that carries an unreviewed suggestion.
// A file with any such column is refused whole.
const suggestionColumnPrefix = "suggested_"

// evidenceColumns are the context columns of the reviewed worklist format,
// which this tool reads past. Every header must be one of these or one of the
// columns above, so a misspelt show_slug or target refuses the file instead of
// being read as empty.
var evidenceColumns = []string{
	"name", "heuristics", "h2_match", "handcheck_verdict", "show_count", "release_count", "has_link",
	"latest_show_slug", "bill_position", "set_type", "in_headline_slot", "bill_size", "show_headliner",
}

func knownColumn(name string) bool {
	switch name {
	case colArtistSlug, colAction, colTarget, colShowSlug, colNote:
		return true
	}
	for _, c := range evidenceColumns {
		if name == c {
			return true
		}
	}
	return false
}

// removeTargetTitle is the one target a remove row may carry: write the
// artist's name to the title of each show it is taken off.
const removeTargetTitle = "title"

// splitActSeparator separates the act names in a split row's target. Act names
// carry commas, ampersands and "and", so none of those can separate them.
const splitActSeparator = "|"

// maxShowTitleLength is the longest title the show update API accepts.
const maxShowTitleLength = 255

// worklistRow is one reviewed decision about one artist.
type worklistRow struct {
	Line       int
	ArtistSlug string
	Action     string
	Target     string
	ShowSlug   string
	Note       string
}

// parseWorklist reads a reviewed CSV. The header must name artist_slug, action
// and target; show_slug, note and the evidence columns are optional, and any
// other column refuses the whole file, as does a column whose name starts with
// "suggested_". Values are trimmed and the action is
// lowercased. A malformed file is an error for the whole file; a malformed row
// is left for refuseMalformedRows, so one bad row never hides the others.
func parseWorklist(r io.Reader) ([]worklistRow, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimPrefix(raw, []byte("\ufeff"))

	reader := csv.NewReader(bytes.NewReader(raw))
	header, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("worklist is empty")
	}
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	index := make(map[string]int, len(header))
	for i, name := range header {
		name = strings.ToLower(strings.TrimSpace(name))
		if strings.HasPrefix(name, suggestionColumnPrefix) {
			return nil, fmt.Errorf("header still carries the suggestion column %q: review every row, then delete the "+
				"suggested_* columns and fill action and target with the decisions", name)
		}
		if _, dup := index[name]; dup {
			return nil, fmt.Errorf("header names column %q twice", name)
		}
		if !knownColumn(name) {
			return nil, fmt.Errorf("header has an unknown column %q; the columns are %s, %s, %s, %s, %s and the evidence columns %s",
				name, colArtistSlug, colAction, colTarget, colShowSlug, colNote, strings.Join(evidenceColumns, ", "))
		}
		index[name] = i
	}
	for _, required := range []string{colArtistSlug, colAction, colTarget} {
		if _, ok := index[required]; !ok {
			return nil, fmt.Errorf("header has no %q column (got %s)", required, strings.Join(header, ","))
		}
	}

	field := func(record []string, name string) string {
		i, ok := index[name]
		if !ok {
			return ""
		}
		return strings.TrimSpace(record[i])
	}

	var rows []worklistRow
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
			Line:       line,
			ArtistSlug: field(record, colArtistSlug),
			Action:     strings.ToLower(field(record, colAction)),
			Target:     field(record, colTarget),
			ShowSlug:   field(record, colShowSlug),
			Note:       field(record, colNote),
		}
		if row == (worklistRow{Line: line}) {
			continue
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("worklist has a header and no rows")
	}
	return rows, nil
}

// slugShaped matches a lowercase, hyphen-joined string with no spaces: the shape
// of a slug, which a rename target or an act name is not.
var slugShaped = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)+$`)

// hasLetterOrDigit reports whether s carries anything a name can be made of.
func hasLetterOrDigit(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

// splitActNames returns the act names of a split target in order, trimmed, and
// an error when the list cannot describe a split of joinedName.
func splitActNames(target, joinedName string) ([]string, error) {
	parts := strings.Split(target, splitActSeparator)
	if len(parts) < 2 {
		return nil, fmt.Errorf("split needs two or more act names separated by %q, got %q; for one act use rename or merge",
			splitActSeparator, target)
	}
	seen := map[string]bool{}
	acts := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		switch {
		case !hasLetterOrDigit(name):
			return nil, fmt.Errorf("split target %q has an empty act name", target)
		case slugShaped.MatchString(name):
			return nil, fmt.Errorf("act name %q looks like a slug; a split takes act names", name)
		case seen[strings.ToLower(name)]:
			return nil, fmt.Errorf("split target %q names %q twice", target, name)
		case joinedName != "" && strings.EqualFold(name, joinedName):
			return nil, fmt.Errorf("act name %q is the joined entity's own name", name)
		}
		seen[strings.ToLower(name)] = true
		acts = append(acts, name)
	}
	return acts, nil
}
