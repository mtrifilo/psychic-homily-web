package catalog

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
)

// =============================================================================
// Crew membership is trusted-tier (PSY-2045)
// =============================================================================

// entityTagCount counts the applications of one tag to one entity, so a refusal
// can be shown to have written nothing and a success to have written one row.
func (suite *TagServiceIntegrationTestSuite) entityTagCount(tagID uint, entityType string, entityID uint) int64 {
	var n int64
	suite.Require().NoError(suite.db.Model(&catalogm.EntityTag{}).
		Where("tag_id = ? AND entity_type = ? AND entity_id = ?", tagID, entityType, entityID).
		Count(&n).Error)
	return n
}

// The membership rule, both directions, per tier, with a descriptive tag as the
// control on the same caller and the same entity: a refusal has to be the
// CATEGORY rule, not the caller being blocked from tagging at all.
//
// Removal is exercised against an edge somebody else made, which is the case
// the rule exists for: stripping the booker that did put on the show.
func (suite *TagServiceIntegrationTestSuite) TestCrewMembershipRequiresTrustedTier() {
	cases := []struct {
		tier    string
		admin   bool
		allowed bool
	}{
		{tier: "new_user"},
		{tier: "contributor"},
		{tier: "trusted_contributor", allowed: true},
		{tier: "local_ambassador", allowed: true},
		{tier: "new_user", admin: true, allowed: true},
	}

	seeder := suite.createAdminUser("crew-membership-seeder")

	for i, tc := range cases {
		label := fmt.Sprintf("%s/admin=%t", tc.tier, tc.admin)

		var user *authm.User
		if tc.admin {
			user = suite.createAdminUser(fmt.Sprintf("crew-membership-admin-%d", i))
		} else {
			user = suite.createTestUserWithTier(fmt.Sprintf("crew-membership-%d", i), tc.tier)
		}

		crew := suite.createTag(fmt.Sprintf("Crew Membership %d", i), catalogm.TagCategoryCrew)
		genre := suite.createTag(fmt.Sprintf("Genre Membership %d", i), catalogm.TagCategoryGenre)
		attachTo := suite.createArtist(fmt.Sprintf("Crew Membership Band %d", i))
		removeFrom := suite.createArtist(fmt.Sprintf("Crew Membership Band Removal %d", i))

		// The control. new_user is below the tier that may CREATE a tag, so the
		// control applies an existing one, which every tier may do.
		_, err := suite.tagService.AddTagToEntity(genre.ID, "", "artist", attachTo, user.ID, "")
		suite.Require().NoError(err, label)
		suite.Require().NoError(suite.tagService.RemoveTagFromEntity(genre.ID, "artist", attachTo, user.ID), label)

		// Somebody else's edge, for the removal half.
		_, err = suite.tagService.AddTagToEntity(crew.ID, "", "artist", removeFrom, seeder.ID, "")
		suite.Require().NoError(err, label)

		_, attachErr := suite.tagService.AddTagToEntity(crew.ID, "", "artist", attachTo, user.ID, "")
		removeErr := suite.tagService.RemoveTagFromEntity(crew.ID, "artist", removeFrom, user.ID)

		if tc.allowed {
			suite.Require().NoError(attachErr, label)
			suite.Require().NoError(removeErr, label)
			suite.Assert().Equal(int64(1), suite.entityTagCount(crew.ID, "artist", attachTo), label)
			suite.Assert().Equal(int64(0), suite.entityTagCount(crew.ID, "artist", removeFrom), label)
			continue
		}

		for direction, err := range map[string]error{"attach": attachErr, "remove": removeErr} {
			suite.Require().Error(err, "%s %s", label, direction)
			var tagErr *apperrors.TagError
			suite.Require().ErrorAs(err, &tagErr, "%s %s", label, direction)
			suite.Assert().Equal(apperrors.CodeTagCategoryTierOnly, tagErr.Code, "%s %s", label, direction)
			suite.Assert().Contains(tagErr.Message, "trusted contributors", "%s %s", label, direction)
		}
		// The refusals wrote nothing and deleted nothing.
		suite.Assert().Equal(int64(0), suite.entityTagCount(crew.ID, "artist", attachTo), label)
		suite.Assert().Equal(int64(1), suite.entityTagCount(crew.ID, "artist", removeFrom), label)
	}
}

