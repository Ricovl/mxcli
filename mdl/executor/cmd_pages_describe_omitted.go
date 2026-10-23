// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// unreconstructedContainers names the container-shaped properties a pluggable
// widget carries in its stored BSON that DESCRIBE did not reproduce.
//
// # Why this exists
//
// Slices 2-3 (PROPOSAL_def_driven_widget_bodies.md) made a widget's object
// lists and child slots WRITABLE from MDL. DESCRIBE PAGE cannot yet read them
// back for an arbitrary pluggable widget: extractObjectLists reconstructs the
// chart-shaped lists it was built for and returns nothing for HTML Element's
// `attributes`, and nothing reconstructs a child slot at all.
//
// Measured on a page mxcli itself authored:
//
//	written   htmlelement frame { attribute a1 (…); tagcontentcontainer body { … } }
//	stored    BSON carries `attributes` with data-x AND `tagContentContainer`
//	          with its DynamicText — the write path is correct
//	described htmlelement frame (tagName: 'div', …)      <- body gone, silently
//
// So describe -> edit -> exec deleted a widget's body and said nothing. That is
// the #965 failure class (an annotation emptying the loop body it sits in), and
// it became reachable the moment the construct could be written.
//
// Reconstructing them is a separate piece of work. Until then the honest
// behaviour is the one slice 4's usage example already follows: say what was
// left out rather than pretend the output is complete. A visible gap is a
// nuisance; a silent one is data loss.
//
// # Detection is from the document, not from a list
//
// A container is a property whose Value holds an `Objects` array (object list)
// or a `Widgets` array (child slot). That is read off the stored BSON, so it
// covers widgets nobody has thought about — the same reason the rest of this
// work reads definitions rather than maintaining keyword tables.
//
// Anything already reconstructed is excluded, so a chart's series list — which
// DESCRIBE does emit — produces no note.
func unreconstructedContainers(w map[string]any, reconstructed []rawObjectList, slots []rawChildSlot) []string {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return nil
	}
	keyMap := buildPropertyTypeKeyMap(w, true)
	if len(keyMap) == 0 {
		return nil
	}

	done := make(map[string]bool, len(reconstructed))
	for _, ol := range reconstructed {
		if ol.Keyword != "" {
			done[ol.Keyword] = true
		}
	}
	// A slot DESCRIBE now reproduces is not a loss, so it must not be named as
	// one — otherwise the note would fire on exactly the case that was fixed.
	// Slots are matched on the stored property key: the keyword they are
	// emitted under comes from the widget definition (a Repeater's `content`
	// is `template`, mendixlabs/mxcli#1400), which the keyword derived below
	// does not reproduce.
	doneKeys := make(map[string]bool, len(slots))
	for _, cs := range slots {
		if cs.PropertyKey != "" {
			doneKeys[cs.PropertyKey] = true
		}
	}

	seen := make(map[string]bool)
	var out []string
	for _, prop := range getBsonArrayElements(obj["Properties"]) {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}
		// CHILD SLOTS ONLY, and only when they hold widgets.
		//
		// An object list is deliberately excluded even though it is equally
		// unreconstructed, because it cannot be reported without crying wolf: a
		// widget template ships DEFAULT entries in its lists, and they are
		// structurally identical to a user's. Measured on the probe page —
		// which wrote one `attribute` and no `event` at all — the stored
		// document carries one object in each, so warning on object lists named
		// `event` too. A note that fires on defaults is noise, and noise trains
		// people to ignore the notes that matter.
		//
		// A WIDGET inside a slot has no such ambiguity: a template never puts
		// one there, so its presence means someone did. That is the case where
		// a describe -> exec round trip destroys real work, and it is the case
		// worth interrupting for.
		//
		// (getBsonArrayElements strips the leading typed-array marker, so a
		// length of 0 here really is empty — the raw BSON array is length 1.)
		if len(getBsonArrayElements(value["Widgets"])) == 0 {
			continue
		}
		key := keyMap[extractBinaryID(propMap["TypePointer"])]
		if key == "" || doneKeys[key] {
			continue
		}
		// Report the MDL keyword the author would write, not the raw property
		// key, so the note names something they can act on.
		kw := strings.ToLower(deriveObjectListKeyword(key))
		if kw == "" {
			kw = strings.ToLower(key)
		}
		if done[kw] || seen[kw] {
			continue
		}
		seen[kw] = true
		out = append(out, kw)
	}
	sort.Strings(out)
	return out
}

