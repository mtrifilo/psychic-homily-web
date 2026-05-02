package catalog

import (
	"fmt"
	"math"
	"sort"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// FestivalIntelligenceService computes lineup overlap, breakout tracking,
// and series comparison from existing festival data.
type FestivalIntelligenceService struct {
	db *gorm.DB
}

// NewFestivalIntelligenceService creates a new festival intelligence service.
func NewFestivalIntelligenceService(database *gorm.DB) *FestivalIntelligenceService {
	if database == nil {
		database = db.GetDB()
	}
	return &FestivalIntelligenceService{db: database}
}

// tierWeight returns the similarity weight for a billing tier.
func tierWeight(tier string) float64 {
	switch tier {
	case "headliner":
		return 5.0
	case "sub_headliner":
		return 3.0
	case "mid_card":
		return 2.0
	case "undercard":
		return 1.0
	case "local":
		return 0.5
	case "dj":
		return 0.5
	case "host":
		return 0.25
	default:
		return 1.0
	}
}

// tierRank returns a numeric rank for a billing tier (lower = more prominent).
func tierRank(tier string) int {
	switch tier {
	case "headliner":
		return 1
	case "sub_headliner":
		return 2
	case "mid_card":
		return 3
	case "undercard":
		return 4
	case "local":
		return 5
	case "dj":
		return 6
	case "host":
		return 7
	default:
		return 8
	}
}

// rankToTier converts a numeric rank back to a tier string.
func rankToTier(rank int) string {
	switch rank {
	case 1:
		return "headliner"
	case 2:
		return "sub_headliner"
	case 3:
		return "mid_card"
	case 4:
		return "undercard"
	case 5:
		return "local"
	case 6:
		return "dj"
	case 7:
		return "host"
	default:
		return "unknown"
	}
}

// festivalArtistRow is an internal struct for query scanning.
type festivalArtistRow struct {
	FestivalID  uint
	ArtistID    uint
	BillingTier string
}

// artistInfo holds artist lookup data.
type artistInfo struct {
	ID   uint
	Name string
	Slug string
}

// festivalInfo holds festival lookup data.
type festivalInfo struct {
	ID          uint
	Name        string
	Slug        string
	SeriesSlug  string
	EditionYear int
	City        *string
	State       *string
	StartDate   string
}

// ──────────────────────────────────────────────
// GetSimilarFestivals
// ──────────────────────────────────────────────

// GetSimilarFestivals computes Jaccard and tier-weighted similarity for a festival.
func (s *FestivalIntelligenceService) GetSimilarFestivals(festivalID uint, limit int) ([]contracts.SimilarFestival, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 10
	}

	// Verify festival exists
	var festival catalogm.Festival
	if err := s.db.First(&festival, festivalID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("festival not found")
		}
		return nil, fmt.Errorf("failed to get festival: %w", err)
	}

	// Get all festival_artists for the source festival
	var sourceArtists []festivalArtistRow
	if err := s.db.Table("festival_artists").
		Select("festival_id, artist_id, billing_tier").
		Where("festival_id = ?", festivalID).
		Scan(&sourceArtists).Error; err != nil {
		return nil, fmt.Errorf("failed to get source artists: %w", err)
	}

	if len(sourceArtists) == 0 {
		return []contracts.SimilarFestival{}, nil
	}

	sourceArtistIDs := make(map[uint]string) // artistID -> tier
	for _, sa := range sourceArtists {
		sourceArtistIDs[sa.ArtistID] = sa.BillingTier
	}

	// Get all festival_artists for other festivals that share at least one artist
	artistIDs := make([]uint, 0, len(sourceArtistIDs))
	for id := range sourceArtistIDs {
		artistIDs = append(artistIDs, id)
	}

	var otherArtists []festivalArtistRow
	if err := s.db.Table("festival_artists").
		Select("festival_id, artist_id, billing_tier").
		Where("festival_id != ? AND artist_id IN ?", festivalID, artistIDs).
		Scan(&otherArtists).Error; err != nil {
		return nil, fmt.Errorf("failed to get other festival artists: %w", err)
	}

	if len(otherArtists) == 0 {
		return []contracts.SimilarFestival{}, nil
	}

	// Group by festival: compute shared artists, tiers
	type festivalOverlap struct {
		sharedArtists map[uint]string // artistID -> tier at target
		totalArtists  int
	}

	festivalOverlaps := make(map[uint]*festivalOverlap)
	for _, oa := range otherArtists {
		fo, ok := festivalOverlaps[oa.FestivalID]
		if !ok {
			fo = &festivalOverlap{sharedArtists: make(map[uint]string)}
			festivalOverlaps[oa.FestivalID] = fo
		}
		fo.sharedArtists[oa.ArtistID] = oa.BillingTier
	}

	// Get total artist counts for each candidate festival
	candidateFestivalIDs := make([]uint, 0, len(festivalOverlaps))
	for fid := range festivalOverlaps {
		candidateFestivalIDs = append(candidateFestivalIDs, fid)
	}

	type countResult struct {
		FestivalID uint
		Count      int
	}
	var totalCounts []countResult
	if err := s.db.Table("festival_artists").
		Select("festival_id, COUNT(DISTINCT artist_id) as count").
		Where("festival_id IN ?", candidateFestivalIDs).
		Group("festival_id").
		Scan(&totalCounts).Error; err != nil {
		return nil, fmt.Errorf("failed to count artists: %w", err)
	}

	for _, tc := range totalCounts {
		if fo, ok := festivalOverlaps[tc.FestivalID]; ok {
			fo.totalArtists = tc.Count
		}
	}

	// Compute similarity scores and filter by minimum threshold (3+ shared)
	type scoredFestival struct {
		festivalID    uint
		sharedCount   int
		jaccard       float64
		weightedScore float64
		sharedMap     map[uint]string
	}

	var scored []scoredFestival
	sourceCount := len(sourceArtistIDs)

	for fid, fo := range festivalOverlaps {
		sharedCount := len(fo.sharedArtists)
		if sharedCount < 3 {
			continue
		}

		unionCount := sourceCount + fo.totalArtists - sharedCount
		jaccard := 0.0
		if unionCount > 0 {
			jaccard = float64(sharedCount) / float64(unionCount)
		}

		// Compute weighted score using max(source tier, target tier) weight
		var weightedScore float64
		for artistID, targetTier := range fo.sharedArtists {
			sourceTier := sourceArtistIDs[artistID]
			sw := tierWeight(sourceTier)
			tw := tierWeight(targetTier)
			if tw > sw {
				weightedScore += tw
			} else {
				weightedScore += sw
			}
		}

		scored = append(scored, scoredFestival{
			festivalID:    fid,
			sharedCount:   sharedCount,
			jaccard:       math.Round(jaccard*10000) / 10000,
			weightedScore: math.Round(weightedScore*100) / 100,
			sharedMap:     fo.sharedArtists,
		})
	}

	if len(scored) == 0 {
		return []contracts.SimilarFestival{}, nil
	}

	// Sort by weighted score DESC, then Jaccard DESC
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].weightedScore != scored[j].weightedScore {
			return scored[i].weightedScore > scored[j].weightedScore
		}
		return scored[i].jaccard > scored[j].jaccard
	})

	// Limit results
	if len(scored) > limit {
		scored = scored[:limit]
	}

	// Load festival details
	resultFestivalIDs := make([]uint, len(scored))
	for i, sf := range scored {
		resultFestivalIDs[i] = sf.festivalID
	}

	festivalMap := s.loadFestivalInfoMap(resultFestivalIDs)

	// Load artist details for shared artists
	allSharedArtistIDs := make(map[uint]bool)
	for _, sf := range scored {
		for aid := range sf.sharedMap {
			allSharedArtistIDs[aid] = true
		}
	}
	sharedArtistIDsList := make([]uint, 0, len(allSharedArtistIDs))
	for aid := range allSharedArtistIDs {
		sharedArtistIDsList = append(sharedArtistIDsList, aid)
	}
	artistMap := s.loadArtistInfoMap(sharedArtistIDsList)

	// Build responses
	results := make([]contracts.SimilarFestival, len(scored))
	for i, sf := range scored {
		fi := festivalMap[sf.festivalID]

		// Build top shared artists (up to 5), ordered by tier weight
		type sharedWithWeight struct {
			artistID uint
			weight   float64
		}
		var sharedList []sharedWithWeight
		for aid, targetTier := range sf.sharedMap {
			sourceTier := sourceArtistIDs[aid]
			w := tierWeight(sourceTier)
			tw := tierWeight(targetTier)
			if tw > w {
				w = tw
			}
			sharedList = append(sharedList, sharedWithWeight{artistID: aid, weight: w})
		}
		sort.Slice(sharedList, func(a, b int) bool {
			return sharedList[a].weight > sharedList[b].weight
		})

		topCount := 5
		if len(sharedList) < topCount {
			topCount = len(sharedList)
		}
		topShared := make([]contracts.SharedArtist, topCount)
		for j := 0; j < topCount; j++ {
			aid := sharedList[j].artistID
			ai := artistMap[aid]
			topShared[j] = contracts.SharedArtist{
				ArtistID:     aid,
				Name:         ai.Name,
				Slug:         ai.Slug,
				TierAtSource: sourceArtistIDs[aid],
				TierAtTarget: sf.sharedMap[aid],
			}
		}

		results[i] = contracts.SimilarFestival{
			Festival: contracts.FestivalSummary{
				ID:          fi.ID,
				Name:        fi.Name,
				Slug:        fi.Slug,
				SeriesSlug:  fi.SeriesSlug,
				EditionYear: fi.EditionYear,
				City:        fi.City,
				State:       fi.State,
			},
			SharedArtistCount: sf.sharedCount,
			Jaccard:           sf.jaccard,
			WeightedScore:     sf.weightedScore,
			TopShared:         topShared,
		}
	}

	return results, nil
}

