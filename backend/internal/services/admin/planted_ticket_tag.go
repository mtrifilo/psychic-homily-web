package admin

import (
	"net/url"
	"slices"
	"strings"

	"psychic-homily-backend/internal/services/contracts"
)

// plantedTagSource describes one table carrying a contributor-writable ticket
// URL, for the category that reports affiliate tags planted in it.
//
// A tag in a STORED ticket URL is somebody else's: the render surfaces append
// ours to a copy at render time and never write one back. Whose it is depends
// on the row's provenance, which the reason does not carry -- a contributor
// submitted it, or the discovery importer copied a venue listing that already
// had one.
type plantedTagSource struct {
	EntityType string
	Table      string
	NameColumn string
	// Where narrows the rows worth reporting, or is empty for a table whose
	// rows are all published.
	Where string
}

var plantedTagSources = map[string]plantedTagSource{
	categoryShowsPlantedTicketTag: {
		EntityType: "show",
		Table:      "shows",
		NameColumn: "title",
		// Approved is the published set. Past shows stay in it: their pages
		// keep rendering the stored value, so the finding does not expire with
		// the date.
		Where: "status = 'approved'",
	},
	categoryFestivalsPlantedTicketTag: {
		EntityType: "festival",
		Table:      "festivals",
		NameColumn: "name",
	},
}

// plantedTagCandidateSQL is a COARSE pre-filter, not the answer. The Go matcher
// below decides, so the count, the list and the reason are one implementation
// rather than a SQL dialect and a parser that read a query differently.
//
// It admits every value that could carry a parameter at all, which is every
// value containing an `=`: a vendor splits a pair on a LITERAL `=`, so a pair
// spelled without one credits nobody. Matching the parameter NAMES here instead
// would miss a percent-encoded spelling (`%69rmp=`) that the vendor decodes and
// pays on, and the matcher below reads.
func plantedTagCandidateSQL() (string, []any) {
	return "ticket_url IS NOT NULL AND ticket_url LIKE ?", []any{"%=%"}
}

// plantedTagFindings returns every row of one source whose stored ticket URL
// credits an affiliate, newest first.
//
// Whole set, not a page: the count and the list are then the same answer.
// Callers page the slice.
func (s *DataQualityService) plantedTagFindings(category string) ([]*contracts.DataQualityItem, error) {
	source := plantedTagSources[category]

	where, args := plantedTagCandidateSQL()
	if source.Where != "" {
		where = source.Where + " AND " + where
	}

	type row struct {
		ID        uint
		Name      string
		Slug      string
		TicketURL string
	}
	var rows []row
	err := s.db.Raw(`
		SELECT id, `+source.NameColumn+` AS name, COALESCE(slug, '') AS slug, ticket_url
		FROM `+source.Table+`
		WHERE `+where+`
		ORDER BY id DESC
	`, args...).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	items := make([]*contracts.DataQualityItem, 0, len(rows))
	for _, r := range rows {
		param, host, ok := plantedAffiliateTag(r.TicketURL)
		if !ok || host == "" {
			continue
		}
		items = append(items, &contracts.DataQualityItem{
			EntityType: source.EntityType,
			EntityID:   r.ID,
			Name:       r.Name,
			Slug:       r.Slug,
			// The parameter and the host, and nothing else: the value is a
			// third party's account identifier and the rest of the URL is
			// contributor text.
			Reason:    param + " on " + host,
			ShowCount: 0,
		})
	}
	return items, nil
}

// getPlantedTicketTag pages the findings for one category.
func (s *DataQualityService) getPlantedTicketTag(category string, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
	findings, err := s.plantedTagFindings(category)
	if err != nil {
		return nil, 0, err
	}
	return pageItems(findings, limit, offset), int64(len(findings)), nil
}

// pageItems applies limit/offset to an already-ordered slice.
func pageItems(items []*contracts.DataQualityItem, limit, offset int) []*contracts.DataQualityItem {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return []*contracts.DataQualityItem{}
	}
	end := offset + limit
	if limit <= 0 || end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}

// plantedAffiliateTag reports the affiliate parameter a stored ticket URL
// credits and the host it credits it on.
func plantedAffiliateTag(rawURL string) (param, host string, ok bool) {
	// Tab, newline and carriage return are stripped by every URL parser and by
	// the browser before a request goes out, so a key spelled "ir<TAB>mp"
	// reaches the vendor as `irmp` and is credited. A scan that kept them would
	// read an unknown parameter and report nothing. The render side normalizes
	// the same three characters for the same reason.
	scannable := strings.NewReplacer("\t", "", "\n", "", "\r", "").Replace(rawURL)

	trimmed := strings.TrimSpace(scannable)
	if trimmed == "" {
		return "", "", false
	}
	// Text after `#` never reaches the vendor's server, so a parameter spelled
	// there credits nobody.
	beforeFragment, _, _ := strings.Cut(trimmed, "#")

	_, query, hasQuery := strings.Cut(beforeFragment, "?")
	if !hasQuery {
		return "", "", false
	}

	param, ok = firstAffiliateParam(query)
	if !ok {
		return "", "", false
	}
	return param, ticketURLHost(beforeFragment), true
}

// firstAffiliateParam reads the query the way a vendor's server does: `&` and
// `;` both separate pairs, and a key is percent-decoded with `+` meaning space.
// A valueless parameter credits nobody and is not a tag.
//
// Case-sensitive and untrimmed, because that is what a vendor reads: `IRMP`,
// `irmp%20` and `+irmp` are spellings no advertiser credits, and reporting them
// would name an operator to a row nobody is being paid for.
func firstAffiliateParam(query string) (string, bool) {
	for _, pair := range strings.FieldsFunc(query, func(r rune) bool {
		return r == '&' || r == ';'
	}) {
		key, value, hasValue := strings.Cut(pair, "=")
		if !hasValue || value == "" {
			continue
		}
		decoded, err := url.QueryUnescape(key)
		if err != nil {
			// An undecodable key is not a parameter name a vendor reads.
			continue
		}
		if slices.Contains(knownAffiliateParams, decoded) {
			return decoded, true
		}
	}
	return "", false
}

// ticketURLHost reads the host out of a stored ticket URL, supplying the scheme
// a contributor omitted so `ticketweb.com/e/1` still names a host. Returns the
// empty string when the value names none.
//
// The root label's trailing dot is dropped, matching the host the render side
// prints for the same row: `ticketweb.com.` and `ticketweb.com` are one host,
// and a reason that spelled them differently would read as two findings.
func ticketURLHost(rawURL string) string {
	if parsed, err := url.Parse(rawURL); err == nil && parsed.Host != "" {
		return strings.TrimSuffix(parsed.Hostname(), ".")
	}
	if parsed, err := url.Parse("https://" + rawURL); err == nil {
		return strings.TrimSuffix(parsed.Hostname(), ".")
	}
	return ""
}
