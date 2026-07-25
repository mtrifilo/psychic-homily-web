package catalog

import (
	"testing"

	"psychic-homily-backend/internal/services/contracts"
)

// rosterRow is a terse constructor for membership fixtures.
func rosterRow(labelID uint, name string, artistID uint) sceneLabelRosterRow {
	slug := name + "-slug"
	return sceneLabelRosterRow{
		LabelID:  labelID,
		Name:     name,
		Slug:     &slug,
		ArtistID: artistID,
	}
}

func TestBuildSceneLabelHubs_ThresholdBoundary(t *testing.T) {
	tests := []struct {
		name         string
		rows         []sceneLabelRosterRow
		wantHubs     int
		wantSpokes   int
		wantHubbedID uint
	}{
		{
			name:       "two-artist roster stays pairwise",
			rows:       []sceneLabelRosterRow{rosterRow(1, "Duo Records", 10), rosterRow(1, "Duo Records", 11)},
			wantHubs:   0,
			wantSpokes: 0,
		},
		{
			name: "three-artist roster becomes a hub",
			rows: []sceneLabelRosterRow{
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
			rows:       []sceneLabelRosterRow{rosterRow(1, "Solo Records", 10)},
			wantHubs:   0,
			wantSpokes: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hubs, err := buildSceneLabelHubs(tc.rows)
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
func TestBuildSceneLabelHubs_CollapsesCliqueToSpokes(t *testing.T) {
	const rosterSize = 25
	rows := make([]sceneLabelRosterRow, 0, rosterSize)
	for i := uint(0); i < rosterSize; i++ {
		rows = append(rows, rosterRow(7, "12XU", 100+i))
	}

	hubs, err := buildSceneLabelHubs(rows)
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

func TestBuildSceneLabelHubs_HubNodeShape(t *testing.T) {
	slug := "12xu"
	city, state, country := "Austin", "TX", "US"
	rows := []sceneLabelRosterRow{
		{LabelID: 7, Name: "12XU", Slug: &slug, City: &city, State: &state, Country: &country, ArtistID: 10},
		{LabelID: 7, Name: "12XU", Slug: &slug, City: &city, State: &state, Country: &country, ArtistID: 11},
		{LabelID: 7, Name: "12XU", Slug: &slug, City: &city, State: &state, Country: &country, ArtistID: 12},
	}

	hubs, err := buildSceneLabelHubs(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hub := hubs.Nodes[0]

	if hub.EntityType != contracts.SceneNodeKindLabel {
		t.Errorf("entity_type: got %q, want %q", hub.EntityType, contracts.SceneNodeKindLabel)
	}
	if want := uint(sceneLabelNodeIDOffset + 7); hub.ID != want {
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

func TestBuildSceneLabelHubs_RefusesOffsetCollision(t *testing.T) {
	rows := []sceneLabelRosterRow{
		rosterRow(1, "Big Records", sceneLabelNodeIDOffset),
		rosterRow(1, "Big Records", sceneLabelNodeIDOffset+1),
		rosterRow(1, "Big Records", sceneLabelNodeIDOffset+2),
	}

	hubs, err := buildSceneLabelHubs(rows)
	if err == nil {
		t.Fatal("expected an error when an artist id reaches the hub offset")
	}
	// Fail closed: no hubs rather than node IDs that collide with artists.
	if len(hubs.Nodes) != 0 || len(hubs.Spokes) != 0 {
		t.Errorf("expected no hubs on collision, got %d nodes / %d spokes", len(hubs.Nodes), len(hubs.Spokes))
	}
	if hubs.replacesSharedLabelEdge(sceneLabelNodeIDOffset, sceneLabelNodeIDOffset+1) {
		t.Error("a refused build must not claim to replace any edge")
	}
}

func TestReplacesSharedLabelEdge(t *testing.T) {
	// Label 1 is hubbed (3 in-scene artists). Label 2 is a 2-artist overlap
	// between artists 10 and 30, so it keeps its pairwise edge.
	rows := []sceneLabelRosterRow{
		rosterRow(1, "Hubbed Records", 10),
		rosterRow(1, "Hubbed Records", 11),
		rosterRow(1, "Hubbed Records", 12),
		rosterRow(2, "Small Records", 10),
		rosterRow(2, "Small Records", 30),
	}
	hubs, err := buildSceneLabelHubs(rows)
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
	rows := []sceneLabelRosterRow{
		rosterRow(1, "Hubbed Records", 10),
		rosterRow(1, "Hubbed Records", 11),
		rosterRow(1, "Hubbed Records", 12),
		// Artists 10 and 11 also share label 2, which stays below threshold.
		rosterRow(2, "Small Records", 10),
		rosterRow(2, "Small Records", 11),
	}
	hubs, err := buildSceneLabelHubs(rows)
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
func TestBuildSceneLabelHubs_MultiLabelArtistGetsSpokePerHub(t *testing.T) {
	rows := []sceneLabelRosterRow{
		rosterRow(1, "Alpha Records", 10),
		rosterRow(1, "Alpha Records", 11),
		rosterRow(1, "Alpha Records", 12),
		rosterRow(2, "Beta Records", 10),
		rosterRow(2, "Beta Records", 20),
		rosterRow(2, "Beta Records", 21),
	}
	hubs, err := buildSceneLabelHubs(rows)
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

func TestBuildSceneLabelHubs_EmptyInput(t *testing.T) {
	hubs, err := buildSceneLabelHubs(nil)
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

func TestSceneEdgeTypesInclude(t *testing.T) {
	resolved := []string{"member_of", "shared_bills", "shared_label"}
	if !sceneEdgeTypesInclude(resolved, "shared_label") {
		t.Error("shared_label should be found")
	}
	if sceneEdgeTypesInclude(resolved, "side_project") {
		t.Error("side_project should not be found")
	}
	if sceneEdgeTypesInclude(nil, "shared_label") {
		t.Error("empty set contains nothing")
	}
}
