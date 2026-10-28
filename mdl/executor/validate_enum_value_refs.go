// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// `Module.Enum.Key` inside an expression was never resolved
// (mendixlabs/mxcli#1472). The enumeration was known — the expression checker
// reads its values to refuse a quoted string compared with one (E001) — but
// whether the KEY it names exists was never asked, so
//
//	declare $p Enumeration(NanoflowCommons.Platform) = NanoflowCommons.Platform.NoSuchKey;
//
// reported "All references valid", and exec wrote the expression for mxbuild
// to refuse — measured on 11.13.0, CE1613 "The selected enumeration value
// 'NanoflowCommons.Platform.NoSuchKey' no longer exists." at the activity.
//
// Every expression a statement carries is walked, whatever slot holds it — a
// declare, an assignment, a condition, a call argument, a return — because the
// visitor gives every one of them the same shape: a qualified name of three
// parts is `Module` plus `Enum.Key`. Run in the reference tier, which exec runs
// too, so the two cannot disagree.
//
// Could-not-establish is silence: an enumeration neither the project nor the
// script defines (System's are not stored in the project), one the script
// ALTERs, renames or moves, and a name that is also an entity's (a qualified
// attribute in an XPath constraint has the same three parts).

// validateEnumValueRefs reports the enumeration keys that name no value.
func validateEnumValueRefs(ctx *ExecContext, prog *ast.Program, sc *scriptContext) []error {
	if prog == nil || !ctx.Connected() {
		return nil
	}
	script, unjudged := scriptEnumerationValues(prog)
	var stored map[string][]string // enum QN -> values, nil entry = not in the project
	var entities map[string]bool

	values := func(enumQN string) ([]string, bool) {
		if unjudged[enumQN] {
			return nil, false
		}
		if v, ok := script[enumQN]; ok {
			return v, true
		}
		if stored == nil {
			stored = map[string][]string{}
		}
		v, seen := stored[enumQN]
		if !seen {
			mod, name, _ := strings.Cut(enumQN, ".")
			if e := findEnumeration(ctx, mod, name); e != nil {
				for _, ev := range e.Values {
					v = append(v, ev.Name)
				}
				if v == nil {
					v = []string{}
				}
			}
			stored[enumQN] = v
		}
		if v == nil {
			return nil, false
		}
		if entities == nil {
			entities = buildEntityQualifiedNames(ctx)
		}
		if entities[enumQN] {
			return nil, false
		}
		return v, true
	}

	var errs []error
	for i, stmt := range prog.Statements {
		var bad []string
		vals := map[string][]string{}
		seen := map[string]bool{}
		walkQualifiedNameExprs(stmt, func(qn ast.QualifiedName) {
			enum, key, ok := strings.Cut(qn.Name, ".")
			if !ok || qn.Module == "" || enum == "" || key == "" || strings.Contains(key, ".") {
				return
			}
			enumQN := qn.Module + "." + enum
			v, known := values(enumQN)
			if !known || hasValueFold(v, key) || seen[qn.String()] {
				return
			}
			seen[qn.String()] = true
			bad = append(bad, qn.String())
			vals[enumQN] = v
		})
		if len(bad) == 0 {
			continue
		}
		var msgs []string
		for _, b := range bad {
			enumQN := b[:strings.LastIndex(b, ".")]
			msgs = append(msgs, fmt.Sprintf("%s: %s has no value %s (its values: %s)",
				b, enumQN, b[len(enumQN)+1:], strings.Join(vals[enumQN], ", ")))
		}
		if kind, name, excluded := excludedFlow(stmt); excluded {
			sc.warnExcluded(kind, name, msgs)
			continue
		}
		errs = append(errs, mdlerrors.NewValidationf("statement %d: unknown enumeration value "+
			"(mxbuild: CE1613 \"The selected enumeration value … no longer exists.\"):\n  - %s",
			i+1, strings.Join(msgs, "\n  - ")))
	}
	return errs
}

// scriptEnumerationValues returns the values of the enumerations the script
// creates (the last definition wins, as when it runs), and the enumerations
// whose values it changes in a way not worked out here.
func scriptEnumerationValues(prog *ast.Program) (map[string][]string, map[string]bool) {
	values := map[string][]string{}
	unjudged := map[string]bool{}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateEnumerationStmt:
			v := make([]string, 0, len(s.Values))
			for _, ev := range s.Values {
				v = append(v, ev.Name)
			}
			values[s.Name.String()] = v
		case *ast.AlterEnumerationStmt:
			unjudged[s.Name.String()] = true
		case *ast.RenameStmt:
			if strings.EqualFold(s.ObjectType, "enumeration") {
				unjudged[s.Name.String()] = true
				unjudged[s.Name.Module+"."+s.NewName] = true
			}
		case *ast.MoveStmt:
			if s.DocumentType == ast.DocumentTypeEnumeration {
				unjudged[s.Name.String()] = true
				if s.TargetModule != "" {
					unjudged[s.TargetModule+"."+s.Name.Name] = true
				}
			}
		}
	}
	return values, unjudged
}

// excludedFlow reports a flow document the statement creates as excluded,
// whose dangling references Mendix does not validate.
func excludedFlow(stmt ast.Statement) (kind, name string, excluded bool) {
	switch s := stmt.(type) {
	case *ast.CreateMicroflowStmt:
		return "microflow", s.Name.String(), s.Excluded
	case *ast.CreateNanoflowStmt:
		return "nanoflow", s.Name.String(), s.Excluded
	case *ast.CreateRuleStmt:
		return "rule", s.Name.String(), s.Excluded
	}
	return "", "", false
}

func hasValueFold(values []string, key string) bool {
	for _, v := range values {
		if strings.EqualFold(v, key) {
			return true
		}
	}
	return false
}

// walkQualifiedNameExprs calls fn for every qualified-name expression in v, at
// any depth.
func walkQualifiedNameExprs(v any, fn func(ast.QualifiedName)) {
	var walk func(rv reflect.Value)
	walk = func(rv reflect.Value) {
		switch rv.Kind() {
		case reflect.Interface:
			if !rv.IsNil() {
				walk(rv.Elem())
			}
		case reflect.Pointer:
			if rv.IsNil() {
				return
			}
			if q, ok := rv.Interface().(*ast.QualifiedNameExpr); ok {
				fn(q.QualifiedName)
				return
			}
			walk(rv.Elem())
		case reflect.Struct:
			for i := 0; i < rv.NumField(); i++ {
				if rv.Type().Field(i).IsExported() {
					walk(rv.Field(i))
				}
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < rv.Len(); i++ {
				walk(rv.Index(i))
			}
		case reflect.Map:
			iter := rv.MapRange()
			for iter.Next() {
				walk(iter.Value())
			}
		}
	}
	walk(reflect.ValueOf(v))
}
