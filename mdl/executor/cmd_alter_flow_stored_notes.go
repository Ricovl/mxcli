// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A fragment of `alter microflow|nanoflow … insert|replace` attaches its new
// statement to a note already stored in the flow by naming the note's text
// (ako/mxcli#1695):
//
//	insert after log node 'App' 'start' begin
//	  @annotation(existing: 'Validates the order')
//	  log info node 'App' 'middle';
//	end;
//
// By text, not by the `id:` labels describe prints: those are re-derived on
// every describe (n1, n2, … in traversal order) and stored nowhere, so an id
// saved in a script could later name a different note. The text is what the
// canvas shows and describe prints, and it is checked: it must name exactly one
// note of the flow as stored, or the statement is refused — by check and exec
// alike, through resolveStoredNotes, before any fragment is built.
//
// A key of its own, never `text:`: `@annotation(text: '…')` creates a note,
// and a reuse of it would read the same whether it creates one or attaches
// one. `existing:` never creates — a text that names no note is a refusal.

// resolveStoredNotes resolves every `@annotation(existing: '…')` in the
// statement's fragments against the notes of the flow as stored, for the
// fragments' builders, and refuses the statement when a text names no note or
// several. A note an earlier operation of the statement adds is not stored, as
// a target an earlier operation inserts is not addressable.
func (a *alterFlowContext) resolveStoredNotes() error {
	s := a.stmt
	a.storedNotes = map[string]*microflows.Annotation{}
	var notes []*microflows.Annotation
	collectStoredNotes(a.mf.ObjectCollection, &notes)
	for _, op := range s.Operations {
		for _, text := range existingNoteTexts(op.Body) {
			if _, done := a.storedNotes[text]; done {
				continue
			}
			note, err := a.storedNoteNamed(notes, text)
			if err != nil {
				return mdlerrors.NewValidation(fmt.Sprintf("alter %s %s: %s %s: %v", s.Kind(), s.Name, op.Op, op.Target, err))
			}
			a.storedNotes[text] = note
		}
	}
	return nil
}

// collectStoredNotes appends every note of the collection, loop bodies
// included, to out: a note's text names it wherever on the canvas it is, and
// the text is unique only if it is unique in the whole flow.
func collectStoredNotes(oc *microflows.MicroflowObjectCollection, out *[]*microflows.Annotation) {
	if oc == nil {
		return
	}
	for _, obj := range oc.Objects {
		switch o := obj.(type) {
		case *microflows.Annotation:
			*out = append(*out, o)
		case *microflows.LoopedActivity:
			collectStoredNotes(o.ObjectCollection, out)
		}
	}
}

// existingNoteTexts returns the `existing:` texts the statements name, nested
// bodies and error handlers included, in order.
func existingNoteTexts(body []ast.MicroflowStatement) []string {
	var out []string
	var walk func([]ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, s := range stmts {
			if ann := ast.StatementAnnotations(s); ann != nil {
				for _, n := range append(append([]ast.MicroflowAnnotation(nil), ann.Notes...), ann.FreeNotes...) {
					if n.Existing != "" {
						out = append(out, n.Existing)
					}
				}
			}
			for _, nested := range ast.StatementBodies(s) {
				walk(nested)
			}
		}
	}
	walk(body)
	return out
}

