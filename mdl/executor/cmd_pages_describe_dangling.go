// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"strings"
)

// A data view whose `selection` source is a widget DESCRIBE cannot emit
// (mendixlabs/mxcli#1415). The FSM template's NativeShadowModel_Overview has a
// data view listening to a legacy Forms$DataGrid; the grid comes out as a
// "NOT re-executable" comment, and the data view came out as a statement,
// `DataSource: selection grid1`, naming a widget the script does not contain.
// check -p and exec refused the whole page for it:
//
//	widget 'dataView1': selection DataSource references 'grid1' but no widget
//	with that name exists on this page
//
// The data view cannot be written without its source — a selection data view
// with nothing to listen to is not a data view at all — so it is commented out
// with the grid, and labelled with the reason. That keeps the rest of the page
// re-executable, and the dropped part visible rather than silently gone.

// describeBodyState is what one describe body knows about its widgets.
type describeBodyState struct {
	// dropped are the names of widgets emitted only as a comment: the ones
	// MDL cannot spell, everything commented out with a dependent data view,
	// and that data view itself.
	dropped map[string]bool
	// selected are the selection sources emitted as statements in this pass.
	selected map[string]bool
	// inComment is set while a commented-out widget's subtree is rendered:
	// every widget in it is dropped too.
	inComment bool
	// unauthorable are the widgets of this pass MDL cannot author at all.
	unauthorable []unauthorableWidget
}

// unauthorableWidget is a stored widget describe can only print as a comment
// because MDL has no way to write it.
type unauthorableWidget struct {
	Type string // its $Type, e.g. Forms$DataGrid
	Name string // "" when the stored widget has none
}

// describeWidgetBody renders a document's widget body through render. A data
// view can name its selection source before or after that source in document
// order, so a pass that emitted a selection of a widget it later found it
// could not emit is rendered again, now knowing; a body with no such case is
// rendered once.
func describeWidgetBody(ctx *ExecContext, render func(c *ExecContext)) {
	state := &describeBodyState{dropped: map[string]bool{}}
	c := *ctx
	c.describeBody = state
	var buf bytes.Buffer
	for pass := 0; pass < 4; pass++ {
		buf.Reset()
		c.Output = &buf
		state.selected = map[string]bool{}
		state.inComment = false
		state.unauthorable = nil
		render(&c)
		if !state.selectsADroppedWidget() {
			break
		}
	}
	if ctx.unauthorableSink != nil {
		*ctx.unauthorableSink = append(*ctx.unauthorableSink, state.unauthorable...)
	}
	_, _ = ctx.Output.Write(buf.Bytes())
}

func (s *describeBodyState) selectsADroppedWidget() bool {
	for name := range s.selected {
		if s.dropped[name] {
			return true
		}
	}
	return false
}

// selectionSources lists the widgets w's data sources listen to.
func selectionSources(w rawWidget) []string {
	var out []string
	add := func(ds *rawDataSource) {
		if ds != nil && ds.Type == "selection" && ds.Reference != "" {
			out = append(out, ds.Reference)
		}
	}
	add(w.DataSource)
	for _, n := range w.NamedDataSources {
		add(n.DataSource)
	}
	return out
}

// describeDanglingSelection is the hook outputWidgetMDLV3 runs first. It
// reports true when it has emitted w itself, as a comment.
func describeDanglingSelection(ctx *ExecContext, w rawWidget, indent int) bool {
	s := ctx.describeBody
	if s == nil {
		return false
	}
	if s.inComment {
		if w.Name != "" {
			s.dropped[w.Name] = true
		}
		return false
	}
	var missing string
	for _, src := range selectionSources(w) {
		if s.dropped[src] {
			missing = src
			break
		}
	}
	if missing == "" {
		for _, src := range selectionSources(w) {
			s.selected[src] = true
		}
		return false
	}

	// Render the widget as it would be, then emit every line as a comment.
	var buf bytes.Buffer
	c := *ctx
	c.Output = &buf
	s.inComment = true
	outputWidgetMDLV3(&c, w, indent)
	s.inComment = false
	if w.Name != "" {
		s.dropped[w.Name] = true
	}

	prefix := strings.Repeat("  ", indent)
	label := strings.TrimSpace(w.Type + " " + w.Name)
	fmt.Fprintf(ctx.Output, "%s-- NOT re-executable: %s reads the selection of %s, which this description "+
		"cannot emit, so re-running this script drops both\n", prefix, label, missing)
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		fmt.Fprintf(ctx.Output, "%s-- %s\n", prefix, strings.TrimPrefix(line, prefix))
	}
	return true
}

// describeDroppedWidget records that w is emitted only as a comment.
//
// It is called for exactly the widgets MDL cannot author, so it is also where
// the rewrite guard learns what a rewrite would delete (unauthorableSink).
func describeDroppedWidget(ctx *ExecContext, w rawWidget) {
	if ctx.describeBody == nil {
		return
	}
	if w.Name != "" {
		ctx.describeBody.dropped[w.Name] = true
	}
	ctx.describeBody.unauthorable = append(ctx.describeBody.unauthorable, unauthorableWidget{Type: w.Type, Name: w.Name})
}
