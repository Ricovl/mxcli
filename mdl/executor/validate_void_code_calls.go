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
// action) is NOT treated as void by `check`: the author wrote `$X =`, which
// normally asks for a value, and guessing void would silence a real CE0111.
// The editor makes the opposite trade (unknownIsVoid, ako/mxcli#962): a
// squiggle on a call it merely cannot resolve is a false refusal while typing,
// and `check` still runs before anything is written.
//
// Whatever that policy, only a call KNOWN to be void counts for the CE0109
// rule (MDL093): "possibly void" must never turn a use of the output into an
// error.
type voidCodeActions struct {
	// script holds the actions the script itself creates, keyed by
	// codeActionKey, valued true when the action returns Void.
	script map[string]bool
	// open returns the project to read stored actions from, or nil.
	open   func() backend.FullBackend
	opened bool
	b      backend.FullBackend
	cache  map[string]voidness
	// unknownIsVoid makes callIsVoid answer true for an action it cannot
	// resolve. Set by the editor (NewFlowRules); `check` leaves it false.
	unknownIsVoid bool
}

// voidness is what the resolver knows about one action.
type voidness struct {
	void  bool // the action returns Void
	known bool // the action was found, so void is an answer and not a default
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
	r := &voidCodeActions{script: map[string]bool{}, open: open, cache: map[string]voidness{}}
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
	return r.resolve(javaScript, qn).void
}

// resolve looks the named action up: in the script first, then in the project.
func (r *voidCodeActions) resolve(javaScript bool, qn string) voidness {
	if r == nil || qn == "" {
		return voidness{}
	}
	key := codeActionKey(javaScript, qn)
	if v, ok := r.script[key]; ok {
		return voidness{void: v, known: true}
	}
	if v, ok := r.cache[key]; ok {
		return v
	}
	var got voidness
	if b := r.project(); b != nil {
		if javaScript {
			if a, err := b.ReadJavaScriptActionByName(qn); err == nil && a != nil && a.ReturnType != nil {
				got = voidness{void: a.ReturnType.TypeString() == "Void", known: true}
			}
		} else {
			// The Java action reader returns a nil ReturnType for Void
			// (codeActionReturnTypeFromGen); the JavaScript one a VoidType.
			if a, err := b.ReadJavaActionByName(qn); err == nil && a != nil {
				got = voidness{void: a.ReturnType == nil || a.ReturnType.TypeString() == "Void", known: true}
			}
		}
	}
	r.cache[key] = got
	return got
}

// treatAsVoid applies the unknown-action policy to a resolution.
func (r *voidCodeActions) treatAsVoid(v voidness) bool {
	if v.known {
		return v.void
	}
	return r != nil && r.unknownIsVoid
}

// callResolution resolves the action a statement calls; ok is false for a
// statement that is not a Java or JavaScript action call.
func (r *voidCodeActions) callResolution(s ast.MicroflowStatement) (v voidness, ok bool) {
	switch st := s.(type) {
	case *ast.CallJavaActionStmt:
		return r.resolve(false, st.ActionName.String()), true
	case *ast.CallJavaScriptActionStmt:
		return r.resolve(true, st.ActionName.String()), true
	}
	return voidness{}, false
}

// callIsVoid reports whether a statement is a call to a void Java or
// JavaScript action — one whose output name declares nothing. An unresolvable
// action counts as void only under unknownIsVoid.
func (r *voidCodeActions) callIsVoid(s ast.MicroflowStatement) bool {
	v, ok := r.callResolution(s)
	return ok && r.treatAsVoid(v)
}

// callIsKnownVoid is callIsVoid without the unknown-action policy: true only
// for a call to an action the script or the project says returns Void.
func (r *voidCodeActions) callIsKnownVoid(s ast.MicroflowStatement) bool {
	v, ok := r.callResolution(s)
	return ok && v.known && v.void
}

// actionIsVoidCall is callIsVoid for a stored action, used by describe.
func (r *voidCodeActions) actionIsVoidCall(action any) bool {
	switch a := action.(type) {
	case *microflows.JavaActionCallAction:
		return r.treatAsVoid(r.resolve(false, a.JavaAction))
	case *microflows.JavaScriptActionCallAction:
		return r.treatAsVoid(r.resolve(true, a.JavaScriptAction))
	}
	return false
}
