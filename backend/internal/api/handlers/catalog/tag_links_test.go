package catalog

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/utils"
)

func tagLinkPtr(s string) *string { return &s }

// refusedTagLinkValues are the shapes the shared social gate refuses. They are
// listed by what makes each one refusable, because a host allowlist that
// accepted any of them would accept it on every entity, not only on a tag.
var refusedTagLinkValues = []struct {
	name  string
	links catalogm.TagLinks
}{
	{"off-platform instagram host", catalogm.TagLinks{Instagram: tagLinkPtr("https://instagram-login.evil.test/psychichomily")}},
	{"instagram lookalike suffix", catalogm.TagLinks{Instagram: tagLinkPtr("https://instagram.com.evil.test/psychichomily")}},
	{"instagram prefix lookalike", catalogm.TagLinks{Instagram: tagLinkPtr("https://notinstagram.com/psychichomily")}},
	{"off-platform bandcamp host", catalogm.TagLinks{Bandcamp: tagLinkPtr("https://bandcamp.com.evil.test/album/x")}},
	{"non-http scheme on the unanchored website", catalogm.TagLinks{Website: tagLinkPtr("javascript:alert(1)")}},
	{"host-less website", catalogm.TagLinks{Website: tagLinkPtr("https:///path")}},
}

// TestTagLinkRefusalsCarryThePolicysOwnWording pins each refusal to the message
// the POLICY function produces, not to the wrapper that calls it.
//
// Comparing validateTagLinks against ValidateSocialURLs would be comparing a
// call with itself. utils.ValidateSocialHost and utils.ValidateHTTPURL are the
// two rules underneath, and they are what the artist, venue, label and festival
// columns of these names reach as well, so a tag refusal that stopped matching
// them would be a second policy.
func TestTagLinkRefusalsCarryThePolicysOwnWording(t *testing.T) {
	for _, tc := range refusedTagLinkValues {
		t.Run(tc.name, func(t *testing.T) {
			got := validateTagLinks(tc.links)
			if got == nil {
				t.Fatalf("expected a refusal for %+v", tc.links)
			}
			field, value := suppliedTagLink(tc.links)
			label, ok := shared.URLFieldDisplayName(field)
			if !ok {
				t.Fatalf("urlFieldSpecs does not know %q", field)
			}
			want := utils.ValidateHTTPURL(value, label)
			if want == nil {
				want = utils.ValidateSocialHost(field, label, value)
			}
			if want == nil {
				t.Fatalf("neither underlying rule refuses %q", value)
			}
			if !strings.Contains(got.Error(), want.Error()) {
				t.Errorf("tag refusal %q does not carry the policy message %q", got, want)
			}
		})
	}
}

// suppliedTagLink names the one field a refusal case sets.
func suppliedTagLink(links catalogm.TagLinks) (string, string) {
	switch {
	case links.Instagram != nil:
		return "instagram", *links.Instagram
	case links.Bandcamp != nil:
		return "bandcamp", *links.Bandcamp
	case links.Website != nil:
		return "website", *links.Website
	}
	return "", ""
}

// TestTagLinkGateAppliesEachRuleToItsOwnField is the guard on the positional
// call inside validateTagLinks. website takes any host and bandcamp does not,
// so a value accepted as a website and refused as a bandcamp separates the two
// slots: swapping them makes exactly one of these two assertions fail.
func TestTagLinkGateAppliesEachRuleToItsOwnField(t *testing.T) {
	offPlatform := "https://rubberbrotherrecords.test"

	if err := validateTagLinks(catalogm.TagLinks{Website: &offPlatform}); err != nil {
		t.Errorf("website takes any host, got %v", err)
	}
	if err := validateTagLinks(catalogm.TagLinks{Bandcamp: &offPlatform}); err == nil {
		t.Error("bandcamp is anchored to bandcamp.com and must refuse an off-platform host")
	}

	onPlatform := "https://rubberbrother.bandcamp.com"
	if err := validateTagLinks(catalogm.TagLinks{Bandcamp: &onPlatform}); err != nil {
		t.Errorf("an on-platform bandcamp URL must be accepted, got %v", err)
	}
}

