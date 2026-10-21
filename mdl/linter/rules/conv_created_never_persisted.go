// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// CreatedObjectNeverPersistedRule flags a microflow that creates an object of a
// persistent entity without committing it and then does nothing that could
// persist it or hand it on: no commit, no return, no argument to any call, no
// association to another object, no list, no delete or rollback
// (mendixlabs/mxcli#1518).
//
// The object is discarded when the microflow ends. An after-startup seed
// microflow that logged "Seeded 20" while the table stayed empty is the case
// the rule exists for — check, mx check and the build are all silent on it.
//
// The rule is deliberately conservative: it reports only what it can prove.
//   - Only an explicit Commit = No create of an entity the catalog knows to be
//     persistent, in a microflow (a nanoflow's object lives in the client, and
//     is commonly handed to a page through means the flow does not show).
//   - It is flow-insensitive: a commit on any path, including an error
//     handler, counts, so an object committed on one branch only is not
//     reported (an accepted false negative).
//   - Every string in every object of the flow is scanned. The only uses that
//     do NOT count as passing the object on are reading a member of it
//     ($Obj/Attr) and changing it without commit. Anything else that names
//     the variable — a bare $Obj in any expression, or a field holding the
//     variable name — counts as a use, including uses the rule does not know
//     about. When in doubt it stays quiet.
type CreatedObjectNeverPersistedRule struct{}

func NewCreatedObjectNeverPersistedRule() *CreatedObjectNeverPersistedRule {
	return &CreatedObjectNeverPersistedRule{}
}

func (r *CreatedObjectNeverPersistedRule) ID() string       { return "CONV020" }
func (r *CreatedObjectNeverPersistedRule) Name() string     { return "CreatedObjectNeverPersisted" }
func (r *CreatedObjectNeverPersistedRule) Category() string { return "quality" }
func (r *CreatedObjectNeverPersistedRule) DefaultSeverity() linter.Severity {
	return linter.SeverityWarning
}

func (r *CreatedObjectNeverPersistedRule) Description() string {
	return "A microflow creates an object of a persistent entity without committing it, and never commits, returns, passes on, stores or deletes it"
}

func (r *CreatedObjectNeverPersistedRule) Check(ctx *linter.LintContext) []linter.Violation {
	if ctx.Reader() == nil {
		return nil
	}

	persistent := map[string]bool{}
	for e := range ctx.Entities() {
		if e.EntityType == "Persistent" {
			persistent[strings.ToLower(e.QualifiedName)] = true
		}
	}
	if len(persistent) == 0 {
		return nil
	}
	isPersistent := func(qn string) bool { return persistent[strings.ToLower(qn)] }

	var violations []linter.Violation
	for mf := range ctx.Microflows() {
		if mf.MicroflowType != "MICROFLOW" || ctx.IsExcluded(mf.ModuleName) {
			continue
		}
		full, err := ctx.FullMicroflow(model.ID(mf.ID))
		if err != nil || full == nil || full.ObjectCollection == nil {
			continue
		}
		violations = append(violations, findUnpersistedCreates(full.ObjectCollection, mf, isPersistent, r)...)
	}
	return violations
}

// findUnpersistedCreates reports each uncommitted create of a persistent
// entity in oc (loops included) whose output variable is never used in a way
// that could persist it or hand it on.
func findUnpersistedCreates(oc *microflows.MicroflowObjectCollection, mf linter.Microflow,
	isPersistent func(string) bool, r *CreatedObjectNeverPersistedRule) []linter.Violation {

	var violations []linter.Violation
	for _, create := range uncommittedCreates(oc.Objects) {
		if create.OutputVariable == "" || !isPersistent(create.EntityQualifiedName) {
			continue
		}
		if variableEscapes(oc, create) {
			continue
		}
		violations = append(violations, linter.Violation{
			RuleID:   r.ID(),
			Severity: r.DefaultSeverity(),
			Message: fmt.Sprintf("%s '%s.%s' creates $%s (%s) without committing it, and never commits, "+
				"returns or passes it on: the object is discarded when the microflow ends.",
				mf.DocumentNounTitle(), mf.ModuleName, mf.Name, create.OutputVariable, create.EntityQualifiedName),
			Location: linter.Location{
				Module:       mf.ModuleName,
				DocumentType: mf.DocumentNoun(),
				DocumentName: mf.Name,
				DocumentID:   mf.ID,
			},
			Suggestion: fmt.Sprintf("Commit $%s (or create it with commit), return it, or pass it on; "+
				"if it is only a temporary value, use a non-persistent entity.", create.OutputVariable),
		})
	}
	return violations
}

