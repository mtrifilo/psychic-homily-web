package admin

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	handlershared "psychic-homily-backend/internal/api/handlers/shared"
	adminm "psychic-homily-backend/internal/models/admin"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils/urlguard"
)

// TestMain pins the SSRF host guard (PSY-1675) to a fixed resolution table so
// the approve-time re-check (PSY-1692) is judged against known addresses rather
// than the machine's DNS. Package-level state: no test in this package may call
// t.Parallel while relying on it.
//
// example.test is reserved for documentation and never resolves in the real
// world, so the hostile fixture cannot accidentally reach a live host.
func TestMain(m *testing.M) {
	restore := urlguard.Default.UseResolver(urlguard.MapResolver{
		"example.com":         {"93.184.216.34"},
		"rebind.example.test": {"169.254.169.254"},
	})
	code := m.Run()
	restore()
	os.Exit(code)
}

// ssrfApprovalImageURLs is PSY-1675's bypass corpus, every entry a value that
// could already be sitting in a pending_entity_edits row: written before the
// submit-time guard shipped, or through a write path that missed it. Approval is
// where such a value would go live.
var ssrfApprovalImageURLs = []struct{ name, value string }{
	{"cloud metadata literal", "https://169.254.169.254/latest/meta-data/"},
	{"ipv6-mapped metadata", "https://[::ffff:169.254.169.254]/x.jpg"},
	{"ipv6-mapped metadata, hex groups", "https://[::ffff:a9fe:a9fe]/x.jpg"},
	{"ipv4-compatible loopback", "https://[::127.0.0.1]/x.jpg"},
	{"decimal loopback", "https://2130706433/x.jpg"},
	{"hex loopback", "https://0x7f000001/x.jpg"},
	{"octal loopback", "https://0177.0.0.1/x.jpg"},
	{"short-form loopback", "https://127.1/x.jpg"},
	{"userinfo hiding the host", "https://example.com@169.254.169.254/x.jpg"},
	{"localhost", "https://localhost/x.jpg"},
	{"localhost with a trailing dot", "https://localhost./x.jpg"},
	{"gcp metadata by name", "https://metadata.google.internal/computeMetadata/v1/"},
	{"rfc1918", "https://10.0.0.5/x.jpg"},
	{"cgnat", "https://100.64.0.1/x.jpg"},
	{"oracle metadata", "https://192.0.0.192/x.jpg"},
	{"non-http scheme", "javascript:alert(1)"},
	{"name resolving to cloud metadata", "https://rebind.example.test/x.jpg"},
}

// TestFetchedURLFieldsMatchHandlerRegistry is the tripwire for the two copies of
// the fetched-field list drifting apart. urlFieldSpecs in
// internal/api/handlers/shared is canonical; fetchedURLFields here is the
// apply-side counterpart (see its doc comment for why it is a copy). Marking
// another field `fetched` there without adding it here would silently leave the
// approve path unguarded for that field.
//
// This test file is the one place the layering rule is relaxed: a TEST may
// import across layers to assert two layers agree, which is precedented here
// (internal/services/catalog/venue_bill_network_test.go). Production code in
// internal/services must not.
//
// A pure unit test: no database, no resolver.
func TestFetchedURLFieldsMatchHandlerRegistry(t *testing.T) {
	canonical := handlershared.FetchedURLFieldNames()
	applySide := make([]string, 0, len(fetchedURLFields))
	for field := range fetchedURLFields {
		applySide = append(applySide, field)
	}
	assert.ElementsMatch(t, canonical, applySide,
		"the approve path must re-validate exactly the fields the handler registry marks `fetched`")
}

// TestShapedURLFieldsMatchHandlerRegistry is the same tripwire for the `shape`
// rules (PSY-1966): add one to urlFieldSpecs without adding it to
// shapedURLFields and the approve path silently stops guarding that field, so a
// row queued before the rule shipped would still go live.
func TestShapedURLFieldsMatchHandlerRegistry(t *testing.T) {
	canonical := handlershared.ShapeRuledURLFieldNames()
	applySide := make([]string, 0, len(shapedURLFields))
	for field := range shapedURLFields {
		applySide = append(applySide, field)
	}
	assert.ElementsMatch(t, canonical, applySide,
		"the approve path must re-validate exactly the fields the handler registry gives a `shape` rule")
}