// refusedTagLinkExample is one shape from the table above. The handler tests
// use a single case: the table proves the RULE, these prove each handler runs
// it before the service sees anything.
var refusedTagLinkExample = catalogm.TagLinks{
	Instagram: tagLinkPtr("https://instagram.com.evil.test/psychichomily"),
}

func TestCreateTagHandlerRefusesBeforeTheService(t *testing.T) {
	called := false
	mock := &testhelpers.MockTagService{
		CreateTagFn: func(string, *string, *uint, string, bool, *uint, catalogm.TagLinks) (*catalogm.Tag, error) {
			called = true
			return &catalogm.Tag{ID: 1}, nil
		},
	}
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
	req := &CreateTagRequest{}
	req.Body.Name = "Rubber Brother Records"
	req.Body.Category = catalogm.TagCategoryCrew
	req.Body.Instagram = refusedTagLinkExample.Instagram

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})
	if _, err := h.CreateTagHandler(ctx, req); err == nil {
		t.Fatal("expected a refusal")
	}
	if called {
		t.Error("a refused link reached the service")
	}
}

func TestUpdateTagHandlerRefusesBeforeTheService(t *testing.T) {
	called := false
	mock := &testhelpers.MockTagService{
		UpdateTagFn: func(uint, *string, *string, *uint, *string, *bool, catalogm.TagLinks) (*catalogm.Tag, error) {
			called = true
			return &catalogm.Tag{ID: 1}, nil
		},
	}
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
	req := &UpdateTagRequest{TagID: "1"}
	req.Body.Instagram = refusedTagLinkExample.Instagram

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})
	if _, err := h.UpdateTagHandler(ctx, req); err == nil {
		t.Fatal("expected a refusal")
	}
	if called {
		t.Error("a refused link reached the service")
	}
}

func TestCreateTagHandlerPassesAcceptedLinksThrough(t *testing.T) {
	var seen catalogm.TagLinks
	mock := &testhelpers.MockTagService{
		CreateTagFn: func(_ string, _ *string, _ *uint, _ string, _ bool, _ *uint, links catalogm.TagLinks) (*catalogm.Tag, error) {
			seen = links
			return &catalogm.Tag{ID: 4, Name: "Rubber Brother Records", Slug: "rubber-brother-records"}, nil
		},
	}
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
	req := &CreateTagRequest{}
	req.Body.Name = "Rubber Brother Records"
	req.Body.Category = catalogm.TagCategoryCrew
	req.Body.Website = tagLinkPtr("https://rubberbrotherrecords.test")
	req.Body.Instagram = tagLinkPtr("https://www.instagram.com/rubberbrother")
	req.Body.Bandcamp = tagLinkPtr("https://rubberbrother.bandcamp.com")

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})
	if _, err := h.CreateTagHandler(ctx, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seen.Website == nil || *seen.Website != "https://rubberbrotherrecords.test" {
		t.Errorf("website not forwarded: %+v", seen.Website)
	}
	if seen.Instagram == nil || *seen.Instagram != "https://www.instagram.com/rubberbrother" {
		t.Errorf("instagram not forwarded: %+v", seen.Instagram)
	}
	if seen.Bandcamp == nil || *seen.Bandcamp != "https://rubberbrother.bandcamp.com" {
		t.Errorf("bandcamp not forwarded: %+v", seen.Bandcamp)
	}
}

// TestUpdateTagHandlerDistinguishesOmittedFromCleared is the reason the links
// travel as pointers: a request that does not mention a column must not clear
// it, and a request that sends an empty string must.
func TestUpdateTagHandlerDistinguishesOmittedFromCleared(t *testing.T) {
	var seen catalogm.TagLinks
	mock := &testhelpers.MockTagService{
		UpdateTagFn: func(_ uint, _ *string, _ *string, _ *uint, _ *string, _ *bool, links catalogm.TagLinks) (*catalogm.Tag, error) {
			seen = links
			return &catalogm.Tag{ID: 1}, nil
		},
	}
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})

	req := &UpdateTagRequest{TagID: "1"}
	req.Body.Website = tagLinkPtr("")
	if _, err := h.UpdateTagHandler(ctx, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seen.Website == nil || *seen.Website != "" {
		t.Errorf("an empty website should reach the service as a supplied empty value, got %+v", seen.Website)
	}
	if seen.Instagram != nil || seen.Bandcamp != nil {
		t.Error("an omitted column must not be supplied")
	}
}

