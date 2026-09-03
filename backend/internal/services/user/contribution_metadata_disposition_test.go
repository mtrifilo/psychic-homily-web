package user

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
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
// forget write nobody watches. A typed action constant would be checked by the
// compiler in the offending package, which is better still and is a change to
// every call site rather than to this one file.
//
// IT PINS THE INVENTORY, NOT THE BEHAVIOUR. That an allowlisted key actually
// survives the projection, and that an unlisted one does not, is
// contributor_profile_test.go's job.

// auditActionArgIndex is the position of the deciding argument in both writers'
// signatures: LogAction(actorID, ACTION, …) and LogEntityEdit(actorID, TYPE, …).
const auditActionArgIndex = 1

// auditActionWriters maps each audit-writing method to the prefix the timeline
// puts in front of the argument at auditActionArgIndex.
//
// LogAction is passed the action itself. LogEntityEdit is passed an entity type,
// and entityEditAuditQuery synthesises `edit_<entity_type>` for the rows it
// writes, so the type becomes an action here by that same concatenation.
var auditActionWriters = map[string]string{
	"LogAction":     "",
	"LogEntityEdit": "edit_",
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

// sweptTree is the result of the one source sweep this file performs.
//
// Cached: three tests read it, the walk parses every non-test file under
// internal/, and the tree cannot change between them within a run.
type sweptTree struct {
	actions []writtenAction
	// unresolved names the call sites whose action could not be read. Carried
	// rather than reported at collection time so that the one test that owns
	// this failure reports it once instead of all three reporting it.
	unresolved []string
	err        error
}

var sweepOnce = sync.OnceValue(sweepAuditWriters)

// auditActionsWritten returns the swept tree, failing the calling test if the
// sweep itself could not run.
func auditActionsWritten(t *testing.T) []writtenAction {
	t.Helper()
	swept := sweepOnce()
	if swept.err != nil {
		t.Fatalf("the audit-writer sweep failed: %v", swept.err)
	}
	if len(swept.actions) == 0 {
		t.Fatal("the sweep found no audit writers at all, so it is guarding nothing")
	}
	return swept.actions
}

// backendRoot walks up from the test's working directory to the module root, so
// the sweep does not depend on where `go test` was invoked from.
func backendRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot read working directory: %w", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above %q, so the sweep has no tree to walk", dir)
		}
		dir = parent
	}
}

// sweepAuditWriters parses every non-test file under internal/ and returns the
// actions its audit writers produce.
//
// UNRESOLVABLE IS A FAILURE, not a skip. An action the sweep cannot read is an
// action nobody dispositioned, and skipping it would make the guard quietly
// partial in exactly the case it exists for.
func sweepAuditWriters() sweptTree {
	base, err := backendRoot()
	if err != nil {
		return sweptTree{err: err}
	}
	root := filepath.Join(base, "internal")

	var swept sweptTree
	fset := token.NewFileSet()

	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return fmt.Errorf("cannot parse %s: %w", path, parseErr)
		}

		// String literals bound to each identifier in the WHOLE FILE, so a call
		// site passing a local variable resolves to the strings that variable
		// can hold. File-wide rather than function-wide on purpose: it
		// over-approximates when one name is reused across functions, and an
		// over-approximation demands MORE dispositions than the writers need,
		// which is the direction that cannot leak.
		//
		// Built LAZILY: 25 of the 500-odd files under internal/ hold an audit
		// writer, and the rest would pay a second full AST walk for a map
		// nothing reads.
		var literals map[string][]writtenAction
		fileLiterals := func() map[string][]writtenAction {
			if literals == nil {
				literals = stringLiteralsByIdent(file)
			}
			return literals
		}

		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			prefix, ok := auditActionWriters[selector.Sel.Name]
			if !ok || len(call.Args) <= auditActionArgIndex {
				return true
			}
			position := fset.Position(call.Pos()).String()
			resolved, ok := resolveActionExpr(call.Args[auditActionArgIndex], fileLiterals())
			if !ok {
				swept.unresolved = append(swept.unresolved, position)
				return true
			}
			for _, r := range resolved {
				swept.actions = append(swept.actions, writtenAction{
					action:   prefix + r.action,
					position: position,
					isPrefix: r.isPrefix,
				})
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		return sweptTree{err: walkErr}
	}
	return swept
}

