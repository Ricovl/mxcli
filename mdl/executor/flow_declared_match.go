// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// declaredMatches reports whether a declared microflow AST value (from the
// script) states the same thing as the stored flow's AST value (from parsing
// what `describe` prints for it). It is the equality diff-then-patch matches
// statements by (ADR-0012 decision 3; plan item 4.2g).
//
// It is structural equality with one asymmetry: canvas GEOMETRY the script
// leaves out is not a difference. A statement written without `@position`,
// `@curve`, `@anchor`, `@merge` or `@start` has no opinion about where its node
// sits or how its flow bends, so it matches the stored statement wherever that
// is drawn — `create or modify` changes only what differs (R1), and a layout
// nobody stated is not a difference. A statement that DOES state geometry must
// state the stored one: a moved node is a real change, and one the splice does
// not make (it only places new nodes), so it must not pass as unchanged.
//
// Everything else is compared exactly, including captions, colours and notes,
// which are content Studio Pro shows rather than layout.
func declaredMatches(declared, stored any) bool {
	return matchValue(reflect.ValueOf(declared), reflect.ValueOf(stored), matchDeclared)
}

// sameIgnoringLayout reports whether two statements differ in canvas geometry
// at most — a node moved, a connector redrawn.
func sameIgnoringLayout(declared, stored any) bool {
	return matchValue(reflect.ValueOf(declared), reflect.ValueOf(stored), matchAnyLayout)
}

// sameExceptPositions reports whether two statements differ at most in where
// their nodes are drawn (@position, @start) — which the patch makes by moving
// the stored nodes (ako/mxcli#818) — and not in how their flows are drawn.
func sameExceptPositions(declared, stored any) bool {
	return matchValue(reflect.ValueOf(declared), reflect.ValueOf(stored), matchAnyPosition)
}

// matchMode is how matchValue treats geometry.
type matchMode int

const (
	// matchDeclared: geometry the declared side leaves out is not a
	// difference; geometry it states must be the stored one.
	matchDeclared matchMode = iota
	// matchAnyLayout: no geometry is a difference.
	matchAnyLayout
	// matchAnyPosition: a node's position is not a difference; the rest of
	// the geometry is compared as for matchDeclared.
	matchAnyPosition
)

// positionFields are the geometry fields of a statement's annotations that
// say where its node is drawn, as opposed to how its flows are.
var positionFields = map[string]bool{"Position": true, "Start": true}

// The geometry is what stripFlowLayout clears for `mxcli layout flows`; a
// note's size is included here because the writer defaults it when absent.
var (
	annotationsStructType = reflect.TypeOf(ast.ActivityAnnotations{})
	noteStructType        = reflect.TypeOf(ast.MicroflowAnnotation{})
	paramStructType       = reflect.TypeOf(ast.MicroflowParam{})
	retrieveStructType    = reflect.TypeOf(ast.RetrieveStmt{})
	sourceExprStructType  = reflect.TypeOf(ast.SourceExpr{})
)

// geometryFields names, per AST type, the fields that hold canvas geometry and
// that a script may therefore leave nil without that being a difference.
var geometryFields = map[reflect.Type]map[string]bool{
	annotationsStructType: {
		"Position": true, "Anchor": true, "TrueBranchAnchor": true, "FalseBranchAnchor": true,
		"IteratorAnchor": true, "BodyTailAnchor": true, "Curve": true, "Merge": true, "Start": true,
	},
	noteStructType:  {"Position": true, "Size": true},
	paramStructType: {"Position": true},
}