// The gate reads the tag's STORED category, so a request that names an existing
// crew tag under some other category does not talk its way past it, and the
// column's unconstrained casing does not either.
func (suite *TagServiceIntegrationTestSuite) TestCrewMembershipGateReadsStoredCategory() {
	user := suite.createTestUserWithTier("crew-stored-category", "contributor")

	oddCase := &catalogm.Tag{Name: "Ascetic House", Slug: "ascetic-house-odd-case", Category: "Crew"}
	suite.Require().NoError(suite.db.Create(oddCase).Error)

	lied := suite.createTag("Lied About Crew", catalogm.TagCategoryCrew)

	for _, tc := range []struct {
		name     string
		tagID    uint
		category string
	}{
		{name: "odd-cased stored category", tagID: oddCase.ID},
		{name: "request claims genre", tagID: lied.ID, category: catalogm.TagCategoryGenre},
	} {
		artistID := suite.createArtist("Stored Category Band " + tc.name)
		_, err := suite.tagService.AddTagToEntity(tc.tagID, "", "artist", artistID, user.ID, tc.category)
		suite.Require().Error(err, tc.name)
		var tagErr *apperrors.TagError
		suite.Require().ErrorAs(err, &tagErr, tc.name)
		suite.Assert().Equal(apperrors.CodeTagCategoryTierOnly, tagErr.Code, tc.name)
	}
}

// =============================================================================
// Inline creation stores the typed name (PSY-2045)
// =============================================================================

// The name a contributor typed is what the tag is called, and the slug is
// derived from it. Every category, not only crew: crew is where it shows,
// because a booker is a proper noun.
func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_InlineCreate_KeepsTypedName() {
	admin := suite.createAdminUser("typed-name-admin")
	contributor := suite.createTestUserWithTier("typed-name-contributor", "contributor")

	for _, tc := range []struct {
		userID   uint
		typed    string
		category string
		wantSlug string
	}{
		{userID: admin.ID, typed: "Rubber Brother Records", category: catalogm.TagCategoryCrew, wantSlug: "rubber-brother-records"},
		{userID: contributor.ID, typed: "Desert Rock", category: catalogm.TagCategoryGenre, wantSlug: "desert-rock"},
		{userID: contributor.ID, typed: "  Tempe, AZ  ", category: catalogm.TagCategoryLocale, wantSlug: "tempe-az"},
	} {
		artistID := suite.createArtist("Typed Name Band " + tc.wantSlug)
		_, err := suite.tagService.AddTagToEntity(0, tc.typed, "artist", artistID, tc.userID, tc.category)
		suite.Require().NoError(err, tc.typed)

		tag, err := suite.tagService.GetTagBySlug(tc.wantSlug)
		suite.Require().NoError(err, tc.typed)
		suite.Require().NotNil(tag, tc.typed)
		suite.Assert().Equal(strings.TrimSpace(tc.typed), tag.Name, tc.typed)
		suite.Assert().Equal(tc.wantSlug, tag.Slug, tc.typed)
	}
}

// Storing the typed name would let a second spelling of the same party mint a
// near-duplicate under a "-2" slug, so the duplicate lookup keys on the derived
// slug as well as on the normalized name.
func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_InlineCreate_TypedNameDoesNotDuplicate() {
	admin := suite.createAdminUser("typed-name-dedup-admin")

	first := suite.createArtist("Dedup Band One")
	_, err := suite.tagService.AddTagToEntity(0, "Gracie's Tax Bar", "artist", first, admin.ID, catalogm.TagCategoryOther)
	suite.Require().NoError(err)

	created, err := suite.tagService.GetTagBySlug("gracies-tax-bar")
	suite.Require().NoError(err)
	suite.Require().NotNil(created)
	before := suite.countTags()

	for _, spelling := range []string{"gracies-tax-bar", "Gracies Tax Bar", "GRACIE'S TAX BAR"} {
		artistID := suite.createArtist("Dedup Band " + spelling)
		et, err := suite.tagService.AddTagToEntity(0, spelling, "artist", artistID, admin.ID, catalogm.TagCategoryOther)
		suite.Require().NoError(err, spelling)
		suite.Assert().Equal(created.ID, et.TagID, spelling)
	}
	suite.Assert().Equal(before, suite.countTags(), "no second row for a second spelling")
}