// uncommittedCreates returns the create actions with an explicit Commit = No,
// at any loop depth. An empty commit value is unknown, not No, and is skipped.
func uncommittedCreates(objects []microflows.MicroflowObject) []*microflows.CreateObjectAction {
	var out []*microflows.CreateObjectAction
	for _, obj := range objects {
		switch o := obj.(type) {
		case *microflows.ActionActivity:
			if c, ok := o.Action.(*microflows.CreateObjectAction); ok && c.Commit == microflows.CommitTypeNo {
				out = append(out, c)
			}
		case *microflows.LoopedActivity:
			if o.ObjectCollection != nil {
				out = append(out, uncommittedCreates(o.ObjectCollection.Objects)...)
			}
		}
	}
	return out
}

// variableEscapes reports whether anything in the flow, other than create
// itself, names create's output variable in a way that is not merely reading
// a member or changing it without commit.
func variableEscapes(oc *microflows.MicroflowObjectCollection, create *microflows.CreateObjectAction) bool {
	name := create.OutputVariable
	// $Name as a whole word, case-insensitively, and what follows it.
	ref := regexp.MustCompile(`(?i)\$` + regexp.QuoteMeta(name) + `\b(\s*/)?`)
	w := &escapeWalker{
		name:    name,
		ref:     ref,
		skip:    create,
		visited: map[uintptr]bool{},
	}
	w.walk(reflect.ValueOf(oc))
	return w.escapes
}

type escapeWalker struct {
	name    string
	ref     *regexp.Regexp
	skip    *microflows.CreateObjectAction
	visited map[uintptr]bool
	escapes bool
}

func (w *escapeWalker) walk(v reflect.Value) {
	if w.escapes || !v.IsValid() {
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		p := v.Pointer()
		if w.visited[p] {
			return
		}
		w.visited[p] = true
		if v.Type() == reflect.TypeOf(w.skip) && v.Interface().(*microflows.CreateObjectAction) == w.skip {
			return
		}
		if ch, ok := v.Interface().(*microflows.ChangeObjectAction); ok && w.isUncommittedChangeOfVar(ch) {
			// Changing the object without commit does not persist it; the
			// new member values are still scanned (setting an association
			// of the object to itself would be a use).
			for _, c := range ch.Changes {
				w.walk(reflect.ValueOf(c))
			}
			return
		}
		w.walk(v.Elem())
	case reflect.Interface:
		if !v.IsNil() {
			w.walk(v.Elem())
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				w.walk(v.Field(i))
			}
		}
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return // raw bytes hold no expression
		}
		for i := 0; i < v.Len(); i++ {
			w.walk(v.Index(i))
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			w.walk(iter.Key())
			w.walk(iter.Value())
		}
	case reflect.String:
		w.escapes = w.stringUses(v.String())
	}
}

func (w *escapeWalker) isUncommittedChangeOfVar(ch *microflows.ChangeObjectAction) bool {
	return strings.EqualFold(ch.ChangeVariable, w.name) && ch.Commit == microflows.CommitTypeNo
}

// stringUses reports whether s uses the variable as anything other than the
// owner of a member being read. A field holding exactly the variable name is
// a variable-name field (commit, delete, rollback, loop source, retrieve over
// an association, a page's passed object...) and is a use.
func (w *escapeWalker) stringUses(s string) bool {
	if strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(s), "$"), w.name) {
		return true
	}
	for _, m := range w.ref.FindAllStringSubmatch(s, -1) {
		if m[1] == "" { // not followed by '/': the object itself
			return true
		}
	}
	return false
}