func matchValue(d, s reflect.Value, mode matchMode) bool {
	if !d.IsValid() || !s.IsValid() {
		return d.IsValid() == s.IsValid()
	}
	if d.Type() != s.Type() {
		return false
	}
	switch d.Kind() {
	case reflect.Pointer:
		if d.IsNil() && s.IsNil() {
			return true
		}
		if d.Type().Elem() == annotationsStructType {
			// A statement written with no annotations at all states no
			// geometry, and no caption, colour or note either.
			if d.IsNil() {
				d = reflect.New(annotationsStructType)
			}
			if s.IsNil() {
				s = reflect.New(annotationsStructType)
			}
		}
		if d.IsNil() || s.IsNil() {
			return false
		}
		return matchValue(d.Elem(), s.Elem(), mode)
	case reflect.Interface:
		if d.IsNil() || s.IsNil() {
			return d.IsNil() == s.IsNil()
		}
		return matchValue(d.Elem(), s.Elem(), mode)
	case reflect.Struct:
		geo := geometryFields[d.Type()]
		for i := 0; i < d.NumField(); i++ {
			df := d.Field(i)
			name := d.Type().Field(i).Name
			skip := mode == matchAnyLayout ||
				(mode == matchAnyPosition && d.Type() == annotationsStructType && positionFields[name])
			if geo[name] && df.Kind() == reflect.Pointer && (skip || df.IsNil()) {
				continue
			}
			if d.Type() == sourceExprStructType && name == "Source" {
				// Whitespace around an expression is not part of it: the
				// writer stores it as written, describe drops it
				// (describeExpr), so the stored side never has it.
				if strings.TrimSpace(df.String()) != strings.TrimSpace(s.Field(i).String()) {
					return false
				}
				continue
			}
			if d.Type() == retrieveStructType && name == "Where" {
				if !sameStoredConstraint(df, s.Field(i)) {
					return false
				}
				continue
			}
			if !matchValue(df, s.Field(i), mode) {
				return false
			}
		}
		return true
	case reflect.Slice, reflect.Array:
		// nil and empty are the same list.
		if d.Len() != s.Len() {
			return false
		}
		for i := 0; i < d.Len(); i++ {
			if !matchValue(d.Index(i), s.Index(i), mode) {
				return false
			}
		}
		return true
	case reflect.Map:
		if d.Len() != s.Len() {
			return false
		}
		for _, k := range d.MapKeys() {
			sv := s.MapIndex(k)
			if !sv.IsValid() || !matchValue(d.MapIndex(k), sv, mode) {
				return false
			}
		}
		return true
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return d.IsNil() == s.IsNil()
	default:
		return d.Equal(s)
	}
}

// reflectElem returns the struct a statement pointer points at, or the zero
// Value when it is not a pointer to a struct.
func reflectElem(v any) reflect.Value {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return reflect.Value{}
	}
	return rv.Elem()
}

// sameStoredConstraint compares two retrieve where clauses as the XPath
// constraint each stores (ako/mxcli#839).
//
// The AST is the wrong measure here: the bracketed form an author writes,
// `where [a and b]`, is read as XPath and keeps its source text, while the bare
// form describe prints, `where a and b`, is read as an expression, so one
// constraint parses to two different trees — different node types, `and`
// against `AND`, the author's line breaks in one and not the other. What a
// retrieve means is the constraint it stores, so that is what is compared,
// with whitespace between tokens ignored: the writer lays a long constraint
// out over several lines itself (FormatXPathConstraint), and a line break
// between two tokens states nothing. Whitespace inside a string literal is
// data and is compared as written.
func sameStoredConstraint(d, s reflect.Value) bool {
	de, _ := d.Interface().(ast.Expression)
	se, _ := s.Interface().(ast.Expression)
	return xpathTokens(retrieveXPathConstraint(de)) == xpathTokens(retrieveXPathConstraint(se))
}

// xpathTokens is an XPath constraint with the whitespace between its tokens
// normalised: a run of it becomes one space, and none is kept inside a
// bracket or parenthesis. String literals are copied as they are.
func xpathTokens(xpath string) string {
	var b strings.Builder
	var last byte // the last byte written
	space := false
	for i := 0; i < len(xpath); {
		c := xpath[i]
		switch c {
		case ' ', '\t', '\n', '\r':
			space = true
			i++
			continue
		}
		if space && last != 0 && last != '[' && last != '(' && c != ']' && c != ')' {
			b.WriteByte(' ')
		}
		space = false
		end := i + 1
		if c == '\'' {
			end = xpathLiteralEnd(xpath, i)
		}
		b.WriteString(xpath[i:end])
		last = xpath[end-1]
		i = end
	}
	return b.String()
}
