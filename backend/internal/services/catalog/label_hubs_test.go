package catalog

import (
	"testing"

	"psychic-homily-backend/internal/services/contracts"
)

// rosterRow is a terse constructor for membership fixtures.
func rosterRow(labelID uint, name string, artistID uint) labelRosterRow {
	slug := name + "-slug"
	return labelRosterRow{
		LabelID:  labelID,
		Name:     name,
		Slug:     &slug,
		ArtistID: artistID,
	}
}

// sluglessRosterRow is a membership on a label that has no page to open.
func sluglessRosterRow(labelID uint, name string, artistID uint) labelRosterRow {
	return labelRosterRow{LabelID: labelID, Name: name, Slug: nil, ArtistID: artistID}
}

// artistIDsFromRows is the artist set for a fixture whose payload contains
// every artist carrying a membership row — the ordinary case, where the roster
// read and the node set agree. Tests that need them to DISAGREE pass their own
// set instead.
func artistIDsFromRows(rows []labelRosterRow) []uint {
	seen := make(map[uint]struct{}, len(rows))
	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		if _, ok := seen[r.ArtistID]; ok {
			continue
		}
		seen[r.ArtistID] = struct{}{}
		ids = append(ids, r.ArtistID)
	}
	return ids
}

func TestBuildLabelHubs_ThresholdBoundary(t *testing.T) {
	tests := []struct {
		name         string
		rows         []labelRosterRow
		wantHubs     int
		wantSpokes   int
		wantHubbedID uint
	}{
		{
			name:       "two-artist roster stays pairwise",
			rows:       []labelRosterRow{rosterRow(1, "Duo Records", 10), rosterRow(1, "Duo Records", 11)},
			wantHubs:   0,
			wantSpokes: 0,
		},
		{
			name: "three-artist roster becomes a hub",
			rows: []labelRosterRow{
				rosterRow(1, "Trio Records", 10),
				rosterRow(1, "Trio Records", 11),
				rosterRow(1, "Trio Records", 12),
			},
			wantHubs:     1,
			wantSpokes:   3,
			wantHubbedID: 1,
		},
		{
			name:       "single-artist roster is not a hub",
			rows:       []labelRosterRow{rosterRow(1, "Solo Records", 10)},
			wantHubs:   0,
			wantSpokes: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hubs, err := buildLabelHubs(tc.rows, artistIDsFromRows(tc.rows))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(hubs.Nodes) != tc.wantHubs {
				t.Errorf("hub nodes: got %d, want %d", len(hubs.Nodes), tc.wantHubs)
			}
			if len(hubs.Spokes) != tc.wantSpokes {
				t.Errorf("spokes: got %d, want %d", len(hubs.Spokes), tc.wantSpokes)
			}
			if tc.wantHubs > 0 {
				if _, ok := hubs.hubbedLabels[tc.wantHubbedID]; !ok {
					t.Errorf("label %d should be hubbed", tc.wantHubbedID)
				}
			}
		})
	}
}

// A 25-artist roster is the Austin 12XU shape: 300 pairwise edges become 25
// spokes. The saving is the whole point of the ticket, so pin the arithmetic.
func TestBuildLabelHubs_CollapsesCliqueToSpokes(t *testing.T) {
	const rosterSize = 25
	rows := make([]labelRosterRow, 0, rosterSize)
	for i := uint(0); i < rosterSize; i++ {
		rows = append(rows, rosterRow(7, "12XU", 100+i))
	}

	hubs, err := buildLabelHubs(rows, artistIDsFromRows(rows))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hubs.Nodes) != 1 {
		t.Fatalf("hub nodes: got %d, want 1", len(hubs.Nodes))
	}
	if len(hubs.Spokes) != rosterSize {
		t.Errorf("spokes: got %d, want %d", len(hubs.Spokes), rosterSize)
	}
	cliqueEdges := rosterSize * (rosterSize - 1) / 2
	if len(hubs.Spokes) >= cliqueEdges {
		t.Errorf("spokes (%d) should be far fewer than the clique it replaces (%d)", len(hubs.Spokes), cliqueEdges)
	}
}

