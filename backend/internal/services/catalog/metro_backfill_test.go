package catalog

import (
	"testing"

	"psychic-homily-backend/internal/services/geo"
)

// fakeMetroGeo implements geo.Geocoder; only ResolveMetro is exercised here.
type fakeMetroGeo struct{ metros map[string]string } // city -> CBSA code

func (f fakeMetroGeo) Resolve(string, string, string) (geo.Result, bool) { return geo.Result{}, false }
func (f fakeMetroGeo) ResolveUSState(string) (string, geo.USStateStatus) {
	return "", geo.USStateNotFound
}
func (f fakeMetroGeo) ResolveMetro(city, _, _ string) (geo.Metro, bool) {
	if c, ok := f.metros[city]; ok {
		return geo.Metro{CBSACode: c, Name: c}, true
	}
	return geo.Metro{}, false
}

type fakeMetroStore struct {
	rows    []metroRow
	updates map[uint]*string
}

func (f *fakeMetroStore) LoadMetroRows() ([]metroRow, error) { return f.rows, nil }
func (f *fakeMetroStore) UpdateMetro(id uint, metro *string) error {
	if f.updates == nil {
		f.updates = map[uint]*string{}
	}
	f.updates[id] = metro
	return nil
}

func mp(s string) *string { return &s }

// TestReconcileMetros covers every action: set a NULL, leave a correct one,
// correct a stale one, clear one whose location no longer resolves. (The fake
// keys on the city string, so the two Pasadenas are modeled as distinct cities.)
func TestReconcileMetros(t *testing.T) {
	store := &fakeMetroStore{rows: []metroRow{
		{ID: 1, City: "PasadenaCA", State: "CA", Metro: nil},        // set → 31080
		{ID: 2, City: "Chicago", State: "IL", Metro: mp("16980")},   // unchanged
		{ID: 3, City: "PasadenaTX", State: "TX", Metro: mp("31080")}, // changed (stale LA) → 26420
		{ID: 4, City: "Nowhere", State: "ZZ", Metro: mp("99999")},   // cleared (no longer resolves)
	}}
	g := fakeMetroGeo{metros: map[string]string{"PasadenaCA": "31080", "Chicago": "16980", "PasadenaTX": "26420"}}

	rep, err := reconcileMetros(store, g, false)
	if err != nil {
		t.Fatalf("reconcileMetros: %v", err)
	}
	if rep.Scanned != 4 || rep.Set != 1 || rep.Unchanged != 1 || rep.Changed != 1 || rep.Cleared != 1 {
		t.Fatalf("report = %+v, want Scanned4/Set1/Unchanged1/Changed1/Cleared1", rep)
	}
	if got := store.updates[1]; got == nil || *got != "31080" {
		t.Errorf("artist 1 metro = %v, want 31080", got)
	}
	if _, wrote := store.updates[2]; wrote {
		t.Errorf("unchanged row must not be written")
	}
	if got := store.updates[3]; got == nil || *got != "26420" {
		t.Errorf("artist 3 metro = %v, want corrected 26420", got)
	}
	if got, wrote := store.updates[4]; !wrote || got != nil {
		t.Errorf("artist 4 metro = %v (wrote=%v), want cleared to nil", got, wrote)
	}
}

// TestReconcileMetros_DryRun classifies changes but writes nothing.
func TestReconcileMetros_DryRun(t *testing.T) {
	store := &fakeMetroStore{rows: []metroRow{
		{ID: 1, City: "Chicago", State: "IL", Metro: nil},
	}}
	g := fakeMetroGeo{metros: map[string]string{"Chicago": "16980"}}

	rep, err := reconcileMetros(store, g, true)
	if err != nil {
		t.Fatalf("reconcileMetros: %v", err)
	}
	if rep.Set != 1 {
		t.Errorf("Set = %d, want 1", rep.Set)
	}
	if len(store.updates) != 0 {
		t.Errorf("dry-run must not write, wrote %v", store.updates)
	}
}

func TestMetroDecision(t *testing.T) {
	g := fakeMetroGeo{metros: map[string]string{"Chicago": "16980"}}
	tests := []struct {
		name           string
		city           string
		current        *string
		wantAction     metroAction
		wantDesiredNil bool
	}{
		{"null → set", "Chicago", nil, metroSet, false},
		{"correct → unchanged", "Chicago", mp("16980"), metroUnchanged, false},
		{"stale → changed", "Chicago", mp("99999"), metroChanged, false},
		{"unresolvable but set → cleared", "Nowhere", mp("16980"), metroCleared, true},
		{"unresolvable + null → unchanged", "Nowhere", nil, metroUnchanged, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired, act := metroDecision(g, tt.city, "", "", tt.current)
			if act != tt.wantAction {
				t.Errorf("action = %d, want %d", act, tt.wantAction)
			}
			if (desired == nil) != tt.wantDesiredNil {
				t.Errorf("desired nil = %v, want %v", desired == nil, tt.wantDesiredNil)
			}
		})
	}
}
