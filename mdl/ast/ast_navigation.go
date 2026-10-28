// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// CreateNavigationStmt represents: CREATE [OR MODIFY] NAVIGATION <profile> [clauses...]
// A clause it states replaces what it names (a { } menu block the whole menu);
// an omitted clause keeps the stored value (mendixlabs/mxcli#1522).
type CreateNavigationStmt struct {
	ProfileName  string           // e.g. "Responsive"
	HomePages    []NavHomePageDef // HOME PAGE/MICROFLOW ... [FOR role]
	LoginPage    *QualifiedName   // LOGIN PAGE ...
	NotFoundPage *QualifiedName   // NOT FOUND PAGE ...
	MenuItems    []NavMenuItemDef // MENU (...) block
	HasMenuBlock bool             // true if MENU (...) was present (even if empty → clears menu)
	SyncEntries  []NavSyncDef     // SYNC (...) block — offline synchronization
	HasSyncBlock bool             // true if SYNC (...) was present (even if empty → clears the list)
	// ThrowSyncError is ON SYNC ERROR THROW|CONTINUE, and is a POINTER so an
	// omitted clause leaves the stored value alone. A plain bool would make
	// every rewrite that never mentions it reset the flag to false — the
	// guard-don't-drop failure, in the one property on this statement that is
	// a bare boolean and so has no "unset" value of its own.
	ThrowSyncError *bool
	// ProgressiveWebApp is the PROGRESSIVE WEB APP clause; nil when the
	// statement does not mention it, so the stored settings are left alone.
	ProgressiveWebApp *types.NavPWASpec
	// AppTitle is APP TITLE '<text>' — Studio Pro's "Application title", the
	// browser tab's prefix — written as the project default language's item
	// of Navigation$NavigationProfile.AppTitle. AppIcon is APP ICON <image>,
	// the Images$Image the profile's AppIcon names; "" is APP ICON NONE, which
	// clears it. Both are pointers so an omitted clause keeps what is stored
	// (ako/mxcli#1756).
	AppTitle       *string
	AppIcon        *string
	CreateOrModify bool // true if CREATE OR REPLACE/MODIFY was used
}

func (s *CreateNavigationStmt) isStatement() {}

// NavSyncDef represents one `SYNC <entity> <mode>` line inside a SYNC block.
//
// Mode carries the STORED enum member, not the word the user typed: the visitor
// maps ONLINE/ALL/NEVER/NONE/NONE PRESERVE DATA/WHERE onto Online/All/Never/
// None/NoneAndPreserveData/Constrained, so nothing downstream has to know the
// spelling. Studio Pro's captions ("All Objects", "By XPath") are not members
// of the enumeration and never appear here.
type NavSyncDef struct {
	Entity QualifiedName
	Mode   string
	// Constraint is the XPath from a WHERE clause, and is set only when Mode is
	// Constrained — the two are derived from the same alternative precisely so
	// they cannot disagree.
	Constraint string
}

// NavHomePageDef represents a HOME PAGE or HOME MICROFLOW clause.
type NavHomePageDef struct {
	IsPage bool // true = PAGE, false = MICROFLOW or NANOFLOW
	// IsNanoflow is `home nanoflow` — a native profile's flow home. A web
	// profile's flow home is a microflow, a native one's a nanoflow.
	IsNanoflow bool
	Target     QualifiedName  // the page or microflow qualified name
	ForRole    *QualifiedName // nil for default home, set for role-based
}

// NavMenuItemDef represents a MENU ITEM or MENU sub-menu definition.
type NavMenuItemDef struct {
	Caption   string         // from STRING_LITERAL
	Page      *QualifiedName // PAGE target
	Microflow *QualifiedName // MICROFLOW target
	SignOut   bool           // SIGN_OUT — the third action a menu item can carry
	// Action is the item's OnClick as the widget action expression it is
	// (ako/mxcli#980): every kind a menu item stores — show page, call
	// microflow / nanoflow, open link, create object, sign out, nothing — with
	// its `with ( … )` settings. Set for every action the script states,
	// including the old clause spelling; Page / Microflow / SignOut still name
	// the simple targets for the checks that read them. nil: no action stated.
	Action *ActionV3
	Icon   string // the qualified name, for the collection and image kinds
	// IconKind says WHICH of Mendix's three icon elements was written. They are
	// not variants of one value — a glyph carries a numeric code and no name —
	// so a single Icon string could express only one of the three, and the other
	// two were destroyed on rewrite.
	IconKind types.MenuIconKind
	// IconCode is the glyph's numeric character code, set only for MenuIconGlyph.
	IconCode int
	Items    []NavMenuItemDef // Sub-items (for MENU 'caption' (...))
}

// CreateMenuStmt is `create [or modify] menu Module.Name ( <items> )` — a
// standalone Menus$MenuDocument, not the menu inside a navigation profile.
// Items reuse NavMenuItemDef so both constructs share one item syntax.
//
// Like CREATE NAVIGATION, this is a full replacement: the item list given is the
// document's complete contents, so an omitted item is a removed item.
type CreateMenuStmt struct {
	CreateGuard             // `create … if not exists` (ako/mxcli#731)
	Folder           string // Folder path within module (empty = leave placement alone)
	Name             QualifiedName
	Items            []NavMenuItemDef
	CreateOrModify   bool // CREATE OR MODIFY / OR REPLACE
	Documentation    string
	DocumentationSet bool // see mendixlabs/mxcli#1018: absent preserves, empty clears
}