func TestBuildLabelHubs_HubNodeShape(t *testing.T) {
	slug := "12xu"
	city, state, country := "Austin", "TX", "US"
	rows := []labelRosterRow{
		{LabelID: 7, Name: "12XU", Slug: &slug, City: &city, State: &state, Country: &country, ArtistID: 10},
		{LabelID: 7, Name: "12XU", Slug: &slug, City: &city, State: &state, Country: &country, ArtistID: 11},
		{LabelID: 7, Name: "12XU", Slug: &slug, City: &city, State: &state, Country: &country, ArtistID: 12},
	}

	hubs, err := buildLabelHubs(rows, artistIDsFromRows(rows))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hub := hubs.Nodes[0]

	if hub.EntityType != contracts.SceneNodeKindLabel {
		t.Errorf("entity_type: got %q, want %q", hub.EntityType, contracts.SceneNodeKindLabel)
	}
	if want := uint(labelHubNodeIDOffset + 7); hub.ID != want {
		t.Errorf("hub id: got %d, want %d (offset-namespaced)", hub.ID, want)
	}
	// A hub in a cluster would be pulled into that cluster's hull and inflate
	// its legend count; clusters describe where artists play.
	if hub.ClusterID != "" {
		t.Errorf("cluster_id: got %q, want empty", hub.ClusterID)
	}
	// A hub always carries its roster's spokes, so it must never be shelved as
	// not-yet-connected.
	if hub.IsIsolate {
		t.Error("hub should never be an isolate")
	}
	if hub.City != city || hub.State != state || hub.Country != country {
		t.Errorf("home caption fields: got %q/%q/%q, want %q/%q/%q",
			hub.City, hub.State, hub.Country, city, state, country)
	}

	for _, spoke := range hubs.Spokes {
		if spoke.Type != contracts.SceneEdgeTypeOnLabel {
			t.Errorf("spoke type: got %q, want %q", spoke.Type, contracts.SceneEdgeTypeOnLabel)
		}
		if spoke.SourceID != hub.ID {
			t.Errorf("spoke source: got %d, want hub %d", spoke.SourceID, hub.ID)
		}
		// Spokes bridge a cluster-less hub to artists in many clusters; the
		// weak cross-cluster strength would let the hub drift off its roster.
		if spoke.IsCrossCluster {
			t.Error("spokes must not be flagged cross-cluster")
		}
	}
}

func TestBuildLabelHubs_RefusesOffsetCollision(t *testing.T) {
	rows := []labelRosterRow{
		rosterRow(1, "Big Records", labelHubNodeIDOffset),
		rosterRow(1, "Big Records", labelHubNodeIDOffset+1),
		rosterRow(1, "Big Records", labelHubNodeIDOffset+2),
	}

	hubs, err := buildLabelHubs(rows, artistIDsFromRows(rows))
	if err == nil {
		t.Fatal("expected an error when an artist id reaches the hub offset")
	}
	// Fail closed: no hubs rather than node IDs that collide with artists.
	if len(hubs.Nodes) != 0 || len(hubs.Spokes) != 0 {
		t.Errorf("expected no hubs on collision, got %d nodes / %d spokes", len(hubs.Nodes), len(hubs.Spokes))
	}
	if hubs.replacesSharedLabelEdge(labelHubNodeIDOffset, labelHubNodeIDOffset+1) {
		t.Error("a refused build must not claim to replace any edge")
	}
}

