package community

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

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
			}, billSourceBody)
			if err != nil {
				t.Fatalf("set_type %q: unexpected error: %v", want, err)
			}
			got := assoc.artists[0].SetType
			if got == nil || *got != want {
				t.Errorf("set_type %q: got %v", want, got)
			}
		}
	})

	t.Run("an absent key is the only way to say the slot is unknown", func(t *testing.T) {
		assoc, err := buildShowAssociations(validVenue, []ShowArtistInput{
			{Name: "Boris", SetType: nil},
		}, billSourceBody)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if assoc.artists[0].SetType != nil {
			t.Errorf("expected nil set_type, got %q", *assoc.artists[0].SetType)
		}
	})

	t.Run("out-of-vocabulary values are rejected", func(t *testing.T) {
		// "support" and "host" are real third-party terms the INGEST-side
		// NormalizeSetType handles; the API contract is strict on purpose, so an
		// admin sending them gets a named 422 instead of a coerced role.
		//
		// "" and "   " are in this list on purpose. The generated OpenAPI enum
		// rejects any PRESENT value outside the vocabulary, so treating a blank
		// as "absent" here would make the handler accept what the schema refuses,
		// and these tests, which bypass the schema, would certify a contract no
		// HTTP client can actually get. Casing and padding variants fail for the
		// same reason.
		for _, bad := range []string{"", "   ", "support", "host", "Headliner", " headliner ", "opener2", "PERFORMER"} {
			value := bad
			_, err := buildShowAssociations(validVenue, []ShowArtistInput{
				{Name: "Boris", SetType: &value},
			}, billSourceBody)
			testhelpers.AssertHumaError(t, err, 422)
		}
	})

	// Named for set_type specifically: the legacy is_headliner flag does NOT arm
	// this handler-level suppression, because billIsCurated never reads it. The
	// show service's own suppression does read it, which
	// TestFulfillShow_LegacyFlagAloneStillWritesOneHeadliner pins end to end.
	t.Run("stating a set_type anywhere suppresses the position-0 headliner guess", func(t *testing.T) {
		// The regression this guards: resolveArtistRole reads position 0 as the
		// headliner for an act with no signal at all. An admin who marks the
		// SECOND act headliner and leaves the first alone would otherwise get two
		// set_type='headliner' rows, and every reader resolves the headliner as
		// `set_type='headliner' ORDER BY position ASC LIMIT 1`, so the act nobody
		// designated would win.
		headliner := contracts.SetTypeHeadliner
		assoc, err := buildShowAssociations(validVenue, []ShowArtistInput{
			{Name: "Earth"},
			{Name: "Boris", SetType: &headliner},
		}, billSourceBody)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if assoc.artists[0].IsHeadliner == nil || *assoc.artists[0].IsHeadliner {
			t.Errorf("uncurated first act must be pinned non-headliner, got %v", assoc.artists[0].IsHeadliner)
		}
		if assoc.artists[1].SetType == nil || *assoc.artists[1].SetType != contracts.SetTypeHeadliner {
			t.Errorf("curated headliner lost: %v", assoc.artists[1].SetType)
		}
	})

	t.Run("an entirely undescribed bill is left alone", func(t *testing.T) {
		// No entry states anything, so the position-0 fallback must still apply.
		// This is what keeps the change additive for callers that predate
		// set_type on this endpoint.
		assoc, err := buildShowAssociations(validVenue, []ShowArtistInput{
			{Name: "Earth"},
			{Name: "Boris"},
		}, billSourceBody)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for i, a := range assoc.artists {
			if a.IsHeadliner != nil {
				t.Errorf("artists[%d].IsHeadliner must stay nil on an undescribed bill, got %v", i, *a.IsHeadliner)
			}
		}
	})

	t.Run("an explicit is_headliner is never overwritten", func(t *testing.T) {
		dj := contracts.SetTypeDJ
		explicitFalse := false
		assoc, err := buildShowAssociations(validVenue, []ShowArtistInput{
			{Name: "Earth", IsHeadliner: &explicitFalse},
			{Name: "DJ Sleep", SetType: &dj},
		}, billSourceBody)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if assoc.artists[0].IsHeadliner == nil || *assoc.artists[0].IsHeadliner {
			t.Error("explicit false must survive")
		}
	})

	t.Run("the rejection names the offending bill index", func(t *testing.T) {
		bad := "support"
		_, err := buildShowAssociations(validVenue, []ShowArtistInput{
			{Name: "Boris"},
			{Name: "Earth", SetType: &bad},
		}, billSourceBody)
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
		}, billSourceBody)
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