// storedNoteNamed returns the one note of notes whose text is text. A note's
// line breaks are compared as LF: Studio Pro stores CRLF, and a script saved by
// an editor that writes LF names the same note (mendixlabs/mxcli#1505).
func (a *alterFlowContext) storedNoteNamed(notes []*microflows.Annotation, text string) (*microflows.Annotation, error) {
	var matches []*microflows.Annotation
	for _, n := range notes {
		if lfLineBreaks(n.Caption) == lfLineBreaks(text) {
			matches = append(matches, n)
		}
	}
	ref := "`@annotation(existing: " + mdlQuoted(text) + ")`"
	switch {
	case len(matches) == 1:
		return matches[0], nil
	case len(matches) > 1:
		return nil, fmt.Errorf("%s names %d notes of the stored flow, so it does not say which one to attach: %s. "+
			"A note is attached by its text only when no other note of the flow reads the same; reword the one "+
			"you mean first (in Studio Pro, or with `create or modify`)",
			ref, len(matches), a.describeNotes(matches, false))
	case len(notes) == 0:
		return nil, fmt.Errorf("%s names a note of the stored flow, and the flow has no notes; "+
			"`existing:` attaches a note that is already there — to add one, write `@annotation %s`",
			ref, mdlQuoted(text))
	}
	return nil, fmt.Errorf("%s names no note of the stored flow: none reads that text. The flow's notes: %s. "+
		"`existing:` attaches a note that is already there, named by its text exactly as describe prints it; "+
		"to add a new note, write `@annotation %s`",
		ref, a.describeNotes(notes, true), mdlQuoted(text))
}

// describeNotes names notes for a refusal: each one's text (when withText),
// where it is drawn, and what it is attached to.
func (a *alterFlowContext) describeNotes(notes []*microflows.Annotation, withText bool) string {
	const most = 10
	var parts []string
	for i, n := range notes {
		if i == most {
			parts = append(parts, fmt.Sprintf("and %d more", len(notes)-most))
			break
		}
		var b strings.Builder
		if withText {
			b.WriteString(quoteNoteText(n.Caption) + " ")
		}
		p := n.GetPosition()
		fmt.Fprintf(&b, "at (%d, %d)", p.X, p.Y)
		if targets := a.noteTargets(n.ID); len(targets) > 0 {
			b.WriteString(", attached to " + strings.Join(targets, " and "))
		} else {
			b.WriteString(", attached to nothing")
		}
		parts = append(parts, b.String())
	}
	return strings.Join(parts, "; ")
}

// quoteNoteText quotes a note's text with escapes, which shows a difference in
// invisible characters for what it is, cut short when it is long.
func quoteNoteText(text string) string {
	if r := []rune(text); len(r) > 60 {
		text = string(r[:60]) + "…"
	}
	return strconv.Quote(text)
}

// noteTargets names what the stored note id is attached to, as describe
// prints each statement.
func (a *alterFlowContext) noteTargets(id model.ID) []string {
	var out []string
	for _, af := range a.mf.ObjectCollection.AnnotationFlows {
		other := af.DestinationID
		if af.DestinationID == id {
			other = af.OriginID
		} else if af.OriginID != id {
			continue
		}
		out = append(out, a.objectLabel(other))
	}
	return out
}

// objectLabel is the stored object id as a reader finds it: its statement as
// describe prints it, or its kind.
func (a *alterFlowContext) objectLabel(id model.ID) string {
	for _, c := range a.cands {
		if c.ID == id && c.Statement != "" {
			return "`" + c.Statement + "`"
		}
	}
	for _, c := range a.cands {
		if c.ID == id {
			return "the " + strings.ToLower(strings.TrimPrefix(fmt.Sprintf("%T", c.Object), "*microflows."))
		}
	}
	return "an object that cannot be addressed"
}

// takeStoredNoteLines takes the lines from stored notes out of frag: they are
// drawn once the fragment is in the flow (linkStoredNotes), where both ends
// are where they will stay. Left in, the splice would see a line to an object
// that is not the fragment's and refuse a fragment it can otherwise write in
// place (an activity with an error handler, one inside a loop body).
func (a *alterFlowContext) takeStoredNoteLines(frag *backend.MicroflowFragment) []*microflows.AnnotationFlow {
	if frag == nil || len(a.storedNotes) == 0 {
		return nil
	}
	stored := map[model.ID]bool{}
	for _, n := range a.storedNotes {
		stored[n.ID] = true
	}
	var lines []*microflows.AnnotationFlow
	kept := frag.AnnotationFlows[:0:0]
	for _, af := range frag.AnnotationFlows {
		if stored[af.OriginID] {
			lines = append(lines, af)
			continue
		}
		kept = append(kept, af)
	}
	frag.AnnotationFlows = kept
	return lines
}

