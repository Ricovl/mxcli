// SPDX-License-Identifier: Apache-2.0

package executor

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

	mdlast "github.com/mendixlabs/mxcli/mdl/ast"
)

// projectCheckExemptDocTypes lists the doc types stmtCreateInfo can return that
// deliberately have NO project-existence check, with the reason. An entry here
// is a decision; anything else missing from setFor is the bug this test exists
// to catch.
var projectCheckExemptDocTypes = map[string]string{
	// CREATE MODULE on an existing module is a no-op: execCreateModule prints
	// "already exists" and returns nil (exit 0). `create module M;` is the
	// standard script preamble, so flagging it would be a false positive on
	// essentially every script.
	"module": "CREATE MODULE is idempotent at exec — it prints 'already exists' and exits 0",
}

// docTypesFromSwitch parses validate_duplicates.go and returns, for the named
// function, either the first string literal of every return statement (kind
// "return") or every case-clause string literal (kind "case").
//
// Reading the real source rather than restating the lists is the point: this is
// the "two lists, nothing comparing them" defect class, and a guard that keeps
// its own third copy of the list would join the problem rather than fix it.
func docTypesFromSwitch(t *testing.T, funcName, kind string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "validate_duplicates.go", nil, 0)
	if err != nil {
		t.Fatalf("parse validate_duplicates.go: %v", err)
	}

	out := map[string]bool{}
	var fn *ast.FuncDecl
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == funcName {
			fn = fd
			break
		}
	}
	if fn == nil {
		t.Fatalf("function %s not found in validate_duplicates.go", funcName)
	}

	lit := func(e ast.Expr) (string, bool) {
		bl, ok := e.(*ast.BasicLit)
		if !ok || bl.Kind != token.STRING {
			return "", false
		}
		v, err := strconv.Unquote(bl.Value)
		if err != nil || v == "" {
			return "", false
		}
		return v, true
	}

	ast.Inspect(fn, func(n ast.Node) bool {
		switch kind {
		case "return":
			if r, ok := n.(*ast.ReturnStmt); ok && len(r.Results) > 0 {
				if v, ok := lit(r.Results[0]); ok {
					out[v] = true
				}
			}
		case "case":
			if c, ok := n.(*ast.CaseClause); ok {
				for _, e := range c.List {
					if v, ok := lit(e); ok {
						out[v] = true
					}
				}
			}
		}
		return true
	})
	if len(out) == 0 {
		t.Fatalf("extracted no doc types from %s (%s) — the guard would pass vacuously", funcName, kind)
	}
	return out
}

// TestEveryCreateDocTypeIsProjectChecked is the guard for the defect behind the
// `check --references` under-report: stmtCreateInfo classified more document
// types than projectNameSets.setFor knew about, so a plain CREATE of an
// existing association, rule or javascript action passed `check --references`
// and then died half-way through `exec`, leaving the project part-modified.
//
// Nothing compared the two switches, so the gap was silent. This does.
func TestEveryCreateDocTypeIsProjectChecked(t *testing.T) {
	created := docTypesFromSwitch(t, "stmtCreateKind", "return")
	checked := docTypesFromSwitch(t, "setFor", "case")

	var missing []string
	for dt := range created {
		if checked[dt] {
			continue
		}
		if _, exempt := projectCheckExemptDocTypes[dt]; exempt {
			continue
		}
		missing = append(missing, dt)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("stmtCreateInfo returns doc types that projectNameSets.setFor does not check: %v\n"+
			"A plain CREATE of one of these against a project that already has it passes "+
			"`check --references` and then fails at exec, part-way through the script.\n"+
			"Add a set for it in projectNameSets/loadProjectNameSets/setFor, or, if the CREATE is "+
			"genuinely idempotent at exec, record it in projectCheckExemptDocTypes with the reason.",
			missing)
	}

	// The exemption list must not outlive its reason: an entry that setFor has
	// since grown a case for, or that stmtCreateInfo no longer produces, is
	// stale and would mask a real gap.
	for dt := range projectCheckExemptDocTypes {
		if !created[dt] {
			t.Errorf("projectCheckExemptDocTypes has %q, which stmtCreateInfo no longer returns — drop it", dt)
		}
		if checked[dt] {
			t.Errorf("projectCheckExemptDocTypes has %q, but setFor now checks it — drop the exemption", dt)
		}
	}
}

