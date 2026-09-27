// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"

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
	return matchValue(reflect.ValueOf(declared), reflect.ValueOf(stored), false)
}

// sameIgnoringLayout reports whether two statements differ in canvas geometry
// at most — a node moved, a connector redrawn — which is a change the splice
// does not make.
func sameIgnoringLayout(declared, stored any) bool {
	return matchValue(reflect.ValueOf(declared), reflect.ValueOf(stored), true)
}

// The geometry is what stripFlowLayout clears for `mxcli layout flows`; a
// note's size is included here because the writer defaults it when absent.
var (
	annotationsStructType = reflect.TypeOf(ast.ActivityAnnotations{})
	noteStructType        = reflect.TypeOf(ast.MicroflowAnnotation{})
	paramStructType       = reflect.TypeOf(ast.MicroflowParam{})
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

func matchValue(d, s reflect.Value, anyLayout bool) bool {
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
		return matchValue(d.Elem(), s.Elem(), anyLayout)
	case reflect.Interface:
		if d.IsNil() || s.IsNil() {
			return d.IsNil() == s.IsNil()
		}
		return matchValue(d.Elem(), s.Elem(), anyLayout)
	case reflect.Struct:
		geo := geometryFields[d.Type()]
		for i := 0; i < d.NumField(); i++ {
			df := d.Field(i)
			if geo[d.Type().Field(i).Name] && df.Kind() == reflect.Pointer && (anyLayout || df.IsNil()) {
				continue
			}
			if !matchValue(df, s.Field(i), anyLayout) {
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
			if !matchValue(d.Index(i), s.Index(i), anyLayout) {
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
			if !sv.IsValid() || !matchValue(d.MapIndex(k), sv, anyLayout) {
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
