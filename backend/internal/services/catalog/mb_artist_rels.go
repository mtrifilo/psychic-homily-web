package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm/clause"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// PSY-1382 MusicBrainz artist-rels → graph edges.
//
// MB mapping (locked after live spike on Sonic Youth / Thurston Moore /
// Chelsea Wolfe / Dan Snaith / Burial):
//
//	"member of band" (type-id 5be4c609-9afa-4ea0-910b-12ffb71e3821)
//	  → RelationshipTypeMemberOf
//	"is person"      (type-id dd9886f2-1dfe-4270-97db-283f6839a666)
//	  → RelationshipTypeSideProject
//	    MusicBrainz has no true "side project" relation. "is person" links a
//	    legal name to a performance name (Dan Snaith ↔ Caribou / Daphni) and is
//	    the closest automated signal for alternate artist identities. True band
//	    side-projects (Chelsea Wolfe → Mrs. Piss) stay "member of band" → member_of.
//	All other artist-rels types (married, parent, sibling, collaboration,
//	supporting musician, …) are ignored.
//
// Include ended=true rows (fan-recognizable history). Same pair may appear many
// times (per-instrument attrs; ended+current duplicates) — MapMBArtistRels
// dedupes to one intent per (peerMBID, edgeType). Missing peers (no local
// musicbrainz_artist_id match) are skipped, never stubbed. Score is binary 1.0.

const (
	mbRelTypeMemberOfBand = "member of band"
	mbRelTypeIsPerson     = "is person"
)

// MBArtistRel is the catalog-local view of one MusicBrainz artist-rels row so
// this package need not import pipeline (PSY-1246 cycle constraint). The
// mbadapter package bridges pipeline.MusicBrainzClient → this type.
type MBArtistRel struct {
	Type       string
	TypeID     string
	Direction  string
	Ended      bool
	Attributes []string
	PeerMBID   string
	PeerName   string
	PeerType   string
}

// artistArtistRelsClient looks up MusicBrainz artist-artist relations for an MBID.
type artistArtistRelsClient interface {
	LookupArtistArtistRelations(ctx context.Context, mbid string) ([]MBArtistRel, error)
}

// SetArtistRelsClient injects the shared MusicBrainz artist-rels client
// (via mbadapter). A nil client makes DeriveMusicBrainzArtistRels a no-op
// that returns a zero result — tests and lean boots stay safe.
func (s *ArtistRelationshipService) SetArtistRelsClient(c artistArtistRelsClient) {
	s.mbRels = c
}

// mbArtistEdgeIntent is one deduped peer edge before local-ID resolution.
type mbArtistEdgeIntent struct {
	PeerMBID string
	RelType  string // catalogm.RelationshipTypeMemberOf | SideProject
	Ended    bool   // true if ANY contributing MB row had ended=true
}

// MapMBArtistRels converts MusicBrainz artist-rels into edge intents.
// Pure / unit-testable — no DB or HTTP.
func MapMBArtistRels(rels []MBArtistRel) []mbArtistEdgeIntent {
	seen := make(map[string]*mbArtistEdgeIntent, len(rels))
	order := make([]string, 0, len(rels))

	for _, r := range rels {
		relType := mapMBRelType(r.Type)
		if relType == "" || r.PeerMBID == "" {
			continue
		}
		key := r.PeerMBID + "\x00" + relType
		if existing, ok := seen[key]; ok {
			if r.Ended {
				existing.Ended = true
			}
			continue
		}
		intent := &mbArtistEdgeIntent{
			PeerMBID: r.PeerMBID,
			RelType:  relType,
			Ended:    r.Ended,
		}
		seen[key] = intent
		order = append(order, key)
	}

	out := make([]mbArtistEdgeIntent, 0, len(order))
	for _, key := range order {
		out = append(out, *seen[key])
	}
	return out
}

