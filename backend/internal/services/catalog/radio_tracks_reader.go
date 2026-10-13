package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// radioTrackSortColumns maps each listing sort to its radio_tracks column.
var radioTrackSortColumns = map[contracts.RadioTrackSort]string{
	contracts.RadioTrackSortPlays:           "play_count",
	contracts.RadioTrackSortStationRows:     "station_row_count",
	contracts.RadioTrackSortStationFamilies: "station_family_count",
}

// RadioTracksReader serves the internal top-tracks listing.
type RadioTracksReader struct {
	db *gorm.DB
}

// NewRadioTracksReader constructs a reader over radio_tracks.
func NewRadioTracksReader(database *gorm.DB) *RadioTracksReader {
	return &RadioTracksReader{db: database}
}

// radioTrackListRow is a radio_tracks row plus its key count.
type radioTrackListRow struct {
	catalogm.RadioTrack
	KeyCount int `gorm:"column:key_count"`
}

// ListTopRadioTracks returns up to limit keyed tracks ranked by the sort
// column, descending, then by play count and id. The unkeyed marker row is
// never listed; its play count is reported as UnkeyedPlays.
func (r *RadioTracksReader) ListTopRadioTracks(ctx context.Context, sort contracts.RadioTrackSort, limit int) (*contracts.RadioTrackListing, error) {
	if r.db == nil {
		return nil, errors.New("database not initialized")
	}
	column, ok := radioTrackSortColumns[sort]
	if !ok {
		return nil, fmt.Errorf("unknown radio track sort %q", sort)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", limit)
	}

	listing := &contracts.RadioTrackListing{}
	var rows []radioTrackListRow
	// One snapshot, so the totals describe the same rebuild as the rows.
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw(`SELECT t.*, (SELECT count(*) FROM radio_track_keys k WHERE k.track_id = t.id) AS key_count
FROM radio_tracks t
WHERE NOT t.unkeyed
ORDER BY t.`+column+` DESC, t.play_count DESC, t.id
LIMIT ?`, limit).Scan(&rows).Error; err != nil {
			return fmt.Errorf("listing radio tracks: %w", err)
		}
		if err := tx.Raw(`SELECT count(*) FILTER (WHERE NOT unkeyed),
       coalesce(sum(play_count) FILTER (WHERE unkeyed), 0),
       (SELECT last_success_at FROM background_service_runs WHERE name = ?)
FROM radio_tracks`, RadioTracksRebuildLoopName).Row().Scan(&listing.TotalTracks, &listing.UnkeyedPlays, &listing.LastRebuiltAt); err != nil {
			return fmt.Errorf("counting radio tracks: %w", err)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}

	listing.Tracks = make([]contracts.RadioTrackSummary, 0, len(rows))
	for _, row := range rows {
		listing.Tracks = append(listing.Tracks, radioTrackSummary(row))
	}
	return listing, nil
}

func radioTrackSummary(row radioTrackListRow) contracts.RadioTrackSummary {
	return contracts.RadioTrackSummary{
		ID:                    row.ID,
		ArtistKey:             row.ArtistKey,
		TitleKey:              row.TitleKey,
		DisplayArtist:         row.DisplayArtist,
		DisplayTitle:          row.DisplayTitle,
		PlayCount:             row.PlayCount,
		StationRowCount:       row.StationRowCount,
		StationFamilyCount:    row.StationFamilyCount,
		ShowCount:             row.ShowCount,
		FirstAiredOn:          row.FirstAiredOn.Format(time.DateOnly),
		LastAiredOn:           row.LastAiredOn.Format(time.DateOnly),
		MBRecordingID:         row.MBRecordingID,
		MBRecordingIDConflict: row.MBRecordingIDConflict,
		KeyCount:              row.KeyCount,
		AlbumPlayCount:        row.AlbumPlayCount,
		LabelPlayCount:        row.LabelPlayCount,
		RebuiltAt:             row.RebuiltAt,
	}
}