// showRequestPayload marshals a minimal fulfillable show payload, optionally
// carrying the contributor's own bill (PSY-1858). Variadic so the PSY-1705
// call sites, which predate the bill, read exactly as they did.
func showRequestPayload(t *testing.T, title string, artists ...communitym.ShowRequestArtist) json.RawMessage {
	t.Helper()
	city := "Phoenix"
	state := "AZ"
	raw, err := communitym.MarshalPayload(communitym.ShowRequestPayload{
		Title:     title,
		EventDate: "2026-09-12T21:00:00-07:00",
		City:      &city,
		State:     &state,
		Artists:   artists,
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

	// The pre-claim read must find the row PENDING: every pre-claim check is
	// scoped to a row Decide can actually act on, so a mock that answers
	// (nil, nil) here would describe a request that does not exist.
	pending := pendingRequest(60, "show")
	pending.Payload = &payload

	var got *contracts.CreateShowRequest
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) { return pending, nil },
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string, expectedUpdatedAt *time.Time) (*communitym.EntityRequest, error) {
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
	pending := pendingRequest(61, "show")
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) { return pending, nil },
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string, expectedUpdatedAt *time.Time) (*communitym.EntityRequest, error) {
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
// (CreateShow stamps submitted_by, which is a FK to users). Variadic artists
// give the row a contributor's own bill (PSY-1858).
func (s *EntityRequestSetTypeIntegrationSuite) showOrphan(title string, artists ...communitym.ShowRequestArtist) *communitym.EntityRequest {
	requester := testhelpers.CreateTestUser(s.deps.DB)
	payload := showRequestPayload(s.T(), title, artists...)
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

// A headliner curated anywhere but position 0 must be the ONLY headliner row.
// Without suppressPositionInference the uncurated first act also resolves to
// 'headliner', and since every reader takes `set_type='headliner' ORDER BY
// position ASC LIMIT 1`, the act nobody designated would win and the admin's
// stated headliner would be discarded.
func (s *EntityRequestSetTypeIntegrationSuite) TestFulfillShow_CuratedHeadlinerIsTheOnlyHeadliner() {
	h := s.rescueHandler(s.showOrphan("Headliner Billed Second"))

	headliner := contracts.SetTypeHeadliner

	req := &AdminFulfillEntityRequestRequest{ID: "1"}
	req.Body.ShowVenue = &ShowVenueInput{Name: "Crescent Ballroom", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{
		{Name: "Earth"},
		{Name: "Boris", SetType: &headliner},
	}

	resp, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body.CreatedEntityID)

	got := s.setTypesByPosition(*resp.Body.CreatedEntityID)
	s.Equal(contracts.SetTypePerformer, got[0],
		"the uncurated first act must not be inferred as a second headliner")
	s.Equal(contracts.SetTypeHeadliner, got[1])

	// And prove it end to end the way readers see it: exactly one headliner row,
	// and it is the act the admin named.
	var headlinerNames []string
	s.Require().NoError(s.deps.DB.
		Table("show_artists").
		Select("artists.name").
		Joins("JOIN artists ON artists.id = show_artists.artist_id").
		Where("show_artists.show_id = ? AND show_artists.set_type = ?", *resp.Body.CreatedEntityID, contracts.SetTypeHeadliner).
		Order("show_artists.position ASC").
		Scan(&headlinerNames).Error)
	s.Equal([]string{"Boris"}, headlinerNames)
}

// The same bill stated only through the LEGACY FLAG. buildShowAssociations does
// not arm on it (billIsCurated reads set_type alone), so this bill reaches the
// show service untouched by the handler and is caught there instead: CreateShow
// suppresses position inference once any act names the headline slot, by either
// spelling.
func (s *EntityRequestSetTypeIntegrationSuite) TestFulfillShow_LegacyFlagAloneStillWritesOneHeadliner() {
	h := s.rescueHandler(s.showOrphan("Legacy Flag Billed Second"))

	isHeadliner := true

	req := &AdminFulfillEntityRequestRequest{ID: "1"}
	req.Body.ShowVenue = &ShowVenueInput{Name: "Crescent Ballroom", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{
		{Name: "Earth"},
		{Name: "Boris", IsHeadliner: &isHeadliner},
	}

	resp, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body.CreatedEntityID)

	got := s.setTypesByPosition(*resp.Body.CreatedEntityID)
	s.Equal(contracts.SetTypePerformer, got[0],
		"the silent first act must not be inferred as a second headliner")
	s.Equal(contracts.SetTypeHeadliner, got[1])
}

// An admin who states only non-headliner roles has not named a headliner, and
// the fulfillment must not invent one. The show still has to be usable: readers
// COALESCE the headliner lookup down to plain position order, and the dedup
// pre-check falls back to the first act, so this asserts the show is created and
// no row claims the headliner slot.
func (s *EntityRequestSetTypeIntegrationSuite) TestFulfillShow_NoHeadlinerIsNotInvented() {
	h := s.rescueHandler(s.showOrphan("Headliner Unknown"))

	performer := contracts.SetTypePerformer
	dj := contracts.SetTypeDJ

	req := &AdminFulfillEntityRequestRequest{ID: "1"}
	req.Body.ShowVenue = &ShowVenueInput{Name: "Lunchbox", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{
		{Name: "Act One", SetType: &performer},
		{Name: "Act Two", SetType: &dj},
		{Name: "Act Three"},
	}

	resp, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body.CreatedEntityID)

	got := s.setTypesByPosition(*resp.Body.CreatedEntityID)
	s.Equal(contracts.SetTypePerformer, got[0])
	s.Equal(contracts.SetTypeDJ, got[1])
	s.Equal(contracts.SetTypePerformer, got[2])

	var headliners int64
	s.Require().NoError(s.deps.DB.
		Table("show_artists").
		Where("show_id = ? AND set_type = ?", *resp.Body.CreatedEntityID, contracts.SetTypeHeadliner).
		Count(&headliners).Error)
	s.Zero(headliners, "no act stated the headliner slot, so none may claim it")
}

// With no curated role anywhere, the bill keeps the legacy shape: position 0 is
// the headliner (the one inference the vocabulary still sanctions) and every
// other act is 'performer'. This is what keeps the change additive for callers
// that predate set_type on this endpoint.
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

	// Scoped to the names this test would have created, so the assertion stands
	// on its own rather than on a sibling test having truncated first.
	var shows, artists, venues int64
	s.Require().NoError(s.deps.DB.Model(&catalogm.Show{}).Where("title = ?", "Rejected Bill").Count(&shows).Error)
	s.Require().NoError(s.deps.DB.Model(&catalogm.Artist{}).Where("name = ?", "Boris").Count(&artists).Error)
	s.Require().NoError(s.deps.DB.Model(&catalogm.Venue{}).Where("name = ?", "Nowhere").Count(&venues).Error)
	s.Zero(shows, "no show may be created when a bill role is invalid")
	s.Zero(artists, "no artist may be find-or-created when a bill role is invalid")
	s.Zero(venues, "no venue may be find-or-created when a bill role is invalid")
}