func TestReplacesSharedLabelEdge(t *testing.T) {
	// Label 1 is hubbed (3 in-set artists). Label 2 is a 2-artist overlap
	// between artists 10 and 30, so it keeps its pairwise edge.
	rows := []labelRosterRow{
		rosterRow(1, "Hubbed Records", 10),
		rosterRow(1, "Hubbed Records", 11),
		rosterRow(1, "Hubbed Records", 12),
		rosterRow(2, "Small Records", 10),
		rosterRow(2, "Small Records", 30),
	}
	hubs, err := buildLabelHubs(rows, artistIDsFromRows(rows))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name           string
		source, target uint
		want           bool
	}{
		{"pair sharing only a hubbed label is replaced", 10, 11, true},
		{"pair sharing only a below-threshold label is kept", 10, 30, false},
		{"pair with no shared label is kept", 11, 30, false},
		{"unknown artist is kept", 10, 999, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := hubs.replacesSharedLabelEdge(tc.source, tc.target); got != tc.want {
				t.Errorf("replacesSharedLabelEdge(%d, %d): got %v, want %v", tc.source, tc.target, got, tc.want)
			}
		})
	}
}

// A pair on BOTH a hubbed and a below-threshold label must keep its edge: the
// small label has no hub to carry it, so dropping the edge would delete the
// only evidence of that connection.
func TestReplacesSharedLabelEdge_MixedHubbedAndSmallLabel(t *testing.T) {
	rows := []labelRosterRow{
		rosterRow(1, "Hubbed Records", 10),
		rosterRow(1, "Hubbed Records", 11),
		rosterRow(1, "Hubbed Records", 12),
		// Artists 10 and 11 also share label 2, which stays below threshold.
		rosterRow(2, "Small Records", 10),
		rosterRow(2, "Small Records", 11),
	}
	hubs, err := buildLabelHubs(rows, artistIDsFromRows(rows))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hubs.replacesSharedLabelEdge(10, 11) {
		t.Error("pair sharing an un-hubbed label too must keep its pairwise edge")
	}
	// The pair that shares only the hubbed label is still replaced.
	if !hubs.replacesSharedLabelEdge(11, 12) {
		t.Error("pair sharing only the hubbed label should be replaced")
	}
}

// Multi-label artists get one spoke per hub — the shape that guarantees no
// clique can reappear through a second roster.
func TestBuildLabelHubs_MultiLabelArtistGetsSpokePerHub(t *testing.T) {
	rows := []labelRosterRow{
		rosterRow(1, "Alpha Records", 10),
		rosterRow(1, "Alpha Records", 11),
		rosterRow(1, "Alpha Records", 12),
		rosterRow(2, "Beta Records", 10),
		rosterRow(2, "Beta Records", 20),
		rosterRow(2, "Beta Records", 21),
	}
	hubs, err := buildLabelHubs(rows, artistIDsFromRows(rows))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hubs.Nodes) != 2 {
		t.Fatalf("hub nodes: got %d, want 2", len(hubs.Nodes))
	}

	spokesForArtist10 := 0
	for _, s := range hubs.Spokes {
		if s.TargetID == 10 {
			spokesForArtist10++
		}
	}
	if spokesForArtist10 != 2 {
		t.Errorf("artist on two hubbed labels: got %d spokes, want 2", spokesForArtist10)
	}
}

func TestBuildLabelHubs_EmptyInput(t *testing.T) {
	hubs, err := buildLabelHubs(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hubs.Nodes) != 0 || len(hubs.Spokes) != 0 {
		t.Errorf("expected no hubs for empty input")
	}
	if hubs.replacesSharedLabelEdge(1, 2) {
		t.Error("no hubs means no edge is replaced")
	}
}

