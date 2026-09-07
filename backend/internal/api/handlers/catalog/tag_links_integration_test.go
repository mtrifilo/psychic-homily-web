package catalog

import (
	"fmt"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Outbound links (PSY-1888), end to end through the handler, the service and
// the columns. Methods on TagHandlerIntegrationSuite, which tag_integration_test.go
// runs.

func (s *TagHandlerIntegrationSuite) TestTagLinks_StoredAndReturned() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	website := "https://rubberbrotherrecords.test"
	instagram := "https://instagram.com/rubberbrother"
	bandcamp := "https://rubberbrother.bandcamp.com"

	req := &CreateTagRequest{}
	req.Body.Name = "Rubber Brother Records"
	req.Body.Category = catalogm.TagCategoryCrew
	req.Body.Website = &website
	req.Body.Instagram = &instagram
	req.Body.Bandcamp = &bandcamp

	resp, err := s.handler.CreateTagHandler(ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body.Social.Website)
	s.Equal(website, *resp.Body.Social.Website)
	s.Require().NotNil(resp.Body.Social.Instagram)
	s.Equal(instagram, *resp.Body.Social.Instagram)
	s.Require().NotNil(resp.Body.Social.Bandcamp)
	s.Equal(bandcamp, *resp.Body.Social.Bandcamp)

	// The detail endpoint the tag page reads carries the same three columns.
	detail, err := s.handler.GetTagDetailHandler(s.deps.Ctx, &GetTagDetailRequest{TagID: resp.Body.Slug})
	s.Require().NoError(err)
	s.Require().NotNil(detail.Body.Social.Website)
	s.Equal(website, *detail.Body.Social.Website)
	s.Require().NotNil(detail.Body.Social.Bandcamp)
	s.Equal(bandcamp, *detail.Body.Social.Bandcamp)
}

func (s *TagHandlerIntegrationSuite) TestTagLinks_RefusedHostIsNotStored() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	lookalike := "https://instagram.com.evil.test/rubberbrother"
	req := &CreateTagRequest{}
	req.Body.Name = "Refused Crew"
	req.Body.Category = catalogm.TagCategoryCrew
	req.Body.Instagram = &lookalike

	_, err := s.handler.CreateTagHandler(ctx, req)
	s.Require().Error(err)
	s.Contains(err.Error(), "must be a link on instagram.com")

	var count int64
	s.deps.DB.Model(&catalogm.Tag{}).Where("name = ?", "Refused Crew").Count(&count)
	s.EqualValues(0, count, "a refused link must not create the tag")
}

func (s *TagHandlerIntegrationSuite) TestTagLinks_UpdateKeepsOmittedClearsEmpty() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	website := "https://rubberbrotherrecords.test"
	instagram := "https://instagram.com/rubberbrother"
	createReq := &CreateTagRequest{}
	createReq.Body.Name = "Update Crew"
	createReq.Body.Category = catalogm.TagCategoryCrew
	createReq.Body.Website = &website
	createReq.Body.Instagram = &instagram
	created, err := s.handler.CreateTagHandler(ctx, createReq)
	s.Require().NoError(err)

	// An update mentioning neither column leaves both alone.
	name := "Update Crew Renamed"
	untouched := &UpdateTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	untouched.Body.Name = &name
	kept, err := s.handler.UpdateTagHandler(ctx, untouched)
	s.Require().NoError(err)
	s.Require().NotNil(kept.Body.Social.Website)
	s.Equal(website, *kept.Body.Social.Website)
	s.Require().NotNil(kept.Body.Social.Instagram)

	// An empty string clears that column, and only that column.
	empty := ""
	clearReq := &UpdateTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	clearReq.Body.Instagram = &empty
	cleared, err := s.handler.UpdateTagHandler(ctx, clearReq)
	s.Require().NoError(err)
	s.Nil(cleared.Body.Social.Instagram)
	s.Require().NotNil(cleared.Body.Social.Website)
	s.Equal(website, *cleared.Body.Social.Website)

	var stored catalogm.Tag
	s.Require().NoError(s.deps.DB.First(&stored, created.Body.ID).Error)
	s.Nil(stored.Instagram, "a cleared column is NULL, not an empty string")
}

// TestTagLinks_UpdateRefusalLeavesTheRowAlone: a refused value must not take
// the fields sent beside it with it, and must not half-apply.
func (s *TagHandlerIntegrationSuite) TestTagLinks_UpdateRefusalLeavesTheRowAlone() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	website := "https://rubberbrotherrecords.test"
	createReq := &CreateTagRequest{}
	createReq.Body.Name = "Refusal Crew"
	createReq.Body.Category = catalogm.TagCategoryCrew
	createReq.Body.Website = &website
	created, err := s.handler.CreateTagHandler(ctx, createReq)
	s.Require().NoError(err)

	newName := "Refusal Crew Renamed"
	lookalike := "https://instagram.com.evil.test/x"
	bad := &UpdateTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	bad.Body.Name = &newName
	bad.Body.Instagram = &lookalike
	_, err = s.handler.UpdateTagHandler(ctx, bad)
	s.Require().Error(err)

	var stored catalogm.Tag
	s.Require().NoError(s.deps.DB.First(&stored, created.Body.ID).Error)
	s.Equal("Refusal Crew", stored.Name)
	s.Nil(stored.Instagram)
	s.Require().NotNil(stored.Website)
	s.Equal(website, *stored.Website)
}

// TestTagLinks_AcceptedOnAnyCategory: the columns are category-agnostic. The
// crew-only rule is the admin editor's, not the schema's.
func (s *TagHandlerIntegrationSuite) TestTagLinks_AcceptedOnAnyCategory() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	website := "https://zine.test"
	req := &CreateTagRequest{}
	req.Body.Name = "Locale With Link"
	req.Body.Category = catalogm.TagCategoryLocale
	req.Body.Website = &website

	resp, err := s.handler.CreateTagHandler(ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body.Social.Website)
	s.Equal(website, *resp.Body.Social.Website)
}

// TestTagLinks_InlineCreateTakesNoLinks: the entity-page inline create path is
// contributor-reachable and mints a tag with no links, whatever the caller
// sends, because its request shape carries none.
func (s *TagHandlerIntegrationSuite) TestTagLinks_InlineCreateTakesNoLinks() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &AddTagToEntityRequest{EntityType: "artist", EntityID: "1"}
	req.Body.TagName = "inline-minted"
	req.Body.Category = catalogm.TagCategoryOther
	_, err := s.handler.AddTagToEntityHandler(ctx, req)
	s.Require().NoError(err)

	var stored catalogm.Tag
	s.Require().NoError(s.deps.DB.Where("name = ?", "inline-minted").First(&stored).Error)
	s.Nil(stored.Website)
	s.Nil(stored.Instagram)
	s.Nil(stored.Bandcamp)
}
