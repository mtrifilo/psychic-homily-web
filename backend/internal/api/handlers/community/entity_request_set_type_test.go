package community

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	communitysvc "psychic-homily-backend/internal/services/community"
	"psychic-homily-backend/internal/services/contracts"
)

// PSY-1705: the entity-request fulfillment path can carry a curated bill role.
// Before this, a show approved out of the moderation queue could only say
// "headliner or not", so a request whose source stated a support act or a DJ
// lost that role at fulfillment.

// Go struct tags must be constant literals, so the OpenAPI enum on
// ShowArtistInput.set_type cannot be built from contracts.SetTypeVocabulary().
// This test is the join, mirroring the catalog handler's guard on Artist: a
// value added to the vocabulary without widening this tag would leave the
// decide/rescue endpoints rejecting a role the show service accepts.
func TestShowArtistInputSetTypeEnumTagMatchesVocabulary(t *testing.T) {
	field, ok := reflect.TypeOf(ShowArtistInput{}).FieldByName("SetType")
	require.True(t, ok, "ShowArtistInput.SetType must exist")

	assert.Equal(t, contracts.SetTypeVocabularyCSV(), field.Tag.Get("enum"),
		"the enum tag on show_artists[].set_type must list exactly the contracts vocabulary, in order")
}

// buildShowAssociations is the single conversion both the decide and the rescue
// endpoint run before anything is claimed, so its set_type handling is the
// contract for both.
func TestBuildShowAssociations_SetType(t *testing.T) {
	validVenue := &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}

	t.Run("every vocabulary value passes through verbatim", func(t *testing.T) {
		for _, want := range contracts.SetTypeVocabulary() {
			value := want
			assoc, err := buildShowAssociations(validVenue, []ShowArtistInput{
				{Name: "Boris", SetType: &value},
			})
			if err != nil {
				t.Fatalf("set_type %q: unexpected error: %v", want, err)
			}
			got := assoc.artists[0].SetType
			if got == nil || *got != want {
				t.Errorf("set_type %q: got %v", want, got)
			}
		}
	})

	t.Run("absent stays absent so the service applies its default", func(t *testing.T) {
		blank := "   "
		empty := ""
		cases := map[string]*string{
			"nil":             nil,
			"empty string":    &empty,
			"whitespace only": &blank,
		}
		for name, input := range cases {
			assoc, err := buildShowAssociations(validVenue, []ShowArtistInput{
				{Name: "Boris", SetType: input},
			})
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", name, err)
			}
			if assoc.artists[0].SetType != nil {
				t.Errorf("%s: expected nil set_type, got %q", name, *assoc.artists[0].SetType)
			}
		}
	})

	t.Run("out-of-vocabulary values are rejected", func(t *testing.T) {
		// "support" and "host" are real third-party terms the INGEST-side
		// NormalizeSetType handles; the API contract is strict on purpose, so an
		// admin sending them gets a named 422 instead of a coerced role. Casing
		// and padding variants must fail for the same reason the OpenAPI enum
		// fails them: the handler must not be laxer than its published schema.
		for _, bad := range []string{"support", "host", "Headliner", " headliner ", "opener2", "PERFORMER"} {
			value := bad
			_, err := buildShowAssociations(validVenue, []ShowArtistInput{
				{Name: "Boris", SetType: &value},
			})
			testhelpers.AssertHumaError(t, err, 422)
		}
	})

	t.Run("the rejection names the offending bill index", func(t *testing.T) {
		bad := "support"
		_, err := buildShowAssociations(validVenue, []ShowArtistInput{
			{Name: "Boris"},
			{Name: "Earth", SetType: &bad},
		})
		testhelpers.AssertHumaErrorWithDetail(t, err, 422,
			`show_artists[1].set_type "support" is not a valid set type (allowed: `+
				contracts.SetTypeVocabularyCSV()+`)`)
	})

	t.Run("a curated role rides alongside the legacy headliner flag", func(t *testing.T) {
		dj := contracts.SetTypeDJ
		headliner := true
		assoc, err := buildShowAssociations(validVenue, []ShowArtistInput{
			{Name: "Boris", IsHeadliner: &headliner},
			{Name: "DJ Earth", SetType: &dj},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if assoc.artists[0].SetType != nil {
			t.Errorf("first artist should carry no curated role, got %q", *assoc.artists[0].SetType)
		}
		if assoc.artists[0].IsHeadliner == nil || !*assoc.artists[0].IsHeadliner {
			t.Error("legacy is_headliner must still ride through")
		}
		if assoc.artists[1].SetType == nil || *assoc.artists[1].SetType != contracts.SetTypeDJ {
			t.Errorf("second artist set_type: got %v", assoc.artists[1].SetType)
		}
	})
}

// showRequestPayload marshals a minimal fulfillable show payload.
func showRequestPayload(t *testing.T, title string) json.RawMessage {
	t.Helper()
	city := "Phoenix"
	state := "AZ"
	raw, err := communitym.MarshalPayload(communitym.ShowRequestPayload{
		Title:     title,
		EventDate: "2026-09-12T21:00:00-07:00",
		City:      &city,
		State:     &state,
	})
	if err != nil {
		t.Fatalf("marshal show payload: %v", err)
	}
	return raw
}

// Approving a show request carries each act's curated role onto the created
// show, which is the whole point of PSY-1705.
func TestAdminDecide_ApproveShow_CarriesSetType(t *testing.T) {
	payload := showRequestPayload(t, "Boris with Earth and a DJ")
	decided := pendingRequest(60, "show")
	decided.Payload = &payload
	decided.DecisionState = communitym.EntityRequestStateApproved

	var got *contracts.CreateShowRequest
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				got = req
				return &contracts.ShowResponse{ID: 500}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	headliner := contracts.SetTypeHeadliner
	support := contracts.SetTypeDirectSupport
	dj := contracts.SetTypeDJ

	req := &AdminDecideEntityRequestRequest{ID: "60"}
	req.Body.Decision = "approved"
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{
		{Name: "Boris", SetType: &headliner},
		{Name: "Earth", SetType: &support},
		{Name: "DJ Sleep", SetType: &dj},
		{Name: "Uncurated Opener"},
	}

	if _, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected CreateShow to be called")
	}
	want := []*string{&headliner, &support, &dj, nil}
	if len(got.Artists) != len(want) {
		t.Fatalf("expected %d artists, got %d", len(want), len(got.Artists))
	}
	for i, w := range want {
		switch {
		case w == nil && got.Artists[i].SetType != nil:
			t.Errorf("artists[%d]: expected no curated role, got %q", i, *got.Artists[i].SetType)
		case w != nil && (got.Artists[i].SetType == nil || *got.Artists[i].SetType != *w):
			t.Errorf("artists[%d]: expected %q, got %v", i, *w, got.Artists[i].SetType)
		}
	}
}

