package venueaddress

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/utils"
)

// Extraction methods, reported per hit.
const (
	MethodJSONLD    = "json-ld"
	MethodMicrodata = "microdata"
	MethodAI        = "ai"
)

// candidate is one postal address a page publishes, with the name of the
// entity the page attaches it to when it gives one.
type candidate struct {
	Name     string
	Street   string
	City     string
	State    string
	Postcode string
	Method   string
}

// structuredCandidates returns every schema.org address the document
// publishes, JSON-LD first, then microdata.
func structuredCandidates(doc *html.Node) []candidate {
	var out []candidate
	walk(doc, func(n *html.Node) {
		if n.DataAtom == atom.Script && strings.EqualFold(strings.TrimSpace(attr(n, "type")), "application/ld+json") {
			var v interface{}
			if err := json.Unmarshal([]byte(textOf(n)), &v); err == nil {
				out = append(out, jsonLDCandidates(v)...)
			}
		}
	})
	return append(out, microdataCandidates(doc)...)
}

// jsonLDCandidates walks a decoded JSON-LD value and returns one candidate
// per object that carries an "address", named by that object's "name".
func jsonLDCandidates(v interface{}) []candidate {
	var out []candidate
	switch t := v.(type) {
	case []interface{}:
		for _, item := range t {
			out = append(out, jsonLDCandidates(item)...)
		}
	case map[string]interface{}:
		if addr, ok := t["address"]; ok {
			name := jsonString(t["name"])
			for _, a := range asList(addr) {
				if c, ok := jsonLDAddress(a); ok {
					c.Name = name
					out = append(out, c)
				}
			}
		}
		for k, child := range t {
			if k == "address" {
				continue
			}
			out = append(out, jsonLDCandidates(child)...)
		}
	}
	return out
}

func jsonLDAddress(v interface{}) (candidate, bool) {
	switch a := v.(type) {
	case string:
		c := parseAddressLine(a)
		c.Method = MethodJSONLD
		return c, c.Street != ""
	case map[string]interface{}:
		c := candidate{
			Street:   jsonString(a["streetAddress"]),
			City:     jsonString(a["addressLocality"]),
			State:    jsonString(a["addressRegion"]),
			Postcode: jsonString(a["postalCode"]),
			Method:   MethodJSONLD,
		}
		return c, c.Street != ""
	}
	return candidate{}, false
}

// parseAddressLine splits a one-line address ("1035 N Western Ave, Chicago,
// IL 60622") into its parts: the first comma- or newline-separated part is the
// street, the second the city, and a third "ST 12345" part the state and
// postcode.
func parseAddressLine(s string) candidate {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' })
	if len(parts) == 0 {
		return candidate{}
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	c := candidate{Street: parts[0]}
	if len(parts) > 1 {
		c.City = parts[1]
	}
	if len(parts) > 2 {
		fields := strings.Fields(parts[2])
		if len(fields) > 0 {
			c.State = fields[0]
		}
		if len(fields) > 1 {
			c.Postcode = fields[1]
		}
	}
	return c
}

func asList(v interface{}) []interface{} {
	if l, ok := v.([]interface{}); ok {
		return l
	}
	return []interface{}{v}
}

// jsonString reads a JSON-LD text value: a string, the first string of an
// array, or an object's "@value" / "name".
func jsonString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case []interface{}:
		for _, item := range t {
			if s := jsonString(item); s != "" {
				return s
			}
		}
	case map[string]interface{}:
		if s := jsonString(t["@value"]); s != "" {
			return s
		}
		return jsonString(t["name"])
	}
	return ""
}

// microdataCandidates reads itemtype=".../PostalAddress" scopes, named by the
// "name" property of the scope that contains them.
func microdataCandidates(doc *html.Node) []candidate {
	var out []candidate
	walk(doc, func(n *html.Node) {
		if !strings.Contains(strings.ToLower(attr(n, "itemtype")), "schema.org/postaladdress") {
			return
		}
		c := candidate{Method: MethodMicrodata}
		walk(n, func(p *html.Node) {
			if p == n || nearestScope(p) != n {
				return
			}
			switch attr(p, "itemprop") {
			case "streetAddress":
				c.Street = itempropValue(p)
			case "addressLocality":
				c.City = itempropValue(p)
			case "addressRegion":
				c.State = itempropValue(p)
			case "postalCode":
				c.Postcode = itempropValue(p)
			}
		})
		if parent := nearestScope(n); parent != nil {
			walk(parent, func(p *html.Node) {
				if c.Name == "" && attr(p, "itemprop") == "name" && nearestScope(p) == parent {
					c.Name = itempropValue(p)
				}
			})
		}
		if c.Street != "" {
			out = append(out, c)
		}
	})
	return out
}

// nearestScope returns the closest strict ancestor carrying itemscope.
func nearestScope(n *html.Node) *html.Node {
	for p := n.Parent; p != nil; p = p.Parent {
		if hasAttr(p, "itemscope") {
			return p
		}
	}
	return nil
}

func itempropValue(n *html.Node) string {
	if v := attr(n, "content"); v != "" {
		return strings.TrimSpace(v)
	}
	return collapseSpace(textOf(n))
}

// skippedElements carry no visible text.
var skippedElements = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Noscript: true, atom.Template: true,
	atom.Svg: true, atom.Head: true, atom.Iframe: true,
}

// blockElements end a line of visible text.
var blockElements = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.Br: true, atom.Li: true, atom.Tr: true,
	atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Section: true, atom.Article: true, atom.Footer: true, atom.Header: true,
	atom.Address: true, atom.Td: true, atom.Ul: true, atom.Ol: true, atom.Table: true,
	atom.Nav: true, atom.Aside: true, atom.Main: true,
}

