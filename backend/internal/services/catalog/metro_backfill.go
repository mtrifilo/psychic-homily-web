package catalog

import (
	"fmt"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
)

// PSY-1255 step B: reconcile the denormalized `metro` (CBSA code) column on
// artists and venues. metro is DERIVED from (city, state, country) via
// geo.ResolveMetro, so it must equal that derivation at all times for the scene
// rollup to be correct. Every FOREGROUND service write path sets it alongside the
// location: artists via the create funnel (FindOrCreateArtistTx, covering admin
// create + data-sync import + show-inline/discovery/seed, …) + UpdateArtist + the
// contribution-edit apply; venues
// via applyGeocoding (create + UpdateVenue) + the contribution-edit apply +
// data-sync import. Only the BACKGROUND location writers — the artist location-
// enrichment fill and the offline state/location backfills (step 0) — change an
// entity's location WITHOUT touching metro, so it drifts.
// This reconciler recomputes metro for EVERY row and writes only the ones that
// differ — the backstop, run after a location/state backfill, and a no-op on a
// clean second run.

// MetroReport is the structured outcome of a reconcile run.
type MetroReport struct {
	Scanned   int      // rows considered
	Set       int      // metro was NULL, now populated
	Changed   int      // metro had a different (stale) value, corrected
	Cleared   int      // metro was set but the location no longer resolves → NULL
	Unchanged int      // already correct
	Errors    []string // per-row update failures (live run)
}

// metroRow is the location-only projection both artists and venues reduce to, so
// one reconcile core serves both despite their differing model field types.
type metroRow struct {
	ID      uint
	City    string
	State   string
	Country string
	Metro   *string
}

// metroStore abstracts loading the location rows of one entity table and writing
// a corrected metro, so the reconcile core is unit-testable with a fake.
type metroStore interface {
	LoadMetroRows() ([]metroRow, error)
	UpdateMetro(id uint, metro *string) error
}

// ReconcileArtistMetros recomputes and corrects artists.metro. Dry-run reports
// what would change without writing.
func ReconcileArtistMetros(db *gorm.DB, g geo.Geocoder, dryRun bool) (*MetroReport, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return reconcileMetros(&gormArtistMetroStore{db: db}, g, dryRun)
}

// ReconcileVenueMetros recomputes and corrects venues.metro.
func ReconcileVenueMetros(db *gorm.DB, g geo.Geocoder, dryRun bool) (*MetroReport, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return reconcileMetros(&gormVenueMetroStore{db: db}, g, dryRun)
}

// ReconcileFestivalMetros recomputes and corrects festivals.metro (PSY-1278).
func ReconcileFestivalMetros(db *gorm.DB, g geo.Geocoder, dryRun bool) (*MetroReport, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return reconcileMetros(&gormFestivalMetroStore{db: db}, g, dryRun)
}

// reconcileMetros is the store-agnostic core: for each row, recompute the metro
// and write only when it differs from what's stored. Idempotent.
func reconcileMetros(store metroStore, g geo.Geocoder, dryRun bool) (*MetroReport, error) {
	rows, err := store.LoadMetroRows()
	if err != nil {
		return nil, fmt.Errorf("load rows: %w", err)
	}
	report := &MetroReport{Scanned: len(rows)}
	for _, r := range rows {
		desired, act := metroDecision(g, r.City, r.State, r.Country, r.Metro)
		switch act {
		case metroUnchanged:
			report.Unchanged++
			continue
		case metroSet:
			report.Set++
		case metroChanged:
			report.Changed++
		case metroCleared:
			report.Cleared++
		}
		if dryRun {
			continue
		}
		if err := store.UpdateMetro(r.ID, desired); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("entity %d update: %v", r.ID, err))
		}
	}
	return report, nil
}

type metroAction int

const (
	metroUnchanged metroAction = iota
	metroSet
	metroChanged
	metroCleared
)

