package catalog

// Airing-feed adapter tests (PSY-1509). All parsing runs against inline
// fixtures — snapshots of the real provider responses probed 2026-07-23 —
// served from httptest servers. Tests never hit live provider APIs.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// =============================================================================
// KEXP
// =============================================================================

// kexpAiringShowsFixture mirrors the real GET /v2/shows/?limit=1 shape
// (2026-07-23: the listing carries NO end_time field).
const kexpAiringShowsFixture = `{"count":67199,"next":null,"previous":null,"results":[{"id":67334,"program":37,"program_name":"Eastern Echoes","host_names":["Diana Ratsamee"],"start_time":"2026-07-23T19:00:00-07:00"}]}`

// kexpAiringTimeslotsFixture mirrors GET /v2/timeslots/ — the weekly grid with
// the end bound the shows listing lacks. 2026-07-23 is a Thursday → weekday 4
// (KEXP's ISO-like convention, verified against this same broadcast).
const kexpAiringTimeslotsFixture = `{"count":2,"next":null,"previous":null,"results":[
	{"program":33,"weekday":4,"start_date":"2020-08-20","end_date":null,"start_time":"16:00:00","end_time":"19:00:00"},
	{"program":37,"weekday":4,"start_date":"2023-09-13","end_date":null,"start_time":"19:00:00","end_time":"22:00:00"}]}`

