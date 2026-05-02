package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// CollectionGraphHandlerSuite covers the handler-level concerns that the
// services-package test (collection_graph_test.go) cannot:
//   - viewerID resolution from middleware context (anonymous vs authed)
//   - mapCollectionError → huma HTTP error mapping (404, 403)
//   - parseTypesQueryParam wiring through the request struct
//
// Business-logic coverage (graph composition, isolate flag, type-filter
// short-circuit) lives in internal/services/collection_graph_test.go.
type CollectionGraphHandlerSuite struct {
	suite.Suite
	deps    *handlerIntegrationDeps
	handler *CollectionHandler
}

func (s *CollectionGraphHandlerSuite) SetupSuite() {
	s.deps = setupHandlerIntegrationDeps(s.T())
	// auditLog is ok to be nil for read-only paths — the handler doesn't write
	// audit entries on graph reads.
	s.handler = NewCollectionHandler(s.deps.collectionService, s.deps.auditLogService)
}

func (s *CollectionGraphHandlerSuite) TearDownTest() {
	cleanupTables(s.deps.db)
}

func (s *CollectionGraphHandlerSuite) TearDownSuite() {
	s.deps.testDB.Cleanup()
}

func TestCollectionGraphHandler(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(CollectionGraphHandlerSuite))
}

func (s *CollectionGraphHandlerSuite) seedPrivateCollection(creator *models.User, title string) *contracts.CollectionDetailResponse {
	resp, err := s.deps.collectionService.CreateCollection(creator.ID, &contracts.CreateCollectionRequest{
		Title:    title,
		IsPublic: false,
	})
	s.Require().NoError(err)
	return resp
}

// TestHandler_AnonymousViewerSeesPublicCollection: an anonymous request (no
// user in context) successfully reads a public collection's graph. Verifies
// that an absent user is treated as viewerID=0 by the handler, not as an error.
func (s *CollectionGraphHandlerSuite) TestHandler_AnonymousViewerSeesPublicCollection() {
	user := createTestUser(s.deps.db)
	// Build a publicly-visible collection by going through the publish-gate
	// path. PSY-356 requires >=3 items + >=50-char description.
	priv := s.seedPrivateCollection(user, "Public Through Gate")
	for i := 0; i < 3; i++ {
		artist := createArtist(s.deps.db, fmt.Sprintf("GateArtist-%d-%d", i, time.Now().UnixNano()))
		_, err := s.deps.collectionService.AddItem(priv.Slug, user.ID, &contracts.AddCollectionItemRequest{
			EntityType: models.CollectionEntityArtist,
			EntityID:   artist.ID,
		})
		s.Require().NoError(err)
	}
	desc := strings.Repeat("a", 60)
	pub := true
	_, err := s.deps.collectionService.UpdateCollection(priv.Slug, user.ID, false, &contracts.UpdateCollectionRequest{
		Description: &desc,
		IsPublic:    &pub,
	})
	s.Require().NoError(err)

	// Anonymous: no user in context.
	resp, err := s.handler.GetCollectionGraphHandler(context.Background(), &GetCollectionGraphRequest{Slug: priv.Slug})
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Equal(priv.Slug, resp.Body.Collection.Slug)
}

// TestHandler_AuthedNonOwnerOnPrivateCollection_403: an authed user who is
// not the creator hitting a private collection's graph endpoint gets a 403.
// Verifies that mapCollectionError correctly translates ErrCollectionForbidden
// into huma.Error403Forbidden — the unique handler-layer concern.
func (s *CollectionGraphHandlerSuite) TestHandler_AuthedNonOwnerOnPrivateCollection_403() {
	creator := createTestUser(s.deps.db)
	other := createTestUser(s.deps.db)
	priv := s.seedPrivateCollection(creator, "Locked Down")

	ctx := context.WithValue(context.Background(), middleware.UserContextKey, other)
	resp, err := s.handler.GetCollectionGraphHandler(ctx, &GetCollectionGraphRequest{Slug: priv.Slug})
	s.Require().Error(err)
	s.Nil(resp)

	var statusErr huma.StatusError
	s.Require().True(errors.As(err, &statusErr), "expected huma.StatusError, got %T", err)
	s.Equal(403, statusErr.GetStatus())
}