// TestEveryDropDocTypeIsCreatable is the mirror: stmtDropInfo feeds the
// droppedFromProject registry that suppresses a conflict after a DROP. A doc
// type DROP knows and CREATE does not is harmless, but the reverse means a
// `drop X; create X;` pair reports a conflict the script already resolved.
func TestEveryDropDocTypeIsCreatable(t *testing.T) {
	created := docTypesFromSwitch(t, "stmtCreateKind", "return")
	dropped := docTypesFromSwitch(t, "stmtDropInfo", "return")

	var missing []string
	for dt := range created {
		if dropped[dt] {
			continue
		}
		missing = append(missing, dt)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("stmtCreateInfo returns doc types stmtDropInfo does not classify: %v\n"+
			"`drop X; create X;` would then report a project conflict the script already resolved.",
			missing)
	}
}

// TestIfNotExistsCountsAsIdempotent guards the third idempotency spelling.
//
// stmtCreateInfo reported OR MODIFY and OR REPLACE but not IF NOT EXISTS, so a
// re-runnable domain script — the form that exists precisely to be re-run —
// was told its `create entity if not exists` conflicted with the project, for
// a statement exec skips with "already exists — skipped".
//
// Since #731 every document kind carries the guard (ast.CreateGuard), and
// stmtCreateInfo reads it once for all of them. This walks every known
// statement type rather than restating a list: each one that carries the guard
// and is classified as a create must count as idempotent once guarded — and,
// as the control, not before.
func TestIfNotExistsCountsAsIdempotent(t *testing.T) {
	checked := 0
	for _, stmt := range allKnownStatements() {
		g, ok := stmt.(mdlast.IfNotExistsCreate)
		if !ok {
			continue
		}
		dt, _, before := stmtCreateInfo(stmt)
		if dt == "" {
			continue // not a create the duplicate checks track
		}
		checked++
		if before {
			t.Errorf("%T counts as idempotent without any guard — the control is void", stmt)
			continue
		}
		g.SetCreateIfNotExists(false)
		if _, _, after := stmtCreateInfo(stmt); !after {
			t.Errorf("%T: `create … if not exists` is not idempotent to stmtCreateInfo: "+
				"it would be reported as a conflict for a statement exec cleanly skips", stmt)
		}
	}
	if checked == 0 {
		t.Fatal("no guarded create statement is classified by stmtCreateInfo — the guard would pass vacuously")
	}
}

// createStmtsNotClassified lists the Create*Stmt types in mdl/ast that
// stmtCreateKind deliberately does not classify, with the reason. An entry is
// a decision; any other Create*Stmt it does not classify is the gap
// TestEveryCreateStmtIsClassified exists to catch.
var createStmtsNotClassified = map[string]string{
	"CreateValidationRuleStmt": "a validation rule is anonymous: the statement names the attribute it constrains, and " +
		"exec sets that attribute's rule rather than refusing an existing one",
	"CreateTranslationsStmt": "not a named element: exec's refusals (a translation into the source language) are " +
		"CheckExecRefusals' (ako/mxcli#906)",
	"CreateExternalEntitiesStmt": "a bulk import with no name of its own: each entity it would create is looked up " +
		"in the service contract, which a script-level name check cannot see",
	"CreateAnnotationStmt": "KNOWN GAP (ako/mxcli#557 follow-up): an annotation has no name — exec refuses a plain " +
		"CREATE whose caption's first line matches an existing note in the module, which a qualified-name set " +
		"cannot express",
}

