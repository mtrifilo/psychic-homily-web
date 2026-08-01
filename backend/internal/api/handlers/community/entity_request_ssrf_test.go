package community

import (
	"encoding/json"
	"os"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils/urlguard"
)

// TestMain pins the SSRF host guard (PSY-1675) to a fixed resolution table.
// entity-request payloads carry image_url, and the queue's create and fulfill
// boundaries now resolve it — without this, every test in this package that
// files a payload with an example.com flyer would depend on the machine's DNS.
//
// example.test is reserved for documentation and never resolves in the real
// world, which is exactly why the hostile cases use it: nothing here can
// accidentally reach a live host.
func TestMain(m *testing.M) {
	restore := urlguard.Default.UseResolver(urlguard.MapResolver{
		"example.com":         {"93.184.216.34"},
		"rebind.example.test": {"169.254.169.254"},
	})
	code := m.Run()
	restore()
	os.Exit(code)
}

// ssrfImageURLs are the image_url values that must not reach the queue, one per
// bypass shape PSY-1672's edge guard was hardened against plus the hostname
// case only a resolver can see.
var ssrfImageURLs = []struct{ name, value string }{
	{"cloud metadata literal", "https://169.254.169.254/latest/meta-data/"},
	{"ipv6-mapped metadata", "https://[::ffff:169.254.169.254]/x.jpg"},
	{"decimal loopback", "https://2130706433/x.jpg"},
	{"octal loopback", "https://0177.0.0.1/x.jpg"},
	{"userinfo hiding the host", "https://example.com@169.254.169.254/x.jpg"},
	{"localhost with a trailing dot", "https://localhost./x.jpg"},
	{"rfc1918", "https://10.0.0.5/x.jpg"},
	{"name resolving to cloud metadata", "https://rebind.example.test/x.jpg"},
}

// TestCreateEntityRequest_RejectsSSRFImageURL: the queue-create boundary refuses
// a hostile flyer for every entity type whose payload carries one, so the value
// never reaches the queue at all.
func TestCreateEntityRequest_RejectsSSRFImageURL(t *testing.T) {
	payloads := map[string]func(imageURL string) string{
		"artist": func(u string) string { return `{"name":"Boris","image_url":"` + u + `"}` },
		"label":  func(u string) string { return `{"name":"Hydra Head","image_url":"` + u + `"}` },
		"venue": func(u string) string {
			return `{"name":"Trunk Space","city":"Phoenix","state":"AZ","image_url":"` + u + `"}`
		},
		"show": func(u string) string {
			return `{"title":"Boris","event_date":"2026-07-04","image_url":"` + u + `"}`
		},
	}
	for entityType, build := range payloads {
		for _, c := range ssrfImageURLs {
			t.Run(entityType+"/"+c.name, func(t *testing.T) {
				h := NewEntityRequestHandler(
					&testhelpers.MockEntityRequestService{
						CreateRequestFn: func(*authm.User, string, []byte, string, []byte, bool) (*communitym.EntityRequest, error) {
							t.Fatal("service must NOT be called for an SSRF image_url")
							return nil, nil
						},
					},
					nil, nil,
				)
				req := &CreateEntityRequestRequest{}
				req.Body.EntityType = entityType
				req.Body.Payload = json.RawMessage(build(c.value))
				_, err := h.CreateEntityRequestHandler(erUserCtx(), req)
				testhelpers.AssertHumaError(t, err, 422)
			})
		}
	}
}

// TestCreateEntityRequest_AcceptsPublicImageURL is the other half: the guard
// must not have closed the ordinary path.
func TestCreateEntityRequest_AcceptsPublicImageURL(t *testing.T) {
	called := false
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(*authm.User, string, []byte, string, []byte, bool) (*communitym.EntityRequest, error) {
				called = true
				return pendingRequest(9, "artist"), nil
			},
		},
		nil,
		&testhelpers.MockAuditLogService{},
	)
	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "artist"
	req.Body.Payload = json.RawMessage(`{"name":"Boris","image_url":"https://example.com/boris.jpg"}`)
	if _, err := h.CreateEntityRequestHandler(erUserCtx(), req); err != nil {
		t.Fatalf("a public flyer must still queue, got: %v", err)
	}
	if !called {
		t.Error("expected the request to reach the service")
	}
}

// TestAdminDecide_RejectsSSRFImageURLAtFulfill covers the row that predates the
// create-time guard (or whose host's DNS answer moved inward afterwards): the
// admin's approve is the moment the value becomes a live entity's flyer, so it
// is re-checked there and the catalog create never happens.
func TestAdminDecide_RejectsSSRFImageURLAtFulfill(t *testing.T) {
	for _, c := range ssrfImageURLs {
		t.Run(c.name, func(t *testing.T) {
			// Bypass the create-time guard the way a legacy row does: build the
			// payload directly rather than through the handler.
			payload := json.RawMessage(`{"name":"Evil","image_url":"` + c.value + `"}`)
			decided := pendingRequest(41, "artist")
			decided.Payload = &payload
			decided.DecisionState = communitym.EntityRequestStateApproved

			h := NewEntityRequestHandler(
				&testhelpers.MockEntityRequestService{
					DecideFn: func(uint, uint, communitym.EntityRequestDecisionState, *string) (*communitym.EntityRequest, error) {
						return decided, nil
					},
				},
				&testhelpers.MockEntityRequestFulfiller{
					CreateArtistFn: func(*contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
						t.Fatal("fulfiller must NOT be called for an SSRF image_url")
						return nil, nil
					},
				},
				&testhelpers.MockAuditLogService{},
			)

			req := &AdminDecideEntityRequestRequest{ID: "41"}
			req.Body.Decision = "approved"
			if _, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req); err == nil {
				t.Fatal("expected the approve to fail on the hostile flyer")
			}
		})
	}
}
