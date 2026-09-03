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
// writer reached by any other spelling is invisible here, and nothing fails: a
// method value, a wrapper under a different name, a model built field by field
// from an empty literal, a helper in another module, a raw SQL insert.
//
// THE TWO HALVES OF THAT BLIND SPOT FAIL IN OPPOSITE DIRECTIONS, and this is the
// sentence to read before deciding a gap is acceptable. For METADATA the runtime
// default covers it: an action this file never saw has no entry in
// contributionMetadataKeys, and the projection publishes nothing for it. For the
// ENTITY REQUEST ID SPACE it does not: contributionEntityRequestActions is what
// tells the gate a row's entity_id is a request id, and an action missing from
// it is judged against whichever catalog row shares that number. A writer of
// entity-request rows that this sweep cannot see is therefore a real defect with
// no test behind it, which is why the model literals below are swept at all.
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
// Cached: four tests read it, the walk parses every non-test file under
// internal/, and the tree cannot change between them within a run.
type sweptTree struct {
	actions []writtenAction
	// unresolved names the call sites whose action could not be read. Carried
	// rather than reported at collection time so that the one test that owns
	// this failure reports it once instead of every reader reporting it.
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
	// refused holds names the sweep must not answer for, whatever else binds
	// them. Two things land here, and both would otherwise produce a CONFIDENT
	// WRONG ANSWER rather than a refusal:
	//
	//   - a name the function RECEIVES (a parameter or a receiver), because a
	//     wrapper's action comes from its caller and any literal bound to that
	//     name elsewhere in the file belongs to a different call;
	//   - a name bound ANYWHERE to something the sweep cannot read, because the
	//     literals it also holds are then only part of what it can be.
	refused map[string]bool
}

func (sc identScope) resolve(name string) ([]writtenAction, bool) {
	if sc.refused[name] {
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
	fileRefused := map[string]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			bindLiterals(spec, fileBound, fileRefused)
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
		sweepNode(fset, fn.Body, scopeOf(fn, fileBound, fileRefused), swept, !isWriterItself)
	}
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok {
			sweepNode(fset, gen, identScope{bound: fileBound, refused: fileRefused}, swept, true)
		}
	}
}

// scopeOf builds the resolution scope for one function: the literals its body
// binds and the file's, minus every name it receives or binds unreadably.
//
// NAMED RESULTS ARE NOT REFUSED. A result is assigned only inside the body, so
// the body's own bindings are its whole set, which is exactly what an ordinary
// local's are.
func scopeOf(fn *ast.FuncDecl, fileBound map[string][]writtenAction, fileRefused map[string]bool) identScope {
	sc := identScope{bound: map[string][]writtenAction{}, refused: map[string]bool{}}
	for name, values := range fileBound {
		sc.bound[name] = values
	}
	for name := range fileRefused {
		sc.refused[name] = true
	}
	refuseFields(fn.Recv, sc.refused)
	if fn.Type != nil {
		refuseFields(fn.Type.Params, sc.refused)
	}
	if fn.Body == nil {
		return sc
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.FuncLit:
			refuseFields(n.Type.Params, sc.refused)
		case *ast.AssignStmt:
			// A COMPOUND ASSIGNMENT BINDS NOTHING ON ITS OWN. `action += "x"`
			// produces whatever the name already held plus "x", so reading its
			// right-hand side as the name's value records a string no writer
			// ever passes. Refusing is the only answer the sweep can give
			// without evaluating the program.
			compound := n.Tok != token.DEFINE && n.Tok != token.ASSIGN
			for i, lhs := range n.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok {
					continue
				}
				if compound || i >= len(n.Rhs) {
					// The other case is a multi-value assignment: what this
					// name receives is a call's result, which is unreadable.
					sc.refused[ident.Name] = true
					continue
				}
				recordLiteral(ident.Name, n.Rhs[i], sc.bound, sc.refused)
			}
		case *ast.RangeStmt:
			// A RANGE VARIABLE RECEIVES ITS VALUE FROM THE SEQUENCE, so it is
			// refused like a parameter. Without this it would inherit whatever
			// the FILE binds to the same name, which is a wrong answer rather
			// than a refusal.
			for _, operand := range []ast.Expr{n.Key, n.Value} {
				if ident, ok := operand.(*ast.Ident); ok {
					sc.refused[ident.Name] = true
				}
			}
		case *ast.ValueSpec:
			bindLiterals(n, sc.bound, sc.refused)
		}
		return true
	})
	return sc
}

