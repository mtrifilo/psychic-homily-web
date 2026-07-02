package catalog

import "time"

// ArtistCommunity is one Leiden similarity community's display metadata
// (PSY-1262). The whole table is rebuilt atomically alongside
// artists.community_id by the nightly compute, so rows never mutate in
// place — a partition is immutable until the next recompute replaces it.
type ArtistCommunity struct {
	// ID matches artists.community_id (dense 0..k-1, deterministically
	// numbered by each community's smallest member artist ID). Community 0 is
	// a real id — autoIncrement:false stops GORM from dropping the zero-value
	// PK out of the INSERT (it is assigned, not sequence-generated).
	ID uint `json:"id" gorm:"column:id;primaryKey;autoIncrement:false"`
	// LabelArtistID is the community's highest-strength member — the anchor
	// for the "Around {artist}" display label.
	LabelArtistID uint      `json:"label_artist_id" gorm:"column:label_artist_id"`
	MemberCount   int       `json:"member_count" gorm:"column:member_count"`
	ComputedAt    time.Time `json:"computed_at" gorm:"column:computed_at"`

	LabelArtist Artist `json:"-" gorm:"foreignKey:LabelArtistID"`
}

// TableName specifies the table name for ArtistCommunity.
func (ArtistCommunity) TableName() string {
	return "artist_communities"
}
