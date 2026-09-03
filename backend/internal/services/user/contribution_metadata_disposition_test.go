package user

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Every audit action written anywhere under internal/ must have a recorded
// position on what its metadata may publish.
//
// audit_logs.metadata and entity_edit_audit_logs.metadata are written by around
// a hundred call sites in nine packages and read by GET
// /users/{username}/contributions, which takes optional auth and serves the
// `contributions` field visible by default. Every writer therefore decides, by
// accident, what becomes public under its actor's own username. The allowlist in
// contributor_profile.go is the answer; this file is what stops a writer from
// shipping without consulting it.
//
// It walks the SOURCE rather than a registry the writers call, because there is
// no such registry: the writers pass an action string to a service, and any
// enforcement inside that service would be a run-time refusal on a fire-and-
// forget write nobody watches. A build-time sweep of the call sites is the one
// place the omission is loud.
//
// IT PINS THE INVENTORY, NOT THE BEHAVIOUR. That an allowlisted key actually
// survives the projection, and that an unlisted one does not, is
// contributor_profile_test.go's job.

// auditActionWriters are the two methods that write an audit row, and the
// ARGUMENT INDEX of the action each one decides.
//
// LogAction takes the action itself. LogEntityEdit takes an entity type and the
// timeline synthesises `edit_<entity_type>` for the rows it writes
// (entityEditAuditQuery), so the type at that index becomes an action here by
// the same concatenation.
var auditActionWriters = map[string]struct {
	argIndex int
	prefix   string
}{
	"LogAction":     {argIndex: 1},
	"LogEntityEdit": {argIndex: 1, prefix: "edit_"},
}

// writtenAction is one action string a writer can produce, with where it was
// found so a failure names a file rather than a string.
type writtenAction struct {
	action   string
	position string
	// isPrefix marks an action assembled at run time from a literal prefix and
	// a value, so only the prefix is known statically.
	isPrefix bool
}

// backendRoot walks up from the test's working directory to the module root, so
// the sweep does not depend on where `go test` was invoked from.
func backendRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot read working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %q, so the audit-writer sweep has no tree to walk", dir)
		}
		dir = parent
	}
}

// collectWrittenAuditActions parses every non-test file under internal/ and
// returns the actions its audit writers produce.
//
// UNRESOLVABLE IS A FAILURE, not a skip. An action the sweep cannot read is an
// action nobody dispositioned, and skipping it would make the guard quietly
// partial in exactly the case it exists for.
func collectWrittenAuditActions(t *testing.T) []writtenAction {
	t.Helper()
	root := filepath.Join(backendRoot(t), "internal")

	var found []writtenAction
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("cannot parse %s: %v", path, parseErr)
		}

		// Literal assignments to each identifier in the WHOLE FILE, so a call
		// site passing a local variable resolves to the strings that variable
		// can hold. File-wide rather than function-wide on purpose: it
		// over-approximates when one name is reused across functions, and an
		// over-approximation demands MORE dispositions than the writers need,
		// which is the direction that cannot leak.
		literals := stringLiteralsByIdent(file)

		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			writer, ok := auditActionWriters[selector.Sel.Name]
			if !ok {
				return true
			}
			if len(call.Args) <= writer.argIndex {
				return true
			}
			position := fset.Position(call.Pos()).String()
			resolved, ok := resolveActionExpr(call.Args[writer.argIndex], literals)
			if !ok {
				t.Errorf("%s: cannot read the action this audit writer produces.\n"+
					"The metadata allowlist is keyed by action, so an action the sweep "+
					"cannot resolve is one nobody dispositioned. Pass a string literal, "+
					"a literal prefix concatenated with a value, or a local variable "+
					"assigned string literals in the same file.", position)
				return true
			}
			for _, r := range resolved {
				found = append(found, writtenAction{
					action:   writer.prefix + r.action,
					position: position,
					isPrefix: r.isPrefix,
				})
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("cannot walk %s: %v", root, err)
	}
	if len(found) == 0 {
		t.Fatalf("the sweep of %s found no audit writers at all, so it is guarding nothing", root)
	}
	return found
}

// stringLiteralsByIdent maps each identifier assigned a string literal anywhere
// in the file to every literal it is assigned. Both `x := "a"` and `x = "b"`
// count, so a variable set in one branch and reset in another resolves to both.
func stringLiteralsByIdent(file *ast.File) map[string][]writtenAction {
	literals := map[string][]writtenAction{}
	ast.Inspect(file, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assign.Lhs {
			ident, ok := lhs.(*ast.Ident)
			if !ok || i >= len(assign.Rhs) {
				continue
			}
			// Nested resolution is not attempted: the right-hand side must be a
			// literal or a literal-prefixed concatenation, never another
			// variable, so this cannot follow a chain into a value the sweep
			// would have to guess at.
			if resolved, ok := resolveActionExpr(assign.Rhs[i], nil); ok {
				literals[ident.Name] = append(literals[ident.Name], resolved...)
			}
		}
		return true
	})
	return literals
}