// visibleText returns the document's rendered text, one block per line, with
// whitespace collapsed and empty lines dropped.
func visibleText(doc *html.Node) string {
	var b strings.Builder
	var rec func(n *html.Node)
	rec = func(n *html.Node) {
		if n.Type == html.ElementNode && skippedElements[n.DataAtom] {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
		if n.Type == html.ElementNode && blockElements[n.DataAtom] {
			b.WriteByte('\n')
		}
	}
	rec(doc)
	var lines []string
	for _, line := range strings.Split(b.String(), "\n") {
		if line = collapseSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// trimForPrompt keeps a long page's opening and, at greater length, its end,
// because a venue address most often sits in the footer or a contact block.
func trimForPrompt(text string, head, tail int) string {
	if len(text) <= head+tail {
		return text
	}
	h := text[:runeBoundary(text, head)]
	t := text[runeBoundary(text, len(text)-tail):]
	return h + "\n[...]\n" + t
}

// runeBoundary moves i back to the start of the rune it falls inside.
func runeBoundary(s string, i int) int {
	for i > 0 && i < len(s) && s[i]&0xC0 == 0x80 {
		i--
	}
	return i
}

var poBox = regexp.MustCompile(`(?i)\bp\.?\s*o\.?\s*box\b|\bpost\s+office\s+box\b`)

// cleanStreet reduces a published street value to the line stored in
// venues.address: whitespace collapsed, trailing punctuation dropped, and a
// city (with whatever follows it) cut off when the value repeats it.
func cleanStreet(street, city string) string {
	s := collapseSpace(strings.ReplaceAll(street, "\n", ", "))
	if city = strings.TrimSpace(city); city != "" {
		if i := strings.Index(strings.ToLower(s), ", "+strings.ToLower(city)); i > 0 {
			s = s[:i]
		}
	}
	return strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s), ",.;:"))
}

// usableStreet reports whether s reads as a street line: letters and a house
// number, not a P.O. box, and of plausible length.
func usableStreet(s string) bool {
	if len(s) < 4 || len(s) > 200 || poBox.MatchString(s) {
		return false
	}
	hasDigit, hasLetter := false, false
	for _, r := range s {
		hasDigit = hasDigit || unicode.IsDigit(r)
		hasLetter = hasLetter || unicode.IsLetter(r)
	}
	return hasDigit && hasLetter
}

// genericNameTokens are words too common in venue names to identify one on
// their own: "Hall" alone must not match "Lincoln Hall".
var genericNameTokens = map[string]bool{
	"the": true, "and": true, "of": true, "at": true, "hall": true, "bar": true,
	"club": true, "theatre": true, "theater": true, "room": true, "lounge": true,
	"tavern": true, "pub": true, "ballroom": true, "house": true, "cafe": true,
	"stage": true, "music": true, "venue": true, "center": true, "centre": true,
	"arts": true, "art": true, "gallery": true, "brewery": true, "brewing": true,
	"co": true, "company": true, "saint": true, "street": true, "live": true,
}

// NamesMatch reports whether two venue names plausibly name the same place:
// identical once folded (case, diacritics, punctuation, apostrophes, a leading
// "the"), or one name's words all appear in the other and the two share a word
// that is not generic. "Turner Hall" matches "Turner Hall Ballroom" and
// "Schuba's" matches "Schubas Tavern"; "Hall" does not match "Lincoln Hall".
func NamesMatch(a, b string) bool {
	ta, tb := nameTokens(a), nameTokens(b)
	if len(ta) == 0 || len(tb) == 0 {
		return false
	}
	if strings.Join(ta, " ") == strings.Join(tb, " ") {
		return true
	}
	short, long := ta, tb
	if len(short) > len(long) {
		short, long = long, short
	}
	inLong := make(map[string]bool, len(long))
	for _, t := range long {
		inLong[t] = true
	}
	distinctive := false
	for _, t := range short {
		if !inLong[t] {
			return false
		}
		distinctive = distinctive || !genericNameTokens[t]
	}
	return distinctive
}

// nameTokens is the venue-dedup key (utils.NormalizeVenueName: apostrophes,
// "&", a leading "the", street and venue-type abbreviations) with diacritics
// folded on top, split into words.
func nameTokens(s string) []string {
	return strings.Fields(geo.FoldPlaceName(utils.NormalizeVenueName(s)))
}

// CityMatches reports whether a city printed on a page names the venue's city:
// the same place under the geocoder's folding, or the venue city appearing as
// whole words inside a longer printed value ("Chicago IL").
func CityMatches(printed, venueCity string) bool {
	if geo.SamePlaceName(printed, venueCity) {
		return true
	}
	p, v := geo.FoldPlaceName(printed), geo.FoldPlaceName(venueCity)
	return v != "" && strings.Contains(" "+p+" ", " "+v+" ")
}

// appearsIn reports whether street is printed in text, comparing folded forms
// so case, punctuation, and spacing differences do not matter.
func appearsIn(street, text string) bool {
	s := geo.FoldPlaceName(street)
	return s != "" && strings.Contains(" "+geo.FoldPlaceName(text)+" ", " "+s+" ")
}

func walk(n *html.Node, fn func(*html.Node)) {
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, fn)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func textOf(n *html.Node) string {
	var b strings.Builder
	walk(n, func(c *html.Node) {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
			b.WriteByte(' ')
		}
	})
	return b.String()
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
