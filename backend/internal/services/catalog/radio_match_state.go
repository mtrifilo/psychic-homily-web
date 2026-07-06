package catalog

import (
	catalogm "psychic-homily-backend/internal/models/catalog"

	"gorm.io/gorm"
)

// artistRematchMatchStates returns play match_state values eligible for a rematch
// sweep. Force includes rows the matcher previously marked exhausted.
func artistRematchMatchStates(force bool) []string {
	if force {
		return []string{
			catalogm.RadioPlayMatchStateUnmatched,
			catalogm.RadioPlayMatchStateNoMatch,
			catalogm.RadioPlayMatchStateAmbiguous,
		}
	}
	return []string{catalogm.RadioPlayMatchStateUnmatched}
}

func scopePlaysForArtistRematch(q *gorm.DB, tableAlias string, force bool) *gorm.DB {
	return q.Where(tableAlias+".artist_id IS NULL").
		Where(tableAlias+".match_state IN ?", artistRematchMatchStates(force))
}