func mapMBRelType(mbType string) string {
	switch mbType {
	case mbRelTypeMemberOfBand:
		return catalogm.RelationshipTypeMemberOf
	case mbRelTypeIsPerson:
		return catalogm.RelationshipTypeSideProject
	default:
		return ""
	}
}

func mbDetailType(relType string) string {
	switch relType {
	case catalogm.RelationshipTypeMemberOf:
		return mbRelTypeMemberOfBand
	case catalogm.RelationshipTypeSideProject:
		return mbRelTypeIsPerson
	default:
		return ""
	}
}

// DeriveMusicBrainzArtistRels walks every artist with a musicbrainz_artist_id,
// fetches artist-rels, maps to member_of / side_project, and reconciles via
// upsertAndReconcileDerived (PSY-1332). Idempotent and re-runnable.
//
// Full rebuild each cycle (~1 req/s × N MBIDs). Acceptable on the 24h ticker.
// Ctx cancel stops between lookups.
func (s *ArtistRelationshipService) DeriveMusicBrainzArtistRels(ctx context.Context) (contracts.MusicBrainzArtistRelsResult, error) {
	return s.deriveMusicBrainzArtistRels(ctx, MusicBrainzArtistRelsOptions{})
}

// MusicBrainzArtistRelsOptions configures a derive/CLI run.
type MusicBrainzArtistRelsOptions struct {
	// Limit caps how many MBID artists we LOOK UP (0 = all). Peer resolution
	// still uses the full in-DB MBID map so edges to artists outside the limit
	// still resolve.
	Limit int
	// DryRun collects edges but skips upsertAndReconcileDerived.
	DryRun bool
}

// DeriveMusicBrainzArtistRelsWithOptions is the CLI entrypoint (limit / dry-run).
func (s *ArtistRelationshipService) DeriveMusicBrainzArtistRelsWithOptions(ctx context.Context, opts MusicBrainzArtistRelsOptions) (contracts.MusicBrainzArtistRelsResult, error) {
	return s.deriveMusicBrainzArtistRels(ctx, opts)
}

