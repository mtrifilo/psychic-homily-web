package catalog

import (
	"strings"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// =============================================================================
// A TAG'S DISPLAYED COUNT IS ALSO ITS ORDERING KEY
// =============================================================================
//
// The /tags listing renders a number beside every tag and orders the listing by
// popularity, and the two have to be the same expression. When they are not, the
// page is SELECTED on one key and SHOWN with another: a tag displaying 0 sits
// above a tag displaying 40, and a reader who can see the order but not the
// gated rows learns from the position that the 0 is not really 0. Ordering by a
// count that includes withheld rows publishes those rows as a rank.
//
// So each facet scope has ONE aggregate, built here. The listing LEFT JOINs it to
// order itself and then reads the same aggregate for the numbers it prints.
// `tags.usage_count`, the denormalised column, is not the ordering key for either
// public tag listing, GET /tags or GET /tags/search; services/shared/tag_usage.go
// names the places it is still read and why none of them is this.
//
// EVERY SCOPE IS PUBLIC TIER. These listings are anonymous, and a shared ranking
// that varied by credential would report the difference between two callers.

// tagUsageCountAlias is the alias the listing binds the count aggregate to.
// Distinct from `tags` and from every table the aggregates name, so the join
// cannot shadow the outer query's own alias.
const tagUsageCountAlias = "tag_usage_counts"

// tagUsageCountQuery is a (tag_id, count) aggregate over ALL tags in one facet
// scope, carried as SQL plus binds so the same statement can be read for the
// numbers and joined for the ordering.
//
// Unrestricted by tag id: the ordering is computed before the page is chosen, so
// an aggregate that only knew the page's tags could not have selected it.
type tagUsageCountQuery struct {
	sql  string
	args []interface{}
}

// countsFor returns the aggregate restricted to tagIDs, as tag_id → count.
//
// A tag with no counted rows is ABSENT from the map, and callers read a missing
// key as zero, which is the same contract shared.VisibleTagUsageCounts states.
func (q tagUsageCountQuery) countsFor(db *gorm.DB, tagIDs []uint) (map[uint]int64, error) {
	out := make(map[uint]int64, len(tagIDs))
	if db == nil || len(tagIDs) == 0 {
		return out, nil
	}
	type countRow struct {
		TagID uint
		Count int64
	}
	var rows []countRow
	args := append(append([]interface{}{}, q.args...), tagIDs)
	err := db.Raw(
		"SELECT tag_id, count FROM ("+q.sql+") AS "+tagUsageCountAlias+" WHERE tag_id IN ?",
		args...,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.TagID] = r.Count
	}
	return out, nil
}

// leftJoin renders the aggregate as a LEFT JOIN onto tagIDExpr, plus its binds.
//
// LEFT, so a tag the aggregate counts nothing for still appears in the listing
// with a NULL the ORDER BY coalesces to zero. An inner join would drop it, and a
// tag applied only to gated entities would then vanish from a listing that has
// always shown it.
//
// tagIDExpr is SQL the CALLER controls and is a literal in the calling code.
func (q tagUsageCountQuery) leftJoin(tagIDExpr string) (string, []interface{}) {
	return "LEFT JOIN (" + q.sql + ") " + tagUsageCountAlias +
		" ON " + tagUsageCountAlias + ".tag_id = " + tagIDExpr, q.args
}

// orderBySQL is the popularity ordering over this aggregate: the visible count
// descending, name ascending as the tiebreaker the listing has always used.
func tagUsageCountOrderBySQL() string {
	return "COALESCE(" + tagUsageCountAlias + ".count, 0) DESC, tags.name ASC"
}

// visibleTagUsageCountQuery returns the aggregate for one facet scope.
//
// entityType empty is the GLOBAL count the /tags browse renders. Otherwise the
// count is scoped to that entity type, and for show and festival it is
// TRANSITIVE through the lineup, because no show carries a genre tag directly
// and a facet reading 0 beside three matching shows dead-ends the reader.
//
// cities narrows the show scope to the active city filter and is ignored for
// every other type, which is the rule computeEntityTypeTagCounts already states.
//
// WHICH TYPES ARE TRANSITIVE is artistLineupJunctions' decision, read here
// rather than restated as a switch: a second list of the same two types is a
// second place for the pair to disagree, and disagreeing means a type routed to
// a builder that has no junction for it.
func (s *TagService) visibleTagUsageCountQuery(entityType string, cities []contracts.CityStateFilter) tagUsageCountQuery {
	if entityType == "" {
		sql, args := shared.VisibleTagUsageCountSQL()
		return tagUsageCountQuery{sql: sql, args: args}
	}
	if entityType == catalogm.TagEntityShow && len(cities) > 0 {
		return transitiveArtistTagUsageInShowCitiesQuery(cities)
	}
	if _, transitive := artistLineupJunctions[entityType]; transitive {
		return transitiveArtistTagUsageQuery(entityType)
	}
	return directEntityTypeTagUsageQuery(entityType)
}

// directEntityTypeTagUsageQuery counts entity_tags rows of one entity type.
//
// THE TAGGED ENTITY'S OWN RULE, from the registry, spliced unconditionally: this
// count is rendered on the same anonymous listing whose membership is filtered,
// so counting rows the listing withholds would report by subtraction how many
// carry each tag. VisibleEntityExistsSQL answers TRUE for a type with no rule,
// so there is no per-type branch here to keep in step with the registry.
//
// `show` and `festival` never reach this function: visibleTagUsageCountQuery
// routes both to the transitive branch above, which carries the container's rule
// from the same registry.
func directEntityTypeTagUsageQuery(entityType string) tagUsageCountQuery {
	visible, visibleArgs := shared.VisibleEntityExistsSQL(
		entityType, "entity_tags.entity_id", contracts.ShowViewer{})
	// Placeholders bind by POSITION: the entity type is compared first, the
	// visibility arguments follow it.
	args := []interface{}{entityType}
	args = append(args, visibleArgs...)
	return tagUsageCountQuery{
		sql: "SELECT entity_tags.tag_id AS tag_id, COUNT(*) AS count" +
			" FROM entity_tags WHERE entity_tags.entity_type = ?" +
			" AND " + visible +
			" GROUP BY entity_tags.tag_id",
		args: args,
	}
}