func (s *CreateMenuStmt) isStatement() {}

// DropMenuStmt is `drop menu Module.Name`.
type DropMenuStmt struct {
	DropGuard
	Name QualifiedName
}

func (s *DropMenuStmt) isStatement() {}

// AlterNavigationStmt is `alter navigation <Profile> { … }`: item-level edits
// to a web profile's menu and its page clauses, applied in order to the stored
// profile and written once (accepted proposal
// PROPOSAL_alter_navigation_menu_items). Not to be confused with
// CreateNavigationStmt, which writes the clauses it states.
type AlterNavigationStmt struct {
	ProfileName string
	Operations  []MenuAlterOp
}

func (s *AlterNavigationStmt) isStatement() {}

// AlterMenuStmt is `alter menu Module.Menu { … }`: the item operations of
// AlterNavigationStmt on a menu document, which holds the same items.
type AlterMenuStmt struct {
	Name       QualifiedName
	Operations []MenuAlterOp
}

func (s *AlterMenuStmt) isStatement() {}

// MenuAlterOp is one operation of an ALTER NAVIGATION / ALTER MENU block.
// String spells it as MDL, for messages.
type MenuAlterOp interface {
	menuAlterOp()
	String() string
}

// InsertMenuItemsOp is `insert before|after|into <item> { … }`, or `insert
// { … }` (Position "", Target nil), which appends to the top level.
type InsertMenuItemsOp struct {
	Position string // "before", "after", "into", or "" for the top level
	Target   *types.MenuItemAddress
	Items    []NavMenuItemDef
}

// ReplaceMenuItemOp is `replace <item> with { … }`.
type ReplaceMenuItemOp struct {
	Target types.MenuItemAddress
	Items  []NavMenuItemDef
}

// MoveMenuItemOp is `move <item> before|after|into <item>`.
type MoveMenuItemOp struct {
	Item     types.MenuItemAddress
	Position string // "before", "after", "into"
	Target   types.MenuItemAddress
}

// DropMenuItemsOp is `drop <item>, …`.
type DropMenuItemsOp struct {
	Targets []types.MenuItemAddress
}

// SetMenuItemOp is `set ( Caption: …, OnClick: …, Icon: … ) on <item>`. Only
// the keys written are changed. Item carries the parsed OnClick (Action, Page,
// Microflow, SignOut) and Icon (Icon, IconKind, IconCode — IconKind none with
// SetIcon is `Icon: none`), as a `menu item` would.
type SetMenuItemOp struct {
	Target    types.MenuItemAddress
	Caption   *string
	SetAction bool
	SetIcon   bool
	Item      NavMenuItemDef
}

// SetNavPageClauseOp is a profile clause after `set`: `set home page|microflow
// M.X [for Role]` (Clause "home"), `set login page M.P` ("login"), `set not
// found page M.P` ("notfound").
type SetNavPageClauseOp struct {
	Clause string
	Home   NavHomePageDef // Clause "home"
	Page   QualifiedName  // Clause "login" / "notfound"
}

// DropNavPageClauseOp is a profile clause after `drop`: `drop home page for
// Role` (Clause "home"; Role nil is the bare form, which is refused), `drop
// login page`, `drop not found page`.
type DropNavPageClauseOp struct {
	Clause string
	Role   *QualifiedName
}

func (*InsertMenuItemsOp) menuAlterOp()   {}
func (*ReplaceMenuItemOp) menuAlterOp()   {}
func (*MoveMenuItemOp) menuAlterOp()      {}
func (*DropMenuItemsOp) menuAlterOp()     {}
func (*SetMenuItemOp) menuAlterOp()       {}
func (*SetNavPageClauseOp) menuAlterOp()  {}
func (*DropNavPageClauseOp) menuAlterOp() {}

func (o *InsertMenuItemsOp) String() string {
	if o.Target == nil {
		return "insert { … }"
	}
	return "insert " + o.Position + " " + o.Target.String()
}

func (o *ReplaceMenuItemOp) String() string { return "replace " + o.Target.String() }

func (o *MoveMenuItemOp) String() string {
	return "move " + o.Item.String() + " " + o.Position + " " + o.Target.String()
}

func (o *DropMenuItemsOp) String() string {
	parts := make([]string, len(o.Targets))
	for i, t := range o.Targets {
		parts[i] = t.String()
	}
	return "drop " + strings.Join(parts, ", ")
}

func (o *SetMenuItemOp) String() string {
	var keys []string
	if o.Caption != nil {
		keys = append(keys, "Caption")
	}
	if o.SetAction {
		keys = append(keys, "OnClick")
	}
	if o.SetIcon {
		keys = append(keys, "Icon")
	}
	return "set ( " + strings.Join(keys, ", ") + " ) on " + o.Target.String()
}

func (o *SetNavPageClauseOp) String() string {
	switch o.Clause {
	case "home":
		kind := "microflow"
		if o.Home.IsPage {
			kind = "page"
		}
		s := "set home " + kind + " " + o.Home.Target.String()
		if o.Home.ForRole != nil {
			s += " for " + o.Home.ForRole.String()
		}
		return s
	case "login":
		return "set login page " + o.Page.String()
	}
	return "set not found page " + o.Page.String()
}

func (o *DropNavPageClauseOp) String() string {
	switch o.Clause {
	case "home":
		if o.Role == nil {
			return "drop home page"
		}
		return "drop home page for " + o.Role.String()
	case "login":
		return "drop login page"
	}
	return "drop not found page"
}
