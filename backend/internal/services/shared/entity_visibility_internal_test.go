package shared

import (
	"strings"
	"testing"

	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// Every polymorphic entity_type must have a decided position on visibility
// (PSY-1987).
//
// This is the structural guard the ticket exists to add. The leak it closes was
// not a rule written wrong — it was a rule nobody was forced to write:
// `collection` reached six surfaces through a gate that recognised `show` and
// waved everything else through, and no test failed, because there was nothing
// for a missing decision to fail.
//
// So the cost of adding an entity type is one line in entityVisibilityRules plus
// the judgement it forces. That is the point. It does not check behaviour —
// collection_visibility_test.go and the route matrices do that — it checks that
// a DECISION EXISTS, and that the decision reached the SQL the database runs.
//
// It is the comment-family analogue of TestEveryInboxEntityTypeHasADisposition
// and TestEveryShowAddressableRouteHasADisposition, and it is STRONGER than
// both: those two compare against hand-written inventories that a new constant
// can be omitted from, while this one enumerates the model's own map. An entity
// type that can be commented on cannot be invisible to this test.

func TestEveryCommentEntityTypeHasAVisibilityRule(t *testing.T) {
	for entityType := range engagementm.ValidCommentEntityTypes {
		if _, ok := entityVisibilityRules[string(entityType)]; !ok {
			t.Errorf("comment entity type %q has no entry in entityVisibilityRules, so every gate "+
				"that decides it now refuses it. Add the disposition — `ruleAlwaysVisible` only if "+
				"the model genuinely carries no read-time visibility rule, and say which in the "+
				"comment beside it", entityType)
		}
	}
}

// The registry must not outlive the model. An entry for a type nothing can
// comment on is a rule with no rows, and it puts a value into the SQL allowlist
// that no writer produces — harmless today, misleading to the next reader, and
// the shape a rename leaves behind.
func TestEntityVisibilityRegistryHasNoStaleEntries(t *testing.T) {
	for entityType := range entityVisibilityRules {
		if _, ok := engagementm.ValidCommentEntityTypes[engagementm.CommentEntityType(entityType)]; !ok {
			t.Errorf("entityVisibilityRules records %q, which is not a valid comment entity type — "+
				"remove it, or say why a rule outlives its rows", entityType)
		}
	}
}

// The two GATED types are pinned by name, so flipping either to
// `ruleAlwaysVisible` fails here rather than passing quietly and reopening the
// leak. Everything else must be alwaysVisible: a new type arriving with its own
// rule has to change this list, and changing it is the moment somebody reads the
// paragraph above it.
func TestOnlyShowsAndCollectionsAreGated(t *testing.T) {
	gated := map[string]entityVisibilityRule{
		CommentEntityTypeShow:       ruleShow,
		CommentEntityTypeCollection: ruleCollection,
	}
	for entityType, rule := range entityVisibilityRules {
		wantRule, isGated := gated[entityType]
		if isGated {
			if rule != wantRule {
				t.Errorf("%q is recorded as %v, but it has a read-time visibility rule of its own "+
					"and must be gated by it", entityType, rule)
			}
			continue
		}
		if rule != ruleAlwaysVisible {
			t.Errorf("%q is gated by %v but is not in this test's list of gated types — "+
				"add it there with the reason, so the pin and the registry cannot drift", entityType, rule)
		}
	}
}

// The SQL allowlist is derived from the registry, and this checks the derivation
// actually happened rather than trusting that it did: a registry entry that
// never reaches the emitted statement is a decision the database does not know
// about.
func TestRegisteredEntityTypesReachTheEmittedSQL(t *testing.T) {
	sql, _ := VisibleCommentEntitySQL("e.entity_type", "e.entity_id", contracts.ShowViewer{UserID: 7})
	for entityType := range entityVisibilityRules {
		quoted := "'" + entityType + "'"
		if !strings.Contains(registeredEntityTypeList, quoted) {
			t.Errorf("%q is registered but missing from registeredEntityTypeList", entityType)
		}
		if !strings.Contains(sql, quoted) {
			t.Errorf("%q is registered but missing from the emitted predicate", entityType)
		}
	}
}

// Every registered type has its plural alias, because the aliases are built from
// the registry and a build that stopped doing that would refuse a legitimate
// path segment rather than wave a gated one through — a failure that looks like
// a product bug.
func TestEveryRegisteredEntityTypeHasItsPluralAlias(t *testing.T) {
	for entityType := range entityVisibilityRules {
		canonical, ok := entityTypeAliases[entityType+"s"]
		if !ok || canonical != entityType {
			t.Errorf("the plural of %q does not resolve back to it (got %q, present=%v)",
				entityType, canonical, ok)
		}
	}
}

// The Go gate refuses anything unregistered, whatever the caller passes and
// whoever is asking.
//
// Three of the four call-site families pass UNVALIDATED path text straight in,
// so this is not a theoretical input. An admin is included because an admin
// bypass added to the dispatch would be the one way an unregistered type could
// still be served.
func TestEntityVisibleToFailsClosedOnAnUnregisteredType(t *testing.T) {
	// A checker that grants everything, so the only thing that can refuse below
	// is the registry itself.
	permissive := allEntitiesVisibleChecker{}
	viewers := []struct {
		name   string
		viewer contracts.ShowViewer
	}{
		{"anonymous", contracts.ShowViewer{}},
		{"an authenticated caller", contracts.ShowViewer{UserID: 7}},
		{"an admin", contracts.ShowViewer{UserID: 7, IsAdmin: true}},
	}
	unregistered := []string{
		"", "user", "scene", "radio_show", "tag", "collectionx", "sho",
		// The shape a future entity type arrives in: plausible, plural, and
		// nobody has decided about it.
		"festival_edition", "playlists",
	}
	for _, v := range viewers {
		for _, entityType := range unregistered {
			if EntityVisibleTo(permissive, entityType, 1, v.viewer) {
				t.Errorf("EntityVisibleTo answered visible for unregistered type %q to %s", entityType, v.name)
			}
			// The SQL and fan-out spellings must agree with the Go one, or a
			// route refuses what a listing still renders.
			cond, args := CommentEntityRecipientsSQL(entityType, 1, "u.id", "u.is_admin")
			if cond != "1 = 0" || args != nil {
				t.Errorf("CommentEntityRecipientsSQL for unregistered type %q = %q, want a refusal", entityType, cond)
			}
			fence, _, fenced := EntityIdentityFenceSQL(entityType, "t", v.viewer)
			if !fenced || fence != "FALSE" {
				t.Errorf("EntityIdentityFenceSQL for unregistered type %q = (%q, fenced=%v), want a closed fence",
					entityType, fence, fenced)
			}
		}
	}
}

// A registered type is decided by its rule, and case and the plural spelling do
// not change the answer. A gate that "show" passes but "Show" or "shows" slips
// through is not a gate.
func TestEntityVisibleToNormalisesTheSpelling(t *testing.T) {
	denying := noEntitiesVisibleChecker{}
	for _, entityType := range []string{"show", "Show", " SHOW ", "shows", "collection", "COLLECTIONS"} {
		if EntityVisibleTo(denying, entityType, 1, contracts.ShowViewer{UserID: 7}) {
			t.Errorf("a denying checker was bypassed by the spelling %q", entityType)
		}
	}
	// The always-visible arm answers without consulting a checker at all, so a
	// denying one cannot make an artist disappear.
	for _, entityType := range []string{"artist", "Artists", "venue", "release", "label", "festival"} {
		if !EntityVisibleTo(denying, entityType, 1, contracts.ShowViewer{UserID: 7}) {
			t.Errorf("the always-visible arm refused %q", entityType)
		}
	}
}

// A nil checker refuses the types that need one and serves the types that do
// not. A construction bug must fail closed exactly where it matters without
// taking six public surfaces down with it.
func TestEntityVisibleToWithANilChecker(t *testing.T) {
	viewer := contracts.ShowViewer{UserID: 7, IsAdmin: true}
	for _, entityType := range []string{"show", "collection"} {
		if EntityVisibleTo(nil, entityType, 1, viewer) {
			t.Errorf("a nil checker answered visible for %q", entityType)
		}
	}
	for _, entityType := range []string{"artist", "venue", "release", "label", "festival"} {
		if !EntityVisibleTo(nil, entityType, 1, viewer) {
			t.Errorf("a nil checker refused %q, which has no rule for it to have applied", entityType)
		}
	}
}

// The composite carries the args of BOTH arms and carries them in statement
// order, because placeholders in a raw statement bind by POSITION. An arm whose
// binds went missing leaves the statement short and Postgres rejects the whole
// query — every surface that uses it 500s together.
func TestVisibleCommentEntitySQLArgsMatchItsPlaceholders(t *testing.T) {
	for _, v := range []struct {
		name   string
		viewer contracts.ShowViewer
	}{
		{"anonymous", contracts.ShowViewer{}},
		{"an authenticated caller", contracts.ShowViewer{UserID: 7}},
		{"an admin", contracts.ShowViewer{UserID: 7, IsAdmin: true}},
	} {
		sql, args := VisibleCommentEntitySQL("e.entity_type", "e.entity_id", v.viewer)
		if got := strings.Count(sql, "?"); got != len(args) {
			t.Errorf("VisibleCommentEntitySQL for %s emitted %d placeholders for %d args: %s",
				v.name, got, len(args), sql)
		}
	}
}

// allEntitiesVisibleChecker grants everything, so a refusal can only come from
// the registry.
type allEntitiesVisibleChecker struct{}

func (allEntitiesVisibleChecker) ShowVisibleTo(uint, contracts.ShowViewer) bool       { return true }
func (allEntitiesVisibleChecker) CollectionVisibleTo(uint, contracts.ShowViewer) bool { return true }

// noEntitiesVisibleChecker refuses everything, so a pass can only come from the
// always-visible arm.
type noEntitiesVisibleChecker struct{}

func (noEntitiesVisibleChecker) ShowVisibleTo(uint, contracts.ShowViewer) bool       { return false }
func (noEntitiesVisibleChecker) CollectionVisibleTo(uint, contracts.ShowViewer) bool { return false }