// An invalid role is a 422 BEFORE the row is claimed. This is why the handler
// validates rather than leaning on the show service's backstop: the service's
// check runs inside fulfillment, which on the decide path is after the claim,
// so a rejection there would strand the request as approved-but-unfulfilled.
func TestAdminDecide_ApproveShow_InvalidSetType422BeforeClaim(t *testing.T) {
	decideCalled := false
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				decideCalled = true
				return nil, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				t.Error("CreateShow must not be called for an invalid set_type")
				return nil, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	bad := "support"
	req := &AdminDecideEntityRequestRequest{ID: "61"}
	req.Body.Decision = "approved"
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{{Name: "Boris", SetType: &bad}}

	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
	if decideCalled {
		t.Error("Decide must NOT be called when a bill role is invalid")
	}
}

// The rescue endpoint shares buildShowAssociations, so it carries roles too.
func TestAdminFulfill_Show_CarriesSetType(t *testing.T) {
	payload := showRequestPayload(t, "Deferred Show")
	orphan := approvedUnfulfilledRequest(62, "show")
	orphan.Payload = &payload

	var got *contracts.CreateShowRequest
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) { return orphan, nil },
			ClaimRescueFulfillmentFn: func(requestID, createdEntityID uint) (bool, error) {
				return true, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				got = req
				return &contracts.ShowResponse{ID: 501}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	special := contracts.SetTypeSpecialGuest
	req := &AdminFulfillEntityRequestRequest{ID: "62"}
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{{Name: "Boris", SetType: &special}}

	if _, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected CreateShow to be called")
	}
	if got.Artists[0].SetType == nil || *got.Artists[0].SetType != contracts.SetTypeSpecialGuest {
		t.Errorf("set_type: got %v", got.Artists[0].SetType)
	}
}