// metroDecision computes the metro a row SHOULD have and classifies the change
// versus what it currently has. geo.ResolveMetro already refuses to guess an
// unpinned multi-state name, so a non-resolving location yields nil (→ cleared if
// one was stored). Both nil, or equal codes, is metroUnchanged — for which it
// returns a nil desired, since the caller must not write an unchanged row.
func metroDecision(g geo.Geocoder, city, state, country string, current *string) (*string, metroAction) {
	desired := geo.MetroPointer(g, city, state, country)
	switch {
	case strPtrEq(current, desired):
		return nil, metroUnchanged
	case current == nil:
		return desired, metroSet
	case desired == nil:
		return desired, metroCleared
	default:
		return desired, metroChanged
	}
}

func strPtrEq(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// --- gorm stores -------------------------------------------------------------

type gormArtistMetroStore struct{ db *gorm.DB }

func (s *gormArtistMetroStore) LoadMetroRows() ([]metroRow, error) {
	var artists []catalogm.Artist
	// A city-less row can't gain a metro — BUT a row whose city was cleared after
	// a metro was stamped MUST still be scanned so the now-stale metro gets cleared
	// (else metroCleared is unreachable and the "every row reconciled" contract
	// breaks). So: rows with a city OR an already-set metro.
	if err := s.db.
		Where("(city IS NOT NULL AND TRIM(city) <> '') OR metro IS NOT NULL").
		Order("id").Find(&artists).Error; err != nil {
		return nil, err
	}
	rows := make([]metroRow, len(artists))
	for i := range artists {
		a := &artists[i]
		rows[i] = metroRow{ID: a.ID, City: derefString(a.City), State: derefString(a.State), Country: derefString(a.Country), Metro: a.Metro}
	}
	return rows, nil
}

func (s *gormArtistMetroStore) UpdateMetro(id uint, metro *string) error {
	return s.db.Model(&catalogm.Artist{}).Where("id = ?", id).Update("metro", metro).Error
}

type gormVenueMetroStore struct{ db *gorm.DB }

func (s *gormVenueMetroStore) LoadMetroRows() ([]metroRow, error) {
	var venues []catalogm.Venue
	if err := s.db.Order("id").Find(&venues).Error; err != nil {
		return nil, err
	}
	rows := make([]metroRow, len(venues))
	for i := range venues {
		v := &venues[i]
		// Venue city/state are non-nullable strings; country is a pointer.
		rows[i] = metroRow{ID: v.ID, City: v.City, State: v.State, Country: derefString(v.Country), Metro: v.Metro}
	}
	return rows, nil
}

func (s *gormVenueMetroStore) UpdateMetro(id uint, metro *string) error {
	return s.db.Model(&catalogm.Venue{}).Where("id = ?", id).Update("metro", metro).Error
}

type gormFestivalMetroStore struct{ db *gorm.DB }

func (s *gormFestivalMetroStore) LoadMetroRows() ([]metroRow, error) {
	var festivals []catalogm.Festival
	// Same shape as the artist store: festival city is nullable, and a row whose
	// city was cleared after a metro was stamped must still be scanned so the
	// stale metro gets cleared (see the artist store's rationale).
	if err := s.db.
		Where("(city IS NOT NULL AND TRIM(city) <> '') OR metro IS NOT NULL").
		Order("id").Find(&festivals).Error; err != nil {
		return nil, err
	}
	rows := make([]metroRow, len(festivals))
	for i := range festivals {
		f := &festivals[i]
		rows[i] = metroRow{ID: f.ID, City: derefString(f.City), State: derefString(f.State), Country: derefString(f.Country), Metro: f.Metro}
	}
	return rows, nil
}

func (s *gormFestivalMetroStore) UpdateMetro(id uint, metro *string) error {
	return s.db.Model(&catalogm.Festival{}).Where("id = ?", id).Update("metro", metro).Error
}