// A pair sharing a hubbed label AND a slug-less (un-hubbable) label must keep
// its pairwise edge: no hub represents the slug-less label, so dropping the
// edge would delete the only evidence of that relationship. Regression test for
// the roster query's original slug filter, which hid slug-less labels from the
// drop rule entirely.
func TestReplacesSharedLabelEdge_SlugLessLabelKeepsEdge(t *testing.T) {
	rows := []labelRosterRow{
		rosterRow(1, "Hubbed Records", 10),
		rosterRow(1, "Hubbed Records", 11),
		rosterRow(1, "Hubbed Records", 12),
		// Artists 10 and 11 also share an unlinkable label.
		sluglessRosterRow(2, "Unlinkable Records", 10),
		sluglessRosterRow(2, "Unlinkable Records", 11),
	}

	hubs, err := buildLabelHubs(rows, artistIDsFromRows(rows))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hubs.Nodes) != 1 {
		t.Fatalf("only the slugged label may hub: got %d hubs", len(hubs.Nodes))
	}
	if hubs.replacesSharedLabelEdge(10, 11) {
		t.Error("pair also sharing a slug-less label must keep its pairwise edge")
	}
	if !hubs.replacesSharedLabelEdge(11, 12) {
		t.Error("pair sharing only the hubbed label should still be replaced")
	}
}

// A slug-less label with a 3+ roster must not hub at all (it would promise a
// click it cannot honor), and its roster keeps pairwise edges.
func TestBuildLabelHubs_SlugLessLabelNeverHubs(t *testing.T) {
	rows := []labelRosterRow{
		sluglessRosterRow(5, "No Slug Records", 10),
		sluglessRosterRow(5, "No Slug Records", 11),
		sluglessRosterRow(5, "No Slug Records", 12),
	}
	hubs, err := buildLabelHubs(rows, artistIDsFromRows(rows))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hubs.Nodes) != 0 || len(hubs.Spokes) != 0 {
		t.Errorf("slug-less label must not hub: got %d nodes / %d spokes", len(hubs.Nodes), len(hubs.Spokes))
	}
	if hubs.replacesSharedLabelEdge(10, 11) {
		t.Error("with no hub, the pairwise edge must survive")
	}
}

// The offset guard must cover every artist in the set, not only those carrying
// label rows: an artist with no label memberships never reaches rosterRows, so
// a roster-only check would let hubs mint IDs in the colliding range.
func TestBuildLabelHubs_RefusesCollisionFromArtistWithoutLabels(t *testing.T) {
	rows := []labelRosterRow{
		rosterRow(1, "Normal Records", 10),
		rosterRow(1, "Normal Records", 11),
		rosterRow(1, "Normal Records", 12),
	}
	// This artist is in the payload but has no artist_labels rows.
	artistIDs := []uint{10, 11, 12, labelHubNodeIDOffset + 5}

	hubs, err := buildLabelHubs(rows, artistIDs)
	if err == nil {
		t.Fatal("expected refusal when any artist in the set reaches the offset")
	}
	if len(hubs.Nodes) != 0 || len(hubs.Spokes) != 0 {
		t.Errorf("expected no hubs on collision, got %d nodes / %d spokes", len(hubs.Nodes), len(hubs.Spokes))
	}
}

// =============================================================================
// GLOBAL SCOPE (PSY-1722)
// =============================================================================

