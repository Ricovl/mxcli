// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// The mechanical rewrites `mxcli fmt --upgrade` applies (mdl/upgrade) are
// computed here, where the construct is recorded, because the parse tree is the
// only place its tokens and their positions are still at hand. Each rewrite is
// an ast.Fix on the recorded ast.DeprecatedSpelling or ast.LanguageNote; a
// construct with no mechanical rewrite gets a NoFix reason instead, and the
// upgrade reports it rather than guessing.
//
// A Fix edits as little as it can — the tokens that differ between the two
// spellings — so comments, layout and nested constructs survive, and a nested
// fix (a string escape inside a list operation's condition) never overlaps it.

// LanguageChanges returns every change of meaning the visitor gates on the
// language header. The upgrade's registry test requires each to have a
// rewrite or to be listed as having none.
func LanguageChanges() []langver.Change {
	return []langver.Change{
		semicolonRequired, slashIsNotATerminator, backslashIsLiteral, limitOneIsAList,
		listCallForm, setIsMandatory, viewEntityReplaceIsModify, roleReplaceIsModify, whileBlockRequired,
		unknownPropertyKey, misshapedPropertyValue, sessionCommandInScript, showSummaryRemoved, templateLineBreak,
		quotedExpressionText,
	}
}

// fixLastNote attaches a rewrite, or the reason there is none, to the note
// gate just recorded for code.
func (b *Builder) fixLastNote(code string, fix *ast.Fix, noFix string) {
	if n := len(b.langNotes); n > 0 && b.langNotes[n-1].Code == code {
		b.langNotes[n-1].Fix, b.langNotes[n-1].NoFix = fix, noFix
	}
}

// chooseLastNote attaches the choice an upgrade with the project resolves
// (ako/mxcli#860) to the note gate just recorded for code. A nil choice is
// no-op.
func (b *Builder) chooseLastNote(code string, choice *ast.OperandChoice) {
	if n := len(b.langNotes); choice != nil && n > 0 && b.langNotes[n-1].Code == code {
		b.langNotes[n-1].Operand = choice
	}
}

// fixLastDeprecation attaches a structural rewrite, or the reason there is
// none, to the deprecated use just recorded for code.
func (b *Builder) fixLastDeprecation(code string, fix *ast.Fix, noFix string) {
	if n := len(b.deprecations); n > 0 && b.deprecations[n-1].Code == code {
		b.deprecations[n-1].Fix, b.deprecations[n-1].NoFix = fix, noFix
	}
}

// fixOrReason turns an edit list and the reason there is none into the pair
// fixLastNote and fixLastDeprecation take.
func fixOrReason(edits []ast.TextEdit, reason string) (*ast.Fix, string) {
	if reason != "" {
		return nil, reason
	}
	return &ast.Fix{Edits: edits}, ""
}

// insertAt inserts text at rune offset pos.
func insertAt(pos int, text string) ast.TextEdit {
	return ast.TextEdit{Start: pos, Stop: pos, Text: text}
}

// replaceSpan replaces from the start of first to the end of last.
func replaceSpan(first, last antlr.Token, text string) ast.TextEdit {
	return ast.TextEdit{Start: first.GetStart(), Stop: last.GetStop() + 1, Text: text}
}

// replaceGap replaces the runes after aStop and before bStart.
func replaceGap(aStop, bStart int, text string) ast.TextEdit {
	return ast.TextEdit{Start: aStop + 1, Stop: bStart, Text: text}
}

// keywordLike spells word in the case of like: upper-case when like is, else
// lower-case (the canonical case, ADR-0010).
func keywordLike(like, word string) string {
	if like == strings.ToUpper(like) && like != strings.ToLower(like) {
		return strings.ToUpper(word)
	}
	return strings.ToLower(word)
}

// nodeSpan is the rune span of a terminal or a rule context, inclusive.
func nodeSpan(n antlr.Tree) (start, stop int) {
	switch x := n.(type) {
	case antlr.TerminalNode:
		t := x.GetSymbol()
		return t.GetStart(), t.GetStop()
	case antlr.ParserRuleContext:
		if x.GetStart() != nil && x.GetStop() != nil {
			return x.GetStart().GetStart(), x.GetStop().GetStop()
		}
	}
	return -1, -1
}

// nodeText is the source text of a node, as written.
func nodeText(n antlr.Tree) string {
	switch x := n.(type) {
	case antlr.TerminalNode:
		return x.GetText()
	case antlr.ParserRuleContext:
		return extractOriginalText(x)
	}
	return ""
}