// stringLiteralsByIdent maps each identifier bound to a string literal anywhere
// in the file to every literal it can hold. Assignments (`x := "a"`, `x = "b"`)
// and declarations (`const x = "a"`, `var x = "a"`) both count, so a variable set
// in one branch and reset in another resolves to both, and hoisting an action
// into a package constant does not break the sweep.
func stringLiteralsByIdent(file *ast.File) map[string][]writtenAction {
	literals := map[string][]writtenAction{}
	record := func(name string, expr ast.Expr) {
		// Nested resolution is not attempted: the right-hand side must be a
		// literal or a literal-prefixed concatenation, never another variable,
		// so this cannot follow a chain into a value the sweep would guess at.
		if resolved, ok := resolveActionExpr(expr, nil); ok {
			literals[name] = append(literals[name], resolved...)
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch decl := node.(type) {
		case *ast.AssignStmt:
			for i, lhs := range decl.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || i >= len(decl.Rhs) {
					continue
				}
				record(ident.Name, decl.Rhs[i])
			}
		case *ast.ValueSpec:
			for i, name := range decl.Names {
				if i < len(decl.Values) {
					record(name.Name, decl.Values[i])
				}
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
//   - an identifier the file binds string literals to.
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
		bound, ok := literals[e.Name]
		if !ok || len(bound) == 0 {
			return nil, false
		}
		return bound, true
	}
	return nil, false
}

// dispositionedAction reports whether an EXACT action has a recorded position.
//
// Exact entries only. A prefix family's members are never seen as exact actions
// by the sweep, so an action spelled out as a literal has to be spelled out in
// the map too, rather than inheriting a family's disposition by the letters it
// starts with.
func dispositionedAction(action string) bool {
	if _, ok := contributionMetadataKeys[action]; ok {
		return true
	}
	return contributionEntityRequestActions[action]
}

func TestEveryAuditActionHasAMetadataDisposition(t *testing.T) {
	undecided := map[string]string{}
	for _, written := range auditActionsWritten(t) {
		if written.isPrefix {
			if !slices.Contains(contributionMetadataWithheldPrefixes, written.action) {
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
	for action, position := range undecided {
		names = append(names, action+"  ("+position+")")
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

// AN ACTION THE SWEEP CANNOT READ IS ONE NOBODY DISPOSITIONED, so the sweep
// refuses to guess at it. Owned by its own test because the failure is about the
// guard's reach rather than about any one action.
func TestEveryAuditWriterNamesAReadableAction(t *testing.T) {
	auditActionsWritten(t)
	for _, position := range sweepOnce().unresolved {
		t.Errorf("%s: cannot read the action this audit writer produces.\n"+
			"The metadata allowlist is keyed by action, so an action the sweep cannot "+
			"resolve is one nobody dispositioned. Pass a string literal, a literal prefix "+
			"concatenated with a value, or an identifier bound to string literals in the "+
			"same file.", position)
	}
}

// A stale entry is a claim about nothing, and it hides the removal of an action
// a reader believes is still dispositioned.
func TestContributionMetadataKeysHasNoStaleEntries(t *testing.T) {
	written := map[string]bool{}
	var writtenPrefixes []string
	for _, w := range auditActionsWritten(t) {
		if w.isPrefix {
			writtenPrefixes = append(writtenPrefixes, w.action)
			continue
		}
		written[w.action] = true
	}

	var stale []string
	for action := range contributionMetadataKeys {
		if !written[action] {
			stale = append(stale, action)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("%d entr(ies) in contributionMetadataKeys name actions no writer under "+
			"internal/ produces:\n  %v", len(stale), stale)
	}

	for _, prefix := range contributionMetadataWithheldPrefixes {
		if !slices.Contains(writtenPrefixes, prefix) {
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
	written := map[string]bool{}
	for _, w := range auditActionsWritten(t) {
		written[w.action] = true
		if w.isPrefix || !strings.Contains(w.action, "entity_request") {
			continue
		}
		if !contributionEntityRequestActions[w.action] {
			t.Errorf("%s: %q names entity requests but is not in "+
				"contributionEntityRequestActions, so the timeline's entity-type arms will "+
				"read its entity_id as an id in the table its entity_type names. Add it, or "+
				"rename the action if its entity_id really is a catalog id.",
				w.position, w.action)
		}
	}

	for action := range contributionEntityRequestActions {
		if !written[action] {
			t.Errorf("contributionEntityRequestActions names %q, which no writer under "+
				"internal/ produces", action)
		}
		// The family is dispositioned by its membership here, so an entry that
		// ALSO appears in the allowlist publishing keys would be two answers to
		// one question.
		if len(contributionMetadataKeys[action]) > 0 {
			t.Errorf("%q is an entity-request action and publishes metadata keys %v: these "+
				"rows reach only the requester and admins, and their metadata carries the "+
				"superseded-payload digest and the requester's id",
				action, contributionMetadataKeys[action])
		}
	}
}

// THE SHAPES THE SWEEP ACCEPTS, pinned against synthetic source rather than
// against the tree.
//
// The tree spells every action as an inline literal today, so the identifier and
// declaration branches are exercised by nothing that ships. They exist so that
// hoisting an action into a constant, which is the obvious refactor, does not
// fail the guard in a package nobody touched.
func TestResolveActionExprReadsTheAcceptedShapes(t *testing.T) {
	const src = `package p

const hoisted = "const_action"

var declared = "var_action"

func f(s S, entityType string) {
	s.LogAction(1, "literal_action", "artist", 1, nil)
	s.LogAction(1, hoisted, "artist", 1, nil)
	s.LogAction(1, declared, "artist", 1, nil)
	s.LogAction(1, "prefix_"+entityType, "artist", 1, nil)
	local := "local_action"
	if entityType == "x" {
		local = "other_local_action"
	}
	s.LogAction(1, local, "artist", 1, nil)
	s.LogEntityEdit(1, "scene", 1, nil)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "synthetic.go", src, 0)
	if err != nil {
		t.Fatalf("cannot parse the synthetic source: %v", err)
	}
	literals := stringLiteralsByIdent(file)

	var exact, prefixes []string
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		writerPrefix, ok := auditActionWriters[selector.Sel.Name]
		if !ok {
			return true
		}
		resolved, ok := resolveActionExpr(call.Args[auditActionArgIndex], literals)
		if !ok {
			t.Errorf("the sweep could not read the action at %s", fset.Position(call.Pos()))
			return true
		}
		for _, r := range resolved {
			if r.isPrefix {
				prefixes = append(prefixes, writerPrefix+r.action)
				continue
			}
			exact = append(exact, writerPrefix+r.action)
		}
		return true
	})

	slices.Sort(exact)
	exact = slices.Compact(exact)
	slices.Sort(prefixes)

	wantExact := []string{
		"const_action", "edit_scene", "literal_action",
		"local_action", "other_local_action", "var_action",
	}
	if !slices.Equal(exact, wantExact) {
		t.Errorf("exact actions = %v, want %v", exact, wantExact)
	}
	if !slices.Equal(prefixes, []string{"prefix_"}) {
		t.Errorf("prefixes = %v, want [prefix_]", prefixes)
	}
}

// AN EXPRESSION THE SWEEP CANNOT READ ANSWERS FALSE, which is what turns an
// unreadable call site into a failure rather than into a silent gap.
func TestResolveActionExprRefusesWhatItCannotRead(t *testing.T) {
	const src = `package p

func f(s S, action string) {
	s.LogAction(1, action, "artist", 1, nil)
	s.LogAction(1, resolveIt(), "artist", 1, nil)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "synthetic.go", src, 0)
	if err != nil {
		t.Fatalf("cannot parse the synthetic source: %v", err)
	}
	literals := stringLiteralsByIdent(file)

	var refused int
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "LogAction" {
			return true
		}
		if _, ok := resolveActionExpr(call.Args[auditActionArgIndex], literals); !ok {
			refused++
		}
		return true
	})

	if refused != 2 {
		t.Errorf("refused %d unreadable actions, want 2: an unbound parameter and a call "+
			"result are both actions the sweep must decline rather than guess at", refused)
	}
}