// ──────────────────────────────────────────────
// GetFestivalOverlap
// ──────────────────────────────────────────────

// GetFestivalOverlap returns the detailed artist overlap between two festivals.
func (s *FestivalIntelligenceService) GetFestivalOverlap(festivalAID, festivalBID uint) (*contracts.FestivalOverlap, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Verify both festivals exist
	var festA, festB catalogm.Festival
	if err := s.db.First(&festA, festivalAID).Error; err != nil {
		return nil, fmt.Errorf("festival A not found")
	}
	if err := s.db.First(&festB, festivalBID).Error; err != nil {
		return nil, fmt.Errorf("festival B not found")
	}

	// Get artists for both festivals
	var artistsA, artistsB []festivalArtistRow
	s.db.Table("festival_artists").
		Select("festival_id, artist_id, billing_tier").
		Where("festival_id = ?", festivalAID).
		Scan(&artistsA)
	s.db.Table("festival_artists").
		Select("festival_id, artist_id, billing_tier").
		Where("festival_id = ?", festivalBID).
		Scan(&artistsB)

	setA := make(map[uint]string)
	for _, a := range artistsA {
		setA[a.ArtistID] = a.BillingTier
	}
	setB := make(map[uint]string)
	for _, b := range artistsB {
		setB[b.ArtistID] = b.BillingTier
	}

	// Compute shared artists
	var sharedIDs []uint
	for id := range setA {
		if _, ok := setB[id]; ok {
			sharedIDs = append(sharedIDs, id)
		}
	}

	// Compute Jaccard
	unionCount := len(setA) + len(setB) - len(sharedIDs)
	jaccard := 0.0
	if unionCount > 0 {
		jaccard = float64(len(sharedIDs)) / float64(unionCount)
	}

	// Compute weighted score
	var weightedScore float64
	for _, aid := range sharedIDs {
		sw := tierWeight(setA[aid])
		tw := tierWeight(setB[aid])
		if tw > sw {
			weightedScore += tw
		} else {
			weightedScore += sw
		}
	}

	// Load artist info
	artistMap := s.loadArtistInfoMap(sharedIDs)

	// Build shared artist list ordered by tier rank at A
	sort.Slice(sharedIDs, func(i, j int) bool {
		ri := tierRank(setA[sharedIDs[i]])
		rj := tierRank(setA[sharedIDs[j]])
		if ri != rj {
			return ri < rj
		}
		return artistMap[sharedIDs[i]].Name < artistMap[sharedIDs[j]].Name
	})

	sharedArtists := make([]contracts.SharedArtist, len(sharedIDs))
	for i, aid := range sharedIDs {
		ai := artistMap[aid]
		sharedArtists[i] = contracts.SharedArtist{
			ArtistID:     aid,
			Name:         ai.Name,
			Slug:         ai.Slug,
			TierAtSource: setA[aid],
			TierAtTarget: setB[aid],
		}
	}

	aOnlyCount := len(setA) - len(sharedIDs)
	bOnlyCount := len(setB) - len(sharedIDs)

	return &contracts.FestivalOverlap{
		FestivalA: contracts.FestivalSummary{
			ID: festA.ID, Name: festA.Name, Slug: festA.Slug,
			SeriesSlug: festA.SeriesSlug, EditionYear: festA.EditionYear,
			City: festA.City, State: festA.State,
		},
		FestivalB: contracts.FestivalSummary{
			ID: festB.ID, Name: festB.Name, Slug: festB.Slug,
			SeriesSlug: festB.SeriesSlug, EditionYear: festB.EditionYear,
			City: festB.City, State: festB.State,
		},
		SharedArtists: sharedArtists,
		Jaccard:       math.Round(jaccard*10000) / 10000,
		WeightedScore: math.Round(weightedScore*100) / 100,
		AOnlyCount:    aOnlyCount,
		BOnlyCount:    bOnlyCount,
	}, nil
}

