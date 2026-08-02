package catalog

import (
	"fmt"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/services/contracts"
)

// ──────────────────────────────────────────────
// Label hubs
// ──────────────────────────────────────────────
//
// `DeriveSharedLabels` stores one relationship row per artist PAIR, so a label
// with n roster artists inside the set being drawn arrives as a complete clique
// of C(n,2) edges. Measured on the live Austin scene: 300 of 302 edges were a
// single label (12XU, 25 in-scene artists) at ~0.017 normalized weight each —
// the force layout collapses that into an unreadable ball and label
// collision-culling leaves 3 of 25 names visible. The same shape appears on NYC
// (104/106) and LA (76/83), and it worsens as the artist set widens: 12XU's
// roster across the whole catalog is 59 artists, or 1,711 pairwise edges,
// carrying one fact.
//
// "n artists are on this label" is ONE fact, so it is encoded as one hub node
// with n membership spokes instead of C(n,2) pairwise similarity claims.
// Rosters below labelHubMinRoster keep their plain pairwise edge: a hub node for
// a 2-artist overlap would add a node to say strictly less.
//
// Hub membership is derived from the `artist_labels` fact table, never from the
// stored edge's `detail.label_names` (a STRING_AGG of names, which cannot be
// mapped back to label IDs — same live-query discipline as the relationship
// provenance endpoint).
//
// SCOPE: the builder is scope-agnostic. It takes the artist set a payload will
// contain plus that set's membership rows, and every rule is evaluated against
// artists IN THAT SET — GetSceneGraph passes one metro's roster, a catalog-wide
// consumer passes the whole artist set. Roster size is deliberately about what
// the payload can DRAW, not the label's worldwide roster: a label with 40
// artists in the catalog and 2 in this metro is a 2-artist overlap in a scene
// graph, and a hub only in a catalog-wide one.

const (
	// labelHubMinRoster is the in-set roster size at which a label collapses
	// into a hub. 3 is the smallest clique (C(3,2) = 3 edges), so every label
	// clique the payload can contain is covered.
	labelHubMinRoster = 3

	// labelHubNodeIDOffset namespaces label hub node IDs away from artist IDs,
	// which share the response's single numeric node-ID space (the frontend
	// graph stack keys nodes, label styles, and focus state by number). Artist
	// IDs are a serial sequence in the low thousands, so the offset is
	// unreachable in practice — and buildLabelHubs refuses to emit hubs at all
	// if that ever stops being true, rather than minting a node ID that
	// collides with an artist.
	labelHubNodeIDOffset = 2_000_000_000
)

// labelHubs is one hub build: the nodes and spokes to append to a payload, plus
// the lookups its caller needs to drop the pairwise `shared_label` edges the
// hubs replace.
type labelHubs struct {
	// Nodes are the emitted hub nodes, in rosterRows order.
	Nodes []contracts.SceneGraphNode
	// Spokes are the membership edges, one per (hub, in-set roster artist).
	Spokes []contracts.SceneGraphLink
	// labelsByArtist is every in-set artist's full label set — the input to
	// the replaced-edge rule, which needs labels that did NOT become hubs too.
	labelsByArtist map[uint]map[uint]struct{}
	// hubbedLabels is the set of label IDs that became hubs.
	hubbedLabels map[uint]struct{}
}

// labelRosterRow is one (label, artist) membership row.
type labelRosterRow struct {
	LabelID  uint    `gorm:"column:label_id"`
	Name     string  `gorm:"column:name"`
	Slug     *string `gorm:"column:slug"`
	City     *string `gorm:"column:city"`
	State    *string `gorm:"column:state"`
	Country  *string `gorm:"column:country"`
	ArtistID uint    `gorm:"column:artist_id"`
}

