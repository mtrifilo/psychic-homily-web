package pipeline

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// The shape discovery descriptions are assembled in: parts joined with
// ticketDescriptionSeparator, a vendor URL carried as a part beginning with
// ticketDescriptionPrefix. The create path no longer writes such a part; this
// file owns the only remaining knowledge of the spelling, because it has to
// read rows that were written before the URL had a column.
const (
	ticketDescriptionSeparator = " | "
	ticketDescriptionPrefix    = "Tickets: "
)

// TicketDescriptionCleanupOptions controls one cleanup pass.
type TicketDescriptionCleanupOptions struct {
	// DryRun reports what a live run would change and writes nothing.
	DryRun bool
	// Verbose collects a per-row record in the report.
	Verbose bool
}

// TicketDescriptionCleanupRow records what one show row would change to.
type TicketDescriptionCleanupRow struct {
	ShowID         uint
	Source         string
	TicketURL      string
	MovedToColumn  bool
	NewDescription *string
}

// TicketDescriptionCleanupReport summarizes one pass.
type TicketDescriptionCleanupReport struct {
	// Scanned counts rows whose description contains the prefix at all.
	Scanned int
	// Stripped counts rows whose description this pass rewrites.
	Stripped int
	// MovedToColumn counts rows that additionally gain a ticket_url.
	MovedToColumn int
	// SkippedNonURL counts rows whose "Tickets:" part is not an absolute
	// http(s) URL, which this pass leaves alone.
	SkippedNonURL int
	// SkippedOversizeURL counts rows left untouched because the URL is wider
	// than the ticket_url column and the column is empty, so stripping it from
	// the description would destroy the only copy.
	SkippedOversizeURL int
	// BySource counts the stripped rows per shows.source value.
	BySource map[string]int
	// Rows is populated only under Verbose.
	Rows []TicketDescriptionCleanupRow
}

// SourceBreakdown renders BySource in a stable order.
func (r *TicketDescriptionCleanupReport) SourceBreakdown() string {
	if len(r.BySource) == 0 {
		return "none"
	}
	sources := make([]string, 0, len(r.BySource))
	for source := range r.BySource {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	parts := make([]string, 0, len(sources))
	for _, source := range sources {
		parts = append(parts, fmt.Sprintf("%s=%d", source, r.BySource[source]))
	}
	return strings.Join(parts, ", ")
}

// ticketDescriptionSplit is the result of reading one stored description.
type ticketDescriptionSplit struct {
	// Description is what the description becomes, nil when nothing is left.
	Description *string
	// TicketURL is the first absolute http(s) URL the removed parts carried.
	TicketURL string
	// Changed reports whether any part was removed.
	Changed bool
	// SawNonURL reports that a "Tickets:" part was left in place because its
	// value is not an absolute http(s) URL.
	SawNonURL bool
}

// splitTicketDescription removes every "Tickets: <absolute http(s) url>" part
// from a stored description and reports the first URL it removed.
//
// A part whose value is not an absolute http(s) URL is left in place: it is
// prose a human wrote, not the writer's own vendor line, and removing it would
// destroy contributor text.
func splitTicketDescription(description string) ticketDescriptionSplit {
	parts := strings.Split(description, ticketDescriptionSeparator)
	kept := make([]string, 0, len(parts))
	result := ticketDescriptionSplit{}

	for _, part := range parts {
		value, isTicketPart := strings.CutPrefix(part, ticketDescriptionPrefix)
		if !isTicketPart {
			kept = append(kept, part)
			continue
		}
		value = strings.TrimSpace(value)
		if !isAbsoluteHTTPURL(value) {
			result.SawNonURL = true
			kept = append(kept, part)
			continue
		}
		result.Changed = true
		if result.TicketURL == "" {
			result.TicketURL = value
		}
	}

	if !result.Changed {
		return result
	}

	rebuilt := strings.TrimSpace(strings.Join(kept, ticketDescriptionSeparator))
	if rebuilt != "" {
		result.Description = &rebuilt
	}
	return result
}

// isAbsoluteHTTPURL reports whether the value is an http(s) URL naming a host.
// Scheme-less values are rejected: this pass moves a value into a column the
// render surfaces treat as a destination, and supplying a scheme here would
// invent the destination rather than read it.
func isAbsoluteHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

// CleanupTicketDescriptions moves vendor URLs out of stored show descriptions
// and into shows.ticket_url.
//
// The column wins when it already holds a value: the description line is the
// older record, and a populated column was either a later scrape or a human
// edit. Rows are written one at a time so a single unwritable row does not
// abandon the pass, and the pass is idempotent because it rewrites only rows
// whose description still carries the part.
func CleanupTicketDescriptions(db *gorm.DB, opts TicketDescriptionCleanupOptions) (*TicketDescriptionCleanupReport, error) {
	type candidate struct {
		ID          uint
		Source      string
		Description string
		TicketURL   *string
	}

	var candidates []candidate
	err := db.Model(&catalogm.Show{}).
		Select("id, source, description, ticket_url").
		Where("description LIKE ?", "%"+ticketDescriptionPrefix+"%").
		Order("id ASC").
		Scan(&candidates).Error
	if err != nil {
		return nil, fmt.Errorf("scanning descriptions: %w", err)
	}

	report := &TicketDescriptionCleanupReport{
		Scanned:  len(candidates),
		BySource: map[string]int{},
	}

	for _, row := range candidates {
		split := splitTicketDescription(row.Description)
		if split.SawNonURL {
			report.SkippedNonURL++
		}
		if !split.Changed {
			continue
		}

		columnEmpty := row.TicketURL == nil || strings.TrimSpace(*row.TicketURL) == ""
		storable := len(split.TicketURL) <= maxTicketURLLen
		if columnEmpty && !storable {
			report.SkippedOversizeURL++
			continue
		}

		updates := map[string]any{"description": split.Description}
		moved := columnEmpty && storable
		if moved {
			updates["ticket_url"] = split.TicketURL
		}

		report.Stripped++
		report.BySource[row.Source]++
		if moved {
			report.MovedToColumn++
		}
		if opts.Verbose {
			report.Rows = append(report.Rows, TicketDescriptionCleanupRow{
				ShowID:         row.ID,
				Source:         row.Source,
				TicketURL:      split.TicketURL,
				MovedToColumn:  moved,
				NewDescription: split.Description,
			})
		}

		if opts.DryRun {
			continue
		}
		if err := db.Model(&catalogm.Show{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("updating show %d: %w", row.ID, err)
		}
	}

	return report, nil
}
