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

// Every audit action THIS SWEEP CAN SEE must have a recorded position on what
// its metadata may publish.
//
// audit_logs.metadata and entity_edit_audit_logs.metadata are written from over
// a hundred places and read by GET /users/{username}/contributions, which takes
// optional auth and serves the `contributions` field visible by default. Every
// writer therefore decides, by accident, what becomes public under its actor's
// own username. The allowlist in contributor_profile.go is the answer; this file
// is what stops a writer from shipping without consulting it.
//
// WHAT IT SEES, stated as a bound rather than as a claim about the whole tree:
// non-test files under internal/, and within them the two audit-service methods
// named in auditActionWriters plus composite literals of the audit MODEL, which
// is how five writers record an action without going through the service. A
// writer reached by any other spelling — a differently named wrapper method, a
// helper in another module, a raw SQL insert — is invisible here, and nothing
// fails. The runtime default is what covers that case: an action this file never
// saw is an action contributionMetadataKeys has no entry for, and the projection
// publishes nothing for it.
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

// auditModelLiterals are the audit MODELS whose composite literals record an
// action without calling the service, and the field each one records it in.
//
// Five writers build one of these and Create it directly, three of them with a
// real actor id, so their rows reach a public contributions timeline exactly
// like a LogAction row does. Keyed on the type name alone because the model is
// imported under a package alias that differs between packages.
var auditModelLiterals = map[string]string{
	"AuditLog": "Action",
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
		sweepFile(fset, file, &swept)
		return nil
	})
	if walkErr != nil {
		return sweptTree{err: walkErr}
	}
	return swept
}

// identScope is what an identifier passed to a writer may be resolved against.
//
// SCOPED PER FUNCTION, and that is the point rather than an optimisation. A
// file-wide table resolves a wrapper's `action` PARAMETER to whatever literal
// some other function in the file happens to bind to a variable of that name,
// which is a confident WRONG answer where the sweep's whole value is that it
// answers or fails. Shadowed names are refused for the same reason.
type identScope struct {
	// bound holds literals bound to a name by an assignment or declaration
	// within the function, or at file level.
	bound map[string][]writtenAction
	// shadowed holds names the function receives rather than binds:
	// parameters, results and receivers, of the function and of any literal
	// inside it. A name here is REFUSED even when something binds it too, so a
	// name reused both ways fails loudly rather than resolving to half its
	// values.
	shadowed map[string]bool
}

func (sc identScope) resolve(name string) ([]writtenAction, bool) {
	if sc.shadowed[name] {
		return nil, false
	}
	bound, ok := sc.bound[name]
	if !ok || len(bound) == 0 {
		return nil, false
	}
	return bound, true
}

// sweepFile records every action the file's audit writers produce.
//
// It walks the file ONCE PER FUNCTION so each call site is resolved against the
// names in scope where it appears, plus the file's own declarations.
func sweepFile(fset *token.FileSet, file *ast.File, swept *sweptTree) {
	fileBound := map[string][]writtenAction{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			bindLiterals(spec, fileBound)
		}
	}

	// Bodies first, each with its own scope; then the file's declarations, for
	// a writer called from a package-level initialiser.
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		// THE WRITER'S OWN BODY IS NOT A WRITE SITE. LogAction builds the audit
		// model from the action its CALLER passed, so reading its literal would
		// report the method's parameter as an unreadable action while the real
		// call sites are already recorded one frame up.
		_, isWriterItself := auditActionWriters[fn.Name.Name]
		sweepNode(fset, fn.Body, scopeOf(fn, fileBound), swept, !isWriterItself)
	}
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok {
			sweepNode(fset, gen, identScope{bound: fileBound, shadowed: map[string]bool{}}, swept, true)
		}
	}
}

// scopeOf builds the resolution scope for one function: the literals its body
// binds and the file's, minus every name it or a literal inside it receives.
func scopeOf(fn *ast.FuncDecl, fileBound map[string][]writtenAction) identScope {
	sc := identScope{bound: map[string][]writtenAction{}, shadowed: map[string]bool{}}
	for name, values := range fileBound {
		sc.bound[name] = values
	}
	shadowFields(fn.Recv, sc.shadowed)
	if fn.Type != nil {
		shadowFields(fn.Type.Params, sc.shadowed)
		shadowFields(fn.Type.Results, sc.shadowed)
	}
	if fn.Body == nil {
		return sc
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.FuncLit:
			shadowFields(n.Type.Params, sc.shadowed)
			shadowFields(n.Type.Results, sc.shadowed)
		case *ast.AssignStmt:
			for i, lhs := range n.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || i >= len(n.Rhs) {
					continue
				}
				recordLiteral(ident.Name, n.Rhs[i], sc.bound)
			}
		case *ast.ValueSpec:
			bindLiterals(n, sc.bound)
		}
		return true
	})
	return sc
}