func refuseFields(fields *ast.FieldList, refused map[string]bool) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		for _, name := range field.Names {
			refused[name.Name] = true
		}
	}
}

func bindLiterals(spec ast.Spec, bound map[string][]writtenAction, refused map[string]bool) {
	value, ok := spec.(*ast.ValueSpec)
	if !ok {
		return
	}
	for i, name := range value.Names {
		if i >= len(value.Values) {
			continue
		}
		recordLiteral(name.Name, value.Values[i], bound, refused)
	}
}

// recordLiteral binds one name to the strings an expression can produce, and
// REFUSES the name outright when the expression is one it cannot read.
//
// Refusing rather than ignoring is what stops a name assigned a literal in one
// branch and a call result in another from resolving to half its values with
// nothing recorded as unresolved. A partial answer here reads as a complete one
// everywhere downstream.
//
// Nested resolution is not attempted: the expression must be a literal or a
// literal-prefixed concatenation, never another identifier, so this cannot
// follow a chain into a value the sweep would have to guess at.
func recordLiteral(name string, expr ast.Expr, bound map[string][]writtenAction, refused map[string]bool) {
	resolved, ok := resolveActionExpr(expr, identScope{})
	if !ok {
		refused[name] = true
		return
	}
	bound[name] = append(bound[name], resolved...)
}

// sweepNode records the writers inside one node, resolved against one scope.
func sweepNode(fset *token.FileSet, node ast.Node, scope identScope, swept *sweptTree, readModelLiterals bool) {
	if node == nil {
		return
	}
	ast.Inspect(node, func(n ast.Node) bool {
		switch expr := n.(type) {
		case *ast.CallExpr:
			selector, ok := expr.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			writerPrefix, ok := auditActionWriters[selector.Sel.Name]
			if !ok {
				return true
			}
			if len(expr.Args) <= auditActionArgIndex {
				// A writer call with no argument at that position is a spread
				// call, whose action is inside a slice the sweep cannot read.
				// Unresolved rather than skipped, for the reason the header
				// gives.
				swept.unresolved = append(swept.unresolved, fset.Position(n.Pos()).String())
				return true
			}
			recordResolved(fset, expr, writerPrefix, expr.Args[auditActionArgIndex], scope, swept)
			return true

		case *ast.CompositeLit:
			field, ok := auditModelLiterals[compositeTypeName(expr)]
			if !ok || !readModelLiterals {
				return true
			}
			// A CONTAINER OF MODEL LITERALS is the idiomatic batch insert, and
			// its elements carry no type node of their own, so each is read
			// here against the container's element type rather than skipped for
			// having none.
			for _, lit := range modelLiteralsIn(expr) {
				if len(lit.Elts) == 0 {
					// AN EMPTY LITERAL IS A TYPE TOKEN, not a write:
					// `db.Model(&AuditLog{})` names the table for a query. A
					// model built field by field from one is invisible here,
					// which the header states as a bound.
					continue
				}
				value := compositeFieldValue(lit, field)
				if value == nil {
					// A literal that sets other fields and not this one is a
					// write whose action the sweep cannot read: either it is
					// assigned afterwards, or the row records none at all.
					swept.unresolved = append(swept.unresolved, fset.Position(lit.Pos()).String())
					continue
				}
				recordResolved(fset, lit, "", value, scope, swept)
			}
			return true

		default:
			return true
		}
	})
}

// recordResolved reads one write site's action expression and records either the
// actions it produces or the site itself as unresolved.
func recordResolved(fset *token.FileSet, at ast.Node, prefix string, actionExpr ast.Expr,
	scope identScope, swept *sweptTree,
) {
	position := fset.Position(at.Pos()).String()
	resolved, ok := resolveActionExpr(actionExpr, scope)
	if !ok {
		swept.unresolved = append(swept.unresolved, position)
		return
	}
	for _, r := range resolved {
		swept.actions = append(swept.actions, writtenAction{
			action:   prefix + r.action,
			position: position,
			isPrefix: r.isPrefix,
		})
	}
}

