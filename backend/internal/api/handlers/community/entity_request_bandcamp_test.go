package community

import (
	"encoding/json"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
)

// TestAdminDecide_RejectsNonReleaseBandcampEmbedBeforeClaiming covers the queued
// request that predates the create-time rule (PSY-1966).
//
// The assertion that matters is not "the approve failed" but WHERE it failed.
// ValidateEntityRequestPayload already refuses such a payload inside
// fulfillEntity, so the value never reaches a live artist either way, but that
// refusal lands AFTER Decide has claimed the row, leaving it approved with
// nothing created, and a claimed row's payload can no longer be corrected
// (PSY-1948's resubmission replaces PENDING rows only). So this asserts a 422
// AND that Decide was never called.
func TestAdminDecide_RejectsNonReleaseBandcampEmbedBeforeClaiming(t *testing.T) {
	for _, value := range []string{
		"https://evil.test/album/checkout",
		"https://bandcamp.com.attacker.test/album/x",
		"https://kingbuffalo.bandcamp.com",
		"http://kingbuffalo.bandcamp.com/album/regenerator",
	} {
		t.Run(value, func(t *testing.T) {
			// Bypass the create-time rule the way a legacy row does: build the
			// payload directly rather than through the handler.
			payload := json.RawMessage(`{"name":"Evil","bandcamp_embed_url":"` + value + `"}`)
			queued := pendingRequest(51, "artist")
			queued.Payload = &payload

			h := NewEntityRequestHandler(
				&testhelpers.MockEntityRequestService{
					GetRequestFn: func(uint) (*communitym.EntityRequest, error) {
						return queued, nil
					},
					DecideFn: func(uint, uint, communitym.EntityRequestDecisionState, *string, *time.Time) (*communitym.EntityRequest, error) {
						t.Fatal("the row must NOT be claimed when its stored embed URL is not a Bandcamp release")
						return nil, nil
					},
				},
				&testhelpers.MockEntityRequestFulfiller{
					CreateArtistFn: func(*contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
						t.Fatal("fulfiller must NOT be called for a non-release embed URL")
						return nil, nil
					},
				},
				&testhelpers.MockAuditLogService{},
			)

			req := &AdminDecideEntityRequestRequest{ID: "51"}
			req.Body.Decision = "approved"
			_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
			testhelpers.AssertHumaError(t, err, 422)
		})
	}
}

// TestAdminDecide_ApprovesBandcampReleaseEmbed is the positive half: the
// pre-claim check must let a real release page through to the claim and the
// create, and must stand down entirely when the payload carries no embed URL.
func TestAdminDecide_ApprovesBandcampReleaseEmbed(t *testing.T) {
	for name, payloadJSON := range map[string]string{
		"release page":  `{"name":"Boris","bandcamp_embed_url":"https://boris.bandcamp.com/album/pink"}`,
		"field absent":  `{"name":"Boris"}`,
		"cleared field": `{"name":"Boris","bandcamp_embed_url":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			payload := json.RawMessage(payloadJSON)
			queued := pendingRequest(52, "artist")
			queued.Payload = &payload
			decided := pendingRequest(52, "artist")
			decided.Payload = &payload
			decided.DecisionState = communitym.EntityRequestStateApproved

			created := false
			h := NewEntityRequestHandler(
				&testhelpers.MockEntityRequestService{
					GetRequestFn: func(uint) (*communitym.EntityRequest, error) { return queued, nil },
					DecideFn: func(uint, uint, communitym.EntityRequestDecisionState, *string, *time.Time) (*communitym.EntityRequest, error) {
						return decided, nil
					},
				},
				&testhelpers.MockEntityRequestFulfiller{
					CreateArtistFn: func(*contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
						created = true
						return &contracts.ArtistDetailResponse{ID: 89}, nil
					},
				},
				&testhelpers.MockAuditLogService{},
			)

			req := &AdminDecideEntityRequestRequest{ID: "52"}
			req.Body.Decision = "approved"
			if _, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req); err != nil {
				t.Fatalf("a valid payload must still approve, got: %v", err)
			}
			if !created {
				t.Error("expected the approve to reach the fulfiller")
			}
		})
	}
}

// PayloadBandcampEmbedURL's switch is exhaustive on purpose: an unregistered
// type must ERROR rather than answer "no embed URL", so adding an entity_type
// without deciding whether it carries one fails closed.
func TestPayloadBandcampEmbedURL_UnknownTypeFailsClosed(t *testing.T) {
	if _, err := communitym.PayloadBandcampEmbedURL("sandwich", json.RawMessage(`{}`)); err == nil {
		t.Error("an unregistered entity type must return an error, not a nil URL")
	}
	for _, et := range []string{
		communitym.EntityRequestRelease, communitym.EntityRequestLabel,
		communitym.EntityRequestShow, communitym.EntityRequestVenue,
		communitym.EntityRequestFestival,
	} {
		got, err := communitym.PayloadBandcampEmbedURL(et, json.RawMessage(`{}`))
		if err != nil || got != nil {
			t.Errorf("%s carries no embed URL; got (%v, %v)", et, got, err)
		}
	}
}