// linkStoredNotes draws the lines takeStoredNoteLines took, now that the
// fragment is in the flow.
func linkStoredNotes(mut backend.MicroflowMutator, lines []*microflows.AnnotationFlow) error {
	for _, af := range lines {
		if err := mut.LinkNote(af.OriginID, af.DestinationID); err != nil {
			return err
		}
	}
	return nil
}

// attachStoredNote attaches the stored note an `@annotation(existing: '…')`
// names to the activity: a line from that note, which is not created and
// keeps its $ID, text and place. Mentioned twice on one activity, it is
// attached once.
func (fb *flowBuilder) attachStoredNote(note ast.MicroflowAnnotation, activityID model.ID) {
	if msg := existingNoteMisuse(note); msg != "" {
		fb.addError("%s", msg)
		return
	}
	if fb.storedNotes == nil {
		fb.addError("%s", existingNoteOutsideAlter(note.Existing))
		return
	}
	stored, ok := fb.storedNotes[note.Existing]
	if !ok {
		// resolveStoredNotes refuses a text that names no stored note before
		// a fragment is built; this is a caller that built one without it.
		fb.addError("`@annotation(existing: %s)` was not resolved against the stored flow", mdlQuoted(note.Existing))
		return
	}
	for _, af := range fb.annotationFlows {
		if af.OriginID == stored.ID && af.DestinationID == activityID {
			return
		}
	}
	fb.linkAnnotation(stored.ID, activityID)
}

// storedNoteHint points an `@annotation(id: …)` in an alter fragment that
// names no note of the fragment at the form that names a stored one: the
// labels describe prints are not stored, so a fragment cannot see them.
func (fb *flowBuilder) storedNoteHint() string {
	if fb.storedNotes == nil {
		return ""
	}
	return "; a note already stored in the flow is attached by its text, as @annotation(existing: '…')"
}

// existingNoteMisuse is the refusal for `existing:` beside a parameter that
// describes a note of the statement's own, or "" when it stands alone. check
// (the validator) and exec (the builder) both say it.
func existingNoteMisuse(note ast.MicroflowAnnotation) string {
	var extra []string
	if note.Label != "" {
		extra = append(extra, "`id:`")
	}
	if note.Text != "" {
		extra = append(extra, "`text:`")
	}
	if note.Position != nil {
		extra = append(extra, "`position:`")
	}
	if note.Size != nil {
		extra = append(extra, "`size:`")
	}
	if len(extra) == 0 {
		return ""
	}
	return fmt.Sprintf("`@annotation(existing: %s)` attaches a note already stored in the flow, which keeps its own "+
		"text and place, so it takes no %s beside it; to create a note of the statement's own, write "+
		"`@annotation(text: …)` without `existing:`", mdlQuoted(note.Existing), strings.Join(extra, " or "))
}

// existingNoteOutsideAlter is the refusal for `existing:` in a statement that
// states a whole flow, which has no stored flow to attach from.
func existingNoteOutsideAlter(text string) string {
	return fmt.Sprintf("`@annotation(existing: %s)` attaches a note already stored in the flow, which only a "+
		"fragment of `alter microflow|nanoflow … insert|replace` has; a statement that states the whole flow "+
		"declares its notes — `@annotation 'text'`, or `@annotation(id: n1, text: '…')` and then "+
		"`@annotation(id: n1)` to share one", mdlQuoted(text))
}

// existingNoteUnattached is the refusal for `existing:` with no statement
// after it to attach the stored note to.
func existingNoteUnattached(text string) string {
	return fmt.Sprintf("`@annotation(existing: %s)` is followed by no statement to attach the stored note to",
		mdlQuoted(text))
}