// resolveActionExpr reads the action strings one expression can produce.
//
// Three shapes are readable, and everything else answers false:
//
//   - a string literal, which is the action;
//   - a string literal concatenated with anything, which is a PREFIX;
//   - an identifier the file assigns string literals to.
func resolveActionExpr(expr ast.Expr, literals map[string][]writtenAction) ([]writtenAction, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return nil, false
		}
		value, err := strconv.Unquote(e.Value)
		if err != nil {
			return nil, false
		}
		return []writtenAction{{action: value}}, true

	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return nil, false
		}
		left, ok := e.X.(*ast.BasicLit)
		if !ok || left.Kind != token.STRING {
			return nil, false
		}
		prefix, err := strconv.Unquote(left.Value)
		if err != nil {
			return nil, false
		}
		return []writtenAction{{action: prefix, isPrefix: true}}, true

	case *ast.Ident:
		assigned, ok := literals[e.Name]
		if !ok || len(assigned) == 0 {
			return nil, false
		}
		return assigned, true
	}
	return nil, false
}

// dispositionedAction reports whether an action has a recorded position: an
// exact entry, or membership in a family whose prefix was decided.
func dispositionedAction(action string) bool {
	if _, ok := contributionMetadataKeys[action]; ok {
		return true
	}
	for _, prefix := range contributionMetadataWithheldPrefixes {
		if strings.HasPrefix(action, prefix) {
			return true
		}
	}
	return false
}

func TestEveryAuditActionHasAMetadataDisposition(t *testing.T) {
	undecided := map[string]string{}
	for _, written := range collectWrittenAuditActions(t) {
		if written.isPrefix {
			if !containsString(contributionMetadataWithheldPrefixes, written.action) {
				undecided[written.action+"… (prefix)"] = written.position
			}
			continue
		}
		if !dispositionedAction(written.action) {
			undecided[written.action] = written.position
		}
	}

	if len(undecided) == 0 {
		return
	}
	names := make([]string, 0, len(undecided))
	for action := range undecided {
		names = append(names, action+"  ("+undecided[action]+")")
	}
	sort.Strings(names)
	t.Errorf("%d audit action(s) have no recorded position on what their metadata may "+
		"publish:\n  %s\n\naudit_logs.metadata is served under the actor's own username by "+
		"GET /users/{username}/contributions, which takes optional auth and is visible by "+
		"default, so an undispositioned action is a writer deciding by accident what becomes "+
		"public. Add each to contributionMetadataKeys with the keys it may publish, or to nil "+
		"to publish none; a family assembled from a literal prefix goes in "+
		"contributionMetadataWithheldPrefixes instead.",
		len(undecided), strings.Join(names, "\n  "))
}

// A stale entry is a claim about nothing, and it hides the removal of an action
// a reader believes is still dispositioned.
func TestContributionMetadataKeysHasNoStaleEntries(t *testing.T) {
	written := map[string]bool{}
	writtenPrefixes := []string{}
	for _, w := range collectWrittenAuditActions(t) {
		if w.isPrefix {
			writtenPrefixes = append(writtenPrefixes, w.action)
			continue
		}
		written[w.action] = true
	}

	var stale []string
	for action := range contributionMetadataKeys {
		if written[action] {
			continue
		}
		// An exact entry that a prefix family also covers is not stale: it is a
		// member spelled out so its keys can differ from the family's.
		covered := false
		for _, prefix := range writtenPrefixes {
			if strings.HasPrefix(action, prefix) {
				covered = true
				break
			}
		}
		if !covered {
			stale = append(stale, action)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("%d entr(ies) in contributionMetadataKeys name actions no writer under "+
			"internal/ produces:\n  %v", len(stale), stale)
	}

	for _, prefix := range contributionMetadataWithheldPrefixes {
		if !containsString(writtenPrefixes, prefix) {
			t.Errorf("contributionMetadataWithheldPrefixes lists %q, which no writer under "+
				"internal/ produces", prefix)
		}
	}
}

// THE ENTITY REQUEST FAMILY IS THE ONE THE ENTITY-TYPE ARMS MUST NOT JUDGE, and
// its membership is hand-maintained. This is the tripwire on that: an action
// naming entity requests that nobody added to the map would be decided against
// whichever show, artist or venue happens to share the request's id.
//
// The match is on the name because the name is what the writers control, and a
// false positive here is a demand for a decision rather than a leak.
func TestEntityRequestActionsAreRecognisedByName(t *testing.T) {
	for _, written := range collectWrittenAuditActions(t) {
		if written.isPrefix || !strings.Contains(written.action, "entity_request") {
			continue
		}
		if !contributionEntityRequestActions[written.action] {
			t.Errorf("%s: %q names entity requests but is not in "+
				"contributionEntityRequestActions, so the timeline's entity-type arms will "+
				"read its entity_id as an id in the table its entity_type names. Add it, or "+
				"rename the action if its entity_id really is a catalog id.",
				written.position, written.action)
		}
	}

	written := map[string]bool{}
	for _, w := range collectWrittenAuditActions(t) {
		written[w.action] = true
	}
	for action := range contributionEntityRequestActions {
		if !written[action] {
			t.Errorf("contributionEntityRequestActions names %q, which no writer under "+
				"internal/ produces", action)
		}
	}
}

func containsString(haystack []string, needle string) bool {
	for _, candidate := range haystack {
		if candidate == needle {
			return true
		}
	}
	return false
}
