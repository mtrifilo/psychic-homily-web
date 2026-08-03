package catalog

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
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
		return nil, "", nil
	}

	if s.cached != nil && s.cached.id == probe.ID {
		s.cached.freshUntil = now.Add(graphOverviewReadTTL)
		return s.cached.payload, s.cached.etag, nil
	}

	var row catalogm.GraphOverviewSnapshot
	err = s.db.Table("graph_overview_snapshots").
		Select("payload").
		Where("id = ?", probe.ID).
		Scan(&row).Error
	if err != nil {
		return nil, "", fmt.Errorf("failed to read overview snapshot payload: %w", err)
	}
	if len(row.Payload) == 0 {
		// The row was pruned between the probe and this read. Report nothing
		// built rather than an error: the next request re-probes and finds
		// whatever is newest now.
		return nil, "", nil
	}

	var payload contracts.GraphOverview
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		return nil, "", fmt.Errorf("failed to decode overview snapshot payload: %w", err)
	}

	// A strong ETag, not a weak one: the hash is over the exact stored bytes, so
	// two responses with this tag are byte-identical and a client may use it for
	// range requests and cache validation without qualification.
	etag := `"` + probe.ContentHash + `"`
	s.cached = &graphOverviewCached{
		id:         probe.ID,
		etag:       etag,
		payload:    &payload,
		freshUntil: now.Add(graphOverviewReadTTL),
	}
	return s.cached.payload, etag, nil
}
