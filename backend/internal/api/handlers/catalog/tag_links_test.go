package catalog

import (
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
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

// TestTagLinkGateMatchesTheArtistGate pins the tag columns to the SAME rule and
// the SAME wording the artist/venue/label/festival columns of these names get.
// A refusal that read differently here would be a second policy, which is the
// thing this handler deliberately does not have.
func TestTagLinkGateMatchesTheArtistGate(t *testing.T) {
	for _, tc := range refusedTagLinkValues {
		t.Run(tc.name, func(t *testing.T) {
			got := validateTagLinks(tc.links)
			if got == nil {
				t.Fatalf("expected a refusal for %+v", tc.links)
			}
			want := shared.ValidateSocialURLs(
				tc.links.Instagram, nil, nil, nil, nil, nil, tc.links.Bandcamp, tc.links.Website,
			)
			if want == nil || want.Error() != got.Error() {
				t.Errorf("tag refusal %q does not match the shared gate %v", got, want)
			}
		})
	}
}

func TestCreateTagHandlerRefusesRefusedLinks(t *testing.T) {
	for _, tc := range refusedTagLinkValues {
		t.Run(tc.name, func(t *testing.T) {
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
			req.Body.Website = tc.links.Website
			req.Body.Instagram = tc.links.Instagram
			req.Body.Bandcamp = tc.links.Bandcamp

			ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})
			if _, err := h.CreateTagHandler(ctx, req); err == nil {
				t.Fatal("expected a refusal")
			}
			if called {
				t.Error("a refused link reached the service")
			}
		})
	}
}

func TestUpdateTagHandlerRefusesRefusedLinks(t *testing.T) {
	for _, tc := range refusedTagLinkValues {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			mock := &testhelpers.MockTagService{
				UpdateTagFn: func(uint, *string, *string, *uint, *string, *bool, catalogm.TagLinks) (*catalogm.Tag, error) {
					called = true
					return &catalogm.Tag{ID: 1}, nil
				},
			}
			h := NewTagHandler(mock, nil, testhelpers.AllShowsVisible())
			req := &UpdateTagRequest{TagID: "1"}
			req.Body.Website = tc.links.Website
			req.Body.Instagram = tc.links.Instagram
			req.Body.Bandcamp = tc.links.Bandcamp

			ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})
			if _, err := h.UpdateTagHandler(ctx, req); err == nil {
				t.Fatal("expected a refusal")
			}
			if called {
				t.Error("a refused link reached the service")
			}
		})
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