// ──────────────────────────────────────────────
// GetFestivalBreakouts
// ──────────────────────────────────────────────

// GetFestivalBreakouts identifies artists at this festival whose billing tier
// improved compared to their previous festival appearances.
func (s *FestivalIntelligenceService) GetFestivalBreakouts(festivalID uint) (*contracts.FestivalBreakouts, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Verify festival exists and get its info
	var festival catalogm.Festival
	if err := s.db.First(&festival, festivalID).Error; err != nil {
		return nil, fmt.Errorf("festival not found")
	}

	// Get artists at this festival
	var currentArtists []festivalArtistRow
	s.db.Table("festival_artists").
		Select("festival_id, artist_id, billing_tier").
		Where("festival_id = ?", festivalID).
		Scan(&currentArtists)

	if len(currentArtists) == 0 {
		return &contracts.FestivalBreakouts{
			Breakouts:  []contracts.ArtistBreakout{},
			Milestones: []contracts.ArtistMilestone{},
		}, nil
	}

	artistIDs := make([]uint, len(currentArtists))
	currentTiers := make(map[uint]string)
	for i, ca := range currentArtists {
		artistIDs[i] = ca.ArtistID
		currentTiers[ca.ArtistID] = ca.BillingTier
	}

	// Get all festival appearances for these artists (across all festivals)
	type historyRow struct {
		ArtistID    uint
		FestivalID  uint
		BillingTier string
		StartDate   string
		EditionYear int
		Name        string
		Slug        string
	}

	var history []historyRow
	s.db.Table("festival_artists fa").
		Select("fa.artist_id, fa.festival_id, fa.billing_tier, f.start_date, f.edition_year, f.name, f.slug").
		Joins("JOIN festivals f ON f.id = fa.festival_id").
		Where("fa.artist_id IN ? AND f.status IN ('confirmed', 'completed', 'announced')", artistIDs).
		Order("fa.artist_id, f.start_date ASC").
		Scan(&history)

	// Group history by artist
	artistHistory := make(map[uint][]historyRow)
	for _, h := range history {
		artistHistory[h.ArtistID] = append(artistHistory[h.ArtistID], h)
	}

	// Load artist info
	artistMap := s.loadArtistInfoMap(artistIDs)

	var breakouts []contracts.ArtistBreakout
	var milestones []contracts.ArtistMilestone

	for _, aid := range artistIDs {
		hist := artistHistory[aid]
		ai := artistMap[aid]
		currentTier := currentTiers[aid]

		if len(hist) == 1 {
			// First festival appearance - milestone
			milestones = append(milestones, contracts.ArtistMilestone{
				Artist:    contracts.ArtistSummary{ID: ai.ID, Name: ai.Name, Slug: ai.Slug},
				Milestone: "first_festival_appearance",
				Tier:      currentTier,
				Festival:  festival.Name,
			})
			continue
		}

		// Build trajectory
		trajectory := make([]contracts.TrajectoryEntry, len(hist))
		var bestRank int = 99
		for i, h := range hist {
			trajectory[i] = contracts.TrajectoryEntry{
				FestivalName: h.Name,
				FestivalSlug: h.Slug,
				Year:         h.EditionYear,
				Tier:         h.BillingTier,
			}
			r := tierRank(h.BillingTier)
			if r < bestRank {
				bestRank = r
			}
		}

		// Check for tier improvement vs previous appearances
		currentRank := tierRank(currentTier)
		var totalImprovement int
		firstYear := hist[0].EditionYear
		lastYear := hist[len(hist)-1].EditionYear
		yearsSpan := lastYear - firstYear
		if yearsSpan < 1 {
			yearsSpan = 1
		}

		// Calculate improvement: sum of rank decreases between consecutive appearances
		for i := 1; i < len(hist); i++ {
			prevRank := tierRank(hist[i-1].BillingTier)
			currRank := tierRank(hist[i].BillingTier)
			if currRank < prevRank {
				totalImprovement += prevRank - currRank
			}
		}

		if totalImprovement > 0 {
			breakoutScore := float64(totalImprovement) / float64(yearsSpan)
			breakouts = append(breakouts, contracts.ArtistBreakout{
				Artist:          contracts.ArtistSummary{ID: ai.ID, Name: ai.Name, Slug: ai.Slug},
				CurrentTier:     currentTier,
				Trajectory:      trajectory,
				TierImprovement: totalImprovement,
				BreakoutScore:   math.Round(breakoutScore*100) / 100,
			})
		}

		// Check milestones
		// First headliner
		if currentTier == "headliner" {
			isFirstHeadliner := true
			for _, h := range hist[:len(hist)-1] {
				if h.BillingTier == "headliner" {
					isFirstHeadliner = false
					break
				}
			}
			if isFirstHeadliner {
				milestones = append(milestones, contracts.ArtistMilestone{
					Artist:    contracts.ArtistSummary{ID: ai.ID, Name: ai.Name, Slug: ai.Slug},
					Milestone: "first_headliner",
					Tier:      currentTier,
					Festival:  festival.Name,
				})
			}
		}

		// Local graduation
		if currentTier != "local" && currentRank < 5 {
			hadLocal := false
			for _, h := range hist[:len(hist)-1] {
				if h.BillingTier == "local" {
					hadLocal = true
					break
				}
			}
			if hadLocal {
				milestones = append(milestones, contracts.ArtistMilestone{
					Artist:    contracts.ArtistSummary{ID: ai.ID, Name: ai.Name, Slug: ai.Slug},
					Milestone: "local_graduation",
					Tier:      currentTier,
					Festival:  festival.Name,
				})
			}
		}
	}

	// Sort breakouts by breakout score DESC
	sort.Slice(breakouts, func(i, j int) bool {
		return breakouts[i].BreakoutScore > breakouts[j].BreakoutScore
	})

	if breakouts == nil {
		breakouts = []contracts.ArtistBreakout{}
	}
	if milestones == nil {
		milestones = []contracts.ArtistMilestone{}
	}

	return &contracts.FestivalBreakouts{
		Breakouts:  breakouts,
		Milestones: milestones,
	}, nil
}

