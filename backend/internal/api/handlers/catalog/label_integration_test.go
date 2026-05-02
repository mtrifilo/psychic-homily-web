package catalog

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

type LabelHandlerIntegrationSuite struct {
	suite.Suite
	deps    *testhelpers.IntegrationDeps
	handler *LabelHandler
}

func (s *LabelHandlerIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
	s.handler = NewLabelHandler(s.deps.LabelService, s.deps.AuditLogService, nil)
}

func (s *LabelHandlerIntegrationSuite) TearDownTest() {
	testhelpers.CleanupTables(s.deps.DB)
}

func (s *LabelHandlerIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestLabelHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(LabelHandlerIntegrationSuite))
}

// --- Helpers ---

func (s *LabelHandlerIntegrationSuite) createLabelViaService(name string) *contracts.LabelDetailResponse {
	resp, err := s.deps.LabelService.CreateLabel(&contracts.CreateLabelRequest{Name: name})
	s.Require().NoError(err)
	return resp
}

func (s *LabelHandlerIntegrationSuite) createArtistForLabel(name string) *catalogm.Artist {
	artist := &catalogm.Artist{Name: name}
	s.deps.DB.Create(artist)
	return artist
}

func (s *LabelHandlerIntegrationSuite) createReleaseForLabel(title string) *catalogm.Release {
	slug := title
	release := &catalogm.Release{Title: title, Slug: &slug}
	s.deps.DB.Create(release)
	return release
}

// --- ListLabelsHandler ---

func (s *LabelHandlerIntegrationSuite) TestListLabels_Success() {
	s.createLabelViaService("Label A")
	s.createLabelViaService("Label B")
	s.createLabelViaService("Label C")

	req := &ListLabelsRequest{}
	resp, err := s.handler.ListLabelsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Count, 3)
}

func (s *LabelHandlerIntegrationSuite) TestListLabels_Empty() {
	req := &ListLabelsRequest{}
	resp, err := s.handler.ListLabelsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
}

func (s *LabelHandlerIntegrationSuite) TestListLabels_FilterByStatus() {
	s.deps.LabelService.CreateLabel(&contracts.CreateLabelRequest{Name: "Active Label", Status: "active"})
	s.deps.LabelService.CreateLabel(&contracts.CreateLabelRequest{Name: "Defunct Label", Status: "defunct"})

	req := &ListLabelsRequest{Status: "defunct"}
	resp, err := s.handler.ListLabelsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count)
	s.Equal("Defunct Label", resp.Body.Labels[0].Name)
}

// --- GetLabelHandler ---

func (s *LabelHandlerIntegrationSuite) TestGetLabel_ByID() {
	label := s.createLabelViaService("Test Label")

	req := &GetLabelRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	resp, err := s.handler.GetLabelHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Test Label", resp.Body.Name)
}

func (s *LabelHandlerIntegrationSuite) TestGetLabel_BySlug() {
	s.createLabelViaService("Slug Label")

	req := &GetLabelRequest{LabelID: "slug-label"}
	resp, err := s.handler.GetLabelHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Slug Label", resp.Body.Name)
}

func (s *LabelHandlerIntegrationSuite) TestGetLabel_NotFound() {
	req := &GetLabelRequest{LabelID: "99999"}
	_, err := s.handler.GetLabelHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- CreateLabelHandler ---

func (s *LabelHandlerIntegrationSuite) TestCreateLabel_AdminSuccess() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	year := 1988
	req := &CreateLabelRequest{}
	req.Body.Name = "New Label"
	req.Body.FoundedYear = &year
	req.Body.Status = "active"

	resp, err := s.handler.CreateLabelHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("New Label", resp.Body.Name)
	s.Equal(1988, *resp.Body.FoundedYear)
	s.Equal("active", resp.Body.Status)
}

func (s *LabelHandlerIntegrationSuite) TestCreateLabel_EmptyName() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &CreateLabelRequest{}
	req.Body.Name = ""

	_, err := s.handler.CreateLabelHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