// TestHandler_OwnerOnPrivateCollection_200: the creator authed in the context
// can read their own private collection's graph. Verifies the auth-context
// path resolves user.ID and forwards it to the service.
func (s *CollectionGraphHandlerSuite) TestHandler_OwnerOnPrivateCollection_200() {
	creator := createTestUser(s.deps.db)
	priv := s.seedPrivateCollection(creator, "Owner Visible")

	ctx := context.WithValue(context.Background(), middleware.UserContextKey, creator)
	resp, err := s.handler.GetCollectionGraphHandler(ctx, &GetCollectionGraphRequest{Slug: priv.Slug})
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Equal(priv.Slug, resp.Body.Collection.Slug)
}

// TestHandler_MissingSlug_404: hitting the endpoint with a slug that doesn't
// exist returns 404 (mapCollectionError → huma.Error404NotFound).
func (s *CollectionGraphHandlerSuite) TestHandler_MissingSlug_404() {
	resp, err := s.handler.GetCollectionGraphHandler(context.Background(), &GetCollectionGraphRequest{Slug: "no-such-collection-xyz"})
	s.Require().Error(err)
	s.Nil(resp)

	var statusErr huma.StatusError
	s.Require().True(errors.As(err, &statusErr), "expected huma.StatusError, got %T", err)
	s.Equal(404, statusErr.GetStatus())
}

// TestHandler_TypesQueryStringPassesThrough: the comma-separated `types`
// query string is parsed and forwarded into the service. Verified by passing
// a known-bad type and observing zero edges (allowlist short-circuit), which
// proves the query string was actually parsed and reached the service.
func (s *CollectionGraphHandlerSuite) TestHandler_TypesQueryStringPassesThrough() {
	creator := createTestUser(s.deps.db)
	priv := s.seedPrivateCollection(creator, "Type Filter")
	a1 := createArtist(s.deps.db, fmt.Sprintf("TypeA-%d", time.Now().UnixNano()))
	a2 := createArtist(s.deps.db, fmt.Sprintf("TypeB-%d", time.Now().UnixNano()+1))

	for _, art := range []*models.Artist{a1, a2} {
		_, err := s.deps.collectionService.AddItem(priv.Slug, creator.ID, &contracts.AddCollectionItemRequest{
			EntityType: models.CollectionEntityArtist,
			EntityID:   art.ID,
		})
		s.Require().NoError(err)
	}
	src, tgt := models.CanonicalOrder(a1.ID, a2.ID)
	rel := &models.ArtistRelationship{
		SourceArtistID:   src,
		TargetArtistID:   tgt,
		RelationshipType: models.RelationshipTypeSharedBills,
		Score:            5.0,
		AutoDerived:      true,
	}
	s.Require().NoError(s.deps.db.Create(rel).Error)

	ctx := context.WithValue(context.Background(), middleware.UserContextKey, creator)

	// No filter → 1 edge.
	respAll, err := s.handler.GetCollectionGraphHandler(ctx, &GetCollectionGraphRequest{Slug: priv.Slug})
	s.Require().NoError(err)
	s.Len(respAll.Body.Links, 1)

	// "made_up_type" is not in the allowlist → service short-circuits to 0 edges.
	respFiltered, err := s.handler.GetCollectionGraphHandler(ctx, &GetCollectionGraphRequest{Slug: priv.Slug, Types: "made_up_type"})
	s.Require().NoError(err)
	s.Empty(respFiltered.Body.Links, "unknown type should short-circuit, proving the types string was parsed")

	// "shared_bills" → 1 edge of that type.
	respShared, err := s.handler.GetCollectionGraphHandler(ctx, &GetCollectionGraphRequest{Slug: priv.Slug, Types: "shared_bills"})
	s.Require().NoError(err)
	s.Require().Len(respShared.Body.Links, 1)
	s.Equal(models.RelationshipTypeSharedBills, respShared.Body.Links[0].Type)
}

