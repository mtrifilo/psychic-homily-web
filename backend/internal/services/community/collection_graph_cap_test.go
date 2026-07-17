package community

// Unit coverage for the collection-graph node cap (PSY-1475). The cap is the
// mechanism that bounds the frontend's synchronous warmup cost, so its
// ranking semantics are pinned here as pure-function tests (no DB); the
// end-to-end wiring — NodeTotal / NodesTruncated disclosure included — is
// covered by CollectionServiceIntegrationTestSuite in collection_graph_test.go.
// Mirrors the venue bill network's cap tests (PSY-1461).

import (
	"testing"

	"psychic-homily-backend/internal/services/contracts"
)

// TestCapCollectionGraphNodesUnderCapIsNoop: node lists at or under the
// ceiling pass through untouched — same slices, no link dropped.
func TestCapCollectionGraphNodesUnderCapIsNoop(t *testing.T) {
	nodes := []contracts.CollectionGraphNode{
		{ID: 1, EntityType: "artist", Name: "A"},
		{ID: 2, EntityType: "venue", Name: "V"},
	}
	links := []contracts.CollectionGraphLink{
		{SourceID: 1, TargetID: 2, Type: CollectionEdgePlayedAt},
	}

	gotNodes, gotLinks := capCollectionGraphNodes(nodes, links)

	if len(gotNodes) != 2 {
		t.Fatalf("expected 2 nodes untouched, got %d", len(gotNodes))
	}
	if len(gotLinks) != 1 {
		t.Fatalf("expected 1 link untouched, got %d", len(gotLinks))
	}
}

// TestCapCollectionGraphNodesDegreeRanking: over-cap graphs keep the
// highest-degree nodes across entity types, drop the lowest-degree ones
// (isolates first), and preserve the original response ordering among the
// survivors.
func TestCapCollectionGraphNodesDegreeRanking(t *testing.T) {
	// Hub pair (degree 2 each: linked to each other twice via two edge
	// types) + a connected artist↔venue pair (degree 1 each), then enough
	// isolates to push the total 2 over the cap.
	nodes := []contracts.CollectionGraphNode{
		{ID: 1, EntityType: "artist", Name: "Hub A"},
		{ID: 2, EntityType: "artist", Name: "Hub B"},
		{ID: 3, EntityType: "artist", Name: "Edge Artist"},
		{ID: 4, EntityType: "venue", Name: "Edge Venue"},
	}
	for id := uint(5); id <= uint(collectionGraphMaxNodes)+2; id++ {
		nodes = append(nodes, contracts.CollectionGraphNode{
			ID: id, EntityType: "release", IsIsolate: true,
		})
	}
	links := []contracts.CollectionGraphLink{
		{SourceID: 1, TargetID: 2, Type: "shared_bills"},
		{SourceID: 1, TargetID: 2, Type: "shared_label"},
		{SourceID: 3, TargetID: 4, Type: CollectionEdgePlayedAt},
	}

	gotNodes, gotLinks := capCollectionGraphNodes(nodes, links)

	if len(gotNodes) != collectionGraphMaxNodes {
		t.Fatalf("expected %d nodes after cap, got %d", collectionGraphMaxNodes, len(gotNodes))
	}
	got := make(map[uint]bool, len(gotNodes))
	for _, n := range gotNodes {
		got[n.ID] = true
	}
	for _, id := range []uint{1, 2, 3, 4} {
		if !got[id] {
			t.Errorf("connected node %d should survive the cap", id)
		}
	}
	// Ties among the isolates keep original order → the LAST two isolates
	// are the ones dropped.
	for id := uint(collectionGraphMaxNodes) + 1; id <= uint(collectionGraphMaxNodes)+2; id++ {
		if got[id] {
			t.Errorf("trailing isolate %d should be dropped by the cap", id)
		}
	}
	if len(gotLinks) != 3 {
		t.Fatalf("expected all 3 links between kept nodes to survive, got %d", len(gotLinks))
	}
	// Survivors preserve the original response ordering.
	for i := 1; i < len(gotNodes); i++ {
		if gotNodes[i].ID < gotNodes[i-1].ID {
			t.Fatalf("capped nodes must preserve original order; saw %d after %d",
				gotNodes[i].ID, gotNodes[i-1].ID)
		}
	}
}

// TestCapCollectionGraphNodesDropsDanglingLinks: when a dropped node had an
// edge to a kept node, the link is scrubbed — the payload must never ship a
// link pointing at a node that isn't there. The kept partner is left for the
// caller to re-mark as an isolate.
func TestCapCollectionGraphNodesDropsDanglingLinks(t *testing.T) {
	// Shape: node 1 is a hub linked to the two trailing nodes (degree 2);
	// the middle nodes form degree-1 pairs, except one leftover isolate
	// (degree 0). Drops: the isolate (lowest degree) plus — via the
	// original-order tiebreak among the degree-1 nodes — the LAST trailing
	// node. Its hub link becomes dangling and must be scrubbed, while the
	// hub's link to the other trailing node (kept) survives.
	total := collectionGraphMaxNodes + 2
	nodes := make([]contracts.CollectionGraphNode, 0, total)
	for id := uint(1); id <= uint(total); id++ {
		nodes = append(nodes, contracts.CollectionGraphNode{ID: id, EntityType: "artist"})
	}
	var links []contracts.CollectionGraphLink
	// Hub links: 1 ↔ total-1 (partner kept), 1 ↔ total (partner dropped).
	links = append(links,
		contracts.CollectionGraphLink{SourceID: 1, TargetID: uint(total - 1), Type: "similar"},
		contracts.CollectionGraphLink{SourceID: 1, TargetID: uint(total), Type: "similar"},
	)
	// Middle pairs among 2..total-2; the last unpaired ID is the isolate.
	pairCount := 0
	var isolateID uint
	for a := uint(2); a <= uint(total-2); a += 2 {
		if a+1 > uint(total-2) {
			isolateID = a
			break
		}
		links = append(links, contracts.CollectionGraphLink{
			SourceID: a, TargetID: a + 1, Type: "shared_bills",
		})
		pairCount++
	}
	if isolateID == 0 {
		t.Fatal("test shape requires an odd middle-node count producing one isolate")
	}

	gotNodes, gotLinks := capCollectionGraphNodes(nodes, links)

	if len(gotNodes) != collectionGraphMaxNodes {
		t.Fatalf("expected %d nodes after cap, got %d", collectionGraphMaxNodes, len(gotNodes))
	}
	kept := make(map[uint]bool, len(gotNodes))
	for _, n := range gotNodes {
		kept[n.ID] = true
	}
	if kept[uint(total)] {
		t.Fatalf("node %d should be dropped by the original-order tiebreak", total)
	}
	if kept[isolateID] {
		t.Fatalf("isolate %d should be dropped first (degree 0)", isolateID)
	}
	if !kept[uint(total-1)] {
		t.Fatalf("node %d should be kept (degree 1, earlier in tiebreak order)", total-1)
	}
	for _, l := range gotLinks {
		if !kept[l.SourceID] || !kept[l.TargetID] {
			t.Errorf("link %d→%d references a dropped node — dangling links must be scrubbed",
				l.SourceID, l.TargetID)
		}
	}
	// Survivors: the hub↔(total-1) link + every middle pair. The
	// hub↔(total) link is scrubbed with its dropped endpoint.
	if want := pairCount + 1; len(gotLinks) != want {
		t.Fatalf("expected %d links after cap, got %d", want, len(gotLinks))
	}
}