// --- UpdateLabelHandler ---

func (s *LabelHandlerIntegrationSuite) TestUpdateLabel_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	label := s.createLabelViaService("Original Label")

	ctx := testhelpers.CtxWithUser(admin)
	newName := "Updated Label"
	req := &UpdateLabelRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	req.Body.Name = &newName

	resp, err := s.handler.UpdateLabelHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Updated Label", resp.Body.Name)
}

func (s *LabelHandlerIntegrationSuite) TestUpdateLabel_BySlug() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	s.createLabelViaService("Slug Update Label")

	ctx := testhelpers.CtxWithUser(admin)
	newStatus := "inactive"
	req := &UpdateLabelRequest{LabelID: "slug-update-label"}
	req.Body.Status = &newStatus

	resp, err := s.handler.UpdateLabelHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("inactive", resp.Body.Status)
}

func (s *LabelHandlerIntegrationSuite) TestUpdateLabel_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	newName := "New Name"
	req := &UpdateLabelRequest{LabelID: "99999"}
	req.Body.Name = &newName

	_, err := s.handler.UpdateLabelHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- DeleteLabelHandler ---

func (s *LabelHandlerIntegrationSuite) TestDeleteLabel_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	label := s.createLabelViaService("Deletable Label")

	ctx := testhelpers.CtxWithUser(admin)
	req := &DeleteLabelRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	_, err := s.handler.DeleteLabelHandler(ctx, req)
	s.NoError(err)

	// Verify label is gone
	getReq := &GetLabelRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	_, err = s.handler.GetLabelHandler(s.deps.Ctx, getReq)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *LabelHandlerIntegrationSuite) TestDeleteLabel_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	req := &DeleteLabelRequest{LabelID: "99999"}

	_, err := s.handler.DeleteLabelHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- GetLabelRosterHandler ---

func (s *LabelHandlerIntegrationSuite) TestGetLabelRoster_Success() {
	label := s.createLabelViaService("Roster Label")
	artist := s.createArtistForLabel("Roster Artist")
	s.deps.DB.Exec("INSERT INTO artist_labels (artist_id, label_id) VALUES (?, ?)", artist.ID, label.ID)

	req := &GetLabelRosterRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	resp, err := s.handler.GetLabelRosterHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count)
	s.Equal("Roster Artist", resp.Body.Artists[0].Name)
}

func (s *LabelHandlerIntegrationSuite) TestGetLabelRoster_BySlug() {
	label := s.createLabelViaService("Slug Roster Label")
	artist := s.createArtistForLabel("Slug Roster Artist")
	s.deps.DB.Exec("INSERT INTO artist_labels (artist_id, label_id) VALUES (?, ?)", artist.ID, label.ID)

	req := &GetLabelRosterRequest{LabelID: "slug-roster-label"}
	resp, err := s.handler.GetLabelRosterHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count)
}

func (s *LabelHandlerIntegrationSuite) TestGetLabelRoster_Empty() {
	label := s.createLabelViaService("Empty Roster")

	req := &GetLabelRosterRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	resp, err := s.handler.GetLabelRosterHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
}

func (s *LabelHandlerIntegrationSuite) TestGetLabelRoster_LabelNotFound() {
	req := &GetLabelRosterRequest{LabelID: "99999"}
	_, err := s.handler.GetLabelRosterHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- GetLabelCatalogHandler ---

func (s *LabelHandlerIntegrationSuite) TestGetLabelCatalog_Success() {
	label := s.createLabelViaService("Catalog Label")
	release := s.createReleaseForLabel("Catalog Release")
	s.deps.DB.Exec("INSERT INTO release_labels (release_id, label_id, catalog_number) VALUES (?, ?, 'CAT-001')", release.ID, label.ID)

	req := &GetLabelCatalogRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	resp, err := s.handler.GetLabelCatalogHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count)
	s.Equal("Catalog Release", resp.Body.Releases[0].Title)
	s.Equal("CAT-001", *resp.Body.Releases[0].CatalogNumber)
}