// holdsInterpretedEscape reports whether a subtree has an mdl 0 string literal
// whose value differs under mdl 1. Text a rewrite moves must not hold one, or
// the string-escape rewrite would edit text that is no longer there.
func holdsInterpretedEscape(n antlr.Tree) bool {
	if tn, ok := n.(antlr.TerminalNode); ok {
		t := tn.GetSymbol()
		return t.GetTokenType() == parser.MDLLexerSTRING_LITERAL && hasInterpretedEscape(t.GetText()) &&
			!parser.HasStrictEscapes(t.GetInputStream())
	}
	for i := 0; i < n.GetChildCount(); i++ {
		if holdsInterpretedEscape(n.GetChild(i)) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// List operations and aggregates: call form -> statement form (MDL-DEPR003,
// MDL-DEPR004, MDL-V1-LIST).
// ---------------------------------------------------------------------------

// simpleAttribute is an attribute name the statement form's `by` takes as is.
var simpleAttribute = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// callFormFix rewrites a list-operation or aggregate call (a
// ListOperationContext or ListAggregateOperationContext) to the statement form
// that builds the same activity. byExpression says, for find and filter, which
// condition the call form built: `where` (an expression) or `by` (a member).
// It returns the edits, or the reason there are none.
func callFormFix(op antlr.ParserRuleContext, byExpression bool) ([]ast.TextEdit, string) {
	if af, ok := op.(*parser.AggregateFunctionContext); ok {
		return aggregateFunctionFix(af)
	}
	n := op.GetChildCount()
	if n < 4 {
		return nil, "the call is incomplete"
	}
	kwNode, ok := op.GetChild(0).(antlr.TerminalNode)
	if !ok {
		return nil, "the call has no operation keyword"
	}
	rpNode, ok := op.GetChild(n - 1).(antlr.TerminalNode)
	if !ok || rpNode.GetSymbol().GetTokenType() != parser.MDLParserRPAREN {
		return nil, "the call is incomplete"
	}
	kw, rp := kwNode.GetSymbol(), rpNode.GetSymbol()
	var args []antlr.Tree
	for i := 2; i < n-1; i++ {
		c := op.GetChild(i)
		if tn, ok := c.(antlr.TerminalNode); ok && tn.GetSymbol().GetTokenType() == parser.MDLParserCOMMA {
			continue
		}
		args = append(args, c)
	}
	if len(args) == 0 {
		return nil, "the call has no operand"
	}
	word := func(w string) string { return keywordLike(kw.GetText(), w) }

	// infix keeps every argument where it is and replaces only the
	// punctuation: `op(` -> `op `, each `,` -> its keyword, `)` -> nothing.
	infix := func(seps ...string) ([]ast.TextEdit, string) {
		if len(seps) != len(args)-1 {
			return nil, "the call has an unexpected number of arguments"
		}
		first, _ := nodeSpan(args[0])
		edits := []ast.TextEdit{replaceGap(kw.GetStop(), first, " ")}
		for i, sep := range seps {
			_, prevStop := nodeSpan(args[i])
			nextStart, _ := nodeSpan(args[i+1])
			edits = append(edits, replaceGap(prevStop, nextStart, " "+word(sep)+" "))
		}
		_, lastStop := nodeSpan(args[len(args)-1])
		return append(edits, replaceGap(lastStop, rp.GetStop()+1, "")), ""
	}
	// whole replaces everything after the keyword, for the forms that reorder
	// their operands.
	whole := func(text string) ([]ast.TextEdit, string) {
		for _, a := range args {
			if holdsInterpretedEscape(a) {
				return nil, "its operands move, and hold a string with a backslash escape"
			}
		}
		return []ast.TextEdit{{Start: kw.GetStop() + 1, Stop: rp.GetStop() + 1, Text: text}}, ""
	}

	switch kw.GetTokenType() {
	case parser.MDLParserHEAD, parser.MDLParserTAIL, parser.MDLParserCOUNT:
		return infix()
	case parser.MDLParserFIND, parser.MDLParserFILTER:
		if byExpression {
			return infix("where")
		}
		return infix("by")
	case parser.MDLParserSORT:
		return infix("by")
	case parser.MDLParserUNION, parser.MDLParserINTERSECT:
		return infix("with")
	case parser.MDLParserEQUALS_OP:
		return infix("and")
	case parser.MDLParserRANGE:
		if len(args) > 3 {
			return nil, "the call has an unexpected number of arguments"
		}
		return infix([]string{"offset", "limit"}[:len(args)-1]...)
	case parser.MDLParserSUBTRACT:
		// subtract($A, $B) is A minus B, written `subtract $B from $A`.
		if len(args) != 2 {
			return nil, "the call has an unexpected number of arguments"
		}
		return whole(" " + nodeText(args[1]) + " " + word("from") + " " + nodeText(args[0]))
	case parser.MDLParserCONTAINS:
		// contains($List, $Object) is written `contains $Object in $List`.
		if len(args) != 2 {
			return nil, "the call has an unexpected number of arguments"
		}
		return whole(" " + nodeText(args[1]) + " " + word("in") + " " + nodeText(args[0]))
	case parser.MDLParserSUM, parser.MDLParserAVERAGE, parser.MDLParserMINIMUM, parser.MDLParserMAXIMUM:
		if path, ok := args[0].(*parser.AttributePathContext); ok && len(args) == 1 {
			// sum($L.Attr) is `sum $L by Attr`: the builder keeps the last
			// segment of the path as the attribute.
			v, attr := parseAttributePath(path.GetText())
			if !simpleAttribute.MatchString(attr) {
				return nil, fmt.Sprintf("the attribute `%s` is not a plain name, which is all the statement form's `by` takes", attr)
			}
			return whole(" $" + v + " " + word("by") + " " + attr)
		}
		return infix("of")
	case parser.MDLParserALL, parser.MDLParserANY:
		return infix("where")
	case parser.MDLParserREDUCE:
		// reduce($L, expr, initial: seed, returns: T) is
		// `reduce $L from seed as T using expr`: the fold body stays in place,
		// the seed and the type move in front of it.
		opts, ok := args[len(args)-1].(*parser.ReduceFoldOptionsContext)
		if !ok || len(args) != 3 || opts.Expression() == nil || opts.DataType() == nil {
			return nil, "the call has an unexpected shape"
		}
		if holdsInterpretedEscape(opts) {
			return nil, "its seed moves, and holds a string with a backslash escape"
		}
		bodyStart, bodyStop := nodeSpan(args[1])
		return []ast.TextEdit{
			{Start: kw.GetStop() + 1, Stop: bodyStart, Text: " " + nodeText(args[0]) + " " + word("from") + " " +
				extractOriginalText(opts.Expression()) + " " + word("as") + " " + extractOriginalText(opts.DataType()) +
				" " + word("using") + " "},
			replaceGap(bodyStop, rp.GetStop()+1, ""),
		}, ""
	}
	return nil, "there is no statement form for `" + kw.GetText() + "`"
}

// singleListCall returns the list-operation or aggregate call an expression
// consists of, or nil when it is anything else: a nested call, an operator, or
// a call the call-form grammar does not match.
func singleListCall(e antlr.Tree) antlr.ParserRuleContext {
	for e != nil {
		switch x := e.(type) {
		case *parser.ListOperationContext:
			return x
		case *parser.ListAggregateOperationContext:
			return x
		case *parser.AggregateFunctionContext:
			return x
		}
		if e.GetChildCount() != 1 {
			return nil
		}
		e = e.GetChild(0)
	}
	return nil
}

// operandDefs are the definitions of a find/contains operand within its flow,
// by what they tell about its type.
type operandDefs struct {
	inFlow   bool
	declared []bool        // a parameter's or declare's type: String or not
	listDefs int           // retrieve, create list, list statement, loop: never a String
	calls    []ast.FlowRef // `$x = call microflow|nanoflow …`: the flow's return type decides
	unknown  int           // any other `$x = …`, whose type the script does not state
}

// collectOperandDefs finds every definition of variable in the flow that
// encloses at.
func collectOperandDefs(at antlr.Tree, variable string) operandDefs {
	name := strings.TrimPrefix(variable, "$")
	var flow antlr.Tree
	for t := at; t != nil && flow == nil; t = t.GetParent() {
		switch t.(type) {
		case *parser.CreateMicroflowStatementContext, *parser.CreateNanoflowStatementContext, *parser.CreateRuleStatementContext:
			flow = t
		}
	}
	var d operandDefs
	if flow == nil {
		return d
	}
	d.inFlow = true
	var walk func(antlr.Tree)
	walk = func(t antlr.Tree) {
		switch x := t.(type) {
		case antlr.TerminalNode:
			return
		case *parser.MicroflowParameterContext:
			pname := ""
			if pn := x.ParameterName(); pn != nil {
				pname = parameterNameText(pn)
			} else if v := x.VARIABLE(); v != nil {
				pname = strings.TrimPrefix(v.GetText(), "$")
			}
			if pname == name && x.DataType() != nil {
				d.declared = append(d.declared, buildMicroflowDataType(x.DataType()).Kind == ast.TypeString)
			}
			return
		case *parser.DeclareStatementContext:
			if v := x.VARIABLE(); v != nil && v.GetText() == variable && x.DataType() != nil {
				d.declared = append(d.declared, buildMicroflowDataType(x.DataType()).Kind == ast.TypeString)
			}
		case *parser.RetrieveStatementContext:
			if v := x.VARIABLE(); v != nil && v.GetText() == variable {
				d.listDefs++
			}
		case *parser.CreateListStatementContext:
			if v := x.VARIABLE(); v != nil && v.GetText() == variable {
				d.listDefs++
			}
		case *parser.ListOperationStatementContext:
			if v := x.VARIABLE(); v != nil && v.GetText() == variable {
				d.listDefs++
			}
		case *parser.LoopStatementContext:
			if v := x.VARIABLE(0); v != nil && v.GetText() == variable {
				d.listDefs++
			}
		case *parser.CallMicroflowStatementContext:
			// The flow builder types a call's result by the called flow's
			// return type (registerResultVariableType).
			if v := x.VARIABLE(); v != nil && v.GetText() == variable {
				d.calls = append(d.calls, ast.FlowRef{Name: buildQualifiedName(x.QualifiedName())})
			}
		case *parser.CallNanoflowStatementContext:
			if v := x.VARIABLE(); v != nil && v.GetText() == variable {
				d.calls = append(d.calls, ast.FlowRef{Nanoflow: true, Name: buildQualifiedName(x.QualifiedName())})
			}
		default:
			// Any other `$name = …` gives it a type the script may not state
			// (a Java action's result, a REST response, an aggregate). A set
			// only reassigns.
			if prc, ok := t.(antlr.ParserRuleContext); ok && prc.GetChildCount() >= 2 {
				if _, isSet := prc.(*parser.SetStatementContext); !isSet {
					v, ok1 := prc.GetChild(0).(antlr.TerminalNode)
					eq, ok2 := prc.GetChild(1).(antlr.TerminalNode)
					if ok1 && ok2 && v.GetText() == variable && eq.GetSymbol().GetTokenType() == parser.MDLParserEQUALS {
						d.unknown++
					}
				}
			}
		}
		for i := 0; i < t.GetChildCount(); i++ {
			walk(t.GetChild(i))
		}
	}
	walk(flow)
	return d
}

// operandReading says whether a find/contains operand is a String (the call
// is the string function) or not (the List operation), as the mdl 0 flow
// builder decides it from the variable's type when it builds the flow.
//
// A parameter's or declare's type is authoritative. A call's result has the
// called flow's return type, which the builder reads from a flow an earlier
// statement of the script created, else from the project. The first is
// answered here; the second cannot be without the project, so the reading is
// returned pending — the choice the upgrade resolves when it has one
// (ako/mxcli#860) — with the reason there is no fix yet. Any other definition
// whose type the script does not state leaves the reading unknown, with
// pending nil.
func (b *Builder) operandReading(at antlr.Tree, variable, fn string) (isString, known bool, pending *ast.OperandChoice, reason string) {
	d := collectOperandDefs(at, variable)
	if len(d.declared) > 0 {
		for _, k := range d.declared[1:] {
			if k != d.declared[0] {
				return false, false, nil, operandKindUnknown(variable, fn)
			}
		}
		return d.declared[0], true, nil, ""
	}
	if !d.inFlow || d.unknown > 0 || d.listDefs+len(d.calls) == 0 {
		return false, false, nil, operandKindUnknown(variable, fn)
	}
	var kinds []bool
	if d.listDefs > 0 {
		kinds = append(kinds, false)
	}
	var flows []ast.FlowRef
	for _, c := range d.calls {
		switch isStr, state := b.scriptFlowReturn(c); state {
		case scriptDefines:
			kinds = append(kinds, isStr)
		case scriptObscures:
			return false, false, nil, fmt.Sprintf("whether %s is a String (the string function %s) or a list "+
				"(the List operation) depends on what %s returns, and an earlier statement of the script "+
				"drops, renames or moves it or creates it `if not exists`; write `set $x = %s(…);` or "+
				"`$x = %s %s …;` by hand", variable, fn, flowRefText(c), fn, fn, variable)
		default:
			flows = append(flows, c)
		}
	}
	if len(flows) > 0 {
		return false, false, &ast.OperandChoice{Variable: variable, Function: fn, Flows: flows, Known: kinds},
			operandFromProject(variable, fn, flows)
	}
	for _, k := range kinds[1:] {
		if k != kinds[0] {
			return false, false, nil, OperandReadingsDisagree(variable, fn)
		}
	}
	return kinds[0], true, nil, ""
}

// How an earlier statement of the script bears on a called flow.
const (
	scriptSilent   = iota // no earlier statement names it: the project answers
	scriptDefines         // an earlier create says what it returns
	scriptObscures        // an earlier statement leaves it unknowable
)

// scriptFlowReturn reads a called flow's return type from the statements the
// script runs before the current one, which are the ones built so far: the
// flow builder resolves a call against the model as those statements left
// it. The last statement naming the flow decides.
func (b *Builder) scriptFlowReturn(ref ast.FlowRef) (isString bool, state int) {
	returnsString := func(rt *ast.MicroflowReturnType) bool {
		return rt != nil && rt.Type.Kind == ast.TypeString
	}
	for i := len(b.statements) - 1; i >= 0; i-- {
		st := b.statements[i]
		if g, ok := st.(ast.IfNotExistsCreate); ok && g.CreateIfNotExists() {
			// Skipped when the flow exists: either reading may hold.
			switch s := st.(type) {
			case *ast.CreateMicroflowStmt:
				if !ref.Nanoflow && s.Name == ref.Name {
					return false, scriptObscures
				}
			case *ast.CreateNanoflowStmt:
				if ref.Nanoflow && s.Name == ref.Name {
					return false, scriptObscures
				}
			}
			continue
		}
		switch s := st.(type) {
		case *ast.CreateMicroflowStmt:
			if !ref.Nanoflow && s.Name == ref.Name {
				return returnsString(s.ReturnType), scriptDefines
			}
		case *ast.CreateNanoflowStmt:
			if ref.Nanoflow && s.Name == ref.Name {
				return returnsString(s.ReturnType), scriptDefines
			}
		case *ast.DropMicroflowStmt:
			if !ref.Nanoflow && s.Name == ref.Name {
				return false, scriptObscures
			}
		case *ast.DropNanoflowStmt:
			if ref.Nanoflow && s.Name == ref.Name {
				return false, scriptObscures
			}
		case *ast.RenameStmt:
			// A rename of the flow, of another document to its name, or of
			// its module.
			if s.Name.Module == ref.Name.Module || (s.Name.Module == "" && s.Name.Name == ref.Name.Module) {
				return false, scriptObscures
			}
		case *ast.MoveStmt:
			if s.Name == ref.Name || s.TargetModule == ref.Name.Module {
				return false, scriptObscures
			}
		}
	}
	return false, scriptSilent
}

func flowRefText(r ast.FlowRef) string {
	if r.Nanoflow {
		return "nanoflow " + r.Name.String()
	}
	return "microflow " + r.Name.String()
}

func operandKindUnknown(variable, fn string) string {
	return fmt.Sprintf("whether %s is a String (the string function %s) or a list (the List operation) "+
		"depends on a type the script does not state; write `set $x = %s(…);` or `$x = %s %s …;` by hand",
		variable, fn, fn, fn, variable)
}

func operandFromProject(variable, fn string, flows []ast.FlowRef) string {
	names := make([]string, len(flows))
	for i, f := range flows {
		names[i] = flowRefText(f)
	}
	return fmt.Sprintf("whether %s is a String (the string function %s) or a list (the List operation) "+
		"depends on what %s returns, which the script does not state: pass the project to "+
		"`fmt --upgrade` (-p app.mpr) to read it there, or write `set $x = %s(…);` or `$x = %s %s …;` by hand",
		variable, fn, strings.Join(names, " and "), fn, fn, variable)
}

// OperandReadingsDisagree is the reason a find/contains call has no rewrite
// when its operand's definitions give it a String on one path and a list on
// another.
func OperandReadingsDisagree(variable, fn string) string {
	return fmt.Sprintf("%s is a String on one path and a list on another, so `%s(…)` is the string function "+
		"on one and the List operation on the other; write each by hand", variable, fn)
}

// ---------------------------------------------------------------------------
// Terminators and strings.
// ---------------------------------------------------------------------------

// slashLineFix deletes a `/` terminator: its whole line when the `/` is alone
// on it, else the token.
func slashLineFix(slash antlr.Token) ast.TextEdit {
	is := slash.GetInputStream()
	start, stop := slash.GetStart(), slash.GetStop()+1
	size := is.Size()
	at := func(i int) string { return is.GetText(i, i) }
	ls := start
	for ls > 0 && (at(ls-1) == " " || at(ls-1) == "\t") {
		ls--
	}
	le := stop
	for le < size && (at(le) == " " || at(le) == "\t" || at(le) == "\r") {
		le++
	}
	if (ls == 0 || at(ls-1) == "\n") && (le == size || at(le) == "\n") {
		if le < size {
			le++ // and its newline
		}
		return ast.TextEdit{Start: ls, Stop: le}
	}
	return ast.TextEdit{Start: start, Stop: stop}
}

// aggregateFunctionNames maps the SQL aggregate spellings (`min(…)`, `avg(…)`)
// to the Aggregate list statement mdl 0 builds from them
// (buildAggregateFunctionAsCall): the spelling describe writes back.
var aggregateFunctionNames = map[int]string{
	parser.MDLParserCOUNT: "count",
	parser.MDLParserSUM:   "sum",
	parser.MDLParserAVG:   "average",
	parser.MDLParserMIN:   "minimum",
	parser.MDLParserMAX:   "maximum",
}

// aggregateFunctionFix rewrites `$x = min($List.Attr)` — the SQL aggregate
// spelling, which mdl 0 turns into an Aggregate list activity through
// buildSetAggregate — to the statement form of that activity,
// `$x = minimum $List by Attr` (ako/mxcli#838). The edit covers the keyword
// too, since min/max/avg are not the activity's names.
//
// It mirrors buildSetAggregate's reading of the operand: an attribute path
// aggregates its last segment, and a bare variable only counts.
func aggregateFunctionFix(af *parser.AggregateFunctionContext) ([]ast.TextEdit, string) {
	kwNode, ok := af.GetChild(0).(antlr.TerminalNode)
	if !ok || af.RPAREN() == nil {
		return nil, "the call is incomplete"
	}
	kw := kwNode.GetSymbol()
	name, ok := aggregateFunctionNames[kw.GetTokenType()]
	if !ok {
		return nil, "there is no statement form for `" + kw.GetText() + "`"
	}
	if af.STAR() != nil || af.Expression() == nil {
		return nil, "`" + kw.GetText() + "(*)` names no list, so the activity mdl 0 builds from it has none: " +
			"write `$x = count $List;`"
	}
	if af.DISTINCT() != nil {
		return nil, "`distinct` has no Aggregate list equivalent, and mdl 0 drops it: write the statement form " +
			"without it if that is what was meant"
	}
	word := func(w string) string { return keywordLike(kw.GetText(), w) }
	variable, attr := "", ""
	switch e := buildExpression(af.Expression()).(type) {
	case *ast.AttributePathExpr:
		variable = e.Variable
		if len(e.Path) > 0 {
			attr = e.Path[len(e.Path)-1]
		}
	case *ast.VariableExpr:
		variable = e.Name
		if list, a, cut := strings.Cut(e.Name, "."); cut {
			variable, attr = list, a
		}
	default:
		return nil, "the operand is not a variable (a nested call or an expression), and one activity takes a " +
			"variable: write each inner call as a statement of its own"
	}
	rp := af.RPAREN().GetSymbol()
	text := ""
	switch {
	case name == "count" && attr == "":
		text = word(name) + " $" + variable
	case name == "count":
		return nil, "`count` takes a list, not an attribute: write `$x = count $List;`"
	case attr == "":
		return nil, "the operand names no attribute to aggregate: write `$x = " + name + " $List by Attr;`"
	case !simpleAttribute.MatchString(attr):
		return nil, fmt.Sprintf("the attribute `%s` is not a plain name, which is all the statement form's `by` takes", attr)
	default:
		text = word(name) + " $" + variable + " " + word("by") + " " + attr
	}
	return []ast.TextEdit{{Start: kw.GetStart(), Stop: rp.GetStop() + 1, Text: text}}, ""
}
