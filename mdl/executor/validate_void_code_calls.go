// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// voidCodeActions answers one question for the duplicate-variable checks: does
// a Java or JavaScript action call target an action that returns nothing?
//
// A call to a VOID action declares no variable, whatever output name it
// carries (ako/mxcli#953). Studio Pro keeps an output name on such a call — it
// names a JavaScript action's after the action (`$RefreshEntity`) and stores
// UseReturnVariable=true on some — so a model holding two of them is ordinary.
// Measured on mxbuild 11.13.0:
//
//	two void calls with the same output name            0 errors
//	void call `$V = …` then `declare $V String`          0 errors
//	void call `$V = …` then a log message using `$V`     CE0109 "Undefined variable 'V'"
//	void JS call `$W = …` then a non-void JS call `$W`   0 errors
//
// The name is inert: it neither collides with a later variable nor defines one.
//
// An action the resolver cannot find (no project, a runtime-provided System
// action) is NOT treated as void: the author wrote `$X =`, which normally asks
// for a value, and guessing void would silence a real CE0111.
type voidCodeActions struct {
	// script holds the actions the script itself creates, keyed by
	// codeActionKey, valued true when the action returns Void.
	script map[string]bool
	// open returns the project to read stored actions from, or nil.
	open   func() backend.FullBackend
	opened bool
	b      backend.FullBackend
	cache  map[string]bool
}

func codeActionKey(javaScript bool, qn string) string {
	if javaScript {
		return "js:" + qn
	}
	return "java:" + qn
}

// newVoidCodeActions collects the script's own action declarations; open,
// which may be nil, supplies the project for the rest.
func newVoidCodeActions(prog *ast.Program, open func() backend.FullBackend) *voidCodeActions {
	r := &voidCodeActions{script: map[string]bool{}, open: open, cache: map[string]bool{}}
	if prog == nil {
		return r
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateJavaActionStmt:
			r.script[codeActionKey(false, s.Name.String())] = s.ReturnType.Kind == ast.TypeVoid
		case *ast.CreateJavaScriptActionStmt:
			r.script[codeActionKey(true, s.Name.String())] = s.ReturnType.Kind == ast.TypeVoid
		}
	}
	return r
}

func (r *voidCodeActions) project() backend.FullBackend {
	if !r.opened {
		r.opened = true
		if r.open != nil {
			r.b = r.open()
		}
	}
	return r.b
}

// isVoid reports whether the named action is known to return Void.
func (r *voidCodeActions) isVoid(javaScript bool, qn string) bool {
	if r == nil || qn == "" {
		return false
	}
	key := codeActionKey(javaScript, qn)
	if v, ok := r.script[key]; ok {
		return v
	}
	if v, ok := r.cache[key]; ok {
		return v
	}
	void := false
	if b := r.project(); b != nil {
		if javaScript {
			if a, err := b.ReadJavaScriptActionByName(qn); err == nil && a != nil && a.ReturnType != nil {
				void = a.ReturnType.TypeString() == "Void"
			}
		} else {
			// The Java action reader returns a nil ReturnType for Void
			// (codeActionReturnTypeFromGen); the JavaScript one a VoidType.
			if a, err := b.ReadJavaActionByName(qn); err == nil && a != nil {
				void = a.ReturnType == nil || a.ReturnType.TypeString() == "Void"
			}
		}
	}
	r.cache[key] = void
	return void
}

// callIsVoid reports whether a statement is a call to a void Java or
// JavaScript action — one whose output name declares nothing.
func (r *voidCodeActions) callIsVoid(s ast.MicroflowStatement) bool {
	switch st := s.(type) {
	case *ast.CallJavaActionStmt:
		return r.isVoid(false, st.ActionName.String())
	case *ast.CallJavaScriptActionStmt:
		return r.isVoid(true, st.ActionName.String())
	}
	return false
}

// actionIsVoidCall is callIsVoid for a stored action, used by describe.
func (r *voidCodeActions) actionIsVoidCall(action any) bool {
	switch a := action.(type) {
	case *microflows.JavaActionCallAction:
		return r.isVoid(false, a.JavaAction)
	case *microflows.JavaScriptActionCallAction:
		return r.isVoid(true, a.JavaScriptAction)
	}
	return false
}