// On the rescue path an invalid role must 422 before any catalog write, so a
// bad body is never a half-rescue (the row stays cleanly unfulfilled).
func TestAdminFulfill_Show_InvalidSetType422BeforeCreate(t *testing.T) {
	payload := showRequestPayload(t, "Deferred Show")
	orphan := approvedUnfulfilledRequest(63, "show")
	orphan.Payload = &payload

	claimCalled := false
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) { return orphan, nil },
			ClaimRescueFulfillmentFn: func(requestID, createdEntityID uint) (bool, error) {
				claimCalled = true
				return true, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				t.Error("CreateShow must not be called for an invalid set_type")
				return nil, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	bad := "co-headliner"
	req := &AdminFulfillEntityRequestRequest{ID: "63"}
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{{Name: "Boris", SetType: &bad}}

	_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
	if claimCalled {
		t.Error("the rescue must not claim a fulfillment it never performed")
	}
}

// =============================================================================
// INTEGRATION -- the role actually lands in show_artists.set_type
// =============================================================================

// The unit tests above stop at the CreateShow contract. This suite runs the
// rescue endpoint through the REAL fulfiller and the REAL show service against
// Postgres, so it proves the two claims the unit tests can only imply: a
// curated role is written to show_artists.set_type, and an act with no curated
// role lands on 'performer' rather than the 'opener' the pre-PSY-1673 code
// stamped on everything below the headliner.
type EntityRequestSetTypeIntegrationSuite struct {
	suite.Suite
	deps *testhelpers.IntegrationDeps
}

func (s *EntityRequestSetTypeIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
}

func (s *EntityRequestSetTypeIntegrationSuite) TearDownTest() {
	testhelpers.CleanupTables(s.deps.DB)
}

func (s *EntityRequestSetTypeIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestEntityRequestSetTypeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(EntityRequestSetTypeIntegrationSuite))
}

// rescueHandler builds the handler with the real per-entity catalog services
// behind the fulfiller. Only the entity-request STORE is mocked: this suite is
// about what fulfillment writes to the catalog, and the queue row's own
// persistence is covered by the entity-request service's own tests.
func (s *EntityRequestSetTypeIntegrationSuite) rescueHandler(orphan *communitym.EntityRequest) *EntityRequestHandler {
	fulfiller := communitysvc.NewEntityRequestFulfiller(
		s.deps.ArtistService,
		s.deps.VenueService,
		s.deps.LabelService,
		s.deps.ReleaseService,
		s.deps.FestivalService,
		s.deps.ShowService,
	)
	return NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) { return orphan, nil },
			ClaimRescueFulfillmentFn: func(requestID, createdEntityID uint) (bool, error) {
				return true, nil
			},
		},
		fulfiller,
		&testhelpers.MockAuditLogService{},
	)
}

// showOrphan is an approved-but-unfulfilled show request owned by a real user
// (CreateShow stamps submitted_by, which is a FK to users).
func (s *EntityRequestSetTypeIntegrationSuite) showOrphan(title string) *communitym.EntityRequest {
	requester := testhelpers.CreateTestUser(s.deps.DB)
	payload := showRequestPayload(s.T(), title)
	orphan := approvedUnfulfilledRequest(1, communitym.EntityRequestShow)
	orphan.Payload = &payload
	orphan.RequesterID = requester.ID
	return orphan
}

// setTypesByPosition reads the created bill back from the database, keyed by
// bill position, so assertions name the slot rather than a row order.
func (s *EntityRequestSetTypeIntegrationSuite) setTypesByPosition(showID uint) map[int]string {
	var rows []catalogm.ShowArtist
	s.Require().NoError(s.deps.DB.Where("show_id = ?", showID).Order("position").Find(&rows).Error)
	out := make(map[int]string, len(rows))
	for _, r := range rows {
		out[r.Position] = r.SetType
	}
	return out
}