func shadowFields(fields *ast.FieldList, shadowed map[string]bool) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		for _, name := range field.Names {
			shadowed[name.Name] = true
		}
	}
}

func bindLiterals(spec ast.Spec, bound map[string][]writtenAction) {
	value, ok := spec.(*ast.ValueSpec)
	if !ok {
		return
	}
	for i, name := range value.Names {
		if i < len(value.Values) {
			recordLiteral(name.Name, value.Values[i], bound)
		}
	}
}

// recordLiteral binds one name to the strings an expression can produce.
//
// Nested resolution is not attempted: the expression must be a literal or a
// literal-prefixed concatenation, never another identifier, so this cannot
// follow a chain into a value the sweep would have to guess at.
func recordLiteral(name string, expr ast.Expr, bound map[string][]writtenAction) {
	if resolved, ok := resolveActionExpr(expr, identScope{}); ok {
		bound[name] = append(bound[name], resolved...)
	}
}

// sweepNode records the writers inside one node, resolved against one scope.
func sweepNode(fset *token.FileSet, node ast.Node, scope identScope, swept *sweptTree, readModelLiterals bool) {
	if node == nil {
		return
	}
	ast.Inspect(node, func(n ast.Node) bool {
		var prefix string
		var actionExpr ast.Expr

		switch expr := n.(type) {
		case *ast.CallExpr:
			selector, ok := expr.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			writerPrefix, ok := auditActionWriters[selector.Sel.Name]
			if !ok || len(expr.Args) <= auditActionArgIndex {
				return true
			}
			prefix, actionExpr = writerPrefix, expr.Args[auditActionArgIndex]

		case *ast.CompositeLit:
			field, ok := auditModelLiterals[compositeTypeName(expr)]
			if !ok || !readModelLiterals {
				return true
			}
			actionExpr = compositeFieldValue(expr, field)
			if actionExpr == nil {
				// A literal that sets no action field records no action: the
				// zero value is the empty string, which no disposition covers
				// and the projection publishes nothing for.
				return true
			}

		default:
			return true
		}

		position := fset.Position(n.Pos()).String()
		resolved, ok := resolveActionExpr(actionExpr, scope)
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
}

// compositeTypeName is the composite literal's type name without its package
// qualifier, so a model imported under different aliases reads the same.
func compositeTypeName(lit *ast.CompositeLit) string {
	switch typ := lit.Type.(type) {
	case *ast.Ident:
		return typ.Name
	case *ast.SelectorExpr:
		return typ.Sel.Name
	}
	return ""
}

func compositeFieldValue(lit *ast.CompositeLit, field string) ast.Expr {
	for _, element := range lit.Elts {
		kv, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := kv.Key.(*ast.Ident); ok && key.Name == field {
			return kv.Value
		}
	}
	return nil
}

// resolveActionExpr reads the action strings one expression can produce.
//
// Three shapes are readable, and everything else answers false:
//
//   - a string literal, which is the action;
//   - a string literal concatenated with anything, which is a PREFIX;
//   - an identifier the SCOPE binds string literals to, and that the enclosing
//     function does not merely receive.
func resolveActionExpr(expr ast.Expr, scope identScope) ([]writtenAction, bool) {
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
		return scope.resolve(e.Name)
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
		"contributionMetadataWithheldPrefixes instead.\n\nIF THE ACTION'S entity_id IS AN "+
		"entity_requests ID rather than an id in the table its entity_type names, the entry "+
		"belongs in contributionEntityRequestActions instead, and that map's missing-entry "+
		"default is the UNSAFE one: a row it does not name is judged against whichever "+
		"catalog row shares the request's number.",
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
		if !strings.Contains(w.action, "entity_request") {
			continue
		}
		// PREFIX FAMILIES ARE CHECKED TOO. An action built as
		// `"decide_entity_request_" + state` reaches the timeline as a whole
		// action naming a request, and exempting it here would let the family
		// be dispositioned for metadata while its rows are still judged by the
		// entity-type arms.
		if w.isPrefix {
			t.Errorf("%s: %q builds an action naming entity requests at run time. "+
				"contributionEntityRequestActions is an exact-match set, so no member of "+
				"this family can be registered in it: spell each action out as a literal.",
				w.position, w.action)
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

	// NO STALE-ENTRY CHECK ON THIS MAP, unlike contributionMetadataKeys, and the
	// asymmetry is the point: a missing entry here is judged by the wrong arms,
	// so it is the unsafe direction, while a stale one only withholds rows for
	// an action nothing writes. Demanding that every entry name a live writer
	// would make a RETIRED action's entry unshippable, and rows written under
	// the old name outlive the writer.
	for action := range contributionEntityRequestActions {
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

// sweepSource runs the real sweep over one synthetic file, so these tests
// exercise the same path the tree walk does rather than a simplified stand-in.
func sweepSource(t *testing.T, src string) sweptTree {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "synthetic.go", src, 0)
	if err != nil {
		t.Fatalf("cannot parse the synthetic source: %v", err)
	}
	var swept sweptTree
	sweepFile(fset, file, &swept)
	return swept
}

func sweptExactAndPrefixes(swept sweptTree) (exact, prefixes []string) {
	for _, w := range swept.actions {
		if w.isPrefix {
			prefixes = append(prefixes, w.action)
			continue
		}
		exact = append(exact, w.action)
	}
	slices.Sort(exact)
	exact = slices.Compact(exact)
	slices.Sort(prefixes)
	prefixes = slices.Compact(prefixes)
	return exact, prefixes
}

// THE SHAPES THE SWEEP ACCEPTS, pinned against synthetic source rather than
// against the tree.
//
// The tree spells almost every action as an inline literal, so the declaration
// branches are exercised by little that ships. They exist so that hoisting an
// action into a constant, which is the obvious refactor, does not fail the guard
// in a package nobody touched.
func TestSweepReadsTheAcceptedShapes(t *testing.T) {
	swept := sweepSource(t, `package p

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

// The MODEL literal, which five writers use instead of the service.
func g(db DB) {
	db.Create(&adminm.AuditLog{
		ActorID:    &actorID,
		Action:     "model_literal_action",
		EntityType: "tag",
	})
}
`)

	if len(swept.unresolved) != 0 {
		t.Errorf("the sweep could not read %v", swept.unresolved)
	}
	exact, prefixes := sweptExactAndPrefixes(swept)

	wantExact := []string{
		"const_action", "edit_scene", "literal_action", "local_action",
		"model_literal_action", "other_local_action", "var_action",
	}
	if !slices.Equal(exact, wantExact) {
		t.Errorf("exact actions = %v, want %v", exact, wantExact)
	}
	if !slices.Equal(prefixes, []string{"prefix_"}) {
		t.Errorf("prefixes = %v, want [prefix_]", prefixes)
	}
}

// A WRAPPER'S PARAMETER IS REFUSED, NOT SUBSTITUTED.
//
// The scope is per function precisely so that this file cannot answer the
// wrapper's action with a literal some other function binds to a variable of the
// same name. A confident wrong answer here is worse than no guard at all: it
// would report a disposition for an action nobody writes while the action that
// IS written passes unexamined, including past the entity-request tripwire.
func TestSweepRefusesAWrappersParameter(t *testing.T) {
	swept := sweepSource(t, `package p

func approve(s S, id uint) {
	action := "approve_show"
	s.LogAction(id, action, "show", id, nil)
}

func record(s S, actorID uint, action string, entityType string, id uint) {
	s.LogAction(actorID, action, entityType, id, nil)
}
`)

	exact, _ := sweptExactAndPrefixes(swept)
	if !slices.Equal(exact, []string{"approve_show"}) {
		t.Errorf("exact actions = %v, want [approve_show]: the wrapper's parameter must "+
			"resolve to nothing, never to the other function's local", exact)
	}
	if len(swept.unresolved) != 1 {
		t.Errorf("unresolved = %v, want exactly the wrapper's call site", swept.unresolved)
	}
}

// A NAME THE FUNCTION RECEIVES IS REFUSED EVEN WHEN IT ALSO BINDS IT, so a name
// used both ways fails loudly instead of resolving to half its values.
func TestSweepRefusesAShadowedName(t *testing.T) {
	swept := sweepSource(t, `package p

func mixed(s S, action string, flag bool) {
	if flag {
		action = "reassigned_action"
	}
	s.LogAction(1, action, "artist", 1, nil)
}
`)

	if len(swept.actions) != 0 {
		t.Errorf("actions = %v, want none: the name is a parameter as well as an "+
			"assignment target, so its full set of values is not knowable here", swept.actions)
	}
	if len(swept.unresolved) != 1 {
		t.Errorf("unresolved = %v, want exactly the one call site", swept.unresolved)
	}
}

// AN EXPRESSION THE SWEEP CANNOT READ ANSWERS FALSE, which is what turns an
// unreadable call site into a failure rather than into a silent gap.
func TestSweepRefusesWhatItCannotRead(t *testing.T) {
	swept := sweepSource(t, `package p

func f(s S) {
	s.LogAction(1, resolveIt(), "artist", 1, nil)
	s.LogAction(1, someStruct.Field, "artist", 1, nil)
}
`)

	if len(swept.actions) != 0 {
		t.Errorf("actions = %v, want none", swept.actions)
	}
	if len(swept.unresolved) != 2 {
		t.Errorf("refused %v, want two: a call result and a field selector are both "+
			"actions the sweep must decline rather than guess at", swept.unresolved)
	}
}