// createStmtTypesInAST returns the name of every type in mdl/ast that is a
// top-level statement (it has an isStatement method) and is spelled Create*.
//
// This is the guard's own source for "every create statement", independent of
// the switches it checks. Comparing stmtCreateKind against setFor alone could
// not see a statement type absent from BOTH — which is how CREATE USER ROLE
// went unchecked while the guard passed (ako/mxcli#557).
func createStmtTypesInAST(t *testing.T) map[string]bool {
	t.Helper()
	dir := filepath.Join("..", "ast")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read mdl/ast: %v", err)
	}
	fset := token.NewFileSet()
	out := map[string]bool{}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, n), nil, 0)
		if err != nil {
			t.Fatalf("parse mdl/ast/%s: %v", n, err)
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Name.Name != "isStatement" || fd.Recv == nil || len(fd.Recv.List) != 1 {
				continue
			}
			typ := fd.Recv.List[0].Type
			if star, ok := typ.(*ast.StarExpr); ok {
				typ = star.X
			}
			if id, ok := typ.(*ast.Ident); ok && strings.HasPrefix(id.Name, "Create") {
				out[id.Name] = true
			}
		}
	}
	if len(out) < 20 {
		t.Fatalf("found only %d Create*Stmt statement types in mdl/ast — the scan is broken and the guard would pass vacuously", len(out))
	}
	return out
}

// stmtTypesInSwitch returns the type names of every `case *ast.X` clause of
// the named function's type switch in validate_duplicates.go.
func stmtTypesInSwitch(t *testing.T, funcName string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "validate_duplicates.go", nil, 0)
	if err != nil {
		t.Fatalf("parse validate_duplicates.go: %v", err)
	}
	out := map[string]bool{}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != funcName {
			continue
		}
		ast.Inspect(fd, func(n ast.Node) bool {
			c, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, e := range c.List {
				if star, ok := e.(*ast.StarExpr); ok {
					if sel, ok := star.X.(*ast.SelectorExpr); ok {
						out[sel.Sel.Name] = true
					}
				}
			}
			return true
		})
	}
	if len(out) == 0 {
		t.Fatalf("no `case *ast.X` clauses in %s — the guard would pass vacuously", funcName)
	}
	return out
}

// TestEveryCreateStmtIsClassified is the independent half of the guard
// (ako/mxcli#557). TestEveryCreateDocTypeIsProjectChecked compares two
// switches with each other, so a create statement neither of them names is
// invisible to it: CREATE USER ROLE passed `check --references` and was then
// refused by exec, part-way through the script, while that guard stayed green.
// This one takes the list of create statements from mdl/ast itself.
func TestEveryCreateStmtIsClassified(t *testing.T) {
	all := createStmtTypesInAST(t)
	classified := stmtTypesInSwitch(t, "stmtCreateKind")

	var missing []string
	for typ := range all {
		if classified[typ] {
			continue
		}
		if _, exempt := createStmtsNotClassified[typ]; exempt {
			continue
		}
		missing = append(missing, typ)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("mdl/ast has create statements stmtCreateKind does not classify: %v\n"+
			"If exec refuses a plain CREATE of one that already exists, `check --references` passes it "+
			"and exec then fails part-way through the script. Classify it in stmtCreateKind (and give it a "+
			"project set in setFor and a drop in stmtDropInfo), or record in createStmtsNotClassified why "+
			"it has no name to collide on.", missing)
	}
	for typ := range createStmtsNotClassified {
		if !all[typ] {
			t.Errorf("createStmtsNotClassified has %s, which is not a create statement in mdl/ast — drop it", typ)
		}
		if classified[typ] {
			t.Errorf("createStmtsNotClassified has %s, but stmtCreateKind now classifies it — drop the exemption", typ)
		}
	}
}
