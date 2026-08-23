package catalog

import (
	"testing"
	"time"
)

// TestShowAnnounceable pins the one predicate both ends of the PSY-1894
// notification outbox consult. It is a unit test on purpose: this rule decides
// whether real email goes out, and it should be checkable without a database so
// nobody is tempted to skip it.
func TestShowAnnounceable(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	past := now.Add(-24 * time.Hour)

	base := func() *Show {
		return &Show{ID: 1, Status: ShowStatusApproved, EventDate: future}
	}

	cases := []struct {
		name       string
		show       *Show
		wantOK     bool
		wantReason string
	}{
		{"upcoming approved show", base(), true, ""},
		{"nil show", nil, false, NotAnnounceableGone},
		{
			"pending show is not public",
			&Show{ID: 1, Status: ShowStatusPending, EventDate: future},
			false, NotAnnounceableNotPublic,
		},
		{
			"private show is not public",
			&Show{ID: 1, Status: ShowStatusPrivate, EventDate: future},
			false, NotAnnounceableNotPublic,
		},
		{
			"rejected show is not public",
			&Show{ID: 1, Status: ShowStatusRejected, EventDate: future},
			false, NotAnnounceableNotPublic,
		},
		{
			"cancelled show is not announced",
			&Show{ID: 1, Status: ShowStatusApproved, EventDate: future, IsCancelled: true},
			false, NotAnnounceableCancelled,
		},
		{
			// The deliberate asymmetry with cancellation: sold out is information a
			// follower still wants, and the admin approve path has always sent it.
			"sold out show is still announced",
			&Show{ID: 1, Status: ShowStatusApproved, EventDate: future, IsSoldOut: true},
			true, "",
		},
		{
			// The archival-import guard. Without it, ingesting a venue's back
			// catalogue fans years-old listings out as new announcements.
			"past show is not announced",
			&Show{ID: 1, Status: ShowStatusApproved, EventDate: past},
			false, NotAnnounceablePastEvent,
		},
		{
			// Exactly now counts as still upcoming: Before is strict, and a show
			// starting this instant is being announced late rather than wrongly.
			"show starting exactly now is announced",
			&Show{ID: 1, Status: ShowStatusApproved, EventDate: now},
			true, "",
		},
		{
			// Order matters: a cancelled AND past show reports the cancellation,
			// because that is the fact an operator reading the queue needs first.
			"cancellation outranks the past-date rule",
			&Show{ID: 1, Status: ShowStatusApproved, EventDate: past, IsCancelled: true},
			false, NotAnnounceableCancelled,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, reason := ShowAnnounceable(tc.show, now)
			if ok != tc.wantOK {
				t.Fatalf("ShowAnnounceable ok = %v, want %v (reason %q)", ok, tc.wantOK, reason)
			}
			if reason != tc.wantReason {
				t.Fatalf("ShowAnnounceable reason = %q, want %q", reason, tc.wantReason)
			}
		})
	}
}