// TestApprovePendingEdit_RejectsStoredSSRFImageURL is the acceptance case: the
// stored value is applied to the live entity at approval, so a row carrying a
// host that points inward must fail the approval outright.
//
// Failing rather than silently dropping the field is the point, and the row must
// survive the refusal. That is what the status/reviewed_by assertions pin; the
// reasoning for why a pre-transaction return leaves the row untouched lives on
// the check itself in pending_edit.go.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_RejectsStoredSSRFImageURL() {
	for _, c := range ssrfApprovalImageURLs {
		s.Run(c.name, func() {
			user := s.createTestUser()
			reviewer := s.createTestUser()
			artist := s.createTestArtist(fmt.Sprintf("SSRF Test Artist %d", time.Now().UnixNano()))

			// Created through the service, which does NOT validate URLs; that is
			// the submit handler's job. This is precisely the row shape the ticket
			// is about: one that never met the guard.
			created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
				EntityType: "artist",
				EntityID:   artist.ID,
				UserID:     user.ID,
				Changes:    []adminm.FieldChange{{Field: "image_url", OldValue: nil, NewValue: c.value}},
				Summary:    "swap the photo",
			})
			s.Require().NoError(err)

			_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
			s.Require().Error(err, "approval must fail for %s", c.value)

			// Nothing applied.
			var applied struct{ ImageURL *string }
			s.Require().NoError(s.db.Table("artists").
				Select("image_url").Where("id = ?", artist.ID).Scan(&applied).Error)
			s.Nil(applied.ImageURL, "the hostile image_url must not reach the entity")

			// Row still actionable: an admin can reject it with a reason, the
			// contributor can cancel it. A row flipped to approved, or stamped
			// with a reviewer, would be neither.
			var row adminm.PendingEntityEdit
			s.Require().NoError(s.db.First(&row, created.ID).Error)
			s.Equal(adminm.PendingEditStatusPending, row.Status)
			s.Nil(row.ReviewedBy)
			s.Nil(row.ReviewedAt)

			// Rejecting it is the documented way out, and it still works.
			_, rerr := s.svc.RejectPendingEdit(created.ID, reviewer.ID, "image URL points at an internal address")
			s.NoError(rerr, "the refused edit must remain rejectable")
		})
	}
}

// TestApprovePendingEdit_AppliesPublicImageURL confirms the re-check did not
// close the ordinary path: an image on a public host still approves and applies.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_AppliesPublicImageURL() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist(fmt.Sprintf("SSRF Test Artist %d", time.Now().UnixNano()))

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes:    []adminm.FieldChange{{Field: "image_url", OldValue: nil, NewValue: "https://example.com/photo.jpg"}},
		Summary:    "add a photo",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().NoError(err)

	var applied struct{ ImageURL *string }
	s.Require().NoError(s.db.Table("artists").
		Select("image_url").Where("id = ?", artist.ID).Scan(&applied).Error)
	s.Require().NotNil(applied.ImageURL)
	s.Equal("https://example.com/photo.jpg", *applied.ImageURL)
}

// TestApprovePendingEdit_AllowsUnresolvableImageHost pins the posture PSY-1675
// settled on (PR #1781): a host that fails to RESOLVE passes. A flyer host that
// later expires must not make the edit permanently unapprovable, and refusing it
// would close nothing an attacker cannot reach by DNS rebinding anyway.
//
// unresolvable.example.test is absent from the TestMain resolution table, so the
// stub answers NXDOMAIN.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_AllowsUnresolvableImageHost() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist(fmt.Sprintf("SSRF Test Artist %d", time.Now().UnixNano()))

	const dead = "https://unresolvable.example.test/gone.jpg"
	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes:    []adminm.FieldChange{{Field: "image_url", OldValue: nil, NewValue: dead}},
		Summary:    "the host has since expired",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().NoError(err, "a dead host must not make the edit unapprovable")

	var applied struct{ ImageURL *string }
	s.Require().NoError(s.db.Table("artists").
		Select("image_url").Where("id = ?", artist.ID).Scan(&applied).Error)
	s.Require().NotNil(applied.ImageURL)
	s.Equal(dead, *applied.ImageURL)
}

// TestApprovePendingEdit_SSRFCheckRunsAfterDisallowedFieldGate pins the ordering
// the placement depends on. A row carrying BOTH a disallowed column and a
// hostile image_url must still take the PSY-572 path: auto-rejected, with the
// disallowed-fields sentinel. Reversing the two would change that gate's
// behaviour for corrupted submissions, and would leave the row pending where it
// is meant to end up rejected.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_SSRFCheckRunsAfterDisallowedFieldGate() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist(fmt.Sprintf("SSRF Test Artist %d", time.Now().UnixNano()))

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes: []adminm.FieldChange{
			{Field: "image_url", NewValue: "https://169.254.169.254/x.jpg"},
			{Field: "is_admin", NewValue: true},
		},
		Summary: "corrupted submission",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().Error(err)
	s.Require().ErrorIs(err, adminm.ErrPendingEditDisallowedFields)

	var row adminm.PendingEntityEdit
	s.Require().NoError(s.db.First(&row, created.ID).Error)
	s.Equal(adminm.PendingEditStatusRejected, row.Status)
}