// queryLabelRosters loads the membership rows for an artist set. The ordering
// is part of the contract: buildLabelHubs emits hubs in first-seen label order,
// so a stable SQL order is what makes node and link ordering stable across
// requests.
//
// The read is bounded by the artist set at every scope. A catalog-wide consumer
// passes every artist it will draw rather than dropping the bound, because the
// builder can only count and spoke artists the payload contains — an unbounded
// read would fetch rows it must then discard. buildLabelHubs discards them
// safely either way, so a consumer that measures the bounded read to be the
// worse plan can swap in a wider one without changing the rules.
//
// Slug-less labels ARE selected, even though they can never become hubs (a
// hub's payoff is opening the label's page, and an unlinkable hub would occupy
// the canvas promising a click it cannot honor). They must stay visible to
// replacesSharedLabelEdge: DeriveSharedLabels derives edges from all of
// artist_labels, so a pair sharing an un-hubbable label needs its pairwise edge
// kept — filtering them out here would silently delete the only evidence of that
// relationship, the exact outcome the below-threshold keep-rule exists to
// prevent.
func queryLabelRosters(db *gorm.DB, artistIDs []uint) ([]labelRosterRow, error) {
	if len(artistIDs) == 0 {
		return nil, nil
	}
	var rows []labelRosterRow
	err := db.Table("artist_labels AS al").
		Select("l.id AS label_id, l.name, l.slug, l.city, l.state, l.country, al.artist_id").
		Joins("JOIN labels l ON l.id = al.label_id").
		Where("al.artist_id IN ?", artistIDs).
		Order("l.name ASC, l.id ASC, al.artist_id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query label rosters: %w", err)
	}
	return rows, nil
}