// modelLiteralsIn returns the model literals a composite literal holds: itself
// for a plain struct literal, and its elements for a slice, array or map of
// them. Elements of a container carry no type node, which is why they cannot be
// found by the type-name test that reached this container.
func modelLiteralsIn(lit *ast.CompositeLit) []*ast.CompositeLit {
	switch lit.Type.(type) {
	case *ast.ArrayType, *ast.MapType:
	default:
		return []*ast.CompositeLit{lit}
	}
	elements := make([]*ast.CompositeLit, 0, len(lit.Elts))
	for _, element := range lit.Elts {
		if kv, ok := element.(*ast.KeyValueExpr); ok {
			element = kv.Value
		}
		if inner, ok := element.(*ast.CompositeLit); ok {
			elements = append(elements, inner)
		}
	}
	return elements
}

// compositeTypeName is the composite literal's type name without its package
// qualifier, so a model imported under different aliases reads the same, and so
// a container of the model answers its element's name. An element literal of its
// own carries no type node and answers "", which is modelLiteralsIn's job to
// handle.
func compositeTypeName(lit *ast.CompositeLit) string {
	return typeNameOf(lit.Type)
}

func typeNameOf(expr ast.Expr) string {
	switch typ := expr.(type) {
	case *ast.Ident:
		return typ.Name
	case *ast.SelectorExpr:
		return typ.Sel.Name
	case *ast.StarExpr:
		return typeNameOf(typ.X)
	case *ast.ArrayType:
		return typeNameOf(typ.Elt)
	case *ast.MapType:
		return typeNameOf(typ.Value)
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

// entityRequestWriterPath is the path fragment of the files that write entity
// request audit rows. Every action written from one of them names a request.
//
// THE FILE IS THE STRONGER SIGNAL. An action's spelling is what a rename
// changes, and a renamed action that no longer says "entity_request" would slip
// past a name-only tripwire into the map whose missing-entry default is the
// unsafe one. Where the row is written from does not change when it is renamed.
const entityRequestWriterPath = "handlers/community/entity_request"

// namesAnEntityRequest reports whether a swept write site produces an action
// whose entity_id is a request id, by the two signals either of which is enough.
//
// One function so the test that walks the tree and the test that drives
// synthetic sites decide it the same way. A second copy of this expression
// inside a test would pass whatever the tree does, which is what a guard on the
// unsafe default cannot afford.
func namesAnEntityRequest(w writtenAction) bool {
	if strings.Contains(filepath.ToSlash(w.position), entityRequestWriterPath) {
		return true
	}
	return strings.Contains(w.action, "entity_request")
}

// THE ENTITY REQUEST FAMILY IS THE ONE THE ENTITY-TYPE ARMS MUST NOT JUDGE, and
// its membership is hand-maintained. This is the tripwire on that: an action
// naming entity requests that nobody added to the map would be decided against
// whichever show, artist or venue happens to share the request's id.
//
// TWO SIGNALS, either of which is enough: the file the action is written from,
// and the action's own name. The first survives a rename and the second survives
// a writer moving to another file. A false positive from either is a demand for
// a decision rather than a leak.
func TestEntityRequestActionsAreRecognisedByName(t *testing.T) {
	written := map[string]bool{}
	for _, w := range auditActionsWritten(t) {
		written[w.action] = true
		if !namesAnEntityRequest(w) {
			continue
		}
		// PREFIX FAMILIES ARE CHECKED TOO, and this is where the FILE signal
		// earns its place: a family built as `"decide_" + state` records only
		// the literal half, which says nothing about requests, so a name-only
		// test would skip it. Exempting such a family would let it be
		// dispositioned for metadata, which is safe, while its rows are still
		// judged by the entity-type arms, which is not.
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
				"read its entity_id as an id in the table its entity_type names. Add it "+
				"there, NOT to contributionMetadataKeys: an entry in that map silences the "+
				"disposition test while leaving this row judged against whichever catalog "+
				"row shares the request's number.",
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
			t.Errorf("%q is an entity-request action and publishes metadata keys %v: a "+
				"fulfilled request's row is served to EVERY tier, and this metadata carries "+
				"the superseded-payload digest and the requester's id",
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

// A NAME BOUND TO SOMETHING UNREADABLE IS REFUSED ENTIRELY, not answered with
// the half of its values the sweep happens to recognise.
//
// A partial answer reads as a complete one everywhere downstream: nothing is
// recorded as unresolved, so the branch that ships the other action passes both
// the disposition test and the entity-request tripwire.
func TestSweepRefusesAPartiallyReadableLocal(t *testing.T) {
	swept := sweepSource(t, `package p

func mixedLocal(s S, kind string) {
	action := "approve_show"
	if kind == "x" {
		action = actionFor(kind)
	}
	s.LogAction(1, action, "show", 1, nil)
}
`)

	if len(swept.actions) != 0 {
		t.Errorf("actions = %v, want none: one branch of this name is a call result, so "+
			"the literal in the other is not its whole set", swept.actions)
	}
	if len(swept.unresolved) != 1 {
		t.Errorf("unresolved = %v, want exactly the one call site", swept.unresolved)
	}
}

// A NAMED RESULT IS READABLE, because a result is bound only inside the body and
// the body's bindings are therefore its whole set. Refusing it would fail the
// sweep on code the failure message tells the author to write.
func TestSweepReadsANamedResult(t *testing.T) {
	swept := sweepSource(t, `package p

func f(s S) (action string) {
	action = "named_result_action"
	s.LogAction(1, action, "artist", 1, nil)
	return action
}
`)

	exact, _ := sweptExactAndPrefixes(swept)
	if !slices.Equal(exact, []string{"named_result_action"}) {
		t.Errorf("exact = %v, want [named_result_action]", exact)
	}
	if len(swept.unresolved) != 0 {
		t.Errorf("unresolved = %v, want none", swept.unresolved)
	}
}

// A BATCH INSERT IS A WRITE. Its elements carry no type node of their own, so
// without the container's element type they would be invisible: five writers
// already build these literals one at a time, and batching them is a one-line
// change.
func TestSweepReadsModelLiteralsInAContainer(t *testing.T) {
	swept := sweepSource(t, `package p

func g(db DB) {
	db.Create([]adminm.AuditLog{
		{Action: "batched_one", EntityType: "tag"},
		{Action: "batched_two", EntityType: "tag"},
	})
}
`)

	exact, _ := sweptExactAndPrefixes(swept)
	if !slices.Equal(exact, []string{"batched_one", "batched_two"}) {
		t.Errorf("exact = %v, want both batched actions", exact)
	}
}

// AN EMPTY MODEL LITERAL IS A TYPE TOKEN, and a populated one that names no
// action is a write the sweep cannot read.
//
// The distinction matters because `db.Model(&AuditLog{})` appears in this very
// package as a query shell: treating it as an unreadable write would fail the
// guard against the reader rather than against a writer.
func TestSweepDistinguishesATypeTokenFromAnUnreadableWrite(t *testing.T) {
	swept := sweepSource(t, `package p

func queries(db DB) {
	db.Model(&adminm.AuditLog{}).Count(&n)
}

func writes(db DB) {
	db.Create(&adminm.AuditLog{EntityType: "tag", EntityID: 1})
}
`)

	if len(swept.actions) != 0 {
		t.Errorf("actions = %v, want none", swept.actions)
	}
	if len(swept.unresolved) != 1 {
		t.Errorf("unresolved = %v, want exactly the populated literal that names no action",
			swept.unresolved)
	}
}

// A SPREAD CALL HIDES ITS ACTION IN A SLICE, so it is unresolved rather than
// skipped: the sweep declines to answer instead of reporting a writer it never
// read.
func TestSweepRefusesASpreadCall(t *testing.T) {
	swept := sweepSource(t, `package p

func f(s S, args []interface{}) {
	s.LogAction(args...)
}
`)

	if len(swept.actions) != 0 {
		t.Errorf("actions = %v, want none", swept.actions)
	}
	if len(swept.unresolved) != 1 {
		t.Errorf("unresolved = %v, want the spread call site", swept.unresolved)
	}
}

// A PUBLISHED KEY NEEDS A GATE OVER ITS OWN REFERENT, and this is the guard on
// that rule rather than a sentence hoping to be read.
//
// The allowlist's failure direction is safe for WITHHOLDING: an action nobody
// dispositioned publishes nothing. It is not safe for PUBLISHING. A second
// action given keys here ships with whatever those keys name, and every key that
// has ever been a candidate names something the row's own gate did not decide:
// a collection's slug, a reported show's id, a comment's id, a requester's id.
//
// So the set of actions that publish anything is pinned as a list. Growing it is
// a two-line change a reviewer sees, and the entry has to say which gate decides
// the new key's referent per viewer, the way scrubCloneSourceMetadata decides
// the fork attribution's.
var contributionActionsThatPublishKeys = map[string]string{
	contributionCloneAction: "scrubCloneSourceMetadata",
}

func TestOnlyGatedActionsPublishMetadataKeys(t *testing.T) {
	for action, keys := range contributionMetadataKeys {
		gate, expected := contributionActionsThatPublishKeys[action]
		switch {
		case len(keys) > 0 && !expected:
			t.Errorf("%q publishes %v and is not in contributionActionsThatPublishKeys. "+
				"A published key names something the row's own gate did not decide, so it "+
				"needs a per-viewer gate of its own. Add the action here with that gate's "+
				"name, or publish nothing.", action, keys)
		case len(keys) == 0 && expected:
			t.Errorf("%q is listed as publishing keys gated by %s but publishes none, so "+
				"remove it from contributionActionsThatPublishKeys", action, gate)
		}
	}
	for action := range contributionActionsThatPublishKeys {
		if _, ok := contributionMetadataKeys[action]; !ok {
			t.Errorf("contributionActionsThatPublishKeys names %q, which contributionMetadataKeys "+
				"does not disposition at all", action)
		}
	}
}

// A COMPOUND ASSIGNMENT BINDS NOTHING THE SWEEP CAN READ, so the name is
// refused rather than answered with the right-hand side alone.
//
// Reading `action += "purge_entity_request"` as a binding records a string no
// writer ever passes, and the entity-request tripwire then fires on that
// fabricated name and tells the author to register IT. Doing so turns the suite
// green while the action actually written is still absent from the map, and its
// request id is judged against whichever catalog row shares the number.
func TestSweepRefusesACompoundAssignment(t *testing.T) {
	swept := sweepSource(t, `package p

func f(s S, reqID uint) {
	action := "rescue_"
	action += "purge_entity_request"
	s.LogAction(1, action, "artist", reqID, nil)
}
`)

	if len(swept.actions) != 0 {
		t.Errorf("actions = %v, want none: neither half of a compound assignment is the "+
			"string the writer passes", swept.actions)
	}
	if len(swept.unresolved) != 1 {
		t.Errorf("unresolved = %v, want the one call site", swept.unresolved)
	}
}

// A RANGE VARIABLE RECEIVES ITS VALUE FROM THE SEQUENCE, so it is refused like a
// parameter rather than inheriting a file-level binding of the same name.
func TestSweepRefusesARangeVariable(t *testing.T) {
	swept := sweepSource(t, `package p

var action = "file_level_action"

func f(s S, actions []string) {
	for _, action := range actions {
		s.LogAction(1, action, "artist", 1, nil)
	}
}
`)

	if len(swept.actions) != 0 {
		t.Errorf("actions = %v, want none: the loop variable is not the package-level "+
			"binding that shares its name", swept.actions)
	}
	if len(swept.unresolved) != 1 {
		t.Errorf("unresolved = %v, want the one call site", swept.unresolved)
	}
}

// THE TRIPWIRE'S TWO SIGNALS, driven through the function the tree-walking test
// uses rather than through a second copy of its expression.
//
// The file signal is the half that has no other cover: an action renamed away
// from "entity_request", or a prefix family whose literal half never said it,
// carries nothing in its name to catch. Deleting that signal must fail here.
func TestEntityRequestTripwireSignals(t *testing.T) {
	const writerFile = "/repo/backend/internal/api/handlers/community/entity_request.go:412:3"
	const otherFile = "/repo/backend/internal/api/handlers/catalog/artist.go:412:3"

	for _, tc := range []struct {
		name    string
		written writtenAction
		want    bool
		why     string
	}{
		{
			"a prefix family written from the entity-request handlers",
			writtenAction{action: "decide_", position: writerFile, isPrefix: true}, true,
			"its literal half names no request, so only the file it is written from can catch it",
		},
		{
			"an action renamed away from the family's vocabulary",
			writtenAction{action: "requeue_submission", position: writerFile}, true,
			"a rename changes the name and not the file",
		},
		{
			"an action naming requests from somewhere else",
			writtenAction{action: "purge_entity_request", position: otherFile}, true,
			"a writer that moved out of those files still names a request",
		},
		{
			"an ordinary catalog action",
			writtenAction{action: "create_artist", position: otherFile}, false,
			"neither signal fires, and a false positive here would demand a wrong disposition",
		},
	} {
		if got := namesAnEntityRequest(tc.written); got != tc.want {
			t.Errorf("%s: namesAnEntityRequest = %v, want %v: %s", tc.name, got, tc.want, tc.why)
		}
	}
}