// The catalog-wide shape the Map of the Scene overview needs: one builder call
// over the WHOLE artist set. 12XU's global roster is 59 artists — 1,711 pairwise
// `shared_label` edges — and it must arrive as 1 hub + 59 spokes, with a
// 2-artist label and a slug-less label in the same payload keeping theirs.
func TestBuildLabelHubs_GlobalScope(t *testing.T) {
	const globalRosterSize = 59

	var rows []labelRosterRow
	// Label 1: the catalog-wide 12XU roster.
	bigRoster := make([]uint, 0, globalRosterSize)
	for i := uint(0); i < globalRosterSize; i++ {
		artistID := 1000 + i
		bigRoster = append(bigRoster, artistID)
		rows = append(rows, rosterRow(1, "12XU", artistID))
	}
	// Label 2: the 2-artist control, on artists that touch no other label.
	rows = append(rows,
		rosterRow(2, "Duo Records", 2000),
		rosterRow(2, "Duo Records", 2001),
	)
	// Label 3: slug-less, 3 artists — over the threshold but unlinkable.
	rows = append(rows,
		sluglessRosterRow(3, "Unlinkable Records", 3000),
		sluglessRosterRow(3, "Unlinkable Records", 3001),
		sluglessRosterRow(3, "Unlinkable Records", 3002),
	)

	hubs, err := buildLabelHubs(rows, artistIDsFromRows(rows))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only the linkable 3+ roster hubs.
	if len(hubs.Nodes) != 1 {
		t.Fatalf("hub nodes: got %d, want 1 (only 12XU is both over-threshold and linkable)", len(hubs.Nodes))
	}
	if hubs.Nodes[0].Name != "12XU" {
		t.Errorf("hub name: got %q, want %q", hubs.Nodes[0].Name, "12XU")
	}
	if len(hubs.Spokes) != globalRosterSize {
		t.Errorf("spokes: got %d, want %d (one per global roster artist)", len(hubs.Spokes), globalRosterSize)
	}
	if cliqueEdges := globalRosterSize * (globalRosterSize - 1) / 2; cliqueEdges != 1711 {
		t.Fatalf("fixture drift: C(%d,2) = %d, want 1711", globalRosterSize, cliqueEdges)
	}

	// (a) Every pair in the hubbed roster loses its pairwise edge — all 1,711.
	replaced := 0
	for i := 0; i < len(bigRoster); i++ {
		for j := i + 1; j < len(bigRoster); j++ {
			if hubs.replacesSharedLabelEdge(bigRoster[i], bigRoster[j]) {
				replaced++
			}
		}
	}
	if want := globalRosterSize * (globalRosterSize - 1) / 2; replaced != want {
		t.Errorf("replaced pairwise edges: got %d, want %d", replaced, want)
	}

	// (b) The 2-artist control keeps its single pairwise edge.
	if hubs.replacesSharedLabelEdge(2000, 2001) {
		t.Error("a 2-artist label has no hub, so its pairwise edge must survive")
	}

	// (c) The slug-less roster keeps its pairwise edges.
	if hubs.replacesSharedLabelEdge(3000, 3001) {
		t.Error("a slug-less label has no hub, so its pairwise edges must survive")
	}
}

// The builder is bounded by the artist set, not by the breadth of the roster
// read: queryAllLabelRosters returns memberships for artists a payload may not
// contain, and counting those toward the threshold would mint a spoke pointing
// at a node that does not exist.
func TestBuildLabelHubs_IgnoresRosterRowsOutsideArtistSet(t *testing.T) {
	rows := []labelRosterRow{
		rosterRow(1, "Wide Records", 10),
		rosterRow(1, "Wide Records", 11),
		// Artists 12 and 13 are on the label but outside the payload.
		rosterRow(1, "Wide Records", 12),
		rosterRow(1, "Wide Records", 13),
	}

	hubs, err := buildLabelHubs(rows, []uint{10, 11})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hubs.Nodes) != 0 || len(hubs.Spokes) != 0 {
		t.Fatalf("only 2 of the 4 roster artists are in the set, so no hub: got %d nodes / %d spokes",
			len(hubs.Nodes), len(hubs.Spokes))
	}
	// The in-set pair keeps its pairwise edge — nothing is claiming to carry it.
	if hubs.replacesSharedLabelEdge(10, 11) {
		t.Error("without a hub, the in-set pair must keep its pairwise edge")
	}

	// Widening the artist set to the whole roster turns the same rows into a hub.
	wide, err := buildLabelHubs(rows, []uint{10, 11, 12, 13})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wide.Nodes) != 1 || len(wide.Spokes) != 4 {
		t.Errorf("widened set: got %d nodes / %d spokes, want 1 / 4", len(wide.Nodes), len(wide.Spokes))
	}
	for _, spoke := range wide.Spokes {
		if spoke.TargetID < 10 || spoke.TargetID > 13 {
			t.Errorf("spoke target %d is outside the artist set", spoke.TargetID)
		}
	}
}
