package catalog

import (
	"fmt"

	"psychic-homily-backend/internal/api/handlers/shared"
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

// TestTagLinks_ColumnWidthsMatchTheURLRegistry closes the third leg of the cap
// agreement. TestTagLinkCapsMatchTheURLRegistry holds the request structs to
// urlFieldSpecs; this reads the widths back out of the MIGRATED database, so a
// cap raised in the registry and the struct tags but not in the DDL fails here
// rather than as a Postgres 22001 on a rare admin write.
//
// It reads information_schema rather than the migration file, so a later ALTER
// is covered too.
func (s *TagHandlerIntegrationSuite) TestTagLinks_ColumnWidthsMatchTheURLRegistry() {
	for _, field := range []string{"website", "instagram", "bandcamp"} {
		want, ok := shared.URLFieldMaxLength(field)
		s.Require().True(ok, "urlFieldSpecs does not know %q", field)

		var got int
		err := s.deps.DB.Raw(
			`SELECT character_maximum_length FROM information_schema.columns
			 WHERE table_name = 'tags' AND column_name = ?`, field,
		).Scan(&got).Error
		s.Require().NoError(err)
		s.Equal(want, got, "tags.%s is %d wide, urlFieldSpecs caps at %d", field, got, want)
	}
}

// TestTagLinks_MergeDiscardsTheSourcesLinks records what merging does to the
// new columns: nothing carries them to the target, and the source row is hard
// deleted, so they are gone.
//
// This pins the behaviour rather than endorsing it. MergeTags has an explicit
// carry-over inventory (entity tags, votes, aliases, usage_count, is_official)
// and description is already dropped the same way; extending that inventory is
// a product decision this ticket did not make. The test is here so the next
// person changing it is changing something a test names.
func (s *TagHandlerIntegrationSuite) TestTagLinks_MergeDiscardsTheSourcesLinks() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	website := "https://rubberbrotherrecords.test"
	sourceReq := &CreateTagRequest{}
	sourceReq.Body.Name = "Merge Source Crew"
	sourceReq.Body.Category = catalogm.TagCategoryCrew
	sourceReq.Body.Website = &website
	source, err := s.handler.CreateTagHandler(ctx, sourceReq)
	s.Require().NoError(err)

	target := s.createTagViaHandler(admin, "Merge Target Crew", catalogm.TagCategoryCrew)

	mergeReq := &MergeTagsRequest{SourceID: fmt.Sprintf("%d", source.Body.ID)}
	mergeReq.Body.TargetID = target.Body.ID
	_, err = s.handler.MergeTagsHandler(ctx, mergeReq)
	s.Require().NoError(err)

	var merged catalogm.Tag
	s.Require().NoError(s.deps.DB.First(&merged, target.Body.ID).Error)
	s.Nil(merged.Website, "merge does not carry the source's links to the target")

	var sourceCount int64
	s.deps.DB.Model(&catalogm.Tag{}).Where("id = ?", source.Body.ID).Count(&sourceCount)
	s.EqualValues(0, sourceCount, "the source row is hard deleted, taking its links with it")
}
