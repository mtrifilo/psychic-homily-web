package catalog

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/services/contracts"
)

// ──────────────────────────────────────────────
// Graph overview: the read side
// ──────────────────────────────────────────────
//
// The snapshot is written once a night and read on every cold visit to /graph,
// so this service optimizes for the read: the decoded payload is held in
// process and the database is only asked which snapshot is newest.
//
// WHY A DEDICATED CACHE RATHER THAN chartsCache. That cache is built for a
// client-controlled key space (module|window|scene|limit|offset) and pays for
// it with an entry cap and a sweep. This resource has exactly ONE key, and the
// expensive part is not the query — it is decoding a few hundred KB of JSON.
// A one-entry cache keyed on the snapshot id is the whole requirement, and it
// gets a property the generic cache cannot: when the TTL lapses on an UNCHANGED
// snapshot, the probe below confirms the id and the decoded payload is reused
// rather than rebuilt.

const (
	// graphOverviewReadTTL is how long a cached payload is served without
	// re-checking the table. It bounds the visible delay between a nightly
	// build committing and the endpoint serving it, and it bounds the probe
	// query to once a minute per process rather than once a request.
	graphOverviewReadTTL = time.Minute
)

// GraphOverviewService serves the newest precomputed overview map.
type GraphOverviewService struct {
	db *gorm.DB

	// now is injectable so the cache's expiry can be tested without sleeping.
	now func() time.Time

	// mu guards cached AND is held across the reload. Holding it through the
	// decode is deliberate single-flight: a cold process taking simultaneous
	// requests decodes the payload once instead of once per request.
	mu     sync.Mutex
	cached *graphOverviewCached
}

// graphOverviewCached is the one decoded snapshot this process is holding.
type graphOverviewCached struct {
	// id is the snapshot row's primary key — the identity the probe compares
	// against, so an unchanged snapshot never costs a second decode.
	id         uint
	etag       string
	payload    *contracts.GraphOverview
	freshUntil time.Time
}

// NewGraphOverviewService creates the overview read service.
func NewGraphOverviewService(db *gorm.DB) *GraphOverviewService {
	return &GraphOverviewService{db: db, now: time.Now}
}

// GetGraphOverview returns the newest snapshot and its ETag.
//
// A database with no snapshot yet (a fresh environment before the first nightly
// run) returns a nil payload and a nil error — that is a "not built yet", not a
// failure, and the handler turns it into a 503 rather than an empty map that a
// client would cache as the truth.
func (s *GraphOverviewService) GetGraphOverview() (*contracts.GraphOverview, string, error) {
	if s.db == nil {
		return nil, "", fmt.Errorf("database not initialized")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if s.cached != nil && now.Before(s.cached.freshUntil) {
		return s.cached.payload, s.cached.etag, nil
	}

	// The probe reads two small columns, never the payload. That is the whole
	// point: the common case at TTL expiry is "same snapshot as last minute",
	// and confirming that must not cost a payload read.
	var probe struct {
		ID          uint   `gorm:"column:id"`
		ContentHash string `gorm:"column:content_hash"`
	}
	err := s.db.Table("graph_overview_snapshots").
		Select("id, content_hash").
		Order("id DESC").
		Limit(1).
		Scan(&probe).Error
	if err != nil {
		return nil, "", fmt.Errorf("failed to read overview snapshot: %w", err)
	}
	if probe.ID == 0 {
		// Cache the negative too. Between this service deploying and the first
		// nightly build there is no snapshot at all, and without this every
		// anonymous request in that window runs a query.
		s.cached = &graphOverviewCached{freshUntil: now.Add(graphOverviewReadTTL)}
		return nil, "", nil
	}

	if s.cached != nil && s.cached.id == probe.ID {
		s.cached.freshUntil = now.Add(graphOverviewReadTTL)
		return s.cached.payload, s.cached.etag, nil
	}

	// ONE STATEMENT FOR THE RELOAD, and it takes its own id and hash rather than
	// the probe's. Reading `WHERE id = probe.ID` instead would let a retention
	// prune landing between the two queries turn a database holding three
	// healthy snapshots into a 503.
	var row struct {
		ID          uint            `gorm:"column:id"`
		ContentHash string          `gorm:"column:content_hash"`
		Payload     json.RawMessage `gorm:"column:payload"`
	}
	err = s.db.Table("graph_overview_snapshots").
		Select("id, content_hash, payload").
		Order("id DESC").
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return nil, "", fmt.Errorf("failed to read overview snapshot payload: %w", err)
	}
	if row.ID == 0 || len(row.Payload) == 0 {
		s.cached = &graphOverviewCached{freshUntil: now.Add(graphOverviewReadTTL)}
		return nil, "", nil
	}

	var payload contracts.GraphOverview
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		return nil, "", fmt.Errorf("failed to decode overview snapshot payload: %w", err)
	}
	// REFUSE A SNAPSHOT THIS BUILD DOES NOT UNDERSTAND. A deploy that changes
	// the payload shape goes live before the nightly job that rebuilds it, so
	// there is always a window where the newest row is the OLD shape. Serving it
	// would ship a body that disagrees with the schema clients just fetched —
	// and because the ETag would be unchanged, caches would hold it for another
	// day. Reporting "not built yet" makes that window a visible 503 instead.
	if payload.Version != contracts.GraphOverviewVersion {
		slog.Warn("overview snapshot: newest snapshot is a different payload version; serving unavailable until the next build",
			"snapshot_version", payload.Version, "expected_version", contracts.GraphOverviewVersion)
		s.cached = &graphOverviewCached{
			id:         row.ID,
			freshUntil: now.Add(graphOverviewReadTTL),
		}
		return nil, "", nil
	}

	// A WEAK ETag, matching what the calendar feeds already mint. The tag
	// identifies the SNAPSHOT, and two responses carrying it are semantically
	// identical — but not byte-identical, which is the stronger thing a strong
	// validator would promise.
	//
	// THE HASH IS NOT A DIGEST OF THE BYTES ON THE WIRE, and must not be described
	// as one: the column is JSONB, so Postgres normalizes key order and whitespace
	// on the way back out, and Huma's SchemaLinkTransformer adds a `$schema`
	// member to the served body. It is a build-time identity stamp for the
	// snapshot, which is all a validator needs, since the map only ever changes by
	// a new row appearing. Compression settled the question — one snapshot now
	// legitimately goes out under two content-codings, which RFC 9110 §8.8.1
	// forbids a strong validator from sharing — but the byte-for-byte promise was
	// never really ours to make.
	//
	// Weakening costs nothing here. If-None-Match is specified to use WEAK
	// comparison (shared.IfNoneMatchCovers already strips the prefix on both
	// sides), so revalidation and its 304s are unaffected. The only thing a weak
	// validator gives up is Range requests, and this endpoint serves a single
	// whole JSON document with no Accept-Ranges.
	etag := `W/"` + row.ContentHash + `"`
	s.cached = &graphOverviewCached{
		id:         row.ID,
		etag:       etag,
		payload:    &payload,
		freshUntil: now.Add(graphOverviewReadTTL),
	}
	return s.cached.payload, etag, nil
}
