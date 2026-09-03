package shared

import (
	"reflect"
	"strings"
	"testing"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// Every polymorphic entity_type must have a decided position on visibility.
//
// The failure this guards against is not a rule written wrong. It is a rule
// nobody is forced to write: an entity type reaching six surfaces through a gate
// that recognises some types and waves the rest through fails no test, because
// there is nothing for a missing decision to fail.
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

// THE OTHER TWO VOCABULARIES that reach this gate must also be covered.
//
// The registry is keyed on the COMMENT entity types, but three call-site
// families reach it and they enumerate their types separately: the tag route
// walks catalogm.TagEntityTypes, and the collection-backlinks route walks a set
// that communitym.AllCollectionEntityTypes mirrors. Today the comment set is a
// superset of both, and this test is what keeps that true — a type added to
// tagging alone would otherwise be refused at the gate with nothing saying why,
// which is a broken route rather than a leak, but broken quietly.
func TestEveryGatedVocabularyIsCoveredByTheRegistry(t *testing.T) {
	for _, v := range []struct {
		name  string
		types []string
	}{
		{"catalogm.TagEntityTypes", catalogm.TagEntityTypes},
		{"communitym.AllCollectionEntityTypes", communitym.AllCollectionEntityTypes},
	} {
		for _, entityType := range v.types {
			if _, ok := entityVisibilityRules[entityType]; !ok {
				t.Errorf("%s contains %q, which has no entry in entityVisibilityRules — "+
					"the gate refuses it, so that route answers empty for every caller",
					v.name, entityType)
			}
		}
	}
}

// gatedVocabularies is the union of every vocabulary that reaches this registry.
//
// THE UNION, not the comment set. The registry is keyed on the comment entity
// types because those are the values the comment family's rows carry, but the
// tag routes and the collection-backlinks route walk their own sets, and a type
// that is taggable without being commentable is a legitimate thing to add. A
// staleness check written against the comment set alone would force such a type
// to become commentable, or force the guard to be loosened, and the guard is
// the security half.
func gatedVocabularies() map[string]bool {
	all := make(map[string]bool)
	for entityType := range engagementm.ValidCommentEntityTypes {
		all[string(entityType)] = true
	}
	for _, entityType := range catalogm.TagEntityTypes {
		all[entityType] = true
	}
	for _, entityType := range communitym.AllCollectionEntityTypes {
		all[entityType] = true
	}
	return all
}

// The registry must not outlive the vocabularies. An entry for a type nothing
// can comment on, tag or collect is a rule with no rows, and it puts a value
// into the SQL allowlist that no writer produces: harmless today, misleading to
// the next reader, and the shape a rename leaves behind.
func TestEntityVisibilityRegistryHasNoStaleEntries(t *testing.T) {
	known := gatedVocabularies()
	for entityType := range entityVisibilityRules {
		if !known[entityType] {
			t.Errorf("entityVisibilityRules records %q, which appears in no vocabulary that "+
				"reaches this gate. Remove it, or say why a rule outlives its rows", entityType)
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

// `ruleAlwaysVisible` is a CLAIM about a model, and this is what keeps it
// honest.
//
// Every alwaysVisible entry says, in prose, that its model carries no read-time
// visibility rule. Prose does not fail. The day somebody adds `is_private` to
// artists, or turns `label.status` into a moderation state, five gates keep
// answering true and nothing moves, which is the same missing-decision failure
// one level down and the reason to spend a test on it.
//
// It is a NAME-BASED heuristic over the GORM struct, not a proof: it catches a
// column whose name is in the privacy vocabulary below, and it cannot catch a
// rule expressed some other way. What it does buy is that the common shape —
// a boolean or enum column arriving on a model these gates wave through — stops
// being silent.
//
// Fields that DO match and are genuinely not visibility rules carry a waiver
// with the reason. A waiver is a decision, so adding one is the moment somebody
// reads what it is for.
func TestAlwaysVisibleModelsHaveNoPrivacyColumn(t *testing.T) {
	models := map[string]interface{}{
		string(engagementm.CommentEntityArtist):   catalogm.Artist{},
		string(engagementm.CommentEntityVenue):    catalogm.Venue{},
		string(engagementm.CommentEntityRelease):  catalogm.Release{},
		string(engagementm.CommentEntityLabel):    catalogm.Label{},
		string(engagementm.CommentEntityFestival): catalogm.Festival{},
	}

	// Waivers, keyed "entityType.FieldName". Each says why the field is not a
	// read-time visibility rule.
	waived := map[string]string{
		"label.Status":                    "active/inactive/defunct — whether the label still operates; every listing serves all three",
		"festival.Status":                 "announced/confirmed/cancelled/completed — the event's lifecycle, not who may read it",
		"venue.Verified":                  "gates the street ADDRESS at field level (Venue.PublicAddress), never the row",
		"artist.StreamingDiscoveryStatus": "the enrichment pipeline's own state — how far discovery has got, not who may read the artist",
	}

	// The shapes a read-time rule arrives in. Substring-matched against the field
	// name so `IsPublic`, `PublicationStatus` and `DeletedAt` all land.
	privacyVocabulary := []string{
		"Public", "Private", "Visib", "Deleted", "Published", "Status", "Verified", "Moderat",
		"Hidden", "Draft", "Archiv", "Restrict", "Embargo",
	}

	// PROVENANCE TIMESTAMPS ARE NOT RULES, and they are excluded BY NAME rather
	// than by type. A `time.Time` type test run before the vocabulary match would
	// skip `PublishedAt` and `HiddenAt` as well, and those are exactly the shape
	// a read-time rule arrives in: a gate turns `published_at IS NOT NULL` into a
	// WHERE clause as readily as it turns a boolean into one. So the vocabulary
	// decides first, and only these four names are waived for being provenance.
	//
	// gorm.DeletedAt is not in the list and must not be: it is not a time.Time,
	// and a soft-delete column genuinely IS a read-time rule these gates would
	// have to honour.
	provenanceTimestamps := map[string]bool{
		"CreatedAt":      true,
		"UpdatedAt":      true,
		"LastVerifiedAt": true,
		"VerifiedAt":     true,
	}
	isProvenanceTimestamp := func(field reflect.StructField) bool {
		if !provenanceTimestamps[field.Name] {
			return false
		}
		t := field.Type
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		return t == reflect.TypeOf(time.Time{})
	}

	// Every alwaysVisible type must appear above, or the sweep silently covers
	// less than the registry does.
	for entityType, rule := range entityVisibilityRules {
		if rule != ruleAlwaysVisible {
			continue
		}
		if _, ok := models[entityType]; !ok {
			t.Errorf("%q is recorded alwaysVisible but this test has no model for it, so the "+
				"claim that its model carries no privacy column is unchecked", entityType)
		}
	}

	usedWaivers := make(map[string]bool, len(waived))
	for entityType, model := range models {
		modelType := reflect.TypeOf(model)
		for i := 0; i < modelType.NumField(); i++ {
			field := modelType.Field(i)
			// VOCABULARY FIRST. A field that matches nothing is not a candidate,
			// whatever its type; a field that DOES match is skipped only if it is
			// one of the named provenance timestamps.
			matched := ""
			for _, term := range privacyVocabulary {
				if strings.Contains(field.Name, term) {
					matched = term
					break
				}
			}
			if matched == "" {
				continue
			}
			if isProvenanceTimestamp(field) {
				continue
			}
			key := entityType + "." + field.Name
			if _, ok := waived[key]; ok {
				usedWaivers[key] = true
				continue
			}
			t.Errorf("%s.%s looks like a read-time visibility rule (matched %q), but %q is "+
				"registered ruleAlwaysVisible — every gate in this package serves it to "+
				"everybody. Give the entity type a real rule, or add a waiver saying why "+
				"this field is not one.", entityType, field.Name, matched, entityType)
		}
	}

	// A waiver for a field that no longer exists is a claim about nothing, and it
	// hides the removal of the field it was excusing.
	for key := range waived {
		if !usedWaivers[key] {
			t.Errorf("waiver %q matches no field — remove it, or say what it now excuses", key)
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
			if fence, _ := EntityIdentityFenceSQL(entityType, "t", v.viewer); fence != "FALSE" {
				t.Errorf("EntityIdentityFenceSQL for unregistered type %q = %q, want a closed fence",
					entityType, fence)
			}
			if fence, _ := VisibleEntityExistsSQL(entityType, "t.entity_id", v.viewer); fence != "FALSE" {
				t.Errorf("VisibleEntityExistsSQL for unregistered type %q = %q, want a closed fence",
					entityType, fence)
			}
		}
	}
}

// ONLY THE CANONICAL SPELLING IS REGISTERED. Case variants and plurals are
// refused, and that is the convergence this gate owes the service behind it:
// engagement.validateCommentEntityType and catalogm.IsValidTagEntityType both
// compare exactly, so a gate that accepted more spellings than they do would
// answer one refusal for a gated id and a different one for the same id spelled
// another way. One vocabulary, one answer.
func TestEntityVisibleToTakesOnlyTheCanonicalSpelling(t *testing.T) {
	denying := noEntitiesVisibleChecker{}
	permissive := allEntitiesVisibleChecker{}
	viewer := contracts.ShowViewer{UserID: 7}

	// The gated types: refused by their own rule under the canonical spelling.
	for _, entityType := range []string{"show", "collection"} {
		if EntityVisibleTo(denying, entityType, 1, viewer) {
			t.Errorf("a denying checker was bypassed for %q", entityType)
		}
	}
	// The always-visible arm answers without consulting a checker at all, so a
	// denying one cannot make an artist disappear.
	for _, entityType := range []string{"artist", "venue", "release", "label", "festival"} {
		if !EntityVisibleTo(denying, entityType, 1, viewer) {
			t.Errorf("the always-visible arm refused %q", entityType)
		}
	}
	// Every non-canonical spelling of a REGISTERED type is unregistered, even
	// with a checker that grants everything. The permissive checker is what makes
	// this about the vocabulary rather than about the rule.
	for _, entityType := range []string{
		"Show", " SHOW ", "shows", "Collection", "COLLECTIONS", "collections",
		"Artist", "artists", "venues", " artist",
	} {
		if EntityVisibleTo(permissive, entityType, 1, viewer) {
			t.Errorf("the gate accepted the non-canonical spelling %q", entityType)
		}
	}
}

// The gate's vocabulary and the comment service's vocabulary must answer alike,
// or the pair of refusals sorts one kind of id from another.
//
// engagementm.ValidCommentEntityTypes is the service's set. Every member of it
// must be registered, and no spelling outside it may resolve.
func TestGateVocabularyMatchesTheCommentServiceVocabulary(t *testing.T) {
	for entityType := range engagementm.ValidCommentEntityTypes {
		if _, ok := entityVisibilityRuleFor(string(entityType)); !ok {
			t.Errorf("the service accepts %q but the gate does not resolve it, so the route "+
				"refuses a type the service would have served", entityType)
		}
	}
	// The other direction — a registered type no vocabulary accepts — is
	// TestEntityVisibilityRegistryHasNoStaleEntries, over the same set with the
	// same helper. One assertion, one home.
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
