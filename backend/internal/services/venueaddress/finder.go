package venueaddress

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/net/html"

	"psychic-homily-backend/internal/services/contracts"
)

// SourceKind says what a candidate page is to the venue. It decides how much
// the page must prove: the venue's own pages may print a bare street, while a
// ticket vendor's page must name the venue and its city beside the address.
type SourceKind string

const (
	SourceWebsite SourceKind = "website"       // venues.website
	SourceIngest  SourceKind = "ingest_source" // the venue's registered source_configs page
	SourceTicket  SourceKind = "ticket_page"   // an upcoming show's ticket_url
)

// Source is one page to try.
type Source struct {
	URL  string
	Kind SourceKind
}

// Venue identifies the venue whose address is sought.
type Venue struct {
	Name  string
	City  string
	State string
}

// Result is the outcome of trying a venue's sources in order.
type Result struct {
	Found  bool
	Street string // the line to store in venues.address
	City   string // as the page printed it; may be empty
	Method string // MethodJSONLD, MethodMicrodata, or MethodAI
	Source Source // the page the address came from (hit only)
	// Notes records, per source tried, what happened, in order: why a page
	// was unreadable or which address on it was rejected and why.
	Notes []string
}

// PageGetter fetches one page; *Fetcher is the production implementation. An
// error wrapping ErrUnavailable is definitive, anything else transient.
type PageGetter interface {
	Fetch(ctx context.Context, rawURL string) (*Page, error)
}

// AddressExtractor is the AI extraction path for an address printed only as
// text; pipeline.ExtractionService implements it.
type AddressExtractor interface {
	ExtractVenueAddress(ctx context.Context, req contracts.VenueAddressExtractionRequest) (*contracts.VenueAddressExtraction, error)
}

// Prompt budget: the start of a long page plus a larger share of its end,
// where footers and contact blocks print addresses.
const (
	promptHeadChars = 6000
	promptTailChars = 18000
	// aiTimeout bounds one extraction call; the extraction service's HTTP
	// client sets none of its own.
	aiTimeout = 60 * time.Second
)

// Finder looks for a venue's street address on its pages.
type Finder struct {
	pages PageGetter
	ai    AddressExtractor // nil disables the AI fallback
}

// NewFinder returns a Finder; ai may be nil, in which case only schema.org
// addresses are read.
func NewFinder(pages PageGetter, ai AddressExtractor) *Finder {
	return &Finder{pages: pages, ai: ai}
}

// errTransient marks a source whose failure says nothing about the page.
var errTransient = errors.New("transient failure")

// Find tries sources in order and returns the first accepted address. It
// returns a non-nil error only when no address was found AND at least one
// source failed transiently: such a run must not be recorded as a miss,
// because a later run may read the page that failed.
func (f *Finder) Find(ctx context.Context, v Venue, sources []Source) (Result, error) {
	var res Result
	transient := false
	for _, src := range sources {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		got, note, err := f.trySource(ctx, v, src)
		res.Notes = append(res.Notes, fmt.Sprintf("%s (%s): %s", src.URL, src.Kind, note))
		if err != nil {
			transient = true
			continue
		}
		if got.Found {
			got.Notes = res.Notes
			return got, nil
		}
	}
	if transient {
		return res, fmt.Errorf("no address found and at least one page failed transiently: %w", errTransient)
	}
	return res, nil
}

