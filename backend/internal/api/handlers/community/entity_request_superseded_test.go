package community

import (
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
// article, resubmitted as manual with nothing. The live row now says 'manual'
// and drops out of the ai_extraction filter, so the audit row has to carry what
// it stopped saying.
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

	payload, ok := md["superseded_payload"].(*json.RawMessage)
	if !ok || payload == nil {
		t.Fatalf("expected the superseded payload as raw JSON, got %T", md["superseded_payload"])
	}
	if !strings.Contains(string(*payload), "from the source article") {
		t.Errorf("expected the superseded payload's content, got %s", string(*payload))
	}

	detail, ok := md["superseded_source_detail"].(*json.RawMessage)
	if !ok || detail == nil {
		t.Fatalf("expected the superseded source_detail as raw JSON, got %T",
			md["superseded_source_detail"])
	}
	if !strings.Contains(string(*detail), "example.com/article") {
		t.Errorf("expected the superseded source article, got %s", string(*detail))
	}
}

// A first filing supersedes nothing, so it must not claim to. An audit consumer
// keys on the presence of these fields to tell a correction from a fresh
// request.
func TestCreateEntityRequest_FreshFilingAuditRecordsNothingSuperseded(t *testing.T) {
	action, md := captureAuditMetadata(t, nil)

	if action != "queue_entity_request" {
		t.Fatalf("expected action=queue_entity_request, got %s", action)
	}
	for _, key := range []string{
		"superseded_source_context",
		"superseded_source_detail",
		"superseded_payload",
		"superseded_payload_omitted_bytes",
	} {
		if _, present := md[key]; present {
			t.Errorf("a fresh filing must not carry %s", key)
		}
	}
}

// A payload too large to copy still says a payload was destroyed and how big it
// was. Reachable only for a row queued before the boundary caps existed, which
// is precisely the row worth not writing unbounded into audit_logs.
func TestAddSupersededMetadata_OversizedPayloadIsCountedNotCopied(t *testing.T) {
	oversized := json.RawMessage(`{"name":"` + strings.Repeat("x", maxSupersededPayloadBytes) + `"}`)
	md := map[string]interface{}{}

	addSupersededMetadata(md, &communitym.SupersededSubmission{
		Payload:       &oversized,
		SourceContext: communitym.EntityRequestSourceAIExtraction,
	})

	if _, present := md["superseded_payload"]; present {
		t.Error("a payload over the cap must not be copied into the audit row")
	}
	if md["superseded_payload_omitted_bytes"] != len(oversized) {
		t.Errorf("expected the byte count %d to be recorded in its place, got %v",
			len(oversized), md["superseded_payload_omitted_bytes"])
	}
	if md["superseded_source_context"] != communitym.EntityRequestSourceAIExtraction {
		t.Error("the superseded source_context is small and is always recorded")
	}
}

// A payload EXACTLY at the cap is copied: the bound is inclusive, and a test
// that only drives the two extremes would not say which side the boundary sits.
func TestAddSupersededMetadata_PayloadAtTheCapIsCopied(t *testing.T) {
	atCap := json.RawMessage(strings.Repeat("x", maxSupersededPayloadBytes))
	md := map[string]interface{}{}

	addSupersededMetadata(md, &communitym.SupersededSubmission{Payload: &atCap})

	if _, present := md["superseded_payload"]; !present {
		t.Error("a payload at the cap is within it")
	}
	if _, present := md["superseded_payload_omitted_bytes"]; present {
		t.Error("nothing was omitted, so nothing should say so")
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