// writeOmittedContainerNote emits the gap as an MDL comment, so the output
// still parses and re-executes — it simply does not carry the body, and says
// so where the body would have been.
func writeOmittedContainerNote(out io.Writer, prefix string, omitted []string) {
	if len(omitted) == 0 {
		return
	}
	fmt.Fprintf(out, "%s-- NOT SHOWN: %s — this widget stores content DESCRIBE cannot yet\n",
		prefix, strings.Join(omitted, ", "))
	fmt.Fprintf(out, "%s-- reproduce. Re-running this script would DROP it. Inspect the widget with\n", prefix)
	fmt.Fprintf(out, "%s-- `mxcli widget describe <name>` and re-add the block by hand.\n", prefix)
}

// writeOmittedPropsNote emits, as a comment, the properties whose stored value
// differs from the default and that DESCRIBE has no spelling for — the property
// counterpart of writeOmittedContainerNote. The output still parses and
// re-executes; it simply does not carry them, and says so.
func writeOmittedPropsNote(out io.Writer, prefix string, omitted []string) {
	if len(omitted) == 0 {
		return
	}
	fmt.Fprintf(out, "%s-- NOT re-executable: %s — stored with a non-default value that DESCRIBE has\n",
		prefix, strings.Join(omitted, ", "))
	fmt.Fprintf(out, "%s-- no spelling for. Re-running this script would reset it to the widget's default.\n", prefix)
}

// rawHiddenContent is one widgets property of a combo box that holds widgets,
// by property key, with the widgets by name.
type rawHiddenContent struct {
	Property string
	Widgets  []string // a name, or "an unnamed <kind>"
}

// comboBoxHiddenContent reads a combo box's custom content off the stored
// document: every top-level property holding widgets. A combo box has no
// child slot MDL can write (MDL-WIDGET26), so each of them is content the
// description leaves out — unlike a generic pluggable widget, whose slots
// unreconstructedContainers reports only when DESCRIBE did not reproduce them.
func comboBoxHiddenContent(w map[string]any) []rawHiddenContent {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return nil
	}
	keyMap := buildPropertyTypeKeyMap(w, false)
	var out []rawHiddenContent
	for _, prop := range getBsonArrayElements(obj["Properties"]) {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		value, _ := propMap["Value"].(map[string]any)
		content := getBsonArrayElements(value["Widgets"])
		key := keyMap[extractBinaryID(propMap["TypePointer"])]
		if len(content) == 0 || key == "" {
			continue
		}
		hc := rawHiddenContent{Property: key}
		for _, c := range content {
			cw, _ := c.(map[string]any)
			if name := extractString(cw["Name"]); name != "" {
				hc.Widgets = append(hc.Widgets, name)
				continue
			}
			kind := extractString(cw["$Type"])
			kind = strings.ToLower(kind[strings.LastIndex(kind, "$")+1:])
			if kind == "" {
				kind = "widget"
			}
			hc.Widgets = append(hc.Widgets, "an unnamed "+kind)
		}
		out = append(out, hc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Property < out[j].Property })
	return out
}

// writeHiddenContentNote names a combo box's custom content. Executed against
// this page the content is kept while the combo box is left as stored — the
// pluggable passthrough — and lost when the statement changes it or runs
// anywhere else; `alter page … set` changes the combo box and keeps it.
func writeHiddenContentNote(out io.Writer, prefix, widget string, hidden []rawHiddenContent) {
	for _, hc := range hidden {
		fmt.Fprintf(out, "%s-- NOT SHOWN: %s's %s holds %s, custom content MDL cannot author.\n",
			prefix, widget, hc.Property, strings.Join(hc.Widgets, ", "))
		fmt.Fprintf(out, "%s-- Executed on this page it is kept while %s is unchanged; a changed %s, or a run\n",
			prefix, widget, widget)
		fmt.Fprintf(out, "%s-- anywhere else, loses it. Change %s with `alter page … set (…) on %s` instead.\n",
			prefix, widget, widget)
	}
}
