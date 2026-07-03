package catalog

import "time"

// Scene is the lazily-materialized scene registry row (PSY-1339). Scenes are
// still computed aggregations (services/catalog/scene.go) — a row exists here
// only once something id-keyed references the scene (a follow, a curated
// description). Identity anchor is the scope: Metro (CBSA code) for US metro
// scenes, the literal (City, State) for fallback scenes; Slug is a display
// artifact of the principal city, kept unique for lookups.
type Scene struct {
	ID          uint      `gorm:"primaryKey;column:id"`
	Metro       *string   `gorm:"column:metro"`
	City        string    `gorm:"not null;column:city"`
	State       string    `gorm:"not null;column:state"`
	Slug        string    `gorm:"not null;column:slug"`
	Description *string   `gorm:"column:description"`
	CreatedAt   time.Time `gorm:"not null;column:created_at"`
}

// TableName specifies the table name for Scene.
func (Scene) TableName() string { return "scenes" }