// artistLineupJunctions is the lineup table each transitively-counted container
// type is billed through, keyed by the registry's own entity-type spelling.
//
// One key means one entry: within this file the junction table and its container
// column are read from the container type rather than passed beside it, so a
// caller cannot pair `show` with the festival junction and get a count that
// silently answers about the wrong table.
var artistLineupJunctions = map[string]struct {
	table             string
	containerIDColumn string
}{
	catalogm.TagEntityShow:     {"show_artists", "show_id"},
	catalogm.TagEntityFestival: {"festival_artists", "festival_id"},
}

// artistLineupArtistIDColumn is the artist column on every lineup junction.
const artistLineupArtistIDColumn = "artist_id"

// transitiveArtistTagUsageQuery counts DISTINCT container entities (shows or
// festivals) whose lineup includes an artist carrying the tag.
//
// THE CONTAINER CARRIES ITS OWN VISIBILITY RULE, looked up in the registry by
// containerEntityType rather than written out per caller. A show is gated and a
// festival is not, and this function does not know which is which: it splices in
// whatever shared.VisibleEntityExistsSQL answers, which is TRUE for a type with
// no rule and an EXISTS probe for one that has it. A container type that gains a
// rule gains this term in the same edit, and the facet count then still agrees
// with the listing beside it.
//
// A containerEntityType with no lineup junction counts NOTHING rather than
// counting everything, which is the same direction the registry's own zero value
// takes. visibleTagUsageCountQuery routes on this same map, so the branch is
// unreachable through it; it is the answer for a caller that arrives another way.
func transitiveArtistTagUsageQuery(containerEntityType string) tagUsageCountQuery {
	junction, ok := artistLineupJunctions[containerEntityType]
	if !ok {
		return tagUsageCountQuery{sql: "SELECT 0::bigint AS tag_id, 0::bigint AS count WHERE FALSE"}
	}
	containerVisible, containerVisibleArgs := shared.VisibleEntityExistsSQL(
		containerEntityType,
		junction.table+"."+junction.containerIDColumn,
		contracts.ShowViewer{},
	)
	// Placeholders bind by POSITION, so the artist type (in the JOIN) is appended
	// before the container's arguments (in the WHERE that follows it).
	args := []interface{}{catalogm.TagEntityArtist}
	args = append(args, containerVisibleArgs...)
	return tagUsageCountQuery{
		sql: "SELECT entity_tags.tag_id AS tag_id," +
			" COUNT(DISTINCT " + junction.table + "." + junction.containerIDColumn + ") AS count" +
			" FROM " + junction.table +
			" JOIN entity_tags ON entity_tags.entity_type = ?" +
			" AND entity_tags.entity_id = " + junction.table + "." + artistLineupArtistIDColumn +
			" WHERE " + containerVisible +
			" GROUP BY entity_tags.tag_id",
		args: args,
	}
}

// transitiveArtistTagUsageInShowCitiesQuery is the city-scoped show variant.
//
// It filters on the denormalized shows.(city, state), which is the predicate
// GetUpcomingShows uses for its multi-city filter. Sharing that predicate is
// load-bearing: a facet count derived from a different filter than the list can
// offer a non-zero chip that dead-ends at "0 shows".
//
// An empty cities set falls back to the unscoped query rather than emitting an
// empty disjunction.
func transitiveArtistTagUsageInShowCitiesQuery(cities []contracts.CityStateFilter) tagUsageCountQuery {
	if len(cities) == 0 {
		return transitiveArtistTagUsageQuery(catalogm.TagEntityShow)
	}
	conds := make([]string, 0, len(cities))
	args := []interface{}{catalogm.TagEntityArtist}
	for _, cs := range cities {
		conds = append(conds, "(shows.city = ? AND shows.state = ?)")
		args = append(args, cs.City, cs.State)
	}
	// The show rule, from the same registry the unscoped variant reads, applied
	// to the `shows` row this query has ALREADY joined for the city filter. The
	// unscoped variant has no such join and takes the EXISTS form instead; both
	// come from entityVisibilityRuleFor, so neither is a second copy of the rule.
	//
	// The city filter narrows WHICH shows are counted and says nothing about who
	// may see them, so the two terms are ANDed rather than one standing in for
	// the other.
	showVisible, showVisibleArgs := shared.EntityIdentityFenceSQL(
		catalogm.TagEntityShow, "shows", contracts.ShowViewer{})
	args = append(args, showVisibleArgs...)
	return tagUsageCountQuery{
		sql: "SELECT entity_tags.tag_id AS tag_id," +
			" COUNT(DISTINCT show_artists.show_id) AS count" +
			" FROM show_artists" +
			" JOIN entity_tags ON entity_tags.entity_type = ?" +
			" AND entity_tags.entity_id = show_artists.artist_id" +
			" JOIN shows ON shows.id = show_artists.show_id" +
			" WHERE (" + strings.Join(conds, " OR ") + ")" +
			" AND " + showVisible +
			" GROUP BY entity_tags.tag_id",
		args: args,
	}
}
