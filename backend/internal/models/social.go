package models

// Embedded struct for social media links for venues
type Social struct {
	Instagram  *string `gorm:"column:instagram"`
	Facebook   *string `gorm:"column:facebook"`
	Twitter    *string `gorm:"column:twitter"`
	YouTube    *string `gorm:"column:youtube"`
	Spotify    *string `gorm:"column:spotify"`
	SoundCloud *string `gorm:"column:soundcloud"`
	Bandcamp   *string `gorm:"column:bandcamp"`
	Website    *string `gorm:"column:website"`
}
