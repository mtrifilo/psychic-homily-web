package catalog

// WFMU family show dedup (PSY-1073).
//
// Before the station-scoped discovery fix, every discover cycle assigned the
// ENTIRE WFMU DJ index (574 shows) to whichever family station triggered it,
// so each of the four stations carried a full duplicate catalog — the same
// program existed as four distinct radio_shows rows (one per station), each
// with its own copy of the episode/play history.
//
// DedupWFMUFamilyShows collapses each external-show-code group down to one
// canonical row on the station that actually airs the show (the ownership
// map comes from WFMUProvider.FetchShowOwnership; codes absent from the map
// default to the flagship). Episode history merges into the winner —
// duplicate (air_date, external_id) copies keep whichever side logged more
// plays — and import jobs re-point to the winner before loser rows are
// deleted. The whole run executes in one transaction; dry-run executes the
// identical plan and rolls it back, so reported counts always match what a
// live run would do.

import (
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/utils"
)

// WFMUFlagshipSlug is the seeded slug of the WFMU 91.1 flagship station.
const WFMUFlagshipSlug = "wfmu"

// WFMUFamilySlugs are the seeded slugs of the four WFMU-family stations
// (migration 20260502023012). Keep in sync with wfmuStationChannels in
// radio_provider_wfmu.go.
var WFMUFamilySlugs = []string{
	WFMUFlagshipSlug,
	"wfmu-drummer",
	"wfmu-rocknsoulradio",
	"wfmu-sheena",
}

// WFMUDedupStationCounts aggregates per-station outcomes of a dedup run.
type WFMUDedupStationCounts struct {
	ShowsKept         int // canonical rows that ended up on this station
	ShowsReassignedIn int // rows moved onto this station from a sibling
	ShowsDeleted      int // duplicate rows deleted from this station
	EpisodesMovedIn   int // episodes re-pointed to a winner on this station
	EpisodesDeleted   int // duplicate-episode copies deleted from this station's losers/winners
	JobsReassigned    int // radio_import_jobs re-pointed to a winner on this station
}

// WFMUDedupResult is the full outcome of a DedupWFMUFamilyShows run.
type WFMUDedupResult struct {
	DryRun                bool
	GroupsTotal           int // distinct external show codes across the family
	GroupsWithDuplicates  int // codes that had >1 row or a misplaced single row
	ShowsWithNoExternalID int // rows skipped (cannot be grouped)
	SlugsRecanonicalised  int // winners whose -N suffixed slug reverted to the freed base slug
	PerStation            map[string]*WFMUDedupStationCounts // keyed by station slug
}

// errDryRunRollback aborts the dedup transaction after the full plan has
// executed, turning a live run into a dry run with identical counts.
var errDryRunRollback = errors.New("wfmu dedup dry-run rollback")

// DedupWFMUFamilyShows merges duplicated WFMU-family show rows onto their
// canonical stations. ownership maps external show code → owning station
// slug (see WFMUProvider.FetchShowOwnership); codes missing from the map
// default to the flagship. With dryRun=true the full plan executes inside a
// transaction and is rolled back, so the returned counts are exact.
//
// Idempotent: a second run finds every group already singular and on its
// owner, and reports all-zero mutation counts.
func DedupWFMUFamilyShows(db *gorm.DB, ownership map[string]string, dryRun bool) (*WFMUDedupResult, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := &WFMUDedupResult{
		DryRun:     dryRun,
		PerStation: make(map[string]*WFMUDedupStationCounts, len(WFMUFamilySlugs)),
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := runWFMUDedup(tx, ownership, result); err != nil {
			return err
		}
		if dryRun {
			return errDryRunRollback
		}
		return nil
	})
	if errors.Is(err, errDryRunRollback) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