// ──────────────────────────────────────────────
// GetArtistFestivalTrajectory
// ──────────────────────────────────────────────

// GetArtistFestivalTrajectory returns the complete billing tier history
// for an artist across all their festival appearances.
func (s *FestivalIntelligenceService) GetArtistFestivalTrajectory(artistID uint) (*contracts.ArtistTrajectory, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Verify artist exists
	var artist catalogm.Artist
	if err := s.db.First(&artist, artistID).Error; err != nil {
		return nil, fmt.Errorf("artist not found")
	}

	artistSlug := ""
	if artist.Slug != nil {
		artistSlug = *artist.Slug
	}

	// Get all festival appearances
	type historyRow struct {
		FestivalID  uint
		BillingTier string
		EditionYear int
		Name        string
		Slug        string
	}

	var history []historyRow
	s.db.Table("festival_artists fa").
		Select("fa.festival_id, fa.billing_tier, f.edition_year, f.name, f.slug").
		Joins("JOIN festivals f ON f.id = fa.festival_id").
		Where("fa.artist_id = ?", artistID).
		Order("f.start_date ASC").
		Scan(&history)

	if len(history) == 0 {
		return &contracts.ArtistTrajectory{
			Artist:           contracts.ArtistSummary{ID: artist.ID, Name: artist.Name, Slug: artistSlug},
			Appearances:      []contracts.TrajectoryEntry{},
			BestTier:         "",
			TotalAppearances: 0,
			BreakoutScore:    0,
		}, nil
	}

	appearances := make([]contracts.TrajectoryEntry, len(history))
	bestRank := 99
	for i, h := range history {
		appearances[i] = contracts.TrajectoryEntry{
			FestivalName: h.Name,
			FestivalSlug: h.Slug,
			Year:         h.EditionYear,
			Tier:         h.BillingTier,
		}
		r := tierRank(h.BillingTier)
		if r < bestRank {
			bestRank = r
		}
	}

	// Compute breakout score
	var totalImprovement int
	for i := 1; i < len(history); i++ {
		prevRank := tierRank(history[i-1].BillingTier)
		currRank := tierRank(history[i].BillingTier)
		if currRank < prevRank {
			totalImprovement += prevRank - currRank
		}
	}

	yearsSpan := history[len(history)-1].EditionYear - history[0].EditionYear
	if yearsSpan < 1 {
		yearsSpan = 1
	}

	breakoutScore := 0.0
	if totalImprovement > 0 {
		breakoutScore = math.Round(float64(totalImprovement)/float64(yearsSpan)*100) / 100
	}

	return &contracts.ArtistTrajectory{
		Artist:           contracts.ArtistSummary{ID: artist.ID, Name: artist.Name, Slug: artistSlug},
		Appearances:      appearances,
		BestTier:         rankToTier(bestRank),
		TotalAppearances: len(history),
		BreakoutScore:    breakoutScore,
	}, nil
}

