package venueaddress

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestNamesMatch(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"Turner Hall Ballroom", "Turner Hall", true},
		{"Schuba's", "Schubas Tavern", true},
		{"The Empty Bottle", "Empty Bottle", true},
		{"Hi-Dive", "Hi Dive", true},
		{"Club Congress", "Club Congress", true},
		{"Café Racer", "Cafe Racer", true},
		{"7th St Entry", "7th Street Entry", true},
		{"The Vic Theatre", "Vic Theater", true},
		{"Hall", "Lincoln Hall", false},
		{"The Bar", "Valley Bar", false},
		{"Lincoln Hall", "Schubas Tavern", false},
		{"Empty Bottle", "Full Bottle", false},
		{"", "Anything", false},
	}
	for _, tt := range tests {
		if got := NamesMatch(tt.a, tt.b); got != tt.want {
			t.Errorf("NamesMatch(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCityMatches(t *testing.T) {
	tests := []struct {
		printed, city string
		want          bool
	}{
		{"Chicago", "Chicago", true},
		{"Chicago IL 60622", "Chicago", true},
		{"St. Paul", "Saint Paul", true},
		{"Montréal", "Montreal", true},
		{"Evanston", "Chicago", false},
		{"North Chicagoland", "Chicago", false},
	}
	for _, tt := range tests {
		if got := CityMatches(tt.printed, tt.city); got != tt.want {
			t.Errorf("CityMatches(%q, %q) = %v, want %v", tt.printed, tt.city, got, tt.want)
		}
	}
}

func TestUsableStreet(t *testing.T) {
	tests := []struct {
		street string
		want   bool
	}{
		{"1035 N Western Ave", true},
		{"Hauptstrasse 12", true},
		{"Valette Street", false},
		{"P.O. Box 1234", false},
		{"PO Box 77", false},
		{"12", false},
		{strings.Repeat("9 Long Street ", 20), false},
	}
	for _, tt := range tests {
		if got := usableStreet(tt.street); got != tt.want {
			t.Errorf("usableStreet(%q) = %v, want %v", tt.street, got, tt.want)
		}
	}
}

func TestCleanStreet(t *testing.T) {
	tests := []struct {
		street, city, want string
	}{
		{"1807 S. Allport St.", "", "1807 S. Allport St"},
		{"3734 W. Belmont Ave., Chicago, IL 60618", "Chicago", "3734 W. Belmont Ave"},
		{"  401 W   Van Buren St ,", "", "401 W Van Buren St"},
		{"1035 N Western Ave\nChicago IL", "Chicago IL", "1035 N Western Ave"},
	}
	for _, tt := range tests {
		if got := cleanStreet(tt.street, tt.city); got != tt.want {
			t.Errorf("cleanStreet(%q, %q) = %q, want %q", tt.street, tt.city, got, tt.want)
		}
	}
}

func TestParseAddressLine(t *testing.T) {
	c := parseAddressLine("1035 N Western Ave\nChicago, IL 60622")
	if c.Street != "1035 N Western Ave" || c.City != "Chicago" || c.State != "IL" || c.Postcode != "60622" {
		t.Fatalf("parseAddressLine = %+v", c)
	}
	if got := parseAddressLine(""); got.Street != "" {
		t.Fatalf("empty line produced %+v", got)
	}
}

func parse(t *testing.T, doc string) *html.Node {
	t.Helper()
	n, err := html.Parse(bytes.NewReader([]byte(doc)))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestStructuredCandidates_JSONLDNestedLocationAndGraph(t *testing.T) {
	doc := parse(t, `<html><head><script type="application/ld+json">
	{"@graph":[
	  {"@type":"Organization","name":"Promoter Inc","address":{"streetAddress":"1 Office Park","addressLocality":"Chicago"}},
	  {"@type":"Event","name":"A Show","location":{"@type":"MusicVenue","name":"Lincoln Hall",
	    "address":{"@type":"PostalAddress","streetAddress":"2424 N Lincoln Ave","addressLocality":"Chicago","addressRegion":"IL","postalCode":"60614"}}}
	]}</script></head><body></body></html>`)
	cands := structuredCandidates(doc)
	if len(cands) != 2 {
		t.Fatalf("got %d candidates: %+v", len(cands), cands)
	}
	c, ok, why := chooseCandidate(cands, Venue{Name: "Lincoln Hall", City: "Chicago", State: "IL"}, SourceWebsite)
	if !ok || c.Street != "2424 N Lincoln Ave" {
		t.Fatalf("choose = %+v ok=%v why=%s", c, ok, why)
	}
}

func TestStructuredCandidates_Microdata(t *testing.T) {
	doc := parse(t, `<html><body><div itemscope itemtype="https://schema.org/MusicVenue">
	<span itemprop="name">Hi-Dive</span>
	<div itemprop="address" itemscope itemtype="https://schema.org/PostalAddress">
	  <span itemprop="streetAddress">7 S Broadway</span>, <span itemprop="addressLocality">Denver</span>
	</div></div></body></html>`)
	cands := structuredCandidates(doc)
	if len(cands) != 1 || cands[0].Name != "Hi-Dive" || cands[0].Street != "7 S Broadway" || cands[0].City != "Denver" {
		t.Fatalf("microdata candidates = %+v", cands)
	}
}

func TestChooseCandidate_Rules(t *testing.T) {
	v := Venue{Name: "Lincoln Hall", City: "Chicago", State: "IL"}
	tests := []struct {
		name  string
		cands []candidate
		kind  SourceKind
		ok    bool
		want  string
	}{
		{
			name:  "another venue's address is refused",
			cands: []candidate{{Name: "Schubas Tavern", Street: "3159 N Southport Ave", City: "Chicago"}},
			kind:  SourceWebsite,
		},
		{
			name:  "another city is refused",
			cands: []candidate{{Name: "Lincoln Hall", Street: "2424 N Lincoln Ave", City: "Evanston"}},
			kind:  SourceWebsite,
		},
		{
			name:  "own site may print a bare street",
			cands: []candidate{{Street: "2424 N Lincoln Ave"}},
			kind:  SourceWebsite,
			ok:    true,
			want:  "2424 N Lincoln Ave",
		},
		{
			name:  "a ticket page must name the venue and its city",
			cands: []candidate{{Street: "2424 N Lincoln Ave", City: "Chicago"}},
			kind:  SourceTicket,
		},
		{
			name:  "a ticket page naming venue and city is accepted",
			cands: []candidate{{Name: "Lincoln Hall", Street: "2424 N Lincoln Ave", City: "Chicago"}},
			kind:  SourceTicket,
			ok:    true,
			want:  "2424 N Lincoln Ave",
		},
		{
			name:  "two unnamed streets are ambiguous",
			cands: []candidate{{Street: "2424 N Lincoln Ave"}, {Street: "3159 N Southport Ave"}},
			kind:  SourceWebsite,
		},
		{
			name:  "the named one wins over an unnamed one",
			cands: []candidate{{Street: "1 Office Park"}, {Name: "Lincoln Hall", Street: "2424 N Lincoln Ave"}},
			kind:  SourceWebsite,
			ok:    true,
			want:  "2424 N Lincoln Ave",
		},
		{
			name:  "a street without a house number is refused",
			cands: []candidate{{Name: "Lincoln Hall", Street: "Lincoln Avenue"}},
			kind:  SourceWebsite,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, ok, why := chooseCandidate(tt.cands, v, tt.kind)
			if ok != tt.ok || (ok && c.Street != tt.want) {
				t.Fatalf("choose = %q ok=%v why=%q, want %q ok=%v", c.Street, ok, why, tt.want, tt.ok)
			}
			if !ok && why == "" {
				t.Fatal("a refusal must say why")
			}
		})
	}
}

func TestVisibleTextAndTrim(t *testing.T) {
	doc := parse(t, `<html><head><title>x</title><style>.a{}</style></head><body>
	<script>var hidden = "1 Secret St";</script><p>Hello</p><footer>2424 N Lincoln Ave<br>Chicago</footer></body></html>`)
	text := visibleText(doc)
	if strings.Contains(text, "Secret") || !strings.Contains(text, "2424 N Lincoln Ave\nChicago") {
		t.Fatalf("visibleText = %q", text)
	}
	long := strings.Repeat("a", 100) + "TAIL"
	if got := trimForPrompt(long, 10, 4); !strings.HasPrefix(got, strings.Repeat("a", 10)) || !strings.HasSuffix(got, "TAIL") {
		t.Fatalf("trimForPrompt = %q", got)
	}
}

func TestAppearsIn(t *testing.T) {
	text := "Visit us at 1807 S. Allport St. Chicago, IL"
	if !appearsIn("1807 S Allport St", text) {
		t.Error("punctuation differences must not hide a printed street")
	}
	if appearsIn("807 S Allport St", text) {
		t.Error("a street must match on word boundaries")
	}
	cyrillic := "Москва, ул. Ленина 5"
	if appearsIn("ул. Пушкина 5", cyrillic) {
		t.Error("a non-Latin street must not match on its digits alone")
	}
	if !appearsIn("ул. Ленина 5", cyrillic) {
		t.Error("a printed non-Latin street must match")
	}
}

func TestFinder_TicketPagesSkipTheAIPathAndWalledOnesUseNoSlot(t *testing.T) {
	v := Venue{Name: "Lincoln Hall", City: "Chicago", State: "IL"}
	plain := []byte("<p>Lincoln Hall, 2424 N Lincoln Ave, Chicago</p>")
	named := []byte(`<script type="application/ld+json">{"@type":"Event","location":{"name":"Lincoln Hall","address":{"streetAddress":"2424 N Lincoln Ave","addressLocality":"Chicago"}}}</script>`)
	ai := &answerAI{street: "2424 N Lincoln Ave"}

	res, err := NewFinder(fixturePages{"https://t.example/1": plain}, ai).
		Find(context.Background(), v, []Source{{URL: "https://t.example/1", Kind: SourceTicket}})
	if err != nil || res.Found || ai.calls != 0 {
		t.Fatalf("a ticket page without schema.org data must not reach the AI: found=%v calls=%d err=%v", res.Found, ai.calls, err)
	}

	pages := fixturePages{"https://a.example/1": plain, "https://b.example/1": plain, "https://d.example/1": named}
	res, err = NewFinder(pages, nil).Find(context.Background(), v, []Source{
		{URL: "https://walled.example/1", Kind: SourceTicket}, // unavailable: uses no slot
		{URL: "https://a.example/1", Kind: SourceTicket},
		{URL: "https://d.example/1", Kind: SourceTicket},
		{URL: "https://b.example/1", Kind: SourceTicket},
	})
	if err != nil || !res.Found || res.Source.URL != "https://d.example/1" {
		t.Fatalf("res = %+v err = %v", res, err)
	}

	res, _ = NewFinder(pages, nil).Find(context.Background(), v, []Source{
		{URL: "https://a.example/1", Kind: SourceTicket},
		{URL: "https://b.example/1", Kind: SourceTicket},
		{URL: "https://d.example/1", Kind: SourceTicket},
	})
	if res.Found || !strings.Contains(strings.Join(res.Notes, " "), "not tried") {
		t.Fatalf("a third ticket page after two read ones must not be tried: %+v", res)
	}
}