// buildLabelHubs groups membership rows into hubs for artistIDs — the artist set
// the payload will contain.
//
// Callers must pass their FULL artist set, which does two jobs:
//
//  1. Every rule is evaluated in-set. Roster rows for artists outside the set
//     are ignored, so a caller whose roster read is broader than its node set
//     can neither count an absent artist toward the threshold nor mint a spoke
//     pointing at a node the payload lacks.
//  2. The offset collision guard covers the whole set, not just artists
//     carrying label rows: an artist with no memberships never reaches
//     rosterRows, so a roster-only check would let hubs mint IDs in the
//     colliding range while claiming to refuse.
//
// Two conditions return an error rather than degrading silently, because both
// would otherwise produce a plausible-looking empty or colliding build:
// membership rows with no artist set at all (job 1 would discard every row and
// report "no hubs here", which is indistinguishable from a scope that genuinely
// has none), and an artist ID that has reached labelHubNodeIDOffset (past that
// point a hub node ID could collide with an artist node ID and the canvas would
// merge two unrelated entities). The returned zero value fails closed:
// replacesSharedLabelEdge guards on an empty hubbedLabels set, so a refused
// build claims no edge as replaced.
//
// rosterRows must be ordered deterministically by the caller (label name, then
// artist ID) so node and link ordering is stable across requests.
func buildLabelHubs(rosterRows []labelRosterRow, artistIDs []uint) (labelHubs, error) {
	out := labelHubs{
		labelsByArtist: make(map[uint]map[uint]struct{}),
		hubbedLabels:   make(map[uint]struct{}),
	}

	if len(rosterRows) > 0 && len(artistIDs) == 0 {
		return labelHubs{}, fmt.Errorf(
			"buildLabelHubs got %d roster rows with an empty artist set; the caller must pass the artist set its payload contains",
			len(rosterRows),
		)
	}

	inSet := make(map[uint]struct{}, len(artistIDs))
	for _, id := range artistIDs {
		if id >= labelHubNodeIDOffset {
			return labelHubs{}, fmt.Errorf(
				"artist id %d has reached the label-hub node id offset %d; refusing to emit label hubs",
				id, labelHubNodeIDOffset,
			)
		}
		inSet[id] = struct{}{}
	}

	if len(rosterRows) == 0 {
		return out, nil
	}

	// Roster accumulation preserves first-seen label order so the emitted hub
	// nodes inherit the caller's deterministic ordering. Both collections are
	// keyed by LABEL, so they are left unsized rather than hinted at the row
	// count: a catalog-wide build reads every membership row to find a few
	// hundred labels.
	type rosterEntry struct {
		row     labelRosterRow
		artists []uint
	}
	var order []uint
	byLabel := make(map[uint]*rosterEntry)

	for _, r := range rosterRows {
		if _, inPayload := inSet[r.ArtistID]; !inPayload {
			continue
		}

		if out.labelsByArtist[r.ArtistID] == nil {
			out.labelsByArtist[r.ArtistID] = make(map[uint]struct{})
		}
		out.labelsByArtist[r.ArtistID][r.LabelID] = struct{}{}

		entry, ok := byLabel[r.LabelID]
		if !ok {
			entry = &rosterEntry{row: r}
			byLabel[r.LabelID] = entry
			order = append(order, r.LabelID)
		}
		entry.artists = append(entry.artists, r.ArtistID)
	}

	for _, labelID := range order {
		entry := byLabel[labelID]
		if len(entry.artists) < labelHubMinRoster {
			continue
		}
		// An unlinkable label cannot be a hub — its panel's payoff is opening
		// the label page. Its roster keeps the pairwise edges instead, and it
		// stays in labelsByArtist so the drop rule still sees it.
		if derefString(entry.row.Slug) == "" {
			continue
		}
		out.hubbedLabels[labelID] = struct{}{}

		hubNodeID := labelHubNodeIDOffset + labelID
		out.Nodes = append(out.Nodes, contracts.SceneGraphNode{
			ID:         hubNodeID,
			EntityType: contracts.SceneNodeKindLabel,
			Name:       entry.row.Name,
			Slug:       derefString(entry.row.Slug),
			City:       derefString(entry.row.City),
			State:      derefString(entry.row.State),
			Country:    derefString(entry.row.Country),
			// A hub belongs to no venue/community cluster: clusters describe
			// where ARTISTS play, and a label has no primary venue. An empty
			// cluster ID keeps it out of both the hull geometry and the
			// cluster legend's counts.
			ClusterID: "",
			// A hub always has its roster's spokes, so it is never an isolate
			// and never lands on the not-yet-connected shelf.
			IsIsolate: false,
			// Label pages are not per-node audio surfaces; the hub's panel
			// links out rather than embedding a player.
			HasPlayableAudio: false,
		})

		for _, artistID := range entry.artists {
			out.Spokes = append(out.Spokes, contracts.SceneGraphLink{
				SourceID: hubNodeID,
				TargetID: artistID,
				Type:     contracts.SceneEdgeTypeOnLabel,
				// Membership is binary — every roster artist is equally "on"
				// the label — so the score is uniform and the renderer draws
				// spokes at one weight instead of implying a magnitude.
				Score: 1,
				Detail: map[string]any{
					"label_name":  entry.row.Name,
					"roster_size": len(entry.artists),
				},
				// Spokes connect a cluster-less hub to artists in many
				// clusters; flagging them cross-cluster would hand every spoke
				// the weak cross-cluster link strength and let the hub drift
				// away from the roster it anchors.
				IsCrossCluster: false,
			})
		}
	}

	return out, nil
}

// replacesSharedLabelEdge reports whether a stored pairwise `shared_label`
// edge is fully represented by the hubs and can be dropped.
//
// The pair is dropped only when EVERY label the two artists share became a
// hub. A pair that also shares a below-threshold label keeps its edge, because
// that label has no hub to carry the connection — dropping it would silently
// delete the only evidence of a real relationship.
func (h labelHubs) replacesSharedLabelEdge(sourceArtistID, targetArtistID uint) bool {
	if len(h.hubbedLabels) == 0 {
		return false
	}
	sourceLabels, ok := h.labelsByArtist[sourceArtistID]
	if !ok {
		return false
	}
	targetLabels, ok := h.labelsByArtist[targetArtistID]
	if !ok {
		return false
	}

	shared := 0
	for labelID := range sourceLabels {
		if _, both := targetLabels[labelID]; !both {
			continue
		}
		shared++
		if _, hubbed := h.hubbedLabels[labelID]; !hubbed {
			return false
		}
	}
	// No shared label at all means this edge predates the current membership
	// data (a label was unlinked since the last derive run). Leave it alone —
	// no hub is claiming to represent it.
	return shared > 0
}
