package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// WriteJSON writes the report as indented JSON.
func (r *VenueAddressReport) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// HitRate is hits over attempts for one phase, 0 when nothing was attempted.
func (t *VenueAddressPhaseTotals) HitRate() float64 {
	if t == nil || t.Attempted == 0 {
		return 0
	}
	return float64(t.Hits) / float64(t.Attempted)
}

// WriteMarkdown writes the report for a human reviewer: run settings, totals
// per phase and per precision, then one table row per phase attempted.
func (r *VenueAddressReport) WriteMarkdown(w io.Writer) error {
	var b strings.Builder
	mode := "DRY RUN"
	if !r.DryRun {
		mode = "LIVE"
	}
	fmt.Fprintf(&b, "# Venue address backfill (%s)\n\n", mode)
	fmt.Fprintf(&b, "Generated %s. Limit %s, only upcoming: %t, city: %s, AI fallback: %t.\n\n",
		r.GeneratedAt.Format("2006-01-02 15:04 MST"), limitLabel(r.Limit), r.OnlyUpcoming, orDash(r.City), r.AIEnabled)
	fmt.Fprintf(&b, "Venues with no address matching the filters: %d. Processed: %d.", r.Candidates, r.Processed)
	if r.LimitHit {
		b.WriteString(" The limit was reached; a re-run continues.")
	}
	if r.LookupsTableMissing {
		b.WriteString(" The database has no venue_address_lookups table yet, so no recorded misses were skipped.")
	}
	b.WriteString("\n\n## Totals per phase\n\n")
	b.WriteString("| Phase | Attempted | Hits | Hit rate | Misses | Errors | Skipped (recorded miss) | No page |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, phase := range []string{catalogm.VenueAddressPhasePage, catalogm.VenueAddressPhaseName} {
		t := r.Phases[phase]
		if t == nil {
			t = &VenueAddressPhaseTotals{}
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %.0f%% | %d | %d | %d | %d |\n",
			phase, t.Attempted, t.Hits, 100*t.HitRate(), t.Misses, t.Errors, t.SkippedMemo, t.NoSource)
	}
	fmt.Fprintf(&b, "\nWould write: %d. Written: %d.\n\n## Totals per precision (would write)\n\n", r.WouldWrite, r.Written)
	if len(r.Precision) == 0 {
		b.WriteString("None.\n")
	} else {
		b.WriteString("| Precision | Venues |\n| --- | --- |\n")
		labels := make([]string, 0, len(r.Precision))
		for p := range r.Precision {
			labels = append(labels, p)
		}
		sort.Strings(labels)
		for _, p := range labels {
			fmt.Fprintf(&b, "| %s | %d |\n", p, r.Precision[p])
		}
	}

	b.WriteString("\n## Venues\n\n")
	b.WriteString("| ID | Venue | City | Verified | Phase | Outcome | Source or query | Address | Precision | Would write | Notes |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, row := range r.Rows {
		address := row.Address
		if row.MatchedName != "" {
			address += " (OSM: " + row.MatchedName + ")"
		}
		fmt.Fprintf(&b, "| %d | %s | %s, %s | %t | %s | %s | %s | %s | %s | %t | %s |\n",
			row.VenueID, cell(row.Name), cell(row.City), cell(row.State), row.Verified, row.Phase,
			row.Outcome, cell(row.Source), cell(address), orDash(row.Precision), row.WouldWrite,
			cell(strings.Join(row.Notes, "; ")))
	}
	if len(r.Errors) > 0 {
		b.WriteString("\n## Errors\n\n")
		for _, e := range r.Errors {
			fmt.Fprintf(&b, "- %s\n", e)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// cell makes a value safe inside a markdown table cell.
func cell(s string) string {
	s = strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ").Replace(s)
	if s == "" {
		return "-"
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func limitLabel(n int) string {
	if n <= 0 {
		return "none"
	}
	return fmt.Sprintf("%d", n)
}