// ──────────────────────────────────────────────
// GetSeriesComparison
// ──────────────────────────────────────────────

// GetSeriesComparison compares editions of a recurring festival series,
// showing returning artists, newcomers, and retention metrics.
func (s *FestivalIntelligenceService) GetSeriesComparison(seriesSlug string, years []int) (*contracts.SeriesComparison, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if len(years) < 2 {
		return nil, fmt.Errorf("at least 2 years required for comparison")
	}

	// Sort years ascending
	sort.Ints(years)

	// Get festivals in this series for the requested years
	var festivals []catalogm.Festival
	if err := s.db.Where("series_slug = ? AND edition_year IN ?", seriesSlug, years).
		Order("edition_year ASC").
		Find(&festivals).Error; err != nil {
		return nil, fmt.Errorf("failed to get festivals: %w", err)
	}

	if len(festivals) == 0 {
		return nil, fmt.Errorf("no festivals found for series '%s' in the requested years", seriesSlug)
	}

	// Get festival IDs and build editions
	festivalIDs := make([]uint, len(festivals))
	festivalYearMap := make(map[int]uint) // year -> festivalID
	for i, f := range festivals {
		festivalIDs[i] = f.ID
		festivalYearMap[f.EditionYear] = f.ID
	}

	// Get artist counts per festival
	type countResult struct {
		FestivalID uint
		Count      int
	}
	var artistCounts []countResult
	s.db.Table("festival_artists").
		Select("festival_id, COUNT(DISTINCT artist_id) as count").
		Where("festival_id IN ?", festivalIDs).
		Group("festival_id").
		Scan(&artistCounts)

	countMap := make(map[uint]int)
	for _, ac := range artistCounts {
		countMap[ac.FestivalID] = ac.Count
	}

	editions := make([]contracts.SeriesEdition, len(festivals))
	for i, f := range festivals {
		editions[i] = contracts.SeriesEdition{
			FestivalID:  f.ID,
			Name:        f.Name,
			Slug:        f.Slug,
			Year:        f.EditionYear,
			ArtistCount: countMap[f.ID],
		}
	}

	// Get all festival_artist rows for these festivals
	var allArtists []festivalArtistRow
	s.db.Table("festival_artists").
		Select("festival_id, artist_id, billing_tier").
		Where("festival_id IN ?", festivalIDs).
		Scan(&allArtists)

	// Build per-edition artist sets
	type editionArtist struct {
		tier string
	}
	// year -> artistID -> tier
	editionSets := make(map[int]map[uint]string)
	for _, f := range festivals {
		editionSets[f.EditionYear] = make(map[uint]string)
	}
	festivalIDToYear := make(map[uint]int)
	for _, f := range festivals {
		festivalIDToYear[f.ID] = f.EditionYear
	}
	for _, fa := range allArtists {
		year := festivalIDToYear[fa.FestivalID]
		editionSets[year][fa.ArtistID] = fa.BillingTier
	}

	// Find returning artists (appear in 2+ editions)
	artistYears := make(map[uint]map[int]string) // artistID -> year -> tier
	for year, artists := range editionSets {
		for aid, tier := range artists {
			if artistYears[aid] == nil {
				artistYears[aid] = make(map[int]string)
			}
			artistYears[aid][year] = tier
		}
	}

	var returningArtistIDs []uint
	for aid, yearMap := range artistYears {
		if len(yearMap) >= 2 {
			returningArtistIDs = append(returningArtistIDs, aid)
		}
	}

	// Load artist info for returning artists
	artistMap := s.loadArtistInfoMap(returningArtistIDs)

	returningArtists := make([]contracts.ReturningArtist, 0, len(returningArtistIDs))
	for _, aid := range returningArtistIDs {
		ai := artistMap[aid]
		yearMap := artistYears[aid]

		var artistYearList []int
		tiers := make(map[string]string)
		for y, t := range yearMap {
			artistYearList = append(artistYearList, y)
			tiers[fmt.Sprintf("%d", y)] = t
		}
		sort.Ints(artistYearList)

		returningArtists = append(returningArtists, contracts.ReturningArtist{
			Artist: contracts.ArtistSummary{ID: ai.ID, Name: ai.Name, Slug: ai.Slug},
			Years:  artistYearList,
			Tiers:  tiers,
		})
	}

	// Sort returning artists by number of appearances DESC, then name ASC
	sort.Slice(returningArtists, func(i, j int) bool {
		if len(returningArtists[i].Years) != len(returningArtists[j].Years) {
			return len(returningArtists[i].Years) > len(returningArtists[j].Years)
		}
		return returningArtists[i].Artist.Name < returningArtists[j].Artist.Name
	})

	// Find newcomers (artists in latest year but no previous year)
	latestYear := years[len(years)-1]
	latestSet := editionSets[latestYear]
	previousArtists := make(map[uint]bool)
	for _, y := range years[:len(years)-1] {
		for aid := range editionSets[y] {
			previousArtists[aid] = true
		}
	}

	var newcomerIDs []uint
	newcomerTiers := make(map[uint]string)
	for aid, tier := range latestSet {
		if !previousArtists[aid] {
			newcomerIDs = append(newcomerIDs, aid)
			newcomerTiers[aid] = tier
		}
	}

	newcomerArtistMap := s.loadArtistInfoMap(newcomerIDs)
	newcomers := make([]contracts.SeriesNewcomer, 0, len(newcomerIDs))
	for _, aid := range newcomerIDs {
		ai := newcomerArtistMap[aid]
		newcomers = append(newcomers, contracts.SeriesNewcomer{
			Artist: contracts.ArtistSummary{ID: ai.ID, Name: ai.Name, Slug: ai.Slug},
			Tier:   newcomerTiers[aid],
		})
	}

	// Sort newcomers by tier rank
	sort.Slice(newcomers, func(i, j int) bool {
		ri := tierRank(newcomers[i].Tier)
		rj := tierRank(newcomers[j].Tier)
		if ri != rj {
			return ri < rj
		}
		return newcomers[i].Artist.Name < newcomers[j].Artist.Name
	})

	// Compute retention rate (fraction of previous year's artists who return in latest)
	previousYear := years[len(years)-2]
	previousSet := editionSets[previousYear]
	returningFromPrevious := 0
	for aid := range previousSet {
		if _, ok := latestSet[aid]; ok {
			returningFromPrevious++
		}
	}

	retentionRate := 0.0
	if len(previousSet) > 0 {
		retentionRate = math.Round(float64(returningFromPrevious)/float64(len(previousSet))*100) / 100
	}

	// Compute lineup growth (year-over-year change in artist count)
	lineupGrowth := 0.0
	if len(previousSet) > 0 {
		lineupGrowth = math.Round(float64(len(latestSet)-len(previousSet))/float64(len(previousSet))*100) / 100
	}

	return &contracts.SeriesComparison{
		SeriesSlug:       seriesSlug,
		Editions:         editions,
		ReturningArtists: returningArtists,
		Newcomers:        newcomers,
		RetentionRate:    retentionRate,
		LineupGrowth:     lineupGrowth,
	}, nil
}