func TestBuildTagResponseCarriesLinks(t *testing.T) {
	resp := buildTagResponse(&catalogm.Tag{
		ID:        3,
		Name:      "Rubber Brother Records",
		Slug:      "rubber-brother-records",
		Category:  catalogm.TagCategoryCrew,
		Website:   tagLinkPtr("https://rubberbrotherrecords.test"),
		Instagram: tagLinkPtr("https://instagram.com/rubberbrother"),
	})
	if resp.Social.Website == nil || *resp.Social.Website != "https://rubberbrotherrecords.test" {
		t.Errorf("website missing from the response: %+v", resp.Social.Website)
	}
	if resp.Social.Instagram == nil || *resp.Social.Instagram != "https://instagram.com/rubberbrother" {
		t.Errorf("instagram missing from the response: %+v", resp.Social.Instagram)
	}
	if resp.Social.Bandcamp != nil {
		t.Errorf("an unset column must stay null, got %+v", resp.Social.Bandcamp)
	}
}

// TestCreateTagHandlerAcceptsNoLinks keeps the columns optional: the create
// path a caller uses for a genre tag sends none of them and still works.
func TestCreateTagHandlerAcceptsNoLinks(t *testing.T) {
	mock := &testhelpers.MockTagService{
		CreateTagFn: func(_ string, _ *string, _ *uint, _ string, _ bool, _ *uint, links catalogm.TagLinks) (*catalogm.Tag, error) {
			if links.Website != nil || links.Instagram != nil || links.Bandcamp != nil {
				t.Errorf("expected no links supplied, got %+v", links)
			}
			return &catalogm.Tag{ID: 9, Name: "shoegaze", Slug: "shoegaze"}, nil
		},
	}
	h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
	req := &CreateTagRequest{}
	req.Body.Name = "shoegaze"
	req.Body.Category = catalogm.TagCategoryGenre
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})
	if _, err := h.CreateTagHandler(ctx, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestTagLinkCapsMatchTheURLRegistry holds the request structs' maxLength tags
// to urlFieldSpecs. A boundary that accepts more than the registry does would
// let an over-length value reach a column that is exactly this wide, where it
// fails as a Postgres 22001 rather than as a 422 the caller can act on.
//
// It does NOT reach the DDL: the column widths in the migration are a third
// spelling of these numbers and nothing here reads them.
func TestTagLinkCapsMatchTheURLRegistry(t *testing.T) {
	bodies := map[string]reflect.Type{
		"CreateTagRequest": reflect.TypeOf(CreateTagRequest{}.Body),
		"UpdateTagRequest": reflect.TypeOf(UpdateTagRequest{}.Body),
	}
	for _, field := range []string{"website", "instagram", "bandcamp"} {
		want, ok := shared.URLFieldMaxLength(field)
		if !ok {
			t.Fatalf("urlFieldSpecs does not know %q", field)
		}
		for name, body := range bodies {
			tag, found := jsonFieldTag(body, field)
			if !found {
				t.Errorf("%s has no %q field", name, field)
				continue
			}
			if got := tag.Get("maxLength"); got != strconv.Itoa(want) {
				t.Errorf("%s.%s maxLength=%q, registry says %d", name, field, got, want)
			}
		}
	}
}

// jsonFieldTag finds the struct field whose json name is the given one.
func jsonFieldTag(body reflect.Type, jsonName string) (reflect.StructTag, bool) {
	for i := 0; i < body.NumField(); i++ {
		f := body.Field(i)
		if strings.Split(f.Tag.Get("json"), ",")[0] == jsonName {
			return f.Tag, true
		}
	}
	return "", false
}