func runWFMUDedup(tx *gorm.DB, ownership map[string]string, result *WFMUDedupResult) error {
	// Load the family stations. All four must exist — a partial family means
	// the seed migration hasn't run and "default to flagship" could target
	// the wrong database.
	var stations []catalogm.RadioStation
	if err := tx.Where("slug IN ?", WFMUFamilySlugs).Find(&stations).Error; err != nil {
		return fmt.Errorf("loading WFMU family stations: %w", err)
	}
	if len(stations) != len(WFMUFamilySlugs) {
		return fmt.Errorf("expected %d WFMU family stations, found %d (slugs %v): run the WFMU seed migration first",
			len(WFMUFamilySlugs), len(stations), WFMUFamilySlugs)
	}

	stationIDBySlug := make(map[string]uint, len(stations))
	slugByStationID := make(map[uint]string, len(stations))
	var stationIDs []uint
	for _, st := range stations {
		stationIDBySlug[st.Slug] = st.ID
		slugByStationID[st.ID] = st.Slug
		stationIDs = append(stationIDs, st.ID)
	}
	for _, slug := range WFMUFamilySlugs {
		result.PerStation[slug] = &WFMUDedupStationCounts{}
	}

	// Group every family show row by external code.
	var shows []catalogm.RadioShow
	if err := tx.Where("station_id IN ?", stationIDs).Order("id").Find(&shows).Error; err != nil {
		return fmt.Errorf("loading WFMU family shows: %w", err)
	}

	groups := make(map[string][]catalogm.RadioShow)
	for _, show := range shows {
		if show.ExternalID == nil || *show.ExternalID == "" {
			result.ShowsWithNoExternalID++
			continue
		}
		groups[*show.ExternalID] = append(groups[*show.ExternalID], show)
	}
	result.GroupsTotal = len(groups)

	// Deterministic processing order for stable output and reproducible runs.
	codes := make([]string, 0, len(groups))
	for code := range groups {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	flagshipID := stationIDBySlug[WFMUFlagshipSlug]
	var winnersToRecanonicalise []uint
	for _, code := range codes {
		group := groups[code]

		ownerSlug := ownership[code]
		ownerID, ok := stationIDBySlug[ownerSlug]
		if !ok {
			ownerSlug = WFMUFlagshipSlug
			ownerID = flagshipID
		}

		winner, losers := pickWFMUDedupWinner(group, ownerID, flagshipID)
		if len(losers) == 0 && winner.StationID == ownerID {
			result.PerStation[ownerSlug].ShowsKept++
			continue // already singular and correctly placed
		}
		result.GroupsWithDuplicates++

		for _, loser := range losers {
			if err := mergeWFMUDuplicateShow(tx, &winner, loser, ownerID, ownerSlug, slugByStationID, result); err != nil {
				return fmt.Errorf("merging show %q (code %s) loser id=%d into winner id=%d: %w",
					loser.Name, code, loser.ID, winner.ID, err)
			}
		}

		if winner.StationID != ownerID {
			// Safe wrt UNIQUE(station_id, external_id): if the owner station
			// had a row for this code it would have been chosen as winner.
			if err := tx.Model(&catalogm.RadioShow{}).Where("id = ?", winner.ID).
				Update("station_id", ownerID).Error; err != nil {
				return fmt.Errorf("reassigning show id=%d to station %s: %w", winner.ID, ownerSlug, err)
			}
			// Keep the denormalized station_id on the winner's own import
			// jobs consistent with its new home (loser-pointing jobs were
			// already re-pointed during the merges above).
			res := tx.Model(&catalogm.RadioImportJob{}).Where("show_id = ?", winner.ID).
				Update("station_id", ownerID)
			if res.Error != nil {
				return fmt.Errorf("updating import jobs for reassigned show id=%d: %w", winner.ID, res.Error)
			}
			result.PerStation[ownerSlug].JobsReassigned += int(res.RowsAffected)
			result.PerStation[ownerSlug].ShowsReassignedIn++
		}
		result.PerStation[ownerSlug].ShowsKept++
		winnersToRecanonicalise = append(winnersToRecanonicalise, winner.ID)
	}

	return recanonicaliseWFMUShowSlugs(tx, winnersToRecanonicalise, result)
}

// recanonicaliseWFMUShowSlugs reverts -N suffixed slugs on surviving rows to
// their base slug where the merge freed it. The broken discovery created the
// duplicates in arbitrary order, so the canonical-URL slug ("wake") often
// died with a loser while the winner kept a disambiguated one ("wake-4").
// Mirrors the slug pass of cmd/dedup-shows (PSY-559). Skipped when any other
// row — radio show on any station — still holds the base slug.
func recanonicaliseWFMUShowSlugs(tx *gorm.DB, winnerIDs []uint, result *WFMUDedupResult) error {
	for _, id := range winnerIDs {
		var show catalogm.RadioShow
		if err := tx.First(&show, id).Error; err != nil {
			return fmt.Errorf("reloading show id=%d for slug pass: %w", id, err)
		}
		base := utils.GenerateArtistSlug(show.Name)
		if base == "" || show.Slug == base {
			continue
		}
		var taken int64
		if err := tx.Model(&catalogm.RadioShow{}).Where("slug = ?", base).Count(&taken).Error; err != nil {
			return fmt.Errorf("checking base slug %q: %w", base, err)
		}
		if taken > 0 {
			continue
		}
		if err := tx.Model(&catalogm.RadioShow{}).Where("id = ?", id).Update("slug", base).Error; err != nil {
			return fmt.Errorf("recanonicalising slug for show id=%d: %w", id, err)
		}
		result.SlugsRecanonicalised++
	}
	return nil
}

// pickWFMUDedupWinner chooses the canonical row for a code group: the row
// already on the owner station, else the flagship row, else the oldest row
// (lowest ID — the group is pre-sorted by ID). Everything else is a loser.
func pickWFMUDedupWinner(group []catalogm.RadioShow, ownerID, flagshipID uint) (catalogm.RadioShow, []catalogm.RadioShow) {
	winnerIdx := 0
	for i, show := range group {
		if show.StationID == ownerID {
			winnerIdx = i
			break
		}
		if show.StationID == flagshipID && group[winnerIdx].StationID != flagshipID {
			winnerIdx = i
		}
	}

	winner := group[winnerIdx]
	losers := make([]catalogm.RadioShow, 0, len(group)-1)
	for i, show := range group {
		if i != winnerIdx {
			losers = append(losers, show)
		}
	}
	return winner, losers
}

// mergeWFMUDuplicateShow folds one duplicate row into the winner:
//
//  1. For (air_date, external_id) episodes present on BOTH sides, keep the
//     copy with more logged plays (ties keep the winner's) — plays cascade
//     with their episode, so the richer history always survives.
//  2. Re-point the loser's remaining episodes to the winner.
//  3. Re-point radio_import_jobs (plain FK, no cascade) to the winner.
//  4. Copy metadata the winner is missing (host, description, artwork, …)
//     from the loser — it may be the seeded, admin-curated row.
//  5. Delete the loser row.
func mergeWFMUDuplicateShow(
	tx *gorm.DB,
	winner *catalogm.RadioShow, // pointer: adopted metadata must persist across multi-loser merges
	loser catalogm.RadioShow,
	ownerID uint,
	ownerSlug string,
	slugByStationID map[uint]string,
	result *WFMUDedupResult,
) error {
	loserStationCounts := result.PerStation[slugByStationID[loser.StationID]]
	ownerCounts := result.PerStation[ownerSlug]

	// 1a. Winner copies beaten by a richer loser copy. Richness compares
	// actual radio_plays rows, not the denormalized play_count column — a
	// stale counter must never decide which copy's play history survives.
	res := tx.Exec(`
		DELETE FROM radio_episodes w
		WHERE w.show_id = ?
		  AND EXISTS (
			SELECT 1 FROM radio_episodes l
			WHERE l.show_id = ?
			  AND l.air_date = w.air_date
			  AND COALESCE(l.external_id, '') = COALESCE(w.external_id, '')
			  AND (SELECT COUNT(*) FROM radio_plays WHERE episode_id = l.id) >
			      (SELECT COUNT(*) FROM radio_plays WHERE episode_id = w.id)
		  )`, winner.ID, loser.ID)
	if res.Error != nil {
		return fmt.Errorf("deleting outplayed winner episodes: %w", res.Error)
	}
	ownerCounts.EpisodesDeleted += int(res.RowsAffected)

	// 1b. Loser copies that still collide with a surviving winner copy.
	res = tx.Exec(`
		DELETE FROM radio_episodes l
		WHERE l.show_id = ?
		  AND EXISTS (
			SELECT 1 FROM radio_episodes w
			WHERE w.show_id = ?
			  AND w.air_date = l.air_date
			  AND COALESCE(w.external_id, '') = COALESCE(l.external_id, '')
		  )`, loser.ID, winner.ID)
	if res.Error != nil {
		return fmt.Errorf("deleting duplicate loser episodes: %w", res.Error)
	}
	loserStationCounts.EpisodesDeleted += int(res.RowsAffected)

	// 2. Move what's left.
	res = tx.Model(&catalogm.RadioEpisode{}).Where("show_id = ?", loser.ID).
		Update("show_id", winner.ID)
	if res.Error != nil {
		return fmt.Errorf("moving loser episodes: %w", res.Error)
	}
	ownerCounts.EpisodesMovedIn += int(res.RowsAffected)

	// 3. radio_import_jobs.show_id has no ON DELETE clause — re-point jobs
	// (and their station denormalization) before deleting the loser.
	res = tx.Model(&catalogm.RadioImportJob{}).Where("show_id = ?", loser.ID).
		Updates(map[string]interface{}{"show_id": winner.ID, "station_id": ownerID})
	if res.Error != nil {
		return fmt.Errorf("reassigning import jobs: %w", res.Error)
	}
	ownerCounts.JobsReassigned += int(res.RowsAffected)

	// 4. Null-safe metadata adoption (the loser may be the curated seed row).
	// winner's in-memory fields are updated too, so a later loser in the same
	// group can't overwrite metadata adopted from an earlier one.
	updates := map[string]interface{}{}
	if winner.HostName == nil && loser.HostName != nil {
		updates["host_name"] = *loser.HostName
		winner.HostName = loser.HostName
	}
	if winner.Description == nil && loser.Description != nil {
		updates["description"] = *loser.Description
		winner.Description = loser.Description
	}
	if winner.ScheduleDisplay == nil && loser.ScheduleDisplay != nil {
		updates["schedule_display"] = *loser.ScheduleDisplay
		winner.ScheduleDisplay = loser.ScheduleDisplay
	}
	if winner.ArchiveURL == nil && loser.ArchiveURL != nil {
		updates["archive_url"] = *loser.ArchiveURL
		winner.ArchiveURL = loser.ArchiveURL
	}
	if winner.ImageURL == nil && loser.ImageURL != nil {
		updates["image_url"] = *loser.ImageURL
		winner.ImageURL = loser.ImageURL
	}
	if len(updates) > 0 {
		if err := tx.Model(&catalogm.RadioShow{}).Where("id = ?", winner.ID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("adopting loser metadata: %w", err)
		}
	}

	// 5. Drop the duplicate row.
	if err := tx.Delete(&catalogm.RadioShow{}, loser.ID).Error; err != nil {
		return fmt.Errorf("deleting loser show: %w", err)
	}
	loserStationCounts.ShowsDeleted++

	return nil
}