// The acceptance criterion in full: a curated role persists, and an uncurated
// act on the same bill defaults to 'performer'.
func (s *EntityRequestSetTypeIntegrationSuite) TestFulfillShow_PersistsCuratedRoles() {
	h := s.rescueHandler(s.showOrphan("Boris with Earth and a DJ"))

	headliner := contracts.SetTypeHeadliner
	support := contracts.SetTypeDirectSupport
	dj := contracts.SetTypeDJ

	req := &AdminFulfillEntityRequestRequest{ID: "1"}
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{
		{Name: "Boris", SetType: &headliner},
		{Name: "Earth", SetType: &support},
		{Name: "DJ Sleep", SetType: &dj},
		{Name: "Unstated Act"},
	}

	resp, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body.CreatedEntityID)

	got := s.setTypesByPosition(*resp.Body.CreatedEntityID)
	s.Equal(contracts.SetTypeHeadliner, got[0])
	s.Equal(contracts.SetTypeDirectSupport, got[1])
	s.Equal(contracts.SetTypeDJ, got[2])
	// The regression guard: no stated role must NOT become 'opener'.
	s.Equal(contracts.SetTypePerformer, got[3],
		"an act with no stated role must land on the neutral default")
}

// With no curated role anywhere, the bill keeps the legacy shape: position 0 is
// the headliner (the one inference the vocabulary still sanctions) and every
// other act is 'performer'.
func (s *EntityRequestSetTypeIntegrationSuite) TestFulfillShow_NoSetTypeDefaultsToPerformer() {
	h := s.rescueHandler(s.showOrphan("Uncurated Bill"))

	req := &AdminFulfillEntityRequestRequest{ID: "1"}
	req.Body.ShowVenue = &ShowVenueInput{Name: "Rebel Lounge", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{
		{Name: "First Act"},
		{Name: "Second Act"},
		{Name: "Third Act"},
	}

	resp, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body.CreatedEntityID)

	got := s.setTypesByPosition(*resp.Body.CreatedEntityID)
	s.Equal(contracts.SetTypeHeadliner, got[0])
	s.Equal(contracts.SetTypePerformer, got[1])
	s.Equal(contracts.SetTypePerformer, got[2])
}

// A curated role beats the legacy flag all the way to the column, so an admin
// who states 'performer' on the first-billed act gets exactly that rather than
// a silently-restored position-0 headliner guess.
func (s *EntityRequestSetTypeIntegrationSuite) TestFulfillShow_SetTypeOverridesHeadlinerFlag() {
	h := s.rescueHandler(s.showOrphan("Role Beats Flag"))

	performer := contracts.SetTypePerformer
	headlinerFlag := true

	req := &AdminFulfillEntityRequestRequest{ID: "1"}
	req.Body.ShowVenue = &ShowVenueInput{Name: "Trunk Space", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{
		{Name: "Billed First", IsHeadliner: &headlinerFlag, SetType: &performer},
	}

	resp, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body.CreatedEntityID)

	got := s.setTypesByPosition(*resp.Body.CreatedEntityID)
	s.Equal(contracts.SetTypePerformer, got[0],
		"a curated set_type is authoritative over is_headliner")
}

// An invalid role never reaches the catalog: no show, and none of the entities
// the bill would have find-or-created on its way in.
func (s *EntityRequestSetTypeIntegrationSuite) TestFulfillShow_InvalidSetTypeWritesNothing() {
	h := s.rescueHandler(s.showOrphan("Rejected Bill"))

	bad := "support"
	req := &AdminFulfillEntityRequestRequest{ID: "1"}
	req.Body.ShowVenue = &ShowVenueInput{Name: "Nowhere", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{{Name: "Boris", SetType: &bad}}

	_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(s.T(), err, 422)

	var shows, artists, venues int64
	s.Require().NoError(s.deps.DB.Model(&catalogm.Show{}).Count(&shows).Error)
	s.Require().NoError(s.deps.DB.Model(&catalogm.Artist{}).Count(&artists).Error)
	s.Require().NoError(s.deps.DB.Model(&catalogm.Venue{}).Count(&venues).Error)
	s.Zero(shows, "no show may be created when a bill role is invalid")
	s.Zero(artists, "no artist may be find-or-created when a bill role is invalid")
	s.Zero(venues, "no venue may be find-or-created when a bill role is invalid")
}
