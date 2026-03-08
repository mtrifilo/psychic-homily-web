package models

import "time"

// LabelStatus represents the operational status of a label
type LabelStatus string

const (
	LabelStatusActive   LabelStatus = "active"
	LabelStatusInactive LabelStatus = "inactive"
	LabelStatusDefunct  LabelStatus = "defunct"
)

// Label represents a record label
type Label struct {
	ID          uint        `gorm:"primaryKey"`
	Name        string      `gorm:"not null"`
	Slug        *string     `gorm:"column:slug;uniqueIndex"`
	City        *string     `gorm:"column:city"`
	State       *string     `gorm:"column:state"`
	Country     *string     `gorm:"column:country"`
	FoundedYear *int        `gorm:"column:founded_year"`
	Status      LabelStatus `gorm:"column:status;not null;default:'active'"`
	Description *string     `gorm:"column:description"`
	Social      Social      `gorm:"embedded"`
	CreatedAt   time.Time   `gorm:"not null"`
	UpdatedAt   time.Time   `gorm:"not null"`

	// Relationships
	Artists  []Artist  `gorm:"many2many:artist_labels;"`
	Releases []Release `gorm:"many2many:release_labels;"`
}

// TableName specifies the table name for Label
func (Label) TableName() string {
	return "labels"
}

// ArtistLabel represents the junction table between artists and labels
type ArtistLabel struct {
	ArtistID uint `gorm:"primaryKey;column:artist_id"`
	LabelID  uint `gorm:"primaryKey;column:label_id"`
}

// TableName specifies the table name for ArtistLabel
func (ArtistLabel) TableName() string {
	return "artist_labels"
}

// ReleaseLabel represents the junction table between releases and labels
type ReleaseLabel struct {
	ReleaseID     uint    `gorm:"primaryKey;column:release_id"`
	LabelID       uint    `gorm:"primaryKey;column:label_id"`
	CatalogNumber *string `gorm:"column:catalog_number"`
}

// TableName specifies the table name for ReleaseLabel
func (ReleaseLabel) TableName() string {
	return "release_labels"
}