// trySource reads one page: schema.org addresses first, then the AI path. The
// note describes the outcome for the report; err is non-nil only for a
// transient failure.
func (f *Finder) trySource(ctx context.Context, v Venue, src Source) (Result, string, error) {
	page, err := f.pages.Fetch(ctx, src.URL)
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			return Result{}, err.Error(), nil
		}
		return Result{}, "error: " + err.Error(), errTransient
	}
	doc, err := html.Parse(bytes.NewReader(page.HTML))
	if err != nil {
		return Result{}, "unparseable HTML", nil
	}

	// structuredWhy explains a schema.org address the rules refused; it leads
	// every later note so the report shows both attempts.
	c, ok, structuredWhy := chooseCandidate(structuredCandidates(doc), v, src.Kind)
	if ok {
		return hit(c, src), "found " + c.Method + " address", nil
	}
	note := func(last string) string {
		if structuredWhy == "" {
			return last
		}
		return structuredWhy + "; " + last
	}

	if f.ai == nil {
		return Result{}, note("no schema.org address; AI fallback disabled"), nil
	}
	text := visibleText(doc)
	if text == "" {
		return Result{}, note("no visible text"), nil
	}
	// The model reads only the trimmed text, so that is what its answer must
	// be printed in.
	prompt := trimForPrompt(text, promptHeadChars, promptTailChars)
	aiCtx, cancel := context.WithTimeout(ctx, aiTimeout)
	defer cancel()
	ext, err := f.ai.ExtractVenueAddress(aiCtx, contracts.VenueAddressExtractionRequest{
		VenueName: v.Name,
		City:      v.City,
		State:     v.State,
		PageURL:   page.URL,
		PageText:  prompt,
	})
	if err != nil {
		return Result{}, note("AI extraction failed: " + err.Error()), errTransient
	}
	if !ext.Found {
		return Result{}, note("AI found no address for this venue"), nil
	}
	if !appearsIn(ext.Street, prompt) {
		return Result{}, note(fmt.Sprintf("AI address %q is not printed on the page", ext.Street)), nil
	}
	aiCand := candidate{
		Name:   v.Name, // the model was asked for this venue's address only
		Street: ext.Street,
		City:   ext.City,
		Method: MethodAI,
	}
	c, ok, why := chooseCandidate([]candidate{aiCand}, v, src.Kind)
	if !ok {
		return Result{}, note(why), nil
	}
	return hit(c, src), "found AI-extracted address", nil
}

func hit(c candidate, src Source) Result {
	return Result{Found: true, Street: c.Street, City: c.City, Method: c.Method, Source: src}
}

// chooseCandidate applies the acceptance rules to a page's addresses and
// returns the one that belongs to the venue:
//   - the street must read as a street line (usableStreet);
//   - an address the page attaches to a name must name this venue;
//   - an address printed with a city must be in the venue's city;
//   - a ticket page must state both the venue's name and its city, because a
//     vendor page prints addresses of venues other than the one asked about;
//   - several distinct accepted streets are ambiguous unless exactly one of
//     them is attached to the venue's name.
//
// why explains the rejection when nothing is accepted and something was
// considered.
func chooseCandidate(cands []candidate, v Venue, kind SourceKind) (candidate, bool, string) {
	var accepted []candidate
	var reasons []string
	for _, c := range cands {
		c.Street = cleanStreet(c.Street, c.City)
		label := fmt.Sprintf("%s address %q", c.Method, c.Street)
		switch {
		case !usableStreet(c.Street):
			reasons = append(reasons, label+" is not a street line")
		case c.Name != "" && !NamesMatch(c.Name, v.Name):
			reasons = append(reasons, fmt.Sprintf("%s belongs to %q", label, c.Name))
		case c.City != "" && !CityMatches(c.City, v.City):
			reasons = append(reasons, fmt.Sprintf("%s is in %q, not %q", label, c.City, v.City))
		case kind == SourceTicket && (c.Name == "" || c.City == ""):
			reasons = append(reasons, label+" on a ticket page does not state the venue name and city")
		default:
			accepted = append(accepted, c)
		}
	}
	distinct := distinctStreets(accepted)
	if len(distinct) > 1 {
		var named []candidate
		for _, c := range distinct {
			if c.Name != "" {
				named = append(named, c)
			}
		}
		if len(named) == 1 {
			distinct = named
		} else {
			streets := make([]string, len(distinct))
			for i, c := range distinct {
				streets[i] = c.Street
			}
			return candidate{}, false, "ambiguous: several addresses could be the venue's (" + strings.Join(streets, "; ") + ")"
		}
	}
	if len(distinct) == 1 {
		return distinct[0], true, ""
	}
	return candidate{}, false, strings.Join(reasons, "; ")
}

// distinctStreets keeps one candidate per folded street, preferring one the
// page attaches to a name.
func distinctStreets(cands []candidate) []candidate {
	index := map[string]int{}
	var out []candidate
	for _, c := range cands {
		k := strings.ToLower(collapseSpace(c.Street))
		i, seen := index[k]
		switch {
		case !seen:
			index[k] = len(out)
			out = append(out, c)
		case out[i].Name == "" && c.Name != "":
			out[i] = c
		}
	}
	return out
}
