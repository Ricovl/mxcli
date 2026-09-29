// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
)

// carryNoActionExecution copies DisabledDuringExecution from the stored
// document's empty action slots onto the rebuilt ones (ako/mxcli#721 L2).
//
// Every event slot of a widget holds a client action, and an unused slot holds
// a Forms$NoAction that still stores the flag. Studio Pro does not store one
// value for it: across PedApp and ako/TestApp the input widgets' OnChange,
// OnEnter, OnLeave and OnEnterKeyPress NoActions store false on 946 slots and
// true on 380, and the ClickAction slots false on 22 and true on 27. The
// writer wrote true on all of them, so every describe → exec of a page with a
// text box rewrote it.
//
// Carried rather than spelled in MDL, for the reason carryStoredPageHeader
// gives: a NoAction runs nothing, so the flag has no behaviour to ask for, and
// a value nobody asked to change should not change. Only a slot that is a
// NoAction both in the script and in the stored widget of the same name is
// touched; an action the script writes, including one replacing a stored
// NoAction, keeps what its own `with ( … )` says.
//
// A missing or unreadable stored unit leaves the rebuilt values in place: this
// is a fidelity improvement on a rewrite, not a precondition for one.
func (b *Backend) carryNoActionExecution(id model.ID, root element.Element) {
	if b.reader == nil || id == "" || root == nil {
		return
	}
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		return
	}
	var stored bson.D
	if err := bson.Unmarshal(raw, &stored); err != nil {
		return
	}
	carryNoActionExecutionFrom(stored, root)
}

// carryNoActionExecutionFrom is carryNoActionExecution against an already
// decoded stored document.
func carryNoActionExecutionFrom(stored bson.D, root element.Element) {
	storedWidgets := map[string]bson.D{}
	ambiguous := map[string]bool{}
	indexNamedDocs(stored, storedWidgets, ambiguous)

	type namer interface{ Name() string }
	element.Walk(root, func(e element.Element) bool {
		n, ok := e.(namer)
		if !ok || n.Name() == "" || ambiguous[n.Name()] {
			return true
		}
		sw, ok := storedWidgets[n.Name()]
		if !ok {
			return true
		}
		for _, prop := range e.Properties() {
			cp, ok := prop.(element.ChildProperty)
			if !ok {
				continue
			}
			built, ok := cp.ChildElement().(*genPg.NoClientAction)
			if !ok {
				continue
			}
			slot := bsonnav.DGetDoc(sw, prop.Name())
			if slot == nil || bsonnav.DGetString(slot, "$Type") != "Forms$NoAction" {
				continue
			}
			if v, ok := bsonnav.DGet(slot, "DisabledDuringExecution").(bool); ok {
				built.SetDisabledDuringExecution(v)
			}
		}
		return true
	})
}

// indexNamedDocs indexes every named document in d by its Name. A name seen
// twice is ambiguous and is not carried from.
func indexNamedDocs(d bson.D, out map[string]bson.D, ambiguous map[string]bool) {
	if name, ok := bsonnav.DGet(d, "Name").(string); ok && name != "" {
		if _, dup := out[name]; dup {
			ambiguous[name] = true
		}
		out[name] = d
	}
	for _, e := range d {
		indexNamedValue(e.Value, out, ambiguous)
	}
}

func indexNamedValue(v any, out map[string]bson.D, ambiguous map[string]bool) {
	switch x := v.(type) {
	case bson.D:
		indexNamedDocs(x, out, ambiguous)
	case bson.A:
		for _, item := range x {
			indexNamedValue(item, out, ambiguous)
		}
	}
}