// TestApprovePendingEdit_RejectsNonStringImageURL: FieldChange.NewValue is `any`
// decoded from JSONB, so a legacy or hand-written row can carry a number, bool,
// object or array where a URL belongs. Skipping those would leave the row to
// fail at the driver on every approve attempt and sit pending forever. A 422
// names the problem and lets the admin reject it.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_RejectsNonStringImageURL() {
	for _, c := range []struct {
		name  string
		value any
	}{
		{"number", float64(42)},
		{"bool", true},
		{"object", map[string]any{"url": "https://example.com/x.jpg"}},
		{"array", []any{"https://example.com/x.jpg"}},
	} {
		s.Run(c.name, func() {
			user := s.createTestUser()
			reviewer := s.createTestUser()
			artist := s.createTestArtist(fmt.Sprintf("SSRF Test Artist %d", time.Now().UnixNano()))

			created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
				EntityType: "artist",
				EntityID:   artist.ID,
				UserID:     user.ID,
				Changes:    []adminm.FieldChange{{Field: "image_url", NewValue: c.value}},
				Summary:    "malformed row",
			})
			s.Require().NoError(err)

			_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
			s.Require().Error(err)
			s.Contains(err.Error(), "must be a string")

			// Still actionable, same as the hostile-host case.
			var row adminm.PendingEntityEdit
			s.Require().NoError(s.db.First(&row, created.ID).Error)
			s.Equal(adminm.PendingEditStatusPending, row.Status)
		})
	}
}

// cancelAwareResolver models what a REAL resolver does with a dead context and
// what urlguard.MapResolver does not: it returns the context error instead of
// answering. Without it a cancellation test is vacuous, because MapResolver
// answers from a map and never looks at ctx, so the test passes whether or not
// the guard detaches from the caller's context.
type cancelAwareResolver struct{ inner urlguard.MapResolver }

func (r cancelAwareResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return r.inner.LookupIPAddr(ctx, host)
}

// TestApprovePendingEdit_GuardSurvivesCancelledContext pins the reason the guard
// runs on a detached context. urlguard treats a failed lookup as a PASS, and the
// writes below it take no context, so if the caller's cancelled context reached
// the resolver the classification would silently be skipped while the row still
// went live. The hostile value must be refused even when the admin's request
// context is already dead.
//
// Verified non-vacuous: replacing context.WithoutCancel(ctx) with ctx in
// ApprovePendingEdit makes this test fail.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_GuardSurvivesCancelledContext() {
	defer urlguard.Default.UseResolver(cancelAwareResolver{inner: urlguard.MapResolver{
		"example.com":         {"93.184.216.34"},
		"rebind.example.test": {"169.254.169.254"},
	}})()

	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist(fmt.Sprintf("SSRF Test Artist %d", time.Now().UnixNano()))

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		// A NAME, not a literal: only the resolver can classify it, so this case
		// fails if the lookup is skipped. An IP literal would be caught without
		// one and would prove nothing.
		Changes: []adminm.FieldChange{{Field: "image_url", NewValue: "https://rebind.example.test/x.jpg"}},
		Summary: "hostile host",
	})
	s.Require().NoError(err)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = s.svc.ApprovePendingEdit(cancelled, created.ID, reviewer.ID)
	s.Require().Error(err, "a cancelled request context must not disable the host guard")

	var applied struct{ ImageURL *string }
	s.Require().NoError(s.db.Table("artists").
		Select("image_url").Where("id = ?", artist.ID).Scan(&applied).Error)
	s.Nil(applied.ImageURL, "nothing may be applied when the guard refuses")
}

// TestApprovePendingEdit_IgnoresNonURLFieldsAndEmptyValues confirms the re-check
// stays out of the way of everything it does not own: a non-fetched field is not
// classified, and an empty image_url is the clear-the-field gesture, not a
// rejection.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_IgnoresNonURLFieldsAndEmptyValues() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist(fmt.Sprintf("SSRF Test Artist %d", time.Now().UnixNano()))

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes: []adminm.FieldChange{
			{Field: "image_url", NewValue: ""},
			// A website on a loopback host: `website` is not fetched, so it is
			// deliberately NOT classified here.
			{Field: "website", NewValue: "https://127.0.0.1/home"},
		},
		Summary: "clear the photo",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().NoError(err)
}