// The stored name is bounded by the column it goes in. The normalized form is
// what the older bound measures and it can be far shorter than what is typed.
func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_InlineCreate_RejectsNameLongerThanColumn() {
	admin := suite.createAdminUser("typed-name-length-admin")
	artistID := suite.createArtist("Long Name Band")

	typed := "Rubber Brother Records" + strings.Repeat("!", catalogm.MaxTagNameLength)
	suite.Require().Less(len(NormalizeTagName(typed)), 50, "the normalized form must clear the older bound for this test to mean anything")

	_, err := suite.tagService.AddTagToEntity(0, typed, "artist", artistID, admin.ID, catalogm.TagCategoryCrew)
	suite.Require().Error(err)
	var tagErr *apperrors.TagError
	suite.Require().ErrorAs(err, &tagErr)
	suite.Assert().Equal(apperrors.CodeTagNameInvalid, tagErr.Code)
}

// =============================================================================
// Descriptive-category readers (PSY-2045)
// =============================================================================

// Related tags rank co-occurring tags against each other, so a booker is not a
// candidate. The crew tag here co-occurs more often than the genre and would
// otherwise rank first.
func (suite *TagServiceIntegrationTestSuite) TestGetTagDetail_RelatedTags_ExcludeCrew() {
	user := suite.createTestUserWithTier("related-crew", "trusted_contributor")
	focus := suite.createTag("related-crew-focus", catalogm.TagCategoryGenre)
	crew := suite.createTag("Related Crew Booker", catalogm.TagCategoryCrew)
	genre := suite.createTag("related-crew-genre", catalogm.TagCategoryGenre)

	first := suite.createArtist("Related Crew Band One")
	second := suite.createArtist("Related Crew Band Two")

	for _, id := range []uint{first, second} {
		_, err := suite.tagService.AddTagToEntity(focus.ID, "", "artist", id, user.ID, "")
		suite.Require().NoError(err)
		_, err = suite.tagService.AddTagToEntity(crew.ID, "", "artist", id, user.ID, "")
		suite.Require().NoError(err)
	}
	_, err := suite.tagService.AddTagToEntity(genre.ID, "", "artist", first, user.ID, "")
	suite.Require().NoError(err)

	resp, err := suite.tagService.GetTagDetail(focus.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)

	suite.Require().Len(resp.RelatedTags, 1, "the crew tag co-occurs twice and must not be ranked")
	suite.Assert().Equal(genre.ID, resp.RelatedTags[0].ID)
}

// The Go predicate and its SQL spelling are two expressions of one rule, and
// only one of them is exercised by the readers. This runs both over the same
// rows so they cannot drift apart.
func (suite *TagServiceIntegrationTestSuite) TestDescriptiveTagCategoryPredicateMatchesSQL() {
	categories := []string{
		catalogm.TagCategoryGenre,
		catalogm.TagCategoryLocale,
		catalogm.TagCategoryOther,
		catalogm.TagCategoryCrew,
		"Crew",
		" crew ",
		"CREW",
		"crewe",
		"a category this build has never seen",
		"",
	}

	ids := make(map[uint]string, len(categories))
	for i, category := range categories {
		tag := &catalogm.Tag{
			Name:     fmt.Sprintf("Predicate Fixture %d", i),
			Slug:     fmt.Sprintf("predicate-fixture-%d", i),
			Category: category,
		}
		suite.Require().NoError(suite.db.Create(tag).Error)
		ids[tag.ID] = category
	}

	var descriptiveIDs []uint
	suite.Require().NoError(suite.db.Model(&catalogm.Tag{}).
		Where("id IN ?", slices.Collect(maps.Keys(ids))).
		Where(descriptiveTagCategorySQL("tags")).
		Pluck("id", &descriptiveIDs).Error)

	sqlSaysDescriptive := make(map[uint]bool, len(descriptiveIDs))
	for _, id := range descriptiveIDs {
		sqlSaysDescriptive[id] = true
	}
	for id, category := range ids {
		suite.Assert().Equal(
			catalogm.IsDescriptiveTagCategory(category),
			sqlSaysDescriptive[id],
			"category %q is judged differently in Go and in SQL", category,
		)
	}
}

