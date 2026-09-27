// SPDX-License-Identifier: Apache-2.0

package ast

import "strconv"

// ============================================================================
// ALTER PAGE / ALTER SNIPPET — in-place widget tree modification
// ============================================================================

// AlterPageStmt represents: ALTER PAGE/SNIPPET Module.Name { operations }
type AlterPageStmt struct {
	ContainerType string        // "PAGE" or "SNIPPET"
	PageName      QualifiedName // page or snippet qualified name
	Operations    []AlterPageOperation
}

func (s *AlterPageStmt) isStatement() {}

// AlterPageOperation is the interface for individual ALTER PAGE operations.
type AlterPageOperation interface {
	isAlterPageOperation()
}

// WidgetRef is the <target> of a generic ALTER operation as written: the
// element `set … on`, `insert before|after|into`, `replace … with` and `drop`
// address (ADR-0012 decision 2).
//
// One address syntax serves every document type, and the document type's
// resolver (backend.AlterTargetResolver) decides which forms it accepts:
//
//	btnSave            Widget="btnSave"
//	dgProducts.Name    Widget="dgProducts", Column="Name" (a grid column, a scroll-container region)
//	'Approve order'    Caption="Approve order" (content addressing, for elements with no name)
//	hdr@2 / 'x'@2      Ordinal=2 — picks one of several matches; never a guess
//
// The name stays WidgetRef because the page family is the first document type
// on the generic path and every page operation already speaks it.
type WidgetRef struct {
	Widget  string // name (empty when the target is addressed by caption)
	Column  string // sub-element name within Widget (empty for a plain name)
	Caption string // quoted content address; empty when addressed by name
	Ordinal int    // @n, 1-based; 0 when absent
}

// Name returns the full reference string for error messages, as it was written.
func (r WidgetRef) Name() string {
	var s string
	switch {
	case r.Caption != "":
		s = "'" + r.Caption + "'"
	case r.Column != "":
		s = r.Widget + "." + r.Column
	default:
		s = r.Widget
	}
	if r.Ordinal > 0 {
		s += "@" + strconv.Itoa(r.Ordinal)
	}
	return s
}

// Spellings of the generic ALTER that are aliases of its canonical form
// (ADR-0011: an old form warns, and is rewritten mechanically). The visitor
// records which one a statement used; the executor maps it to a deprecation
// code. Nothing downstream of the validator may branch on these: both spellings
// build the identical operation.
const (
	// `set Key = value …` / `set (Key = value, …) …` — R3 puts `:` between a
	// property and its value; `=` is comparison.
	AlterAliasSetEquals = "set-equals"
	// `set Key: value …` — properties are a parenthesised list (R2), even one.
	AlterAliasSetUnparenthesised = "set-unparenthesised"
	// `drop widget a, b` — the target names the element; the kind is its own.
	AlterAliasDropWidget = "drop-widget"
)

// IsColumn returns true if this is a column reference (dotted path).
func (r WidgetRef) IsColumn() bool {
	return r.Column != ""
}

// SetPropertyOp represents: SET prop = value ON widgetRef
// or SET prop = value (page-level, Target.Widget empty)
type SetPropertyOp struct {
	Target     WidgetRef              // empty Widget for page-level SET
	Properties map[string]interface{} // property name -> value
	Legacy     string                 // AlterAlias* when an old spelling was used, else ""
}

func (s *SetPropertyOp) isAlterPageOperation() {}

// InsertWidgetOp represents: INSERT AFTER/BEFORE/INTO widgetRef { widgets }
type InsertWidgetOp struct {
	Position string    // "AFTER", "BEFORE", or "INTO" (append as children of the container)
	Target   WidgetRef // widget to insert relative to, or container to insert into
	Widgets  []*WidgetV3
}

func (s *InsertWidgetOp) isAlterPageOperation() {}

// DropWidgetOp represents: DROP WIDGET ref1, ref2, ...
type DropWidgetOp struct {
	Targets []WidgetRef
	Legacy  string // AlterAliasDropWidget when written `drop widget …`, else ""
}

func (s *DropWidgetOp) isAlterPageOperation() {}

// DropListViewTemplateOp represents:
// DROP TEMPLATE FOR Module.Specialization IN listViewName
//
// A List View specialization template has no name, so it is addressed by the
// entity it renders plus the list view holding it — a page can carry two list
// views with a template for the same entity.
type DropListViewTemplateOp struct {
	Specialization string
	ListView       string
}

func (s *DropListViewTemplateOp) isAlterPageOperation() {}

// ReplaceWidgetOp represents: REPLACE widgetRef WITH { widgets }
type ReplaceWidgetOp struct {
	Target     WidgetRef
	NewWidgets []*WidgetV3
}

func (s *ReplaceWidgetOp) isAlterPageOperation() {}

// AddVariableOp represents: ADD Variables $name: Type = 'default'
type AddVariableOp struct {
	Variable PageVariable
}

func (s *AddVariableOp) isAlterPageOperation() {}

// DropVariableOp represents: DROP Variables $name
type DropVariableOp struct {
	VariableName string // without $ prefix
}

func (s *DropVariableOp) isAlterPageOperation() {}

// SetLayoutOp represents: SET Layout = Module.LayoutName [MAP (Old -> New, ...)]
type SetLayoutOp struct {
	NewLayout QualifiedName     // New layout qualified name
	Mappings  map[string]string // Old placeholder -> New placeholder (nil = auto-map)
}

func (s *SetLayoutOp) isAlterPageOperation() {}

// LayoutMapping represents a single placeholder mapping: Old -> New
type LayoutMapping struct {
	From string
	To   string
}

// AlterPagesLayoutStmt represents the bulk repoint:
//
//	ALTER PAGES [IN <module>] SET LAYOUT = Module.Layout
//	  [MAP (Old AS New, …)] [WHERE LAYOUT = Module.Old]
//
// It is a statement of its own rather than an ALTER PAGE operation because it
// names no page: the set is computed from Module and WhereLayout.
type AlterPagesLayoutStmt struct {
	Module      string            // "" = every module the project owns
	NewLayout   QualifiedName     // the layout to point pages at
	Mappings    map[string]string // Old placeholder -> New placeholder
	WhereLayout *QualifiedName    // nil = every page in scope, whatever its layout
}

func (s *AlterPagesLayoutStmt) isStatement() {}

// AlterPagesStylingStmt is the bulk form of ALTER PAGE's design-property SET:
// set a design property on every widget of one TYPE, across a module or the
// whole project.
//
// The predicate is a widget type and never a name, because a widget name is
// unique only within its page — measured across a blank 11.12.2 project,
// `actionButton1` appears in 30 units, so a name predicate would sweep
// unrelated widgets together (ako/mxcli#515).
type AlterPagesStylingStmt struct {
	Module      string              // "" = every module the project owns
	Assignments []StylingAssignment // reuses ALTER STYLING's assignment shape
	WidgetType  string              // MDL keyword (`datagrid`) or a full widget id
	DryRun      bool                // report the matches and write nothing
}

func (s *AlterPagesStylingStmt) isStatement() {}