// ──────────────────────────────────────────────
// Helper functions
// ──────────────────────────────────────────────

// loadArtistInfoMap loads basic artist info for a list of IDs.
func (s *FestivalIntelligenceService) loadArtistInfoMap(artistIDs []uint) map[uint]artistInfo {
	result := make(map[uint]artistInfo)
	if len(artistIDs) == 0 {
		return result
	}

	var artists []catalogm.Artist
	s.db.Where("id IN ?", artistIDs).Find(&artists)

	for _, a := range artists {
		slug := ""
		if a.Slug != nil {
			slug = *a.Slug
		}
		result[a.ID] = artistInfo{ID: a.ID, Name: a.Name, Slug: slug}
	}
	return result
}

// loadFestivalInfoMap loads basic festival info for a list of IDs.
func (s *FestivalIntelligenceService) loadFestivalInfoMap(festivalIDs []uint) map[uint]festivalInfo {
	result := make(map[uint]festivalInfo)
	if len(festivalIDs) == 0 {
		return result
	}

	var festivals []catalogm.Festival
	s.db.Where("id IN ?", festivalIDs).Find(&festivals)

	for _, f := range festivals {
		result[f.ID] = festivalInfo{
			ID:          f.ID,
			Name:        f.Name,
			Slug:        f.Slug,
			SeriesSlug:  f.SeriesSlug,
			EditionYear: f.EditionYear,
			City:        f.City,
			State:       f.State,
			StartDate:   f.StartDate,
		}
	}
	return result
}