func newKEXPAiringServer(t *testing.T, showsBody, timeslotsBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v2/timeslots/"):
			_, _ = w.Write([]byte(timeslotsBody))
		case strings.HasPrefix(r.URL.Path, "/v2/shows/"):
			_, _ = w.Write([]byte(showsBody))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestKEXPFetchCurrentAirings(t *testing.T) {
	server := newKEXPAiringServer(t, kexpAiringShowsFixture, kexpAiringTimeslotsFixture)
	defer server.Close()
	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	airings, err := provider.FetchCurrentAirings("")
	require.NoError(t, err)
	require.Len(t, airings, 1)
	a := airings[0]

	assert.Equal(t, "37", a.ShowExternalID)
	assert.Equal(t, "Eastern Echoes", a.ShowName)
	assert.Equal(t, "67334", a.Episode.ExternalID)
	assert.Equal(t, "2026-07-23", a.Episode.AirDate)

	wantStart, _ := time.Parse(time.RFC3339, "2026-07-23T19:00:00-07:00")
	require.NotNil(t, a.Episode.StartsAt)
	assert.True(t, a.Episode.StartsAt.Equal(wantStart))

	// End bound resolved from the program's covering timeslot (19:00–22:00).
	wantEnd, _ := time.Parse(time.RFC3339, "2026-07-23T22:00:00-07:00")
	require.NotNil(t, a.Episode.EndsAt, "the covering timeslot supplies the end bound")
	assert.True(t, a.Episode.EndsAt.Equal(wantEnd))
	require.NotNil(t, a.Episode.DurationMinutes)
	assert.Equal(t, 180, *a.Episode.DurationMinutes)
}

func TestKEXPFetchCurrentAirings_NoCoveringSlotLeavesEndNil(t *testing.T) {
	// A special broadcast with no grid slot: the airing is still ingested,
	// just unbounded (never falsely live) — the end is not fabricated.
	server := newKEXPAiringServer(t, kexpAiringShowsFixture, `{"count":0,"results":[]}`)
	defer server.Close()
	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	airings, err := provider.FetchCurrentAirings("")
	require.NoError(t, err)
	require.Len(t, airings, 1)
	assert.NotNil(t, airings[0].Episode.StartsAt)
	assert.Nil(t, airings[0].Episode.EndsAt, "no covering slot → no fabricated end bound")
	assert.Nil(t, airings[0].Episode.DurationMinutes)
}

func TestKEXPFetchCurrentAirings_EmptyFeed(t *testing.T) {
	server := newKEXPAiringServer(t, `{"count":0,"results":[]}`, `{"count":0,"results":[]}`)
	defer server.Close()
	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	airings, err := provider.FetchCurrentAirings("")
	require.NoError(t, err)
	assert.Nil(t, airings)
}

func TestKEXPResolveAiringEnd_PostMidnightStartMatchesPreviousDaySlot(t *testing.T) {
	// A broadcast that starts a few minutes AFTER midnight (Saturday 00:05)
	// inside the previous day's wrapping slot (Friday 23:00–01:00) must still
	// resolve its end from that slot — the start's own weekday no longer
	// matches the slot's, but the slot is still in progress.
	slots := `{"count":1,"results":[{"program":50,"weekday":5,"start_date":"2020-01-01","end_date":null,"start_time":"23:00:00","end_time":"01:00:00"}]}`
	shows := `{"count":1,"results":[{"id":70001,"program":50,"program_name":"Night Show","start_time":"2026-07-25T00:05:00-07:00"}]}`
	server := newKEXPAiringServer(t, shows, slots)
	defer server.Close()
	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	airings, err := provider.FetchCurrentAirings("")
	require.NoError(t, err)
	require.Len(t, airings, 1)
	wantEnd, _ := time.Parse(time.RFC3339, "2026-07-25T01:00:00-07:00")
	require.NotNil(t, airings[0].Episode.EndsAt, "the previous day's wrapping slot covers a post-midnight start")
	assert.True(t, airings[0].Episode.EndsAt.Equal(wantEnd))
}

func TestKEXPResolveAiringEnd_FollowsTimeslotPagination(t *testing.T) {
	// The grid endpoint is paginated like every other KEXP list endpoint: a
	// covering slot on page TWO must still resolve.
	page2 := `{"count":101,"next":null,"results":[{"program":37,"weekday":4,"start_date":"2023-09-13","end_date":null,"start_time":"19:00:00","end_time":"22:00:00"}]}`
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v2/timeslots/") && r.URL.Query().Get("offset") == "100":
			_, _ = w.Write([]byte(page2))
		case strings.HasPrefix(r.URL.Path, "/v2/timeslots/"):
			// Page one: a full page of non-matching slots + a next cursor.
			_, _ = w.Write([]byte(`{"count":101,"next":"` + server.URL + `/v2/timeslots/?limit=100&offset=100","results":[{"program":99,"weekday":1,"start_date":"2020-01-01","end_date":null,"start_time":"00:00:00","end_time":"01:00:00"}]}`))
		case strings.HasPrefix(r.URL.Path, "/v2/shows/"):
			_, _ = w.Write([]byte(kexpAiringShowsFixture))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	airings, err := provider.FetchCurrentAirings("")
	require.NoError(t, err)
	require.Len(t, airings, 1)
	wantEnd, _ := time.Parse(time.RFC3339, "2026-07-23T22:00:00-07:00")
	require.NotNil(t, airings[0].Episode.EndsAt, "a slot on page two of the grid must still resolve the end")
	assert.True(t, airings[0].Episode.EndsAt.Equal(wantEnd))
}

func TestKEXPResolveAiringEnd_MidnightWrapAndGrace(t *testing.T) {
	// A late-night slot wrapping past midnight (23:00–01:00), and a broadcast
	// that started 5 minutes EARLY (22:55) — inside the start grace.
	// 2026-07-24 is a Friday → weekday 5.
	slots := `{"count":1,"results":[{"program":50,"weekday":5,"start_date":"2020-01-01","end_date":null,"start_time":"23:00:00","end_time":"01:00:00"}]}`
	shows := `{"count":1,"results":[{"id":70000,"program":50,"program_name":"Night Show","start_time":"2026-07-24T22:55:00-07:00"}]}`
	server := newKEXPAiringServer(t, shows, slots)
	defer server.Close()
	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	airings, err := provider.FetchCurrentAirings("")
	require.NoError(t, err)
	require.Len(t, airings, 1)
	wantEnd, _ := time.Parse(time.RFC3339, "2026-07-25T01:00:00-07:00")
	require.NotNil(t, airings[0].Episode.EndsAt)
	assert.True(t, airings[0].Episode.EndsAt.Equal(wantEnd), "a wrapped slot ends on the NEXT local day")
}

// =============================================================================
// NTS
// =============================================================================

// ntsAiringLiveFixture builds a /v2/live response for channel 1. broadcast is
// the embedded episode's ORIGINAL air instant — the rerun-guard key.
func ntsAiringLiveFixture(start, end, broadcast, episodeAlias string) string {
	return fmt.Sprintf(`{"results":[{"channel_name":"1","now":{
		"broadcast_title":"CHANNELING W/ IVAN SMAGGHE (R)",
		"start_timestamp":%q,"end_timestamp":%q,
		"embeds":{"details":{
			"name":"Channeling w/ Ivan Smagghe","show_alias":"channeling",
			"episode_alias":%q,"broadcast":%q,
			"mixcloud":"https://www.mixcloud.com/NTSRadio/x/"}}}},
		{"channel_name":"2","now":null}]}`, start, end, episodeAlias, broadcast)
}

func newNTSAiringServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v2/live") {
			_, _ = w.Write([]byte(body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func TestNTSFetchCurrentAirings_FirstRun(t *testing.T) {
	body := ntsAiringLiveFixture(
		"2026-07-24T04:00:00+01:00", "2026-07-24T06:00:00+01:00",
		"2026-07-24T04:00:00+01:00", "channeling-24th-july-2026")
	server := newNTSAiringServer(t, body)
	defer server.Close()
	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	airings, err := provider.FetchCurrentAirings("1")
	require.NoError(t, err)
	require.Len(t, airings, 1)
	a := airings[0]

	assert.Equal(t, "channeling", a.ShowExternalID)
	assert.Equal(t, "Channeling w/ Ivan Smagghe", a.ShowName)
	assert.Equal(t, "channeling/channeling-24th-july-2026", a.Episode.ExternalID)
	assert.Equal(t, "2026-07-24", a.Episode.AirDate)
	wantStart, _ := time.Parse(time.RFC3339, "2026-07-24T04:00:00+01:00")
	wantEnd, _ := time.Parse(time.RFC3339, "2026-07-24T06:00:00+01:00")
	require.NotNil(t, a.Episode.StartsAt)
	assert.True(t, a.Episode.StartsAt.Equal(wantStart))
	require.NotNil(t, a.Episode.EndsAt)
	assert.True(t, a.Episode.EndsAt.Equal(wantEnd))
	require.NotNil(t, a.Episode.DurationMinutes)
	assert.Equal(t, 120, *a.Episode.DurationMinutes)
	require.NotNil(t, a.Episode.ArchiveURL)
	assert.Equal(t, "https://www.mixcloud.com/NTSRadio/x/", *a.Episode.ArchiveURL)
}

func TestNTSFetchCurrentAirings_RerunSkipped(t *testing.T) {
	// The observed 2026-07-24 live shape: a rebroadcast whose embedded episode
	// originally aired 2024-12-03. Ingesting it would rewrite the ARCHIVE
	// episode's identity with a fabricated new airing — it must be skipped.
	body := ntsAiringLiveFixture(
		"2026-07-24T04:00:00+01:00", "2026-07-24T06:00:00+01:00",
		"2024-12-03T18:00:00+00:00", "channeling-3rd-december-2024")
	server := newNTSAiringServer(t, body)
	defer server.Close()
	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	airings, err := provider.FetchCurrentAirings("1")
	require.NoError(t, err)
	assert.Nil(t, airings, "a rerun of an archive episode is never ingested as a new airing")
}

func TestNTSFetchCurrentAirings_RerunSkippedByAliasDateAlone(t *testing.T) {
	// No broadcast field on the embedded episode — the alias-recovered date
	// still identifies the rerun.
	body := ntsAiringLiveFixture(
		"2026-07-24T04:00:00+01:00", "2026-07-24T06:00:00+01:00",
		"", "channeling-3rd-december-2024")
	server := newNTSAiringServer(t, body)
	defer server.Close()
	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	airings, err := provider.FetchCurrentAirings("1")
	require.NoError(t, err)
	assert.Nil(t, airings)
}

func TestNTSFetchCurrentAirings_UndateableEpisodeSkipped(t *testing.T) {
	// FAIL-CLOSED: no broadcast field AND no alias-recoverable date — the
	// episode cannot be verified as a first run, so it must be skipped, not
	// ingested by default (stamping a live window under an archive episode's
	// external id would rewrite its identity).
	body := ntsAiringLiveFixture(
		"2026-07-24T04:00:00+01:00", "2026-07-24T06:00:00+01:00",
		"", "channeling-special")
	server := newNTSAiringServer(t, body)
	defer server.Close()
	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	airings, err := provider.FetchCurrentAirings("1")
	require.NoError(t, err)
	assert.Nil(t, airings, "an undateable episode cannot be verified as a first run — skip")
}

func TestNTSFetchCurrentAirings_BroadcastSkewGuard(t *testing.T) {
	// The broadcast-instant tier: a first run's embedded episode IS this
	// broadcast, so its instant must match the live start within the 1h skew
	// tolerance. A same-day rerun (original evening → overnight repeat, hours
	// apart) must be skipped. Alias carries no date so the instant alone drives.
	fetch := func(broadcast string) ([]RadioAiring, error) {
		body := ntsAiringLiveFixture(
			"2026-07-24T04:00:00+01:00", "2026-07-24T06:00:00+01:00",
			broadcast, "channeling-special")
		server := newNTSAiringServer(t, body)
		defer server.Close()
		provider := NewNTSProviderWithClient(server.Client(), server.URL)
		defer provider.Close()
		return provider.FetchCurrentAirings("1")
	}

	inside, err := fetch("2026-07-24T03:30:00+01:00") // 30min skew
	require.NoError(t, err)
	assert.Len(t, inside, 1, "30min stamping skew → still a first run")

	sameDayRerun, err := fetch("2026-07-23T20:00:00+01:00") // 8h earlier
	require.NoError(t, err)
	assert.Nil(t, sameDayRerun, "an overnight repeat of yesterday evening's broadcast is skipped")
}

func TestNTSFetchCurrentAirings_DateOnlyBroadcastTier(t *testing.T) {
	// NTS sometimes emits a DATE-ONLY broadcast field (the recurring PSY-1152
	// shape). That must be compared at day granularity — never as an instant
	// against a fabricated midnight, which would fail the 1h skew tier for
	// nearly every real first-run.
	fetch := func(broadcast string) ([]RadioAiring, error) {
		body := ntsAiringLiveFixture(
			"2026-07-24T04:00:00+01:00", "2026-07-24T06:00:00+01:00",
			broadcast, "channeling-special")
		server := newNTSAiringServer(t, body)
		defer server.Close()
		provider := NewNTSProviderWithClient(server.Client(), server.URL)
		defer provider.Close()
		return provider.FetchCurrentAirings("1")
	}

	sameDay, err := fetch("2026-07-24")
	require.NoError(t, err)
	assert.Len(t, sameDay, 1, "date-only broadcast on the live start's day → first run, ingested")

	otherDay, err := fetch("2026-07-23")
	require.NoError(t, err)
	assert.Nil(t, otherDay, "date-only broadcast on another day → repeat, skipped")

	garbage, err := fetch("not-a-date")
	require.NoError(t, err)
	assert.Nil(t, garbage, "unparseable broadcast → fail closed")
}

func TestNTSFetchCurrentAirings_AliasDateTier(t *testing.T) {
	// No broadcast field: the alias-recovered date (day granularity) must be
	// the live start's own calendar day.
	fetch := func(alias string) ([]RadioAiring, error) {
		body := ntsAiringLiveFixture(
			"2026-07-24T04:00:00+01:00", "2026-07-24T06:00:00+01:00",
			"", alias)
		server := newNTSAiringServer(t, body)
		defer server.Close()
		provider := NewNTSProviderWithClient(server.Client(), server.URL)
		defer provider.Close()
		return provider.FetchCurrentAirings("1")
	}

	sameDay, err := fetch("channeling-24th-july-2026")
	require.NoError(t, err)
	assert.Len(t, sameDay, 1, "alias dated the live start's own day → ingested")

	otherDay, err := fetch("channeling-23rd-july-2026")
	require.NoError(t, err)
	assert.Nil(t, otherDay, "alias dated another day → skipped as a repeat")
}

func TestNTSFetchCurrentAirings_ImplausibleEndDropped(t *testing.T) {
	// end_timestamp is untrusted: a window longer than the plausibility cap is
	// dropped (row stays unbounded — never falsely live for days).
	body := ntsAiringLiveFixture(
		"2026-07-24T04:00:00+01:00", "2026-07-25T05:00:00+01:00", // 25h "broadcast"
		"2026-07-24T04:00:00+01:00", "channeling-24th-july-2026")
	server := newNTSAiringServer(t, body)
	defer server.Close()
	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	airings, err := provider.FetchCurrentAirings("1")
	require.NoError(t, err)
	require.Len(t, airings, 1)
	assert.NotNil(t, airings[0].Episode.StartsAt)
	assert.Nil(t, airings[0].Episode.EndsAt, "an implausibly long end bound is not frozen")
}

func TestNTSFetchCurrentAirings_MissingEpisodeAliasSkipped(t *testing.T) {
	body := ntsAiringLiveFixture(
		"2026-07-24T04:00:00+01:00", "2026-07-24T06:00:00+01:00",
		"2026-07-24T04:00:00+01:00", "")
	server := newNTSAiringServer(t, body)
	defer server.Close()
	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	airings, err := provider.FetchCurrentAirings("1")
	require.NoError(t, err)
	assert.Nil(t, airings, "no stable episode identity → never guess")
}

func TestNTSFetchCurrentAirings_OtherChannelNothingOnAir(t *testing.T) {
	body := ntsAiringLiveFixture(
		"2026-07-24T04:00:00+01:00", "2026-07-24T06:00:00+01:00",
		"2026-07-24T04:00:00+01:00", "channeling-24th-july-2026")
	server := newNTSAiringServer(t, body)
	defer server.Close()
	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	airings, err := provider.FetchCurrentAirings("2")
	require.NoError(t, err)
	assert.Nil(t, airings, "channel 2 reports now:null")
}

// =============================================================================
// reopenLivePlaylistState (pure)
// =============================================================================

func TestReopenLivePlaylistState(t *testing.T) {
	tests := []struct {
		name         string
		status       string
		state        string
		attempts     int
		playCount    int
		wantState    string
		wantAttempts int
	}{
		{"live complete with plays reopens to partial", catalogm.RadioEpisodeStatusLive, catalogm.RadioPlaylistStateComplete, 0, 12, catalogm.RadioPlaylistStatePartial, 0},
		{"live complete without plays reopens to pending", catalogm.RadioEpisodeStatusLive, catalogm.RadioPlaylistStateComplete, 2, 0, catalogm.RadioPlaylistStatePending, 0},
		{"live unavailable reopens to pending with attempts reset", catalogm.RadioEpisodeStatusLive, catalogm.RadioPlaylistStateUnavailable, 5, 0, catalogm.RadioPlaylistStatePending, 0},
		{"live pending untouched", catalogm.RadioEpisodeStatusLive, catalogm.RadioPlaylistStatePending, 1, 0, catalogm.RadioPlaylistStatePending, 1},
		{"live partial untouched", catalogm.RadioEpisodeStatusLive, catalogm.RadioPlaylistStatePartial, 0, 4, catalogm.RadioPlaylistStatePartial, 0},
		{"non-live complete untouched", catalogm.RadioEpisodeStatusAired, catalogm.RadioPlaylistStateComplete, 0, 12, catalogm.RadioPlaylistStateComplete, 0},
		{"non-live unavailable untouched", catalogm.RadioEpisodeStatusArchived, catalogm.RadioPlaylistStateUnavailable, 5, 0, catalogm.RadioPlaylistStateUnavailable, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, attempts := reopenLivePlaylistState(tt.status, tt.state, tt.attempts, tt.playCount)
			assert.Equal(t, tt.wantState, state)
			assert.Equal(t, tt.wantAttempts, attempts)
		})
	}
}
