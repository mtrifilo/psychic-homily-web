package shared

// PSY-1292: callback invoked when an artist's musicbrainz_artist_id is first written
// (fill-when-empty). The service container wires this to eager discography import,
// gated on ENABLE_ARTIST_DISCOGRAPHY_SWEEP. Lives in shared to avoid import cycles
// between pipeline, enrich, and discography.

var OnArtistMBIDStamped func(artistID uint)

// NotifyArtistMBIDStamped is called by MBID-stamping writers after a successful persist.
func NotifyArtistMBIDStamped(artistID uint) {
	if OnArtistMBIDStamped != nil {
		OnArtistMBIDStamped(artistID)
	}
}