// "Matching POST /tags" is the claim, so the two writers are compared rather
// than described: the same typed name through CreateTag and through the inline
// path must land the same name and the same slug shape.
func (suite *TagServiceIntegrationTestSuite) TestInlineCreateMatchesCreateTagOnNameAndSlug() {
	admin := suite.createAdminUser("parity-admin")

	direct, err := suite.tagService.CreateTag("Gracie's Tax Bar", nil, nil, catalogm.TagCategoryOther, false, nil)
	suite.Require().NoError(err)

	artistID := suite.createArtist("Parity Band")
	_, err = suite.tagService.AddTagToEntity(0, "Gracie's Tax Bar Two", "artist", artistID, admin.ID, catalogm.TagCategoryOther)
	suite.Require().NoError(err)
	inline, err := suite.tagService.GetTagBySlug("gracies-tax-bar-two")
	suite.Require().NoError(err)
	suite.Require().NotNil(inline)

	suite.Assert().Equal("Gracie's Tax Bar", direct.Name)
	suite.Assert().Equal("gracies-tax-bar", direct.Slug)
	suite.Assert().Equal("Gracie's Tax Bar Two", inline.Name)
	suite.Assert().Equal("gracies-tax-bar-two", inline.Slug)
}

// The slug key is scoped to the requested category, so a request for one
// category is never answered with an existing tag of another. Slugs are
// globally unique, so the out-of-category request falls back to a suffixed
// slug, which is what it did before the slug key existed.
func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_InlineCreate_SlugKeyDoesNotCrossCategories() {
	admin := suite.createAdminUser("cross-category-admin")
	// Punctuation differs, so only the DERIVED SLUG connects the two spellings.
	// An exact-name match would be caught by the outer lookup in
	// AddTagToEntity, which is unscoped and older than this key.
	crew := suite.createTag("Tempe, AZ", catalogm.TagCategoryCrew)
	suite.Require().Equal("tempe-az", crew.Slug)

	artistID := suite.createArtist("Cross Category Band")
	et, err := suite.tagService.AddTagToEntity(0, "Tempe AZ", "artist", artistID, admin.ID, catalogm.TagCategoryLocale)
	suite.Require().NoError(err)
	suite.Require().NotEqual(crew.ID, et.TagID, "a locale request must not be answered with the crew tag")

	var created catalogm.Tag
	suite.Require().NoError(suite.db.First(&created, et.TagID).Error)
	suite.Assert().Equal(catalogm.TagCategoryLocale, created.Category)
	suite.Assert().NotEqual(crew.Slug, created.Slug)
}

// The stored value is the trimmed typed name, so the duplicate lookup must key
// on that value too: a padded spelling of an existing tag, sent with no
// category, must resolve to the existing row instead of tripping the unique
// index on LOWER(name).
func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_InlineCreate_PaddedNameResolvesToExistingRow() {
	contributor := suite.createTestUserWithTier("padded-name-contributor", "contributor")

	first := suite.createArtist("Padded Name Band One")
	_, err := suite.tagService.AddTagToEntity(0, "Desert Rock", "artist", first, contributor.ID, catalogm.TagCategoryGenre)
	suite.Require().NoError(err)
	existing, err := suite.tagService.GetTagBySlug("desert-rock")
	suite.Require().NoError(err)
	suite.Require().NotNil(existing)
	before := suite.countTags()

	second := suite.createArtist("Padded Name Band Two")
	_, err = suite.tagService.AddTagToEntity(0, "  Desert Rock  ", "artist", second, contributor.ID, "")
	suite.Require().NoError(err, "a padded spelling with no category must resolve, not 500")
	suite.Assert().Equal(before, suite.countTags(), "no new tag row for a padded spelling")

	var applied int64
	suite.db.Model(&catalogm.EntityTag{}).Where("tag_id = ? AND entity_type = ? AND entity_id = ?", existing.ID, "artist", second).Count(&applied)
	suite.Assert().Equal(int64(1), applied, "the existing tag is what got applied")
}