func (s *LabelHandlerIntegrationSuite) TestGetLabelCatalog_BySlug() {
	label := s.createLabelViaService("Slug Catalog Label")
	release := s.createReleaseForLabel("Slug Catalog Release")
	s.deps.DB.Exec("INSERT INTO release_labels (release_id, label_id) VALUES (?, ?)", release.ID, label.ID)

	req := &GetLabelCatalogRequest{LabelID: "slug-catalog-label"}
	resp, err := s.handler.GetLabelCatalogHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count)
}

func (s *LabelHandlerIntegrationSuite) TestGetLabelCatalog_Empty() {
	label := s.createLabelViaService("Empty Catalog")

	req := &GetLabelCatalogRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	resp, err := s.handler.GetLabelCatalogHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
}

func (s *LabelHandlerIntegrationSuite) TestGetLabelCatalog_LabelNotFound() {
	req := &GetLabelCatalogRequest{LabelID: "99999"}
	_, err := s.handler.GetLabelCatalogHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- AddArtistToLabelHandler ---

func (s *LabelHandlerIntegrationSuite) TestAddArtistToLabel_Success() {
	label := s.createLabelViaService("Link Label")
	artist := s.createArtistForLabel("Link Artist")
	admin := testhelpers.CreateAdminUser(s.deps.DB)

	ctx := testhelpers.CtxWithUser(admin)
	req := &AddArtistToLabelRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	req.Body.ArtistID = artist.ID

	resp, err := s.handler.AddArtistToLabelHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.True(resp.Body.Success)

	// Verify the link was created via roster
	rosterReq := &GetLabelRosterRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	rosterResp, err := s.handler.GetLabelRosterHandler(s.deps.Ctx, rosterReq)
	s.NoError(err)
	s.Equal(1, rosterResp.Body.Count)
	s.Equal("Link Artist", rosterResp.Body.Artists[0].Name)
}

func (s *LabelHandlerIntegrationSuite) TestAddArtistToLabel_Idempotent() {
	label := s.createLabelViaService("Idempotent Label")
	artist := s.createArtistForLabel("Idempotent Artist")
	admin := testhelpers.CreateAdminUser(s.deps.DB)

	ctx := testhelpers.CtxWithUser(admin)
	req := &AddArtistToLabelRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	req.Body.ArtistID = artist.ID

	// First call
	resp, err := s.handler.AddArtistToLabelHandler(ctx, req)
	s.NoError(err)
	s.True(resp.Body.Success)

	// Second call should succeed without error (idempotent)
	resp2, err := s.handler.AddArtistToLabelHandler(ctx, req)
	s.NoError(err)
	s.True(resp2.Body.Success)

	// Verify only one link exists
	rosterReq := &GetLabelRosterRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	rosterResp, err := s.handler.GetLabelRosterHandler(s.deps.Ctx, rosterReq)
	s.NoError(err)
	s.Equal(1, rosterResp.Body.Count)
}

func (s *LabelHandlerIntegrationSuite) TestAddArtistToLabel_LabelNotFound() {
	artist := s.createArtistForLabel("Orphan Artist")
	admin := testhelpers.CreateAdminUser(s.deps.DB)

	ctx := testhelpers.CtxWithUser(admin)
	req := &AddArtistToLabelRequest{LabelID: "99999"}
	req.Body.ArtistID = artist.ID

	_, err := s.handler.AddArtistToLabelHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *LabelHandlerIntegrationSuite) TestAddArtistToLabel_MissingArtistID() {
	label := s.createLabelViaService("No Artist Label")
	admin := testhelpers.CreateAdminUser(s.deps.DB)

	ctx := testhelpers.CtxWithUser(admin)
	req := &AddArtistToLabelRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	// Body.ArtistID is 0 (zero value)

	_, err := s.handler.AddArtistToLabelHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

// --- AddReleaseToLabelHandler ---

func (s *LabelHandlerIntegrationSuite) TestAddReleaseToLabel_Success() {
	label := s.createLabelViaService("Release Link Label")
	release := s.createReleaseForLabel("Release Link Release")
	admin := testhelpers.CreateAdminUser(s.deps.DB)

	ctx := testhelpers.CtxWithUser(admin)
	req := &AddReleaseToLabelRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	req.Body.ReleaseID = release.ID

	resp, err := s.handler.AddReleaseToLabelHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.True(resp.Body.Success)

	// Verify the link was created via catalog
	catalogReq := &GetLabelCatalogRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	catalogResp, err := s.handler.GetLabelCatalogHandler(s.deps.Ctx, catalogReq)
	s.NoError(err)
	s.Equal(1, catalogResp.Body.Count)
	s.Equal("Release Link Release", catalogResp.Body.Releases[0].Title)
}

func (s *LabelHandlerIntegrationSuite) TestAddReleaseToLabel_WithCatalogNumber() {
	label := s.createLabelViaService("Catalog Number Label")
	release := s.createReleaseForLabel("Catalog Number Release")
	admin := testhelpers.CreateAdminUser(s.deps.DB)

	ctx := testhelpers.CtxWithUser(admin)
	catalogNum := "CAT-042"
	req := &AddReleaseToLabelRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	req.Body.ReleaseID = release.ID
	req.Body.CatalogNumber = &catalogNum

	resp, err := s.handler.AddReleaseToLabelHandler(ctx, req)
	s.NoError(err)
	s.True(resp.Body.Success)

	// Verify the catalog number is set
	catalogReq := &GetLabelCatalogRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	catalogResp, err := s.handler.GetLabelCatalogHandler(s.deps.Ctx, catalogReq)
	s.NoError(err)
	s.Equal(1, catalogResp.Body.Count)
	s.NotNil(catalogResp.Body.Releases[0].CatalogNumber)
	s.Equal("CAT-042", *catalogResp.Body.Releases[0].CatalogNumber)
}

func (s *LabelHandlerIntegrationSuite) TestAddReleaseToLabel_Idempotent() {
	label := s.createLabelViaService("Idempotent Release Label")
	release := s.createReleaseForLabel("Idempotent Release")
	admin := testhelpers.CreateAdminUser(s.deps.DB)

	ctx := testhelpers.CtxWithUser(admin)
	req := &AddReleaseToLabelRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	req.Body.ReleaseID = release.ID

	// First call
	resp, err := s.handler.AddReleaseToLabelHandler(ctx, req)
	s.NoError(err)
	s.True(resp.Body.Success)

	// Second call should succeed (idempotent)
	resp2, err := s.handler.AddReleaseToLabelHandler(ctx, req)
	s.NoError(err)
	s.True(resp2.Body.Success)

	// Verify only one link exists
	catalogReq := &GetLabelCatalogRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	catalogResp, err := s.handler.GetLabelCatalogHandler(s.deps.Ctx, catalogReq)
	s.NoError(err)
	s.Equal(1, catalogResp.Body.Count)
}

func (s *LabelHandlerIntegrationSuite) TestAddReleaseToLabel_LabelNotFound() {
	release := s.createReleaseForLabel("Orphan Release")
	admin := testhelpers.CreateAdminUser(s.deps.DB)

	ctx := testhelpers.CtxWithUser(admin)
	req := &AddReleaseToLabelRequest{LabelID: "99999"}
	req.Body.ReleaseID = release.ID

	_, err := s.handler.AddReleaseToLabelHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *LabelHandlerIntegrationSuite) TestAddReleaseToLabel_MissingReleaseID() {
	label := s.createLabelViaService("No Release Label")
	admin := testhelpers.CreateAdminUser(s.deps.DB)

	ctx := testhelpers.CtxWithUser(admin)
	req := &AddReleaseToLabelRequest{LabelID: fmt.Sprintf("%d", label.ID)}
	// Body.ReleaseID is 0 (zero value)

	_, err := s.handler.AddReleaseToLabelHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}
