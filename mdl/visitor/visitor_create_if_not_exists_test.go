// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// createIfNotExistsExempt names the create kinds that do not take
// `if not exists`, and why. Everything else createStatement accepts must
// (ako/mxcli#731, ADR-0010 R1: "if not exists on every document type").
var createIfNotExistsExempt = map[string]string{
	"annotation":       "an annotation has no name to test for existence",
	"index":            "an index is an entity member identified by its columns; alter entity … add index if not exists is its guard",
	"validationrule":   "a validation rule is keyed by its attribute, not by a name",
	"navigation":       "a navigation profile always exists; create navigation configures it",
	"translations":     "translations merge into existing texts; there is nothing to leave alone",
	"externalentities": "a bulk import of many entities, not one element",
}

// createNameRE finds where the element name starts in a createOrReplaceCases
// body: the first qualified name in module M, the bare module name, a user
// role name, or a quoted name (demo user, configuration).
var createNameRE = regexp.MustCompile(`\s(M\.\w|M;|Clerk\b|'Default'|'demo')`)

// withIfNotExists writes `if not exists` in its canonical place: after the
// kind's keywords, before the name — `create persistent entity if not exists
// Shop.Audit (…)`, as in the proposal's §3.
func withIfNotExists(t *testing.T, body string) string {
	t.Helper()
	loc := createNameRE.FindStringIndex(body)
	if loc == nil {
		t.Fatalf("no element name found in %q", body)
	}
	return body[:loc[0]] + " if not exists" + body[loc[0]:]
}

func clearCreateGuard(stmts []ast.Statement) {
	for _, s := range stmts {
		if g, ok := s.(ast.IfNotExistsCreate); ok {
			v := reflect.ValueOf(g).Elem().FieldByName("CreateGuard")
			v.Set(reflect.Zero(v.Type()))
		}
	}
}

// Every document kind accepts `create <kind> if not exists <name>`, the built
// statement carries the guard, and apart from the guard it is the statement a
// plain `create` builds — the guard changes whether it runs, not what it says.
func TestCreateIfNotExistsOnEveryDocumentKind(t *testing.T) {
	kinds := createStatementKinds(t)
	for _, kind := range kinds {
		if _, ok := createOrReplaceCases[kind]; !ok {
			t.Errorf("create kind %q has no case in createOrReplaceCases", kind)
		}
	}
	cases := make(map[string]string, len(createOrReplaceCases)+1)
	for k, v := range createOrReplaceCases {
		cases[k] = v
	}
	cases["entity (view)"] = "view entity M.V (Name: String(100)) as (select c.Name as Name from M.Customer as c);"
	cases["entity (non-persistent)"] = "non-persistent entity M.Filter (Q: String(200));"

	for name, body := range cases {
		if _, exempt := createIfNotExistsExempt[name]; exempt {
			continue
		}
		body = strings.TrimSuffix(body, ";") + ";" // every statement ends with ; (valid under mdl 0 and mdl 1)
		for _, header := range []string{"", "mdl 1;\n"} {
			t.Run(name+"/"+strings.TrimSpace(header), func(t *testing.T) {
				guarded := header + "create " + withIfNotExists(t, body)
				plain := header + "create " + body
				g := mustBuild(t, guarded)
				p := mustBuild(t, plain)
				if len(g.Statements) != 1 || len(p.Statements) != 1 {
					t.Fatalf("want one statement each, got %d and %d", len(g.Statements), len(p.Statements))
				}
				guard, ok := g.Statements[0].(ast.IfNotExistsCreate)
				if !ok {
					t.Fatalf("%q built %T, which does not carry the if-not-exists guard", guarded, g.Statements[0])
				}
				if !guard.CreateIfNotExists() {
					t.Errorf("%q: guard not recorded", guarded)
				}
				if guard.CreateGuardContradicts() {
					t.Errorf("%q: reported as contradicting or modify", guarded)
				}
				if pg, ok := p.Statements[0].(ast.IfNotExistsCreate); ok && pg.CreateIfNotExists() {
					t.Errorf("%q: plain create recorded the guard", plain)
				}
				clearCreateGuard(g.Statements)
				clearCreateGuard(p.Statements)
				if !reflect.DeepEqual(g.Statements, p.Statements) {
					t.Errorf("the guard changed what the statement says:\n guarded: %#v\n plain:   %#v",
						g.Statements, p.Statements)
				}
				if len(g.Deprecations) != 0 {
					t.Errorf("the canonical guarded form recorded deprecations %v", deprecationCodes(g))
				}
			})
		}
	}
}

// `create or modify … if not exists` is recorded as such on every kind, so
// check can refuse it (MDL067) — the two guards contradict each other.
func TestCreateOrModifyIfNotExistsIsRecorded(t *testing.T) {
	for name, body := range createOrReplaceCases {
		if _, exempt := createIfNotExistsExempt[name]; exempt {
			continue
		}
		t.Run(name, func(t *testing.T) {
			prog := mustBuild(t, "create or modify "+withIfNotExists(t, body))
			guard, ok := prog.Statements[0].(ast.IfNotExistsCreate)
			if !ok {
				t.Fatalf("built %T without the guard", prog.Statements[0])
			}
			if !guard.CreateGuardContradicts() {
				t.Error("or modify + if not exists not recorded as contradicting")
			}
		})
	}
}

// The exempt kinds refuse the guard at parse time rather than accepting and
// ignoring it — an ignored guard is a plain create that fails, or worse
// rewrites, on the second run.
func TestCreateIfNotExistsRefusedOnExemptKinds(t *testing.T) {
	for name := range createIfNotExistsExempt {
		body, ok := createOrReplaceCases[name]
		if !ok {
			t.Errorf("exempt kind %q is not a create kind", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			src := "create " + strings.Replace(body, " ", " if not exists ", 1)
			if _, errs := Build(src); len(errs) == 0 {
				t.Errorf("%q parsed; an exempt kind must refuse `if not exists`", src)
			}
		})
	}
}
