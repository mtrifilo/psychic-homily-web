package community

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	communitym "psychic-homily-backend/internal/models/community"
)

// PSY-1978: a replacement is destructive and the row keeps no history, so the
// audit row is the only artifact that can say what was overwritten.

// captureAuditMetadata runs the queue-create handler and returns the metadata of
// the audit row it wrote. The write is fire-and-forget in a goroutine, so it is
// read off a channel rather than after a sleep.
func captureAuditMetadata(
	t *testing.T,
	superseded *communitym.SupersededSubmission,
) (string, map[string]interface{}) {
	t.Helper()

	type logged struct {
		action   string
		metadata map[string]interface{}
	}
	written := make(chan logged, 1)

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(*authm.User, string, []byte, string, []byte, bool) (*communitym.EntityRequest, *communitym.SupersededSubmission, error) {
				return pendingRequest(7, "artist"), superseded, nil
			},
		},
		nil,
		&testhelpers.MockAuditLogService{
			LogActionFn: func(_ uint, action, _ string, _ uint, md map[string]interface{}) {
				written <- logged{action: action, metadata: md}
			},
		},
	)

	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "artist"
	req.Body.Payload = artistPayload(t, "Boris")
	req.Body.SourceContext = communitym.EntityRequestSourceManual

	if _, err := h.CreateEntityRequestHandler(erUserCtx(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case row := <-written:
		return row.action, row.metadata
	case <-time.After(2 * time.Second):
		t.Fatal("the queue-create path must write an audit row")
		return "", nil
	}
}

// The provenance case the ticket names: filed as ai_extraction with a source
// article, resubmitted as manual with nothing. The live row now says 'manual',
// so the audit row has to name what it stopped saying.
func TestCreateEntityRequest_ReplacementAuditCarriesTheSupersededSubmission(t *testing.T) {
	action, md := captureAuditMetadata(t, supersededSubmission())

	if action != "replace_entity_request" {
		t.Fatalf("expected action=replace_entity_request, got %s", action)
	}
	if md["superseded_source_context"] != communitym.EntityRequestSourceAIExtraction {
		t.Errorf("expected the superseded source_context to be recorded, got %v",
			md["superseded_source_context"])
	}
	if md["source_context"] != communitym.EntityRequestSourceManual {
		t.Errorf("the row's CURRENT source_context is still recorded, got %v", md["source_context"])
	}
	if md["superseded_source_detail_present"] != true {
		t.Errorf("expected the superseded source detail to be reported as present, got %v",
			md["superseded_source_detail_present"])
	}

	superseded := supersededSubmission()
	sum := sha256.Sum256(*superseded.Payload)
	if md["superseded_payload_sha256"] != hex.EncodeToString(sum[:]) {
		t.Errorf("expected the superseded payload's digest, got %v", md["superseded_payload_sha256"])
	}
	if md["superseded_payload_bytes"] != len(*superseded.Payload) {
		t.Errorf("expected the superseded payload's length %d, got %v",
			len(*superseded.Payload), md["superseded_payload_bytes"])
	}
}

// audit_logs.metadata is served to anonymous callers through the contributor
// profile, so a replacement must put NOTHING contributor-authored in it. This
// pins the rule by asserting the content is absent and unrecoverable, which a
// future "just record the payload, it is more useful" edit would break.
func TestCreateEntityRequest_ReplacementAuditPublishesNoSubmissionContent(t *testing.T) {
	_, md := captureAuditMetadata(t, supersededSubmission())

	for _, key := range []string{"superseded_payload", "superseded_source_detail"} {
		if _, present := md[key]; present {
			t.Errorf("%s carries contributor-authored content into a publicly readable column", key)
		}
	}
	for key, value := range md {
		text, ok := value.(string)
		if !ok {
			continue
		}
		if strings.Contains(text, "from the source article") || strings.Contains(text, "example.com") {
			t.Errorf("metadata key %s leaks superseded submission content: %s", key, text)
		}
	}
}

// A first filing supersedes nothing, so it must not claim to.
func TestCreateEntityRequest_FreshFilingAuditRecordsNothingSuperseded(t *testing.T) {
	action, md := captureAuditMetadata(t, nil)

	if action != "queue_entity_request" {
		t.Fatalf("expected action=queue_entity_request, got %s", action)
	}
	for _, key := range []string{
		"superseded_source_context",
		"superseded_source_detail_present",
		"superseded_payload_sha256",
		"superseded_payload_bytes",
	} {
		if _, present := md[key]; present {
			t.Errorf("a fresh filing must not carry %s", key)
		}
	}
}

// Every value written is a string, a bool or an int, so the map marshals
// whatever the stored payload contains. A *json.RawMessage holding invalid JSON
// fails json.Marshal for the WHOLE map, and LogAction answers that by storing a
// NULL metadata column — losing request_id and the action's own fields along
// with it. A row queued before a validator existed is exactly such a payload.
func TestAddSupersededMetadata_MarshalsWithAnUnparseableStoredPayload(t *testing.T) {
	junk := json.RawMessage("not json at all")
	md := map[string]interface{}{"request_id": uint(1)}

	addSupersededMetadata(md, &communitym.SupersededSubmission{
		Payload:       &junk,
		SourceContext: communitym.EntityRequestSourceAIExtraction,
	})

	encoded, err := json.Marshal(md)
	if err != nil {
		t.Fatalf("the audit metadata must marshal whatever the row held: %v", err)
	}
	if !strings.Contains(string(encoded), `"request_id":1`) {
		t.Errorf("the action's own fields must survive: %s", encoded)
	}
	if md["superseded_payload_bytes"] != len(junk) {
		t.Errorf("expected the length of what was destroyed, got %v", md["superseded_payload_bytes"])
	}
}

// The record is fixed-size whatever the row held, which is what keeps a
// resubmission loop from growing an append-only table without bound.
func TestAddSupersededMetadata_IsFixedSizeForAnyPayload(t *testing.T) {
	huge := json.RawMessage(`{"name":"` + strings.Repeat("x", 1<<20) + `"}`)
	md := map[string]interface{}{}

	addSupersededMetadata(md, &communitym.SupersededSubmission{Payload: &huge})

	encoded, err := json.Marshal(md)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(encoded) > 256 {
		t.Errorf("a 1 MB payload must not produce a large audit row; got %d bytes", len(encoded))
	}
}

// The admin queue reads original_source_context off the row, so the projection
// has to carry it; without that the moderation card cannot say a request was
// filed as something else.
func TestToAdminEntityRequestView_CarriesOriginalSourceContext(t *testing.T) {
	row := pendingRequest(3, "artist")
	original := communitym.EntityRequestSourceAIExtraction
	row.OriginalSourceContext = &original
	row.SourceContext = communitym.EntityRequestSourceManual

	view := toAdminEntityRequestView(row)

	if view.OriginalSourceContext == nil || *view.OriginalSourceContext != original {
		t.Errorf("expected original_source_context %q on the view, got %v",
			original, view.OriginalSourceContext)
	}
	if view.SourceContext != communitym.EntityRequestSourceManual {
		t.Errorf("the row's current source_context is unchanged, got %q", view.SourceContext)
	}
}

func TestToAdminEntityRequestView_UnreplacedRowHasNoOriginalSourceContext(t *testing.T) {
	view := toAdminEntityRequestView(pendingRequest(4, "artist"))

	if view.OriginalSourceContext != nil {
		t.Errorf("a row that was never replaced states no earlier source, got %v",
			*view.OriginalSourceContext)
	}
}