func (s *ArtistRelationshipService) deriveMusicBrainzArtistRels(ctx context.Context, opts MusicBrainzArtistRelsOptions) (contracts.MusicBrainzArtistRelsResult, error) {
	var result contracts.MusicBrainzArtistRelsResult
	if s.db == nil {
		return result, fmt.Errorf("database not initialized")
	}
	if s.mbRels == nil {
		slog.Warn("DeriveMusicBrainzArtistRels: no MusicBrainz client wired; skipping")
		return result, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	type mbidRow struct {
		ID                  uint   `gorm:"column:id"`
		MusicBrainzArtistID string `gorm:"column:musicbrainz_artist_id"`
	}
	var allWithMBID []mbidRow
	if err := s.db.Model(&catalogm.Artist{}).
		Select("id, musicbrainz_artist_id").
		Where("musicbrainz_artist_id IS NOT NULL AND musicbrainz_artist_id <> ''").
		Order("id ASC").
		Find(&allWithMBID).Error; err != nil {
		return result, fmt.Errorf("list artists with mbid: %w", err)
	}

	mbidToID := make(map[string]uint, len(allWithMBID))
	for _, a := range allWithMBID {
		mbidToID[a.MusicBrainzArtistID] = a.ID
	}

	artists := allWithMBID
	if opts.Limit > 0 && opts.Limit < len(artists) {
		artists = artists[:opts.Limit]
	}
	result.ArtistsScanned = len(artists)

	runStart := time.Now()
	memberRels := make([]catalogm.ArtistRelationship, 0)
	sideRels := make([]catalogm.ArtistRelationship, 0)
	memberSeen := make(map[[2]uint]struct{})
	sideSeen := make(map[[2]uint]struct{})

	for _, a := range artists {
		if err := ctx.Err(); err != nil {
			return result, err
		}

		rels, err := s.mbRels.LookupArtistArtistRelations(ctx, a.MusicBrainzArtistID)
		if err != nil {
			result.LookupsFailed++
			slog.Warn("musicbrainz artist-rels lookup failed",
				"artist_id", a.ID, "mbid", a.MusicBrainzArtistID, "error", err)
			continue
		}

		for _, intent := range MapMBArtistRels(rels) {
			peerID, ok := mbidToID[intent.PeerMBID]
			if !ok {
				result.PeersSkipped++
				continue
			}
			if peerID == a.ID {
				continue
			}
			src, tgt := catalogm.CanonicalOrder(a.ID, peerID)
			key := [2]uint{src, tgt}

			detail, _ := json.Marshal(map[string]interface{}{
				"mb_type": mbDetailType(intent.RelType),
				"ended":   intent.Ended,
			})
			detailRaw := json.RawMessage(detail)

			rel := catalogm.ArtistRelationship{
				SourceArtistID:   src,
				TargetArtistID:   tgt,
				RelationshipType: intent.RelType,
				Score:            1.0,
				AutoDerived:      true,
				Detail:           &detailRaw,
			}

			switch intent.RelType {
			case catalogm.RelationshipTypeMemberOf:
				if _, dup := memberSeen[key]; dup {
					continue
				}
				memberSeen[key] = struct{}{}
				memberRels = append(memberRels, rel)
			case catalogm.RelationshipTypeSideProject:
				if _, dup := sideSeen[key]; dup {
					continue
				}
				sideSeen[key] = struct{}{}
				sideRels = append(sideRels, rel)
			}
		}
	}

	if opts.DryRun {
		result.MemberOfUpserted = int64(len(memberRels))
		result.SideProjectUpserted = int64(len(sideRels))
		return result, nil
	}

	// Full PSY-1332 stale-reconcile only when we looked up every intended
	// artist successfully. A transient MB failure mid-run would otherwise
	// delete still-valid edges for artists we never re-fetched. Partial
	// (--limit) runs also upsert-only so they cannot wipe the rest of the graph.
	reconcile := opts.Limit <= 0 && result.LookupsFailed == 0
	if opts.Limit <= 0 && result.LookupsFailed > 0 {
		slog.Warn("musicbrainz artist-rels: skipping stale reconcile due to lookup failures; upserting only",
			"lookups_failed", result.LookupsFailed,
			"artists_scanned", result.ArtistsScanned,
		)
	}
	memberCount, err := s.persistDerived(catalogm.RelationshipTypeMemberOf, memberRels, runStart, reconcile)
	if err != nil {
		return result, fmt.Errorf("member_of persist: %w", err)
	}
	sideCount, err := s.persistDerived(catalogm.RelationshipTypeSideProject, sideRels, runStart, reconcile)
	if err != nil {
		return result, fmt.Errorf("side_project persist: %w", err)
	}
	result.MemberOfUpserted = memberCount
	result.SideProjectUpserted = sideCount
	return result, nil
}

// persistDerived upserts a derived set; when reconcile is true it also sweeps
// stale auto_derived rows of that type (PSY-1332).
func (s *ArtistRelationshipService) persistDerived(relType string, rels []catalogm.ArtistRelationship, runStart time.Time, reconcile bool) (int64, error) {
	if reconcile {
		return s.upsertAndReconcileDerived(relType, rels, runStart)
	}
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	if len(rels) == 0 {
		return 0, nil
	}
	for i := range rels {
		rels[i].CreatedAt = runStart
		rels[i].UpdatedAt = runStart
	}
	res := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "source_artist_id"}, {Name: "target_artist_id"}, {Name: "relationship_type"}},
		DoUpdates: clause.AssignmentColumns([]string{"score", "detail", "updated_at"}),
	}).CreateInBatches(&rels, derivedUpsertBatchSize)
	if res.Error != nil {
		return 0, fmt.Errorf("%s batch upsert: %w", relType, res.Error)
	}
	return res.RowsAffected, nil
}
